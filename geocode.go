package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/comfforts/cache"
	"github.com/comfforts/cloudstorage"
	"github.com/comfforts/errors"
	"github.com/comfforts/logger"
	"googlemaps.github.io/maps"

	"go.uber.org/zap"
)

type AddressQuery struct {
	Street     string
	City       string
	PostalCode string
	State      string
	Country    string
}

type GeoCoder interface {
	Geocode(ctx context.Context, postalCode, countryCode string) (*Point, error)
	GeocodeAddress(ctx context.Context, addr AddressQuery) (*Point, error)
	GeocodeLatLong(ctx context.Context, lat, long float64, hint string) (*Point, error)
	Clear() error
}

type GeoCodeServiceConfig struct {
	GeocoderKey string `json:"geocoder_key"`
	Cached      bool   `json:"cached"`
	DataDir     string `json:"data_dir"`
	BucketName  string `json:"bucket_name"`
}

type geoCodeService struct {
	config       GeoCodeServiceConfig
	logger       logger.AppLogger
	cache        cache.CacheService
	client       *maps.Client
	cloudStorage cloudstorage.CloudStorage
}

var unmarshalLPoint cache.MarshalFn = func(p interface{}) (interface{}, error) {
	var point Point
	body, err := json.Marshal(p)
	if err != nil {
		appErr := errors.WrapError(err, cache.ERROR_MARSHALLING_CACHE_OBJECT)
		return point, appErr
	}

	err = json.Unmarshal(body, &point)
	if err != nil {
		appErr := errors.WrapError(err, cache.ERROR_UNMARSHALLING_CACHE_JSON)
		return point, appErr
	}
	return point, nil
}

func NewGeoCodeService(
	cfg GeoCodeServiceConfig,
	csc cloudstorage.CloudStorage,
	logger logger.AppLogger,
) (*geoCodeService, error) {
	if cfg.GeocoderKey == "" || logger == nil {
		return nil, errors.NewAppError(errors.ERROR_MISSING_REQUIRED)
	}

	c, err := maps.NewClient(maps.WithAPIKey(cfg.GeocoderKey))
	if err != nil {
		logger.Error("error initializing google maps client")
		return nil, err
	}

	gcSrv := geoCodeService{
		config:       cfg,
		logger:       logger,
		cloudStorage: csc,
		client:       c,
	}

	if cfg.Cached {
		cachePath := filepath.Join(cfg.DataDir, "geo")
		cacheFile := filepath.Join(cachePath, fmt.Sprintf("%s.json", cache.CACHE_FILE_NAME))
		if _, err := fileStats(cacheFile); err != nil {
			if err := gcSrv.downloadCache(); err != nil {
				logger.Error("error getting cache from storage")
			}
		}

		c, err := cache.NewCacheService(cachePath, logger, unmarshalLPoint)
		if err != nil {
			logger.Error("error setting up cache service", zap.Error(err))
			return nil, err
		}
		gcSrv.cache = c
	}

	return &gcSrv, nil
}

func (g *geoCodeService) Geocode(ctx context.Context, postalCode, countryCode string) (*Point, error) {
	if ctx == nil {
		g.logger.Error("context is nil", zap.Error(ErrNilContext))
		return nil, ErrNilContext
	}

	if countryCode == "" {
		countryCode = "USA"
	}

	if g.config.Cached {
		point, exp, err := g.getFromCache(postalCode)
		if err == nil {
			g.logger.Debug("returning cached value", zap.String("key", postalCode), zap.Any("exp", exp))
			return point, nil
		} else {
			g.logger.Error("geocoder cache get error", zap.Error(err), zap.String("key", postalCode))
		}
	}

	req := &maps.GeocodingRequest{
		Components: map[maps.Component]string{
			maps.ComponentPostalCode: postalCode,
			maps.ComponentCountry:    countryCode,
		},
	}
	resp, err := g.client.Geocode(ctx, req)
	if err != nil {
		g.logger.Error(ERROR_GEOCODING_POSTAL, zap.Error(err))
		return nil, ErrGeoCodePostalCode
	}

	if len(resp) < 1 {
		g.logger.Error(NO_RESULTS)
		return nil, ErrGeoCodeNoResults
	}

	r := resp[0]
	pt := &Point{
		Latitude:         r.Geometry.Location.Lat,
		Longitude:        r.Geometry.Location.Lng,
		FormattedAddress: r.FormattedAddress,
	}
	if g.config.Cached {
		err = g.setInCache(postalCode, pt, 0)
		if err != nil {
			g.logger.Error("geocoder cache set error", zap.Error(err), zap.String("key", postalCode))
		}
	}

	return pt, nil
}

