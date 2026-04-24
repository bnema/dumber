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

	_, todayLabel := historyDateKeyAndLabel(time.Date(2026, 4, 24, 8, 30, 0, 0, loc), now)
	_, yesterdayLabel := historyDateKeyAndLabel(time.Date(2026, 4, 23, 22, 30, 0, 0, loc), now)
	_, olderLabel := historyDateKeyAndLabel(time.Date(2026, 4, 20, 9, 0, 0, 0, loc), now)

	assert.Equal(t, "Today", todayLabel)
	assert.Equal(t, "Yesterday", yesterdayLabel)
	assert.Equal(t, "Mon Apr 20", olderLabel)
}
