package usecase

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
)

// homeSitesOverfetch asks history for extra rows so that dropping internal
// URLs and duplicates still fills the requested limit.
const homeSitesOverfetch = 2

// HomeUseCase serves the dumb://home page.
type HomeUseCase struct {
	source  port.HomeConfigSource
	runtime port.RuntimeConfigProvider
	history port.HomeSitesHistory
	opener  port.FileOpener
}

var _ port.HomePage = (*HomeUseCase)(nil)

// NewHomeUseCase creates a HomeUseCase. history and opener may be nil, which
// disables home_sites and home_open_config respectively.
func NewHomeUseCase(
	source port.HomeConfigSource,
	runtime port.RuntimeConfigProvider,
	history port.HomeSitesHistory,
	opener port.FileOpener,
) *HomeUseCase {
	return &HomeUseCase{source: source, runtime: runtime, history: history, opener: opener}
}

// LoadHome returns the redacted home layout and its diagnostics.
func (uc *HomeUseCase) LoadHome(ctx context.Context) (*dto.HomeLoadResponse, error) {
	if uc.source == nil {
		return nil, errors.New("home config source not configured")
	}
	snap, err := uc.source.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load home config: %w", err)
	}
	resp := &dto.HomeLoadResponse{
		Layout:           snap.Layout.RedactedForPage(),
		ConfigPath:       snap.Path,
		ConfigExists:     snap.Exists,
		ParseError:       snap.ParseError,
		ValidationErrors: snap.ValidationErrors,
	}
	if uc.runtime != nil {
		resp.SearchEngine = uc.runtime.Current().UI.DefaultSearchEngine
	}
	return resp, nil
}

// HomeSites returns the top or recent sites, deduplicated by URL, without
// internal dumb:// pages.
func (uc *HomeUseCase) HomeSites(ctx context.Context, req dto.HomeSitesRequest) (*dto.HomeSitesResponse, error) {
	if uc.history == nil {
		return nil, errors.New("home history not configured")
	}
	limit := req.Limit
	if limit == 0 {
		limit = entity.HomeDefaultLimit
	}
	if limit < entity.HomeMinLimit || limit > entity.HomeMaxLimit {
		return nil, fmt.Errorf("limit must be between %d and %d", entity.HomeMinLimit, entity.HomeMaxLimit)
	}

	var (
		entries []*entity.HistoryEntry
		err     error
	)
	switch req.Kind {
	case dto.HomeSitesTop:
		entries, err = uc.history.GetMostVisitedWithin(ctx, limit*homeSitesOverfetch, 0)
	case dto.HomeSitesRecent:
		entries, err = uc.history.GetRecent(ctx, limit*homeSitesOverfetch, 0)
	default:
		return nil, fmt.Errorf("unknown home sites kind %q", req.Kind)
	}
	if err != nil {
		return nil, fmt.Errorf("load home sites: %w", err)
	}

	sites := make([]dto.HomeSite, 0, limit)
	seen := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e == nil || e.URL == "" || strings.HasPrefix(strings.ToLower(e.URL), "dumb://") {
			continue
		}
		if _, dup := seen[e.URL]; dup {
			continue
		}
		seen[e.URL] = struct{}{}
		sites = append(sites, dto.HomeSite{URL: e.URL, Title: e.Title, VisitCount: e.VisitCount})
		if len(sites) == limit {
			break
		}
	}
	return &dto.HomeSitesResponse{Sites: sites}, nil
}

// OpenHomeConfig creates home.toml if needed and opens it in the user's
// default application. The path always comes from the config source.
func (uc *HomeUseCase) OpenHomeConfig(ctx context.Context) (*dto.HomeOpenConfigResponse, error) {
	if uc.source == nil || uc.opener == nil {
		return nil, errors.New("home config opener not configured")
	}
	created, err := uc.source.EnsureExists()
	if err != nil {
		return nil, fmt.Errorf("create home config: %w", err)
	}
	path := uc.source.Path()
	if err := uc.opener.OpenFile(ctx, path); err != nil {
		return nil, fmt.Errorf("open home config: %w", err)
	}
	return &dto.HomeOpenConfigResponse{Path: path, Created: created}, nil
}
