package platformfinance

import (
	"errors"
	"time"
)

var (
	ErrInvalidDateRange   = errors.New("start_date cannot be greater than end_date")
	ErrDateRangeTooLarge  = errors.New("date range cannot exceed 366 days")
	ErrOneSidedDate       = errors.New("start_date and end_date must both be provided or both be empty")
	ErrOwnerVenueMismatch = errors.New("owner_profile_id does not own venue_id")
	ErrInvalidDateFormat  = errors.New("invalid date format")
)

var jakartaLocation *time.Location

func init() {
	var err error
	jakartaLocation, err = time.LoadLocation("Asia/Jakarta")
	if err != nil {
		// Fallback if system doesn't have tzdata, though in prod it should
		jakartaLocation = time.FixedZone("Asia/Jakarta", 7*3600)
	}
}

// GetJakartaLocation returns the Asia/Jakarta location
func GetJakartaLocation() *time.Location {
	return jakartaLocation
}

// ParseAndValidateDates parses start and end dates (YYYY-MM-DD), applies default MTD if empty,
// and returns the UTC half-open boundaries: [utcStart, utcEndExclusive).
func ParseAndValidateDates(startDateStr, endDateStr string) (time.Time, time.Time, error) {
	if (startDateStr == "" && endDateStr != "") || (startDateStr != "" && endDateStr == "") {
		return time.Time{}, time.Time{}, ErrOneSidedDate
	}

	var startDate, endDate time.Time
	var err error

	if startDateStr == "" && endDateStr == "" {
		// MTD Jakarta
		nowWIB := time.Now().In(jakartaLocation)
		startDate = time.Date(nowWIB.Year(), nowWIB.Month(), 1, 0, 0, 0, 0, jakartaLocation)
		endDate = time.Date(nowWIB.Year(), nowWIB.Month(), nowWIB.Day(), 0, 0, 0, 0, jakartaLocation)
	} else {
		startDate, err = time.ParseInLocation("2006-01-02", startDateStr, jakartaLocation)
		if err != nil {
			return time.Time{}, time.Time{}, ErrInvalidDateFormat
		}
		endDate, err = time.ParseInLocation("2006-01-02", endDateStr, jakartaLocation)
		if err != nil {
			return time.Time{}, time.Time{}, ErrInvalidDateFormat
		}
	}

	if startDate.After(endDate) {
		return time.Time{}, time.Time{}, ErrInvalidDateRange
	}

	utcStart := startDate.UTC()
	utcEndExclusive := endDate.AddDate(0, 0, 1).UTC()

	// Dates are inclusive at the API boundary. Validate the length of the
	// half-open interval so 366 report days pass and 367 report days fail.
	if utcEndExclusive.Sub(utcStart) > 366*24*time.Hour {
		return time.Time{}, time.Time{}, ErrDateRangeTooLarge
	}

	return utcStart, utcEndExclusive, nil
}

// DetermineGranularity logic
func DetermineGranularity(utcStart, utcEndExclusive time.Time, requested string) string {
	if requested != "" && requested != "auto" {
		return requested
	}
	daysDiff := int(utcEndExclusive.Sub(utcStart).Hours() / 24)
	if daysDiff <= 31 {
		return "day"
	}
	if daysDiff <= 180 {
		return "week"
	}
	return "month"
}
