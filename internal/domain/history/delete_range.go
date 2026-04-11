package history

import "time"

// DeleteRangeCutoff resolves a delete-range identifier to a cutoff time.
// The boolean reports whether the range means "delete all".
func DeleteRangeCutoff(rangeID string, now time.Time) (time.Time, bool) {
	switch rangeID {
	case "hour":
		return now.Add(-time.Hour), false
	case "day":
		return now.AddDate(0, 0, -1), false
	case "week":
		return now.AddDate(0, 0, -7), false
	case "month":
		return now.AddDate(0, -1, 0), false
	case "all":
		return time.Time{}, true
	default:
		return now, false
	}
}
