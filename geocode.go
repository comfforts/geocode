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

	"gitlab.com/xerra/common/vincenty"
	"go.uber.org/zap"
	"googlemaps.github.io/maps"

	"github.com/comfforts/cache"
	"github.com/comfforts/cloudstorage"
	"github.com/comfforts/errors"
	"github.com/comfforts/logger"
)

type GeoCoder interface {
	Geocode(ctx context.Context, postalCode, countryCode string) (*Point, error)
	GeocodeAddress(ctx context.Context, addr *AddressQuery) (*Point, error)
	GeocodeLatLong(ctx context.Context, lat, long float64, hint string) (*Point, error)
	GetDistance(ctx context.Context, u DistanceUnit, source, dest *Point) (float64, error)
	GetRouteForLatLong(ctx context.Context, origin, destination *Point) ([]*RouteLeg, error)
	GetRouteForAddress(ctx context.Context, origin, destination *AddressQuery) ([]*RouteLeg, error)
	GetRouteMatrixForLatLong(ctx context.Context, origins, destinations []*Point) ([]*RouteLeg, error)
	GetRouteMatrixForAddress(ctx context.Context, origins, destinations []*AddressQuery) ([]*RouteLeg, error)
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

func (g *geoCodeService) GetRouteForLatLong(ctx context.Context, origin, destination *Point) ([]*RouteLeg, error) {
	return g.getRoute(ctx, &maps.DirectionsRequest{
		Origin:      fmt.Sprintf("%.6f %.6f", origin.Latitude, origin.Longitude),
		Destination: fmt.Sprintf("%.6f %.6f", destination.Latitude, destination.Longitude),
	})
}

func (g *geoCodeService) GetRouteForAddress(ctx context.Context, origin, destination *AddressQuery) ([]*RouteLeg, error) {
	return g.getRoute(ctx, &maps.DirectionsRequest{
		Origin:      origin.addressString(),
		Destination: destination.addressString(),
	})
}

func (g *geoCodeService) getRoute(ctx context.Context, req *maps.DirectionsRequest) ([]*RouteLeg, error) {
	routes, _, err := g.client.Directions(context.Background(), req)
	if err != nil {
		g.logger.Error("error getting route", zap.Error(err))
		return nil, err
	}

	routeLegs := []*RouteLeg{}
	for _, rt := range routes {
		for _, l := range rt.Legs {
			routeLegs = append(routeLegs, &RouteLeg{
				Start:    l.StartAddress,
				End:      l.EndAddress,
				Duration: l.Duration,
				Distance: l.Distance.Meters,
			})
		}
	}
	return routeLegs, nil
}

func (g *geoCodeService) GetRouteMatrixForAddress(ctx context.Context, origins, destinations []*AddressQuery) ([]*RouteLeg, error) {
	originStrs := []string{}
	for _, v := range origins {
		originStrs = append(originStrs, v.addressString())
	}

	destStrs := []string{}
	for _, v := range destinations {
		destStrs = append(destStrs, v.addressString())
	}

	return g.getRouteMatrix(ctx, &maps.DistanceMatrixRequest{
		Origins:      originStrs,
		Destinations: destStrs,
	})
}

func (g *geoCodeService) GetRouteMatrixForLatLong(ctx context.Context, origins, destinations []*Point) ([]*RouteLeg, error) {
	originStrs := []string{}
	for _, v := range origins {
		originStrs = append(originStrs, fmt.Sprintf("%.6f %.6f", v.Latitude, v.Longitude))
	}

	destStrs := []string{}
	for _, v := range destinations {
		destStrs = append(destStrs, fmt.Sprintf("%.6f %.6f", v.Latitude, v.Longitude))
	}

	return g.getRouteMatrix(ctx, &maps.DistanceMatrixRequest{
		Origins:      originStrs,
		Destinations: destStrs,
	})
}

func (g *geoCodeService) getRouteMatrix(ctx context.Context, req *maps.DistanceMatrixRequest) ([]*RouteLeg, error) {
	resp, err := g.client.DistanceMatrix(ctx, req)
	if err != nil {
		g.logger.Error("error getting route matrix", zap.Error(err))
		return nil, err
	}

	routeLegs := []*RouteLeg{}
	for i, row := range resp.Rows {
		for j, elem := range row.Elements {
			if resp.OriginAddresses[i] != resp.DestinationAddresses[j] {
				routeLegs = append(routeLegs, &RouteLeg{
					Start:    resp.OriginAddresses[i],
					End:      resp.DestinationAddresses[j],
					Duration: elem.Duration,
					Distance: elem.Distance.Meters,
				})
			}
		}
	}

	return routeLegs, nil
}

func (g *geoCodeService) GeocodeAddress(ctx context.Context, addr *AddressQuery) (*Point, error) {
	if ctx == nil {
		g.logger.Error("context is nil", zap.Error(ErrNilContext))
		return nil, ErrNilContext
	}

	if addr.Country == "" {
		addr.Country = "USA"
	}

	if g.config.Cached {
		key := addr.addressString()
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
		Address: addr.addressString(),
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
		key := addr.addressString()
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

	if g.config.Cached {
		key := g.buildLatLngKey(lat, long)
		point, _, err := g.getFromCache(key)
		if err == nil {
			return point, nil
		} else {
			g.logger.Error("geocoder cache get error", zap.Error(err), zap.String("key", key))
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

	if g.config.Cached {
		key := g.buildLatLngKey(lat, long)
		err = g.setInCache(key, pt, 0)
		if err != nil {
			g.logger.Error("geocoder cache set error", zap.Error(err), zap.String("key", key))
		}
	}

	return pt, nil
}

func (g *geoCodeService) GetDistance(ctx context.Context, u DistanceUnit, source, dest *Point) (float64, error) {
	if source == nil || dest == nil || !source.IsValid() || !dest.IsValid() {
		return 0, ErrInvalidGeoLatLng
	}

	origin := vincenty.LatLng{Latitude: source.Latitude, Longitude: source.Longitude}
	end := vincenty.LatLng{Latitude: dest.Latitude, Longitude: dest.Longitude}
	d := vincenty.Inverse(origin, end)

	switch u {
	case KM:
		return d.Kilometers(), nil
	case MILES:
		return d.Miles(), nil
	case METERS:
		return d.Meters(), nil
	case FEET:
		return d.Feet(), nil
	default:
		return 0, ErrInvalidGeoUnit
	}
}

func (g *geoCodeService) Clear() error {
	if g.config.Cached {
		if g.cache.Updated() {
			g.logger.Info("cleaning up geo code data structures")
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
	}
	return nil
}

func (g *geoCodeService) getFromCache(key string) (*Point, time.Time, error) {
	key = g.buildKey(key)
	val, exp, err := g.cache.Get(key)
	if err != nil {
		return nil, exp, errors.WrapError(err, "error getting cache value for %s", key)
	}

	if exp.Unix() <= time.Now().Unix() {
		return nil, exp, errors.NewAppError("cache expired for %s", key)
	}
	point, ok := val.(Point)
	if !ok {
		return nil, exp, errors.NewAppError("error marshalling value from cache item for %s", key)
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

	key = g.buildKey(key)
	err := g.cache.Set(key, point, cacheFor)
	if err != nil {
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

func (g *geoCodeService) buildKey(key string) string {
	key = strings.ReplaceAll(key, " ", "")
	key = strings.ToLower(key)
	key = url.QueryEscape(key)
	return key
}

func (g *geoCodeService) buildLatLngKey(lat, lng float64) string {
	if lat == 0 || lng == 0 {
		return "lat0lng0"
	}
	key := fmt.Sprintf("lat%.6flng%.6f", lat, lng)
	key = strings.ReplaceAll(key, ".", "dot")
	key = strings.ReplaceAll(key, "-", "min")
	return key
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
