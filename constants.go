package geocode

import (
	"errors"
	"time"
)

const (
	OneYear       = 365 * 24 * 30 * time.Hour
	ThirtyDays    = 24 * 30 * time.Hour
	OneDay        = 24 * time.Hour
	FiveHours     = 5 * time.Hour
	OneHour       = time.Hour
	ThirtyMinutes = 30 * time.Minute
)

const (
	ERROR_GEOCODING_POSTAL  string = "error geocoding postal code"
	ERROR_GEOCODING_ADDRESS string = "error geocoding address"
	ERROR_NO_FILE           string = "%s doesn't exist"
	ERROR_FILE_INACCESSIBLE string = "%s inaccessible"
	ERROR_CREATING_FILE     string = "creating file %s"
	NO_RESULTS              string = "no results found"
	ERR_INVALID_LAT_LNG     string = "invalid geo lat/lng"
	ERR_INVALID_UNIT        string = "invalid geo distance unit"
)

var (
	ErrNilContext        = errors.New("context is nil")
	ErrGeoCodePostalCode = errors.New(ERROR_GEOCODING_POSTAL)
	ErrGeoCodeAddress    = errors.New(ERROR_GEOCODING_ADDRESS)
	ErrGeoCodeNoResults  = errors.New(NO_RESULTS)
	ErrInvalidGeoLatLng  = errors.New(ERR_INVALID_LAT_LNG)
	ErrInvalidGeoUnit    = errors.New(ERR_INVALID_UNIT)
)
