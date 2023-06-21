package main

import (
	"context"
	"os"

	"github.com/comfforts/geocode"
	"github.com/comfforts/logger"
	"go.uber.org/zap"
)

func main() {
	dataDir := os.Getenv("DATA_DIR")
	appLogger := logger.NewTestAppLogger(dataDir)

	geocoderKey := os.Getenv("GEOCODER_KEY")

	gscCfg := geocode.Config{
		GeocoderKey: geocoderKey,
		AppLogger:   appLogger,
	}
	gsc, err := geocode.NewGeoCodeService(gscCfg)
	if err != nil {
		appLogger.Fatal("error setting up goecode service", zap.Error(err))
		return
	}

	testGeocoding(gsc, appLogger)
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
}
