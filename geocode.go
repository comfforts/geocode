package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/comfforts/cache"
	"github.com/comfforts/cloudstorage"
	"github.com/comfforts/errors"
	"github.com/comfforts/logger"

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
	Host        string `json:"host"`
	Path        string `json:"path"`
	Cached      bool   `json:"cached"`
	DataDir     string `json:"data_dir"`
	BucketName  string `json:"bucket_name"`
}

type geoCodeService struct {
	config       GeoCodeServiceConfig
	logger       logger.AppLogger
	cache        cache.CacheService
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

	if cfg.Host == "" {
		cfg.Host = "https://maps.googleapis.com"
	}

	if cfg.Path == "" {
		cfg.Path = "/maps/api/geocode/json"
	}

	gcSrv := geoCodeService{
		config:       cfg,
		logger:       logger,
		cloudStorage: csc,
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

	reqURL := g.postalCodeURL(countryCode, postalCode)
	pts, err := g.geocode(reqURL)
	if err != nil {
		return nil, err
	}

	pt := pts[0]
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

	reqURL := g.addressURL(addr)
	pts, err := g.geocode(reqURL)
	if err != nil {
		reqURL = g.addressComponentURL(addr)
		pts, err = g.geocode(reqURL)
		if err != nil {
			reqURL = g.postalCodeURL(addr.Country, addr.PostalCode)
			pts, err = g.geocode(reqURL)
			if err != nil {
				g.logger.Error(
					"geocoder request error",
					zap.Error(err),
					zap.String("country",
						addr.Country),
					zap.String("postal",
						addr.PostalCode,
					))
				return nil, err
			}
		}
	}

	pt := pts[0]

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

	reqURL := g.latLngURL(lat, long)
	pts, err := g.geocode(reqURL)
	if err != nil {
		return nil, err
	}

	var pt *Point
	for _, p := range pts {
		if strings.Contains(p.FormattedAddress, hint) {
			pt = p
			break
		}
	}
	if pt == nil {
		pt = pts[0]
	}

	if g.config.Cached && hint != "" {
		err = g.setInCache(hint, pt, 0)
		if err != nil {
			g.logger.Error("geocoder cache set error", zap.Error(err), zap.String("key", hint))
		}
	}

	return pt, nil
}

func (g *geoCodeService) geocode(url string) ([]*Point, error) {
	r, err := http.Get(url)
	if err != nil {
		g.logger.Error("geocoder request error", zap.Error(err))
		return nil, err
	}
	defer r.Body.Close()

	var results GeocoderResults
	err = json.NewDecoder(r.Body).Decode(&results)
	if err != nil || &results == (*GeocoderResults)(nil) || len(results.Results) < 1 {
		g.logger.Error(ERROR_GEOCODING_ADDRESS, zap.Error(err))
		return nil, errors.NewAppError(ERROR_GEOCODING_ADDRESS)
	}

	pts := []*Point{}
	for _, r := range results.Results {
		pts = append(pts, &Point{
			Latitude:         r.Geometry.Location.Lat,
			Longitude:        r.Geometry.Location.Lng,
			FormattedAddress: r.FormattedAddress,
		})
	}

	return pts, nil
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

func (g *geoCodeService) latLngURL(lat, long float64) string {
	latLngStr := fmt.Sprintf("%f,%f", lat, long)
	return fmt.Sprintf(
		"%s%s?latlng=%s&sensor=false&key=%s",
		g.config.Host,
		g.config.Path,
		latLngStr,
		g.config.GeocoderKey,
	)
}

func (g *geoCodeService) postalCodeURL(countryCode, postalCode string) string {
	compStr := fmt.Sprintf("country:%s|postal_code:%s", countryCode, postalCode)
	return fmt.Sprintf(
		"%s%s?components=%s&sensor=false&key=%s",
		g.config.Host,
		g.config.Path,
		compStr,
		g.config.GeocoderKey,
	)
}

func (g *geoCodeService) addressURL(addr AddressQuery) string {
	addrStr := url.QueryEscape(g.addressString(addr))
	return fmt.Sprintf(
		"%s%s?address=%s&sensor=false&key=%s",
		g.config.Host,
		g.config.Path,
		addrStr,
		g.config.GeocoderKey,
	)
}

func (g *geoCodeService) addressComponentURL(address AddressQuery) string {
	components := map[string]string{}
	if address.Street != "" {
		components["street_address"] = url.QueryEscape(address.Street)
	}
	if address.City != "" {
		components["locality"] = url.QueryEscape(address.City)
	}
	if address.State != "" {
		components["administrative_area_level_1"] = url.QueryEscape(address.State)
	}
	if address.PostalCode != "" {
		components["postal_code"] = url.QueryEscape(address.PostalCode)
	}
	if address.Country != "" {
		components["country"] = url.QueryEscape(address.Country)
	}
	compStr := ""
	for k, v := range components {
		if compStr == "" {
			compStr = fmt.Sprintf("%s:%s", k, v)
		} else {
			compStr = fmt.Sprintf("%s|%s:%s", compStr, k, v)
		}
	}
	return fmt.Sprintf(
		"%s%s?components=%s&sensor=false&key=%s",
		g.config.Host,
		g.config.Path,
		compStr,
		g.config.GeocoderKey,
	)
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
