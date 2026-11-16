package component

import (
	"fmt"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

// sessionDateTitle compares calendar dates in the viewer's timezone, including
// across daylight-saving transitions, rather than subtracting 24 hours.
func sessionDateTitle(startedAt, now time.Time) string {
	if startedAt.IsZero() {
		return "Unknown date"
	}
	startedAt = startedAt.In(now.Location())
	day := func(t time.Time) string { return t.Format("2006-01-02") }
	switch day(startedAt) {
	case day(now):
		return "Today, " + startedAt.Format("15:04")
	case day(now.AddDate(0, 0, -1)):
		return "Yesterday, " + startedAt.Format("15:04")
	default:
		return startedAt.Format("2 Jan, 15:04")
	}
}

func sessionSubtitle(info entity.SessionInfo) string {
	count := func(n int, noun string) string {
		if n != 1 {
			noun += "s"
		}
		return fmt.Sprintf("%d %s", n, noun)
	}
	text := count(info.PaneCount, "pane")
	if info.TabCount != 0 {
		text += " · " + count(info.TabCount, "tab")
	}
	return text + " · " + info.Session.ShortID()
}
