package ctlkit

import "time"

// DateLayout is how a date is written on the command line.
const DateLayout = "2006-01-02"

// ParseDate reads the value of a date flag as midnight in zone. flag names
// the flag in the error.
func ParseDate(flag, value string, zone *time.Location) (time.Time, error) {
	day, err := time.ParseInLocation(DateLayout, value, zone)
	if err != nil {
		return time.Time{}, Usagef("--%s takes a date as yyyy-mm-dd, not %q", flag, value)
	}
	return day, nil
}

// LocalTime reads a time that a service wrote without a zone, as a time in
// zone. A time written with a zone keeps it. Anything else, such as null or
// an empty string, gives the zero time. Find out which zone the service
// means before you rely on it.
func LocalTime(text string, zone *time.Location) time.Time {
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.999999999", "2006-01-02 15:04:05", DateLayout} {
		if t, err := time.ParseInLocation(layout, text, zone); err == nil {
			return t
		}
	}
	return time.Time{}
}
