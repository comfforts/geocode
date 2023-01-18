package geocode

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/comfforts/cloudstorage"
	"github.com/comfforts/logger"

	"github.com/stretchr/testify/require"
)

const TEST_DIR = "test-data"

type testConfig struct {
	dir         string
	bucket      string
	credsPath   string
	geocoderKey string
}

func TestGeocoder(t *testing.T) {
	for scenario, fn := range map[string]func(
		t *testing.T,
		client GeoCoder,
	){
		"request gecoding succeeds": testGeocode,
	} {
		testCfg := getTestConfig()
		t.Run(scenario, func(t *testing.T) {
			client, teardown := setupTest(t, testCfg)
			defer teardown()
			fn(t, client)
		})
	}
}

func getTestConfig() testConfig {
	return testConfig{
		dir:         fmt.Sprintf("%s/", TEST_DIR),
		bucket:      "mustum-fustum",            // add a valid bucket name
		credsPath:   "creds/mustum-fustum.json", // add valid creds and path
		geocoderKey: "APIKEY%^$&*#APIKEY",       // add valid google geocode api key
	}
}

func setupTest(t *testing.T, testCfg testConfig) (
	client GeoCoder,
	teardown func(),
) {
	t.Helper()

	appLogger := logger.NewTestAppLogger(TEST_DIR)

	cscCfg := cloudstorage.CloudStorageClientConfig{
		CredsPath: testCfg.credsPath,
	}
	csc, err := cloudstorage.NewCloudStorageClient(cscCfg, appLogger)
	require.NoError(t, err)

	gscCfg := GeoCodeServiceConfig{
		DataDir:     TEST_DIR,
		BucketName:  testCfg.bucket,
		GeocoderKey: testCfg.geocoderKey,
	}
	gsc, err := NewGeoCodeService(gscCfg, csc, appLogger)
	require.NoError(t, err)

	return gsc, func() {
		t.Log(" TestGeocoder ended")

		err = os.RemoveAll(TEST_DIR)
		require.NoError(t, err)
	}
}

func testGeocode(t *testing.T, client GeoCoder) {
	postalCode := "92612"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pt, err := client.Geocode(ctx, postalCode, "")
	require.NoError(t, err)
	require.Equal(t, "33.66", fmt.Sprintf("%0.2f", pt.Latitude))
	require.Equal(t, "-117.83", fmt.Sprintf("%0.2f", pt.Longitude))
}
