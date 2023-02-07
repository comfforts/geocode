package geocode

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"strings"
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
		client *geoCodeService,
	){
		"gecoding postal code succeeds":   testGeocodePostalcode,
		"gecoding address succeeds":       testGeocodeAddress,
		"cache compression test succeeds": testCompression,
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
	client *geoCodeService,
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

func testGeocodePostalcode(t *testing.T, client *geoCodeService) {
	postalCode := "92612"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pt, err := client.Geocode(ctx, postalCode, "")
	require.NoError(t, err)
	require.Equal(t, "33.66", fmt.Sprintf("%0.2f", pt.Latitude))
	require.Equal(t, "-117.83", fmt.Sprintf("%0.2f", pt.Longitude))
}

func testGeocodeAddress(t *testing.T, client *geoCodeService) {
	address := AddressQuery{
		Street:     "1600 Amphitheatre Pkwy",
		City:       "Mountain View",
		PostalCode: "94043",
		State:      "CA",
		Country:    "US",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := client.GeocodeAddress(ctx, address)
	require.NoError(t, err)

	address = AddressQuery{
		Street:     "1045 La Avenida St",
		City:       "Mountain View",
		PostalCode: "94043",
		State:      "CA",
		Country:    "US",
	}
	_, err = client.GeocodeAddress(ctx, address)
	require.NoError(t, err)

	address = AddressQuery{
		Street:     "2001 Market St",
		City:       "San Francisco",
		PostalCode: "94114",
		State:      "CA",
		Country:    "US",
	}
	_, err = client.GeocodeAddress(ctx, address)
	require.NoError(t, err)

	address = AddressQuery{
		Street:     "2 Maxwell Ct",
		City:       "San Francisco",
		PostalCode: "94103",
		State:      "CA",
		Country:    "US",
	}
	_, err = client.GeocodeAddress(ctx, address)
	require.NoError(t, err)

	address = AddressQuery{
		PostalCode: "94952",
		Country:    "US",
	}
	_, err = client.GeocodeAddress(ctx, address)
	require.NoError(t, err)
}

func testCompression(t *testing.T, geo *geoCodeService) {
	addr := AddressQuery{
		Street:     "2 Maxwell Ct",
		City:       "San Francisco",
		PostalCode: "94103",
		State:      "CA",
		Country:    "US",
	}
	addrStr := strings.ToLower(geo.addressString(addr))
	addrStr = url.QueryEscape(addrStr)
	fmt.Println("address string: ", addrStr)
	sEnc := base64.StdEncoding.EncodeToString([]byte(addrStr))
	fmt.Printf("encoded address string: %s, len: %d\n", sEnc, len(sEnc))

	sDec, _ := base64.StdEncoding.DecodeString(sEnc)
	fmt.Printf("decoded address string: %s, len: %d\n", string(sDec), len(string(sDec)))
}
