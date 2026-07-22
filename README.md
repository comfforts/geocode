# geocode

`geocode` is a small Go package that wraps Google Maps geocoding, directions, and distance matrix APIs behind a compact interface. It also provides local distance calculations between latitude/longitude points using Vincenty's formula.

## Features

- Geocode a postal code and country into latitude/longitude coordinates.
- Geocode structured address fields into coordinates and a formatted address.
- Reverse geocode latitude/longitude coordinates into a formatted address.
- Fetch route legs for address pairs or latitude/longitude pairs.
- Fetch route matrices for multiple origins and destinations.
- Calculate direct point-to-point distances in kilometers, miles, meters, or feet.

## Installation

```sh
go get github.com/comfforts/geocode
```

## Configuration

The service requires a Google Maps API key with access to the APIs used by your workflow, such as Geocoding, Directions, and Distance Matrix.

```go
cfg := geocode.Config{
	GeocoderKey: os.Getenv("GEOCODER_KEY"),
}
```

The example and tests also use `github.com/comfforts/logger`, which can write logs to `DATA_DIR`.

Common environment variables:

| Variable | Purpose |
| --- | --- |
| `GEOCODER_KEY` | Google Maps API key used by the geocoder service. |
| `DATA_DIR` | Directory used by the logger in examples and tests. |
| `CREDS_PATH` | Optional credentials path used by existing test setup. |
| `BUCKET_NAME` | Optional bucket name used by existing test setup. |

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/comfforts/geocode"
)

func main() {
	ctx := context.Background()

	client, err := geocode.NewGeoCodeService(ctx, geocode.Config{
		GeocoderKey: os.Getenv("GEOCODER_KEY"),
	})
	if err != nil {
		panic(err)
	}

	point, err := client.Geocode(ctx, "94952", "USA")
	if err != nil {
		panic(err)
	}

	fmt.Printf("%s: %f, %f\n", point.FormattedAddress, point.Latitude, point.Longitude)
}
```

## Usage

### Geocode a Postal Code

```go
point, err := client.Geocode(ctx, "92612", "USA")
if err != nil {
	return err
}
```

If `countryCode` is empty, the package defaults it to `USA`.

### Geocode an Address

```go
point, err := client.GeocodeAddress(ctx, &geocode.AddressQuery{
	Street:     "1600 Amphitheatre Pkwy",
	City:       "Mountain View",
	State:      "CA",
	PostalCode: "94043",
	Country:    "US",
})
if err != nil {
	return err
}
```

If `Country` is empty, the package defaults it to `USA`.

### Reverse Geocode Coordinates

```go
point, err := client.GeocodeLatLong(ctx, 37.422, -122.084, "")
if err != nil {
	return err
}
```

The `hint` argument is currently accepted by the interface but is not used in the request.

### Calculate Direct Distance

```go
source := &geocode.Point{Latitude: 37.422, Longitude: -122.084}
dest := &geocode.Point{Latitude: 37.7749, Longitude: -122.4194}

distance, err := client.GetDistance(ctx, geocode.MILES, source, dest)
if err != nil {
	return err
}
```

Supported distance units:

- `geocode.KM`
- `geocode.MILES`
- `geocode.METERS`
- `geocode.FEET`

### Get a Route

```go
legs, err := client.GetRouteForAddress(ctx,
	&geocode.AddressQuery{
		Street:     "1600 Amphitheatre Pkwy",
		City:       "Mountain View",
		State:      "CA",
		PostalCode: "94043",
		Country:    "USA",
	},
	&geocode.AddressQuery{
		Street:     "1045 La Avenida St",
		City:       "Mountain View",
		State:      "CA",
		PostalCode: "94043",
		Country:    "US",
	},
)
if err != nil {
	return err
}
```

Each `RouteLeg` includes start and end points, duration, and distance in meters.

### Get a Route Matrix

```go
origins := []*geocode.AddressQuery{
	{Street: "1600 Amphitheatre Pkwy", City: "Mountain View", State: "CA", PostalCode: "94043", Country: "USA"},
}

destinations := []*geocode.AddressQuery{
	{Street: "1045 La Avenida St", City: "Mountain View", State: "CA", PostalCode: "94043", Country: "US"},
}

legs, err := client.GetRouteMatrixForAddress(ctx, origins, destinations)
if err != nil {
	return err
}
```

Route matrix calls omit origin/destination pairs where the resolved origin and destination address are identical. The returned route legs include distance matrix duration and distance, and the implementation performs follow-up geocoding to populate coordinates for resolved addresses when possible.

## API

The main interface is:

```go
type GeoCoder interface {
	Geocode(ctx context.Context, postalCode, countryCode string) (*Point, error)
	GeocodeAddress(ctx context.Context, addr *AddressQuery) (*Point, error)
	GeocodeLatLong(ctx context.Context, lat, long float64, hint string) (*Point, error)
	GetDistance(ctx context.Context, u DistanceUnit, source, dest *Point) (float64, error)
	GetRouteForLatLong(ctx context.Context, origin, destination *Point) ([]*RouteLeg, error)
	GetRouteForAddress(ctx context.Context, origin, destination *AddressQuery) ([]*RouteLeg, error)
	GetRouteMatrixForLatLong(ctx context.Context, origins, destinations []*Point) ([]*RouteLeg, error)
	GetRouteMatrixForAddress(ctx context.Context, origins, destinations []*AddressQuery) ([]*RouteLeg, error)
}
```

Create an implementation with:

```go
client, err := geocode.NewGeoCodeService(ctx, geocode.Config{
	GeocoderKey: os.Getenv("GEOCODER_KEY"),
})
```

## Running the Example

Set the required environment variables, then run:

```sh
go run ./example
```

The example reads `GEOCODER_KEY` and `DATA_DIR`, creates a geocoder service, and geocodes a sample postal code.

## Tests

The tests exercise live Google Maps API calls and require environment configuration before they can pass consistently:

```sh
export GEOCODER_KEY="your-google-maps-api-key"
export DATA_DIR="/tmp/geocode-test"
export CREDS_PATH="creds/creds.json"
export BUCKET_NAME="your-bucket"
```

Then run:

```sh
go test ./...
```

## Error Handling

The package exposes sentinel errors for common failures:

- `ErrGeoCodePostalCode`
- `ErrGeoCodeAddress`
- `ErrGeoCodeNoResults`
- `ErrInvalidGeoLatLng`
- `ErrInvalidGeoUnit`

`NewGeoCodeService` returns an error when `Config.GeocoderKey` is empty.

## Notes

- Google Maps API usage may incur costs depending on your Google Cloud project configuration.
- `Point.IsValid` treats zero latitude or longitude as invalid, so coordinates on the equator or prime meridian are not considered valid for local distance calculations.
- Address components are joined with spaces in the order `Street`, `City`, `State`, `PostalCode`, `Country`.