func (g *geoCodeService) GeocodeAddress(ctx context.Context, addr AddressQuery) (*Point, error) {
	if ctx == nil {
		g.logger.Error("context is nil", zap.Error(ErrNilContext))
		return nil, ErrNilContext
	}

	if addr.Country == "" {
		addr.Country = "USA"
	}

	if g.config.Cached {
		key := g.addressString(addr)
		point, exp, err := g.getFromCache(key)
		if err == nil {
			g.logger.Debug(
				"returning cached value",
				zap.String("key", key),
				zap.Any("exp", exp),
			)
			return point, nil
		} else {
			g.logger.Error(
				"geocoder cache get error",
				zap.Error(err),
				zap.String("key", key),
			)
		}
	}

	req := &maps.GeocodingRequest{
		Address: g.addressString(addr),
	}

	resp, err := g.client.Geocode(ctx, req)
	if err != nil {
		g.logger.Error(ERROR_GEOCODING_ADDRESS, zap.Error(err))
		return nil, ErrGeoCodeAddress
	}

	if len(resp) < 1 {
		g.logger.Error(NO_RESULTS)
		return nil, ErrGeoCodeNoResults
	}

	r := resp[0]
	pt := &Point{
		Latitude:         r.Geometry.Location.Lat,
		Longitude:        r.Geometry.Location.Lng,
		FormattedAddress: r.FormattedAddress,
	}

	if g.config.Cached {
		key := g.addressString(addr)
		err = g.setInCache(key, pt, 0)
		if err != nil {
			g.logger.Error("geocoder cache set error", zap.Error(err), zap.String("key", key))
		}
	}

	return pt, nil
}

func (g *geoCodeService) GeocodeLatLong(ctx context.Context, lat, long float64, hint string) (*Point, error) {
	if ctx == nil {
		g.logger.Error("context is nil", zap.Error(ErrNilContext))
		return nil, ErrNilContext
	}

	if g.config.Cached && hint != "" {
		point, exp, err := g.getFromCache(hint)
		if err == nil {
			g.logger.Debug("returning cached value", zap.String("key", hint), zap.Any("exp", exp))
			return point, nil
		} else {
			g.logger.Error("geocoder cache get error", zap.Error(err), zap.String("key", hint))
		}
	}

	req := &maps.GeocodingRequest{
		LatLng: &maps.LatLng{
			Lat: lat,
			Lng: long,
		},
	}
	resp, err := g.client.Geocode(ctx, req)
	if err != nil {
		g.logger.Error(ERROR_GEOCODING_ADDRESS, zap.Error(err))
		return nil, ErrGeoCodeAddress
	}

	if len(resp) < 1 {
		g.logger.Error(NO_RESULTS)
		return nil, ErrGeoCodeNoResults
	}

	r := resp[0]
	pt := &Point{
		Latitude:         r.Geometry.Location.Lat,
		Longitude:        r.Geometry.Location.Lng,
		FormattedAddress: r.FormattedAddress,
	}

	if g.config.Cached && hint != "" {
		err = g.setInCache(hint, pt, 0)
		if err != nil {
			g.logger.Error("geocoder cache set error", zap.Error(err), zap.String("key", hint))
		}
	}

	return pt, nil
}

