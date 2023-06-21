package geocode_test

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/comfforts/logger"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/comfforts/geocode"
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
		client geocode.GeoCoder,
	){
		"gecoding postal code succeeds":                testGeocodePostalcode,
		"gecoding address succeeds":                    testGeocodeAddress,
		"gecoding lat/lng succeeds":                    testGeocodeLatLong,
		"gecoding intl lat/lng succeeds":               testIntlLatLong,
		"test distance, succeeds":                      testDistance,
		"test get route for address, succeeds":         testGetRouteForAddress,
		"test get route for lat/long, succeeds":        testGetRouteForLatLong,
		"test get route matrix for lat/long, succeeds": testGetRouteMatrixForLatLong,
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
	client geocode.GeoCoder,
	teardown func(),
) {
	t.Helper()

	appLogger := logger.NewTestAppLogger(testCfg.dir)

	gscCfg := geocode.Config{
		GeocoderKey: testCfg.key,
		AppLogger:   appLogger,
	}
	gsc, err := geocode.NewGeoCodeService(gscCfg)
	require.NoError(t, err)

	return gsc, func() {
		t.Log(" TestGeocoder ended")

		// err = os.RemoveAll(testCfg.dir)
		// require.NoError(t, err)
	}
}

func testGeocodePostalcode(t *testing.T, client geocode.GeoCoder) {
	postalCode := "92612"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pt, err := client.Geocode(ctx, postalCode, "")
	require.NoError(t, err)
	require.Equal(t, "33.66", fmt.Sprintf("%0.2f", pt.Latitude))
	require.Equal(t, "-117.83", fmt.Sprintf("%0.2f", pt.Longitude))
}

func testGeocodeLatLong(t *testing.T, client geocode.GeoCoder) {
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

func testIntlLatLong(t *testing.T, client geocode.GeoCoder) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pt, err := client.GeocodeLatLong(ctx, 22.283939361572266, 114.15818786621094, "Exchange Square")
	require.NoError(t, err)
	fmt.Printf("pt: %v\n", pt)
}

