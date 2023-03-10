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

type testConfig struct {
	dir    string
	bucket string
	path   string
	key    string
}

func TestGeocoder(t *testing.T) {
	for scenario, fn := range map[string]func(
		t *testing.T,
		client *geoCodeService,
	){
		"gecoding postal code succeeds":   testGeocodePostalcode,
		"gecoding address succeeds":       testGeocodeAddress,
		"gecoding lat/lng succeeds":       testGeocodeLatLong,
		"gecoding intl lat/lng succeeds":  testIntlLatLong,
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
	dataDir := os.Getenv("DATA_DIR")
	credsPath := os.Getenv("CREDS_PATH")
	bktName := os.Getenv("BUCKET_NAME")
	geocoderKey := os.Getenv("GEOCODER_KEY")

	return testConfig{
		dir:    dataDir,
		bucket: bktName,
		path:   credsPath,
		key:    geocoderKey,
	}
}

func setupTest(t *testing.T, testCfg testConfig) (
	client *geoCodeService,
	teardown func(),
) {
	t.Helper()

	appLogger := logger.NewTestAppLogger(testCfg.dir)

	cscCfg := cloudstorage.CloudStorageClientConfig{
		CredsPath: testCfg.path,
	}
	csc, err := cloudstorage.NewCloudStorageClient(cscCfg, appLogger)
	require.NoError(t, err)

	gscCfg := GeoCodeServiceConfig{
		DataDir:     testCfg.dir,
		BucketName:  testCfg.bucket,
		Cached:      true,
		GeocoderKey: testCfg.key,
	}
	gsc, err := NewGeoCodeService(gscCfg, csc, appLogger)
	require.NoError(t, err)

	return gsc, func() {
		t.Log(" TestGeocoder ended")
		err := gsc.Clear()
		require.NoError(t, err)

		// err = os.RemoveAll(testCfg.dir)
		// require.NoError(t, err)
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

func testGeocodeLatLong(t *testing.T, client *geoCodeService) {
	postalCode := "92612"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pt, err := client.Geocode(ctx, postalCode, "")
	require.NoError(t, err)
	require.Equal(t, "33.66", fmt.Sprintf("%0.2f", pt.Latitude))
	require.Equal(t, "-117.83", fmt.Sprintf("%0.2f", pt.Longitude))

	pt, err = client.GeocodeLatLong(ctx, pt.Latitude, pt.Longitude, "Irvine")
	require.NoError(t, err)
	fmt.Printf("pt: %v\n", pt)
}

func testIntlLatLong(t *testing.T, client *geoCodeService) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pt, err := client.GeocodeLatLong(ctx, 22.283939361572266, 114.15818786621094, "Exchange Square")
	require.NoError(t, err)
	fmt.Printf("pt: %v\n", pt)
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
