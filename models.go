package geocode

import (
	"fmt"
	"time"
)

type DistanceUnit string

const (
	KM     DistanceUnit = "KM"
	MILES  DistanceUnit = "MILES"
	METERS DistanceUnit = "METERS"
	FEET   DistanceUnit = "FEET"
)

type GeocoderResults struct {
	Results []Result `json:"results"`
	Status  string   `json:"status"`
}

type Result struct {
	AddressComponents []Address `json:"address_components"`
	FormattedAddress  string    `json:"formatted_address"`
	Geometry          Geometry  `json:"geometry"`
	PlaceId           string    `json:"place_id"`
	Types             []string  `json:"types"`
}

// Address store each address is identified by the 'types'
type Address struct {
	LongName  string   `json:"long_name"`
	ShortName string   `json:"short_name"`
	Types     []string `json:"types"`
}

// Geometry store each value in the geometry
type Geometry struct {
	Bounds       Bounds `json:"bounds"`
	Location     LatLng `json:"location"`
	LocationType string `json:"location_type"`
	Viewport     Bounds `json:"viewport"`
}

// Bounds Northeast and Southwest
type Bounds struct {
	Northeast LatLng `json:"northeast"`
	Southwest LatLng `json:"southwest"`
}

// LatLng store the latitude and longitude
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Point struct {
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	FormattedAddress string  `json:"formatted_address"`
}

func (p *Point) IsValid() bool {
	return p.Latitude != 0 && p.Longitude != 0
}

type Range struct {
	Min float64
	Max float64
}
type RangeBounds struct {
	Latitude  Range
	Longitude Range
}

type RouteLeg struct {
	Start    string
	End      string
	Duration time.Duration
	Distance int
}

type AddressQuery struct {
	Street     string
	City       string
	PostalCode string
	State      string
	Country    string
}

func (a *AddressQuery) addressString() string {
	compStr := ""
	if a.Street != "" {
		compStr = a.Street
	}
	if a.City != "" {
		if compStr == "" {
			compStr = a.City
		} else {
			compStr = fmt.Sprintf("%s %s", compStr, a.City)
		}
	}
	if a.State != "" {
		if compStr == "" {
			compStr = a.State
		} else {
			compStr = fmt.Sprintf("%s %s", compStr, a.State)
		}
	}
	if a.PostalCode != "" {
		if compStr == "" {
			compStr = a.PostalCode
		} else {
			compStr = fmt.Sprintf("%s %s", compStr, a.PostalCode)
		}
	}
	if a.Country != "" {
		if compStr == "" {
			compStr = a.Country
		} else {
			compStr = fmt.Sprintf("%s %s", compStr, a.Country)
		}
	}
	return compStr
}
