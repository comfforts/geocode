package main

import (
	"context"
	"os"

	"github.com/comfforts/geocode"
	"github.com/comfforts/logger"
)

func main() {
	dataDir := os.Getenv("DATA_DIR")

	geocoderKey := os.Getenv("GEOCODER_KEY")

	gscCfg := geocode.Config{
		GeocoderKey: geocoderKey,
	}

	l := logger.GetSlogMultiLogger(dataDir)
	ctx := logger.WithLogger(context.Background(), l)

	gsc, err := geocode.NewGeoCodeService(ctx, gscCfg)
	if err != nil {
		l.Error("error setting up goecode service", "error", err.Error())
		return
	}

	testGeocoding(ctx, gsc)
}

func testGeocoding(ctx context.Context, gsc geocode.GeoCoder) {
	l, err := logger.LoggerFromContext(ctx)
	if err != nil {
		l = logger.GetSlogLogger()
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	postalCode, country := "94952", "USA"

	pt, err := gsc.Geocode(ctx, postalCode, country)
	if err != nil {
		l.Error("error geocoding", "error", err.Error(), "postalcode", postalCode, "country", country)
		return
	}
	l.Info("gecoded", "postalcode", postalCode, "country", country, "point", pt)
}