func (g *geoCodeService) Clear() error {
	g.logger.Info("cleaning up geo code data structures")
	if g.config.Cached && g.cache.Updated() {
		err := g.cache.SaveFile()
		if err != nil {
			g.logger.Error("error saving geocoder cache", zap.Error(err))
			return err
		} else {
			err = g.uploadCache()
			if err != nil {
				g.logger.Error("error uploading geocoder cache", zap.Error(err))
				return err
			}
		}
	}
	return nil
}

func (g *geoCodeService) addressComponents(addrQ AddressQuery) map[maps.Component]string {
	components := map[maps.Component]string{}
	if addrQ.Street != "" {
		components[maps.Component("street_address")] = addrQ.Street
	}
	if addrQ.City != "" {
		components[maps.ComponentLocality] = addrQ.City
	}
	if addrQ.State != "" {
		components[maps.Component("administrative_area_level_1")] = addrQ.State
	}
	if addrQ.PostalCode != "" {
		components[maps.ComponentPostalCode] = addrQ.PostalCode
	}
	if addrQ.Country != "" {
		components[maps.ComponentCountry] = addrQ.Country
	}
	return components
}

func (g *geoCodeService) addressString(address AddressQuery) string {
	compStr := ""
	if address.Street != "" {
		compStr = address.Street
	}
	if address.City != "" {
		if compStr == "" {
			compStr = address.City
		} else {
			compStr = fmt.Sprintf("%s %s", compStr, address.City)
		}
	}
	if address.State != "" {
		if compStr == "" {
			compStr = address.State
		} else {
			compStr = fmt.Sprintf("%s %s", compStr, address.State)
		}
	}
	if address.PostalCode != "" {
		if compStr == "" {
			compStr = address.PostalCode
		} else {
			compStr = fmt.Sprintf("%s %s", compStr, address.PostalCode)
		}
	}
	if address.Country != "" {
		if compStr == "" {
			compStr = address.Country
		} else {
			compStr = fmt.Sprintf("%s %s", compStr, address.Country)
		}
	}
	return compStr
}

func (g *geoCodeService) getFromCache(key string) (*Point, time.Time, error) {
	key = strings.ReplaceAll(key, " ", "")
	key = strings.ToLower(key)
	key = url.QueryEscape(key)

	val, exp, err := g.cache.Get(key)
	if err != nil {
		g.logger.Error(cache.ERROR_GET_CACHE, zap.Error(err), zap.String("key", key))
		return nil, exp, err
	}
	point, ok := val.(Point)
	if !ok {
		g.logger.Error(
			"error getting cache point value",
			zap.Error(cache.ErrGetCache),
			zap.String("key", key),
		)
		return nil, exp, cache.ErrGetCache
	}
	return &point, exp, nil
}

func (g *geoCodeService) setInCache(
	key string,
	point *Point,
	cacheFor time.Duration,
) error {
	if cacheFor == 0 {
		cacheFor = OneYear
	}

	key = strings.ReplaceAll(key, " ", "")
	key = strings.ToLower(key)
	key = url.QueryEscape(key)

	err := g.cache.Set(key, point, cacheFor)
	if err != nil {
		g.logger.Error(cache.ERROR_SET_CACHE, zap.Error(err), zap.String("key", key))
		return err
	}
	return nil
}

