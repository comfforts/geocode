package geocode

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
type Range struct {
	Min float64
	Max float64
}
type RangeBounds struct {
	Latitude  Range
	Longitude Range
}
