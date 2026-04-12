package history

import "time"

// DeleteRangeCutoff resolves a delete-range identifier to a cutoff time.
// The booleans report whether the range means "delete all" and whether the
// identifier was recognized.
func DeleteRangeCutoff(rangeID string, now time.Time) (time.Time, bool, bool) {
	switch rangeID {
	case "hour":
		return now.Add(-time.Hour), false, true
	case "day":
		return now.AddDate(0, 0, -1), false, true
	case "week":
		return now.AddDate(0, 0, -7), false, true
	case "month":
		return now.AddDate(0, -1, 0), false, true
	case "all":
		return time.Time{}, true, true
	default:
		return time.Time{}, false, false
	}
}
