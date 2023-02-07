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

var (
	ErrNilContext = errors.NewAppError("context is nil")
)

const (
	ERROR_GEOCODING_POSTALCODE string = "error geocoding postal code"
	ERROR_GEOCODING_ADDRESS    string = "error geocoding address"
	ERROR_NO_FILE              string = "%s doesn't exist"
	ERROR_FILE_INACCESSIBLE    string = "%s inaccessible"
	ERROR_CREATING_FILE        string = "creating file %s"
)
