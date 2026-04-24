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
		wantOK  bool
	}{
		{name: "hour", rangeID: "hour", want: now.Add(-time.Hour), wantAll: false, wantOK: true},
		{name: "day", rangeID: "day", want: time.Date(2026, time.April, 11, 0, 0, 0, 0, time.UTC), wantAll: false, wantOK: true},
		{name: "week", rangeID: "week", want: now.AddDate(0, 0, -7), wantAll: false, wantOK: true},
		{name: "month", rangeID: "month", want: now.AddDate(0, -1, 0), wantAll: false, wantOK: true},
		{name: "all", rangeID: "all", want: time.Time{}, wantAll: true, wantOK: true},
		{name: "unknown", rangeID: "bogus", want: time.Time{}, wantAll: false, wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotAll, gotOK := DeleteRangeCutoff(tt.rangeID, now)
			if !got.Equal(tt.want) {
				t.Fatalf("DeleteRangeCutoff() cutoff = %v, want %v", got, tt.want)
			}
			if gotAll != tt.wantAll {
				t.Fatalf("DeleteRangeCutoff() all = %v, want %v", gotAll, tt.wantAll)
			}
			if gotOK != tt.wantOK {
				t.Fatalf("DeleteRangeCutoff() ok = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}
