package systemviews

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRoute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		uri  string
		want Route
	}{
		{name: "history host", uri: "dumb://history", want: RouteHistory},
		{name: "history opaque", uri: "dumb:history", want: RouteHistory},
		{name: "favorites host", uri: "dumb://favorites", want: RouteFavorites},
		{name: "favorites opaque", uri: "dumb:favorites", want: RouteFavorites},
		{name: "config host", uri: "dumb://config", want: RouteConfig},
		{name: "config opaque", uri: "dumb:config", want: RouteConfig},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ParseRoute(tt.uri); got != tt.want {
				t.Fatalf("ParseRoute(%q) = %v, want %v", tt.uri, got, tt.want)
			}
		})
	}
}

func TestAppRunMountsPlaceholderAndRecordsRoute(t *testing.T) {
	t.Parallel()

	dom := &fakeDOM{}
	app := NewApp(Dependencies{
		DOM:         dom,
		LocationURI: "dumb://history",
	})

	require.NoError(t, app.Run())
	assert.Equal(t, RouteHistory, app.CurrentRoute())
	assert.True(t, dom.mounted)
	assert.Contains(t, dom.html, "history")
	assert.Contains(t, dom.html, "systemviews")
}

func TestAppLoadInitialHistoryRouteRendersEntries(t *testing.T) {
	t.Parallel()

	history := &fakeHistoryService{entries: []*entity.HistoryEntry{{
		URL:   "https://example.com",
		Title: "Example",
	}}}

	app := NewApp(Dependencies{
		History:     history,
		LocationURI: "dumb://history",
	})

	require.NoError(t, app.LoadInitial(context.Background()))
	assert.Equal(t, RouteHistory, app.CurrentRoute())
	assert.Equal(t, 1, len(app.historyEntries))
	assert.Contains(t, app.renderedHTML, "Example")
	assert.Contains(t, app.renderedHTML, "https://example.com")
	assert.True(t, history.called)
	assert.Equal(t, 25, history.limit)
	assert.Equal(t, 0, history.offset)
}

type fakeDOM struct {
	mounted bool
	html    string
}

func (d *fakeDOM) Mount(html string) error {
	d.mounted = true
	d.html = html
	return nil
}

type fakeHistoryService struct {
	called  bool
	limit   int
	offset  int
	entries []*entity.HistoryEntry
}

func (s *fakeHistoryService) Timeline(_ context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	s.called = true
	s.limit = limit
	s.offset = offset
	return s.entries, nil
}

func (s *fakeHistoryService) Search(context.Context, string, int) ([]*entity.HistoryEntry, error) {
	return nil, nil
}

func (s *fakeHistoryService) DeleteEntry(context.Context, int64) error { return nil }

func (s *fakeHistoryService) DeleteRange(context.Context, string) error { return nil }

func (s *fakeHistoryService) Analytics(context.Context) (*entity.HistoryAnalytics, error) {
	return nil, nil
}

func (s *fakeHistoryService) DomainStats(context.Context, int) ([]*entity.DomainStat, error) {
	return nil, nil
}

func (s *fakeHistoryService) DeleteDomain(context.Context, string) error { return nil }
