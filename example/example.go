package main

import (
	"context"
	"os"

	"github.com/comfforts/cloudstorage"
	"github.com/comfforts/geocode"
	"github.com/comfforts/logger"
	"go.uber.org/zap"
)

func main() {
	dataDir := os.Getenv("DATA_DIR")
	logger := logger.NewTestAppLogger(dataDir)

	credsPath := os.Getenv("CREDS_PATH")
	bktName := os.Getenv("BUCKET_NAME")
	geocoderKey := os.Getenv("GEOCODER_KEY")

	cscCfg := cloudstorage.CloudStorageClientConfig{
		CredsPath: credsPath,
	}
	csc, err := cloudstorage.NewCloudStorageClient(cscCfg, logger)
	if err != nil {
		logger.Fatal("error setting up cloud storage client", zap.Error(err))
		return
	}

	gscCfg := geocode.GeoCodeServiceConfig{
		DataDir:     dataDir,
		BucketName:  bktName,
		Cached:      true,
		GeocoderKey: geocoderKey,
	}
	gsc, err := geocode.NewGeoCodeService(gscCfg, csc, logger)
	if err != nil {
		logger.Fatal("error setting up goecode service", zap.Error(err))
		return
	}

	testGeocoding(gsc, logger)
}

func testGeocoding(gsc geocode.GeoCoder, logger logger.AppLogger) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	postalCode, country := "94952", "USA"

	pt, err := gsc.Geocode(ctx, postalCode, country)
	if err != nil {
		logger.Error("error geocoding", zap.Error(err), zap.String("postalcode", postalCode), zap.String("country", country))
		return
	}
	logger.Info("gecoded", zap.String("postalcode", postalCode), zap.String("country", country), zap.Any("point", pt))

	err = gsc.Clear()
	if err != nil {
		logger.Error("error cleaning up cache", zap.Error(err))
		return
	}
}
