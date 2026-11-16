package component

import (
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

func TestSessionDateTitle(t *testing.T) {
	now := time.Date(2026, time.November, 16, 12, 0, 0, 0, time.FixedZone("viewer", 3600))
	for _, tt := range []struct {
		name    string
		started time.Time
		want    string
	}{
		{"today", time.Date(2026, 11, 16, 10, 0, 59, 0, now.Location()), "Today, 10:00"},
		{"yesterday", time.Date(2026, 11, 15, 19, 20, 0, 0, now.Location()), "Yesterday, 19:20"},
		{"older", time.Date(2026, 11, 14, 16, 6, 0, 0, now.Location()), "14 Nov, 16:06"},
		{"local date", time.Date(2026, 11, 15, 23, 30, 0, 0, time.UTC), "Today, 00:30"},
		{"unknown", time.Time{}, "Unknown date"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := sessionDateTitle(tt.started, now); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
	// Calendar yesterday must survive the spring-forward 23-hour day.
	loc, err := time.LoadLocation("Europe/Paris")
	if err != nil {
		t.Fatal(err)
	}
	now = time.Date(2026, 3, 30, 0, 15, 0, 0, loc)
	if got := sessionDateTitle(time.Date(2026, 3, 29, 0, 30, 0, 0, loc), now); got != "Yesterday, 00:30" {
		t.Fatal(got)
	}
}

func TestSessionSubtitle(t *testing.T) {
	for _, tt := range []struct {
		panes, tabs int
		want        string
	}{
		{1, 0, "1 pane · 1af9"},
		{3, 1, "3 panes · 1 tab · 1af9"},
		{0, 2, "0 panes · 2 tabs · 1af9"},
	} {
		info := entity.SessionInfo{Session: &entity.Session{ID: "20261116_100059_1af9"}, PaneCount: tt.panes, TabCount: tt.tabs}
		if got := sessionSubtitle(info); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}
