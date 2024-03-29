package geocode

import (
	"time"

	"github.com/comfforts/errors"
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
	ErrNilContext        = errors.NewAppError("context is nil")
	ErrGeoCodePostalCode = errors.NewAppError(ERROR_GEOCODING_POSTAL)
	ErrGeoCodeAddress    = errors.NewAppError(ERROR_GEOCODING_ADDRESS)
	ErrGeoCodeNoResults  = errors.NewAppError(NO_RESULTS)
	ErrInvalidGeoLatLng  = errors.NewAppError(ERR_INVALID_LAT_LNG)
	ErrInvalidGeoUnit    = errors.NewAppError(ERR_INVALID_UNIT)
)
