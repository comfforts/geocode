package geocode

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/comfforts/cache"
	"github.com/comfforts/cloudstorage"
	"github.com/comfforts/errors"
	"github.com/comfforts/logger"

	"go.uber.org/zap"
)

type GeoCoder interface {
	Geocode(ctx context.Context, postalCode, countryCode string) (*Point, error)
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

func NewGeoCodeService(cfg GeoCodeServiceConfig, csc cloudstorage.CloudStorage, logger logger.AppLogger) (*geoCodeService, error) {
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

	if g.config.Cached {
		point, exp, err := g.getFromCache(postalCode)
		if err == nil {
			g.logger.Debug("returning cached value", zap.String("postalCode", postalCode), zap.Any("exp", exp))
			return point, nil
		} else {
			g.logger.Error("geocoder cache get error", zap.Error(err), zap.String("postalCode", postalCode))
		}
	}

	if countryCode == "" {
		countryCode = "US"
	}

	url := g.postalCodeURL(countryCode, postalCode)
	r, err := http.Get(url)
	if err != nil {
		g.logger.Error("geocoder request error", zap.Error(err), zap.String("postalCode", postalCode))
		return nil, err
	}
	defer r.Body.Close()

	var results GeocoderResults
	err = json.NewDecoder(r.Body).Decode(&results)
	if err != nil || &results == (*GeocoderResults)(nil) || len(results.Results) < 1 {
		g.logger.Error(ERROR_GEOCODING_POSTALCODE, zap.Error(err), zap.String("postalCode", postalCode))
		return nil, errors.NewAppError(ERROR_GEOCODING_POSTALCODE)
	}
	lat, long := results.Results[0].Geometry.Location.Lat, results.Results[0].Geometry.Location.Lng

	geoPoint := Point{
		Latitude:  lat,
		Longitude: long,
	}

	if g.config.Cached {
		err = g.setInCache(postalCode, geoPoint)
		if err != nil {
			g.logger.Error("geocoder cache set error", zap.Error(err), zap.String("postalCode", postalCode))
		}
	}

	return &geoPoint, nil
}

func (g *geoCodeService) Clear() {
	g.logger.Info("cleaning up geo code data structures")
	if g.config.Cached && g.cache.Updated() {
		err := g.cache.SaveFile()
		if err != nil {
			g.logger.Error("error saving geocoder cache", zap.Error(err))
		} else {
			err = g.uploadCache()
			if err != nil {
				g.logger.Error("error uploading geocoder cache", zap.Error(err))
			}
		}
	}
}

func (g *geoCodeService) postalCodeURL(countryCode, postalCode string) string {
	return fmt.Sprintf("%s%s?components=country:%s|postal_code:%s&sensor=false&key=%s", g.config.Host, g.config.Path, countryCode, postalCode, g.config.GeocoderKey)
}

func (g *geoCodeService) getFromCache(postalCode string) (*Point, time.Time, error) {
	val, exp, err := g.cache.Get(postalCode)
	if err != nil {
		g.logger.Error(cache.ERROR_GET_CACHE, zap.Error(err), zap.String("postalCode", postalCode))
		return nil, exp, err
	}
	point, ok := val.(Point)
	if !ok {
		g.logger.Error("error getting cache point value", zap.Error(cache.ErrGetCache), zap.String("postalCode", postalCode))
		return nil, exp, cache.ErrGetCache
	}
	return &point, exp, nil
}

func (g *geoCodeService) setInCache(postalCode string, point Point) error {
	err := g.cache.Set(postalCode, point, OneYear)
	if err != nil {
		g.logger.Error(cache.ERROR_SET_CACHE, zap.Error(err), zap.String("postalCode", postalCode))
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

	cfr, err := cloudstorage.NewCloudFileRequest(g.config.BucketName, filepath.Base(cacheFile), filepath.Dir(cacheFile), fmod)
	if err != nil {
		g.logger.Error("error creating request", zap.Error(err), zap.String("filepath", cacheFile))
		return err
	}

	n, err := g.cloudStorage.UploadFile(ctx, file, cfr)
	if err != nil {
		g.logger.Error("error uploading file", zap.Error(err))
		return err
	}
	g.logger.Info("uploaded file", zap.String("file", filepath.Base(cacheFile)), zap.String("path", filepath.Dir(cacheFile)), zap.Int64("bytes", n))
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
		g.logger.Info("file mod time", zap.Int64("modtime", fmod), zap.String("filepath", cacheFile))
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
			g.logger.Error("error closing file", zap.Error(err), zap.String("filepath", cacheFile))
		}
	}()

	cfr, err := cloudstorage.NewCloudFileRequest(g.config.BucketName, filepath.Base(cacheFile), filepath.Dir(cacheFile), fmod)
	if err != nil {
		g.logger.Error("error creating request", zap.Error(err), zap.String("filepath", cacheFile))
		return err
	}

	n, err := g.cloudStorage.DownloadFile(ctx, f, cfr)
	if err != nil {
		g.logger.Error("error downloading file", zap.Error(err), zap.String("filepath", cacheFile))
		return err
	}
	g.logger.Info("downloaded file", zap.String("file", filepath.Base(cacheFile)), zap.String("path", filepath.Dir(cacheFile)), zap.Int64("bytes", n))
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
