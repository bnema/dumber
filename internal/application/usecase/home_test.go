package usecase

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/port/mocks"
	"bnema.dev/dumber/internal/domain/entity"
)

func TestHomeUseCase_LoadHome_RedactsAndAddsSearchEngine(t *testing.T) {
	ctx := context.Background()
	layout := entity.DefaultHomeLayout()
	layout.Groups = []entity.HomeGroup{{Name: "G", Services: []entity.HomeService{{
		ID: "svc", Name: "Svc",
		Widget: &entity.HomeServiceWidget{
			Type: entity.HomeServiceWidgetAPI, URL: "https://x/?token=SECRET",
			Headers: map[string]string{"X": "SECRET"},
		},
	}}}}
	parseErr := &dto.HomeParseError{File: "/c/home.toml", Line: 2, Column: 1, Message: "bad"}

	source := mocks.NewMockHomeConfigSource(t)
	source.EXPECT().Load(ctx).Return(port.HomeConfigSnapshot{
		Layout: layout, Path: "/c/home.toml", Exists: true, ParseError: parseErr,
	}, nil)
	runtime := mocks.NewMockRuntimeConfigProvider(t)
	runtime.EXPECT().Current().Return(entity.RuntimeConfigSnapshot{
		UI: entity.RuntimeUIConfig{DefaultSearchEngine: "https://s/?q=%s"},
	})

	resp, err := NewHomeUseCase(HomeDeps{Source: source, Runtime: runtime}).LoadHome(ctx)
	require.NoError(t, err)
	assert.Equal(t, "/c/home.toml", resp.ConfigPath)
	assert.True(t, resp.ConfigExists)
	assert.Equal(t, parseErr, resp.ParseError)
	assert.Equal(t, "https://s/?q=%s", resp.SearchEngine)

	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "SECRET")
}

func TestHomeUseCase_LoadHome_SourceError(t *testing.T) {
	ctx := context.Background()
	source := mocks.NewMockHomeConfigSource(t)
	source.EXPECT().Load(ctx).Return(port.HomeConfigSnapshot{}, errors.New("io"))
	_, err := NewHomeUseCase(HomeDeps{Source: source}).LoadHome(ctx)
	require.Error(t, err)
}

func TestHomeUseCase_HomeSites(t *testing.T) {
	entries := []*entity.HistoryEntry{
		{URL: "https://a", Title: "A", VisitCount: 5},
		{URL: "dumb://home", Title: "Home"},
		{URL: "DUMB://history", Title: "History"},
		{URL: "https://a", Title: "A again"},
		{URL: "https://b", Title: "B", VisitCount: 2},
		{URL: "https://c", Title: "C", VisitCount: 1},
	}
	tests := []struct {
		name  string
		req   dto.HomeSitesRequest
		setup func(h *mocks.MockHomeSitesHistory)
		want  []string
		err   bool
	}{
		{
			name: "top with default limit dedupes and skips dumb",
			req:  dto.HomeSitesRequest{Kind: dto.HomeSitesTop},
			setup: func(h *mocks.MockHomeSitesHistory) {
				h.EXPECT().GetMostVisitedWithin(mock.Anything, 16, 0).Return(entries, nil)
			},
			want: []string{"https://a", "https://b", "https://c"},
		},
		{
			name: "recent truncates to limit",
			req:  dto.HomeSitesRequest{Kind: dto.HomeSitesRecent, Limit: 2},
			setup: func(h *mocks.MockHomeSitesHistory) {
				h.EXPECT().GetRecent(mock.Anything, 4, 0).Return(entries, nil)
			},
			want: []string{"https://a", "https://b"},
		},
		{name: "limit too high", req: dto.HomeSitesRequest{Kind: dto.HomeSitesTop, Limit: 51}, err: true},
		{name: "negative limit", req: dto.HomeSitesRequest{Kind: dto.HomeSitesTop, Limit: -1}, err: true},
		{name: "unknown kind", req: dto.HomeSitesRequest{Kind: "all"}, err: true},
		{
			name: "history error",
			req:  dto.HomeSitesRequest{Kind: dto.HomeSitesRecent},
			setup: func(h *mocks.MockHomeSitesHistory) {
				h.EXPECT().GetRecent(mock.Anything, 16, 0).Return(nil, errors.New("db"))
			},
			err: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			history := mocks.NewMockHomeSitesHistory(t)
			if tt.setup != nil {
				tt.setup(history)
			}
			resp, err := NewHomeUseCase(HomeDeps{History: history}).HomeSites(context.Background(), tt.req)
			if tt.err {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			got := make([]string, 0, len(resp.Sites))
			for _, s := range resp.Sites {
				got = append(got, s.URL)
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHomeUseCase_OpenHomeConfig(t *testing.T) {
	ctx := context.Background()
	source := mocks.NewMockHomeConfigSource(t)
	source.EXPECT().EnsureExists().Return(true, nil)
	source.EXPECT().Path().Return("/c/home.toml")
	opener := mocks.NewMockFileOpener(t)
	opener.EXPECT().OpenFile(ctx, "/c/home.toml").Return(nil)

	resp, err := NewHomeUseCase(HomeDeps{Source: source, Opener: opener}).OpenHomeConfig(ctx)
	require.NoError(t, err)
	assert.Equal(t, &dto.HomeOpenConfigResponse{Path: "/c/home.toml", Created: true}, resp)
}

func TestHomeUseCase_OpenHomeConfig_EnsureError(t *testing.T) {
	source := mocks.NewMockHomeConfigSource(t)
	source.EXPECT().EnsureExists().Return(false, errors.New("ro"))
	opener := mocks.NewMockFileOpener(t)
	_, err := NewHomeUseCase(HomeDeps{Source: source, Opener: opener}).OpenHomeConfig(context.Background())
	require.Error(t, err)
}
