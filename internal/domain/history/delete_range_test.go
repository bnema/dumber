package history

import (
	"testing"
	"time"
)

func TestDeleteRangeCutoff(t *testing.T) {
	now := time.Date(2026, time.April, 11, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		rangeID string
		want    time.Time
		wantAll bool
	}{
		{name: "hour", rangeID: "hour", want: now.Add(-time.Hour), wantAll: false},
		{name: "day", rangeID: "day", want: now.AddDate(0, 0, -1), wantAll: false},
		{name: "week", rangeID: "week", want: now.AddDate(0, 0, -7), wantAll: false},
		{name: "month", rangeID: "month", want: now.AddDate(0, -1, 0), wantAll: false},
		{name: "all", rangeID: "all", want: time.Time{}, wantAll: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotAll := DeleteRangeCutoff(tt.rangeID, now)
			if !got.Equal(tt.want) {
				t.Fatalf("DeleteRangeCutoff() cutoff = %v, want %v", got, tt.want)
			}
			if gotAll != tt.wantAll {
				t.Fatalf("DeleteRangeCutoff() all = %v, want %v", gotAll, tt.wantAll)
			}
		})
	}
}