func (g *geoCodeService) uploadCache() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cachePath := filepath.Join(g.config.DataDir, "geo")
	cacheFile := filepath.Join(cachePath, fmt.Sprintf("%s.json", cache.CACHE_FILE_NAME))
	fStats, err := fileStats(cacheFile)
	if err != nil {
		g.logger.Error("error accessing file", zap.Error(err), zap.String("filepath", cacheFile))
		return errors.WrapError(err, ERROR_NO_FILE, cacheFile)
	}
	fmod := fStats.ModTime().Unix()
	g.logger.Info("file mod time", zap.Int64("modtime", fmod), zap.String("filepath", cacheFile))

	file, err := os.Open(cacheFile)
	if err != nil {
		g.logger.Error("error accessing file", zap.Error(err), zap.String("filepath", cacheFile))
		return errors.WrapError(err, ERROR_NO_FILE, cacheFile)
	}
	defer func() {
		if err := file.Close(); err != nil {
			g.logger.Error("error closing file", zap.Error(err), zap.String("filepath", cacheFile))
		}
	}()

	cfr, err := cloudstorage.NewCloudFileRequest(
		g.config.BucketName,
		filepath.Base(cacheFile),
		filepath.Dir(cacheFile),
		fmod,
	)
	if err != nil {
		g.logger.Error("error creating request", zap.Error(err), zap.String("filepath", cacheFile))
		return err
	}

	n, err := g.cloudStorage.UploadFile(ctx, file, cfr)
	if err != nil {
		g.logger.Error("error uploading file", zap.Error(err))
		return err
	}
	g.logger.Info(
		"uploaded file",
		zap.String("file",
			filepath.Base(cacheFile)),
		zap.String("path",
			filepath.Dir(cacheFile)),
		zap.Int64("bytes", n),
	)
	return nil
}

func (g *geoCodeService) downloadCache() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cachePath := filepath.Join(g.config.DataDir, "geo")
	cacheFile := filepath.Join(cachePath, fmt.Sprintf("%s.json", cache.CACHE_FILE_NAME))
	var fmod int64
	fStats, err := fileStats(cacheFile)
	if err == nil {
		fmod := fStats.ModTime().Unix()
		g.logger.Info(
			"file mod time",
			zap.Int64("modtime", fmod),
			zap.String("filepath", cacheFile),
		)
	}

	err = createDirectory(cacheFile)
	if err != nil {
		g.logger.Error("error creating data directory", zap.Error(err), zap.String("filepath", cacheFile))
		return err
	}

	f, err := os.Create(cacheFile)
	if err != nil {
		g.logger.Error("error creating file", zap.Error(err), zap.String("filepath", cacheFile))
		return errors.WrapError(err, ERROR_CREATING_FILE, cacheFile)
	}
	defer func() {
		if err := f.Close(); err != nil {
			g.logger.Error(
				"error closing file",
				zap.Error(err),
				zap.String("filepath", cacheFile),
			)
		}
	}()

	cfr, err := cloudstorage.NewCloudFileRequest(
		g.config.BucketName,
		filepath.Base(cacheFile),
		filepath.Dir(cacheFile),
		fmod,
	)
	if err != nil {
		g.logger.Error(
			"error creating request",
			zap.Error(err),
			zap.String("filepath", cacheFile),
		)
		return err
	}

	n, err := g.cloudStorage.DownloadFile(ctx, f, cfr)
	if err != nil {
		g.logger.Error(
			"error downloading file",
			zap.Error(err),
			zap.String("filepath", cacheFile),
		)
		return err
	}
	g.logger.Info(
		"downloaded file",
		zap.String("file",
			filepath.Base(cacheFile)),
		zap.String("path",
			filepath.Dir(cacheFile)),
		zap.Int64("bytes", n),
	)
	return nil
}

func createDirectory(path string) error {
	_, err := os.Stat(filepath.Dir(path))
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(filepath.Dir(path), os.ModePerm)
			if err == nil {
				return nil
			}
		}
		return err
	}
	return nil
}

func fileStats(filePath string) (fs.FileInfo, error) {
	fStats, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fStats, errors.WrapError(err, ERROR_NO_FILE, filePath)
		} else {
			return fStats, errors.WrapError(err, ERROR_FILE_INACCESSIBLE, filePath)
		}
	}
	return fStats, nil
}