func testGetRouteMatrixForLatLong(t *testing.T, client geocode.GeoCoder) {
	origins := []*geocode.Point{}
	dests := []*geocode.Point{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sPt, err := client.GeocodeAddress(ctx, &geocode.AddressQuery{
		Street:     "6 Turquoise Ct",
		City:       "Petaluma",
		PostalCode: "94952",
		State:      "CA",
		Country:    "USA",
	})
	require.NoError(t, err)
	origins = append(origins, sPt)

	aPt, err := client.GeocodeAddress(ctx, &geocode.AddressQuery{
		Street:     "212 2nd St",
		City:       "Petaluma",
		PostalCode: "94952",
		State:      "CA",
		Country:    "USA",
	})
	require.NoError(t, err)
	dests = append(dests, aPt)
	origins = append(origins, aPt)

	dPt, err := client.GeocodeAddress(ctx, &geocode.AddressQuery{
		Street:     "800 Petaluma Blvd N",
		City:       "Petaluma",
		PostalCode: "94952",
		State:      "CA",
		Country:    "US",
	})
	require.NoError(t, err)
	dests = append(dests, dPt)

	routeLegs, err := client.GetRouteMatrixForLatLong(ctx, origins, dests)
	require.Equal(t, 3, len(routeLegs))
	t.Logf("testGetRouteMatrixForLatLong - routeLegs: %v", routeLegs)
	require.NoError(t, err)
}

func testGetRouteForLatLong(t *testing.T, client geocode.GeoCoder) {
	origin := geocode.AddressQuery{
		Street:     "1600 Amphitheatre Pkwy",
		City:       "Mountain View",
		PostalCode: "94043",
		State:      "CA",
		Country:    "USA",
	}

	destination := geocode.AddressQuery{
		Street:     "1045 La Avenida St",
		City:       "Mountain View",
		PostalCode: "94043",
		State:      "CA",
		Country:    "US",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	oPt, err := client.GeocodeAddress(ctx, &origin)
	require.NoError(t, err)

	dPt, err := client.GeocodeAddress(ctx, &destination)
	require.NoError(t, err)

	routeLegs, err := client.GetRouteForLatLong(ctx, oPt, dPt)
	require.Equal(t, 1, len(routeLegs))
	t.Logf("testGetRouteForLatLong - routeLegs: %v", routeLegs)
	require.NoError(t, err)
}

func testGetRouteForAddress(t *testing.T, client geocode.GeoCoder) {
	origin := geocode.AddressQuery{
		Street:     "1600 Amphitheatre Pkwy",
		City:       "Mountain View",
		PostalCode: "94043",
		State:      "CA",
		Country:    "USA",
	}

	destination := geocode.AddressQuery{
		Street:     "1045 La Avenida St",
		City:       "Mountain View",
		PostalCode: "94043",
		State:      "CA",
		Country:    "US",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	routeLegs, err := client.GetRouteForAddress(ctx, &origin, &destination)
	require.Equal(t, 1, len(routeLegs))
	t.Logf("testGetRouteForAddress - routeLegs: %v", routeLegs)
	require.NoError(t, err)
}

func testGeocodeAddress(t *testing.T, client geocode.GeoCoder) {
	address := geocode.AddressQuery{
		Street:     "1600 Amphitheatre Pkwy",
		City:       "Mountain View",
		PostalCode: "94043",
		State:      "CA",
		Country:    "US",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pt, err := client.GeocodeAddress(ctx, &address)
	require.NoError(t, err)
	assert.Equal(t, pt.FormattedAddress, "Google Building 40, 1600 Amphitheatre Pkwy, Mountain View, CA 94043, USA", "returned address should match")
	t.Logf("geo located to %v", pt)

	address = geocode.AddressQuery{
		Street:     "1045 La Avenida St",
		City:       "Mountain View",
		PostalCode: "94043",
		State:      "CA",
		Country:    "US",
	}
	pt, err = client.GeocodeAddress(ctx, &address)
	require.NoError(t, err)
	assert.Equal(t, pt.FormattedAddress, "1045 La Avenida St, Mountain View, CA 94043, USA", "returned address should match")
	t.Logf("geo located to %v", pt)

	address = geocode.AddressQuery{
		Street:     "2001 Market St",
		City:       "San Francisco",
		PostalCode: "94114",
		State:      "CA",
		Country:    "US",
	}
	pt, err = client.GeocodeAddress(ctx, &address)
	require.NoError(t, err)
	assert.Equal(t, pt.FormattedAddress, "2001 Market St, San Francisco, CA 94114, USA", "returned address should match")
	t.Logf("geo located to %v", pt)

	address = geocode.AddressQuery{
		Street:     "2 Maxwell Ct",
		City:       "San Francisco",
		PostalCode: "94103",
		State:      "CA",
		Country:    "US",
	}
	pt, err = client.GeocodeAddress(ctx, &address)
	require.NoError(t, err)
	assert.Equal(t, pt.FormattedAddress, "2 Maxwell Ct, San Francisco, CA 94103, USA", "returned address should match")
	t.Logf("geo located to %v", pt)

	address = geocode.AddressQuery{
		PostalCode: "94952",
		Country:    "US",
	}
	pt, err = client.GeocodeAddress(ctx, &address)
	require.NoError(t, err)
	assert.Equal(t, pt.FormattedAddress, "Petaluma, CA 94952, USA", "returned address should match")
	t.Logf("geo located to %v", pt)
}

func testDistance(t *testing.T, client geocode.GeoCoder) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pt1, err := client.GeocodeAddress(ctx, &geocode.AddressQuery{
		Street:     "2001 Market St",
		City:       "San Francisco",
		PostalCode: "94114",
		State:      "CA",
		Country:    "USA",
	})
	require.NoError(t, err)

	pt2, err := client.GeocodeAddress(ctx, &geocode.AddressQuery{
		Street:     "2 Maxwell Ct",
		City:       "San Francisco",
		PostalCode: "94103",
		State:      "CA",
		Country:    "USA",
	})
	require.NoError(t, err)
	u := geocode.KM
	d, err := client.GetDistance(ctx, u, pt1, pt2)
	require.NoError(t, err)
	fmt.Printf("%v is %0.2f %s from %v", pt1, d, u, pt2)

	u = geocode.METERS
	d, err = client.GetDistance(ctx, u, pt1, pt2)
	require.NoError(t, err)
	fmt.Printf("%v is %0.2f %s from %v", pt1, d, u, pt2)

	u = geocode.MILES
	d, err = client.GetDistance(ctx, u, pt1, pt2)
	require.NoError(t, err)
	fmt.Printf("%v is %0.2f %s from %v", pt1, d, u, pt2)

	u = geocode.FEET
	d, err = client.GetDistance(ctx, u, pt1, pt2)
	require.NoError(t, err)
	fmt.Printf("%v is %0.2f %s from %v", pt1, d, u, pt2)
}
