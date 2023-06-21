package geocode

import (
	"context"
	"fmt"

	"gitlab.com/xerra/common/vincenty"
	"go.uber.org/zap"
	"googlemaps.github.io/maps"

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
}

type Config struct {
	GeocoderKey string `json:"geocoder_key"`
	logger.AppLogger
}

type geoCodeService struct {
	Config
	client *maps.Client
}

func NewGeoCodeService(cfg Config) (*geoCodeService, error) {
	if cfg.GeocoderKey == "" || cfg.AppLogger == nil {
		return nil, errors.NewAppError(errors.ERROR_MISSING_REQUIRED)
	}

	c, err := maps.NewClient(maps.WithAPIKey(cfg.GeocoderKey))
	if err != nil {
		cfg.Error("error initializing google maps client")
		return nil, err
	}

	gcSrv := geoCodeService{
		Config: cfg,
		client: c,
	}

	return &gcSrv, nil
}

func (g *geoCodeService) Geocode(ctx context.Context, postalCode, countryCode string) (*Point, error) {
	if ctx == nil {
		g.Error("context is nil", zap.Error(ErrNilContext))
		return nil, ErrNilContext
	}

	if countryCode == "" {
		countryCode = "USA"
	}

	req := &maps.GeocodingRequest{
		Components: map[maps.Component]string{
			maps.ComponentPostalCode: postalCode,
			maps.ComponentCountry:    countryCode,
		},
	}
	resp, err := g.client.Geocode(ctx, req)
	if err != nil {
		g.Error(ERROR_GEOCODING_POSTAL, zap.Error(err))
		return nil, ErrGeoCodePostalCode
	}

	if len(resp) < 1 {
		g.Error(NO_RESULTS)
		return nil, ErrGeoCodeNoResults
	}

	r := resp[0]
	pt := &Point{
		Latitude:         r.Geometry.Location.Lat,
		Longitude:        r.Geometry.Location.Lng,
		FormattedAddress: r.FormattedAddress,
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
		g.Error("error getting route", zap.Error(err))
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
		g.Error("error getting route matrix", zap.Error(err))
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
		g.Error("context is nil", zap.Error(ErrNilContext))
		return nil, ErrNilContext
	}

	if addr.Country == "" {
		addr.Country = "USA"
	}

	req := &maps.GeocodingRequest{
		Address: addr.addressString(),
	}

	resp, err := g.client.Geocode(ctx, req)
	if err != nil {
		g.Error(ERROR_GEOCODING_ADDRESS, zap.Error(err))
		return nil, ErrGeoCodeAddress
	}

	if len(resp) < 1 {
		g.Error(NO_RESULTS)
		return nil, ErrGeoCodeNoResults
	}

	r := resp[0]
	pt := &Point{
		Latitude:         r.Geometry.Location.Lat,
		Longitude:        r.Geometry.Location.Lng,
		FormattedAddress: r.FormattedAddress,
	}

	return pt, nil
}

func (g *geoCodeService) GeocodeLatLong(ctx context.Context, lat, long float64, hint string) (*Point, error) {
	if ctx == nil {
		g.Error("context is nil", zap.Error(ErrNilContext))
		return nil, ErrNilContext
	}

	req := &maps.GeocodingRequest{
		LatLng: &maps.LatLng{
			Lat: lat,
			Lng: long,
		},
	}
	resp, err := g.client.Geocode(ctx, req)
	if err != nil {
		g.Error(ERROR_GEOCODING_ADDRESS, zap.Error(err))
		return nil, ErrGeoCodeAddress
	}

	if len(resp) < 1 {
		g.Error(NO_RESULTS)
		return nil, ErrGeoCodeNoResults
	}

	r := resp[0]
	pt := &Point{
		Latitude:         r.Geometry.Location.Lat,
		Longitude:        r.Geometry.Location.Lng,
		FormattedAddress: r.FormattedAddress,
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
