package systemviews

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHistoryDateKeyAndLabelUsesProvidedNow(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("test", 2*60*60)
	now := time.Date(2026, 4, 24, 12, 0, 0, 0, loc)

	todayKey, todayLabel := historyDateKeyAndLabel(time.Date(2026, 4, 24, 8, 30, 0, 0, loc), now)
	yesterdayKey, yesterdayLabel := historyDateKeyAndLabel(time.Date(2026, 4, 23, 22, 30, 0, 0, loc), now)
	olderKey, olderLabel := historyDateKeyAndLabel(time.Date(2026, 4, 20, 9, 0, 0, 0, loc), now)

	assert.Equal(t, "2026-04-24", todayKey)
	assert.Equal(t, "2026-04-23", yesterdayKey)
	assert.Equal(t, "2026-04-20", olderKey)
	assert.Equal(t, "Today", todayLabel)
	assert.Equal(t, "Yesterday", yesterdayLabel)
	assert.Equal(t, "Mon Apr 20", olderLabel)
}
