package usecase

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
)

// homeSitesOverfetch asks history for extra rows so that dropping internal
// URLs and duplicates still fills the requested limit.
const homeSitesOverfetch = 2

// HomeDeps are the dependencies of HomeUseCase. Nil History, Opener or HTTP
// disable home_sites, home_open_config and widgets/actions respectively.
type HomeDeps struct {
	Source  port.HomeConfigSource
	Runtime port.RuntimeConfigProvider
	History port.HomeSitesHistory
	Opener  port.FileOpener
	HTTP    port.HomeHTTPClient
	// Templates renders api widget templates. Nil makes templated widgets
	// report an error.
	Templates port.HomeTemplateRenderer
	// LookupEnv resolves {{env.NAME}}. Defaults to a lookup that finds nothing.
	LookupEnv func(string) (string, bool)
	// Now defaults to time.Now.
	Now func() time.Time
	// IconVersions sets HomeService.IconVersion for the page. Optional.
	IconVersions port.HomeIconVersioner
	// EmbedOrigins receives the embed origins of each loaded layout. Optional.
	EmbedOrigins port.HomeEmbedOriginSetter
}

// HomeUseCase serves the dumb://home page.
type HomeUseCase struct {
	source    port.HomeConfigSource
	runtime   port.RuntimeConfigProvider
	history   port.HomeSitesHistory
	opener    port.FileOpener
	http      port.HomeHTTPClient
	templates port.HomeTemplateRenderer
	lookupEnv func(string) (string, bool)
	now       func() time.Time
	versions  port.HomeIconVersioner
	origins   port.HomeEmbedOriginSetter

	widgetMu       sync.Mutex
	widgetCache    map[string]homeWidgetCacheEntry
	widgetInflight map[string]*homeWidgetFlight
	// templateFailures remembers a failed template per service and widget
	// hash, so a slow or broken template is not re-rendered on every refresh.
	templateFailures map[string]homeTemplateFailure
	// lastLayoutHash is the layout seen last, to prune state of removed
	// services only when the layout changes.
	lastLayoutHash [sha256.Size]byte
	layoutSeen     bool

	actionMu      sync.Mutex
	actionRunning map[string]bool
}

var _ port.HomePage = (*HomeUseCase)(nil)

// NewHomeUseCase creates a HomeUseCase.
func NewHomeUseCase(deps HomeDeps) *HomeUseCase {
	uc := &HomeUseCase{
		source:           deps.Source,
		runtime:          deps.Runtime,
		history:          deps.History,
		opener:           deps.Opener,
		http:             deps.HTTP,
		templates:        deps.Templates,
		lookupEnv:        deps.LookupEnv,
		now:              deps.Now,
		versions:         deps.IconVersions,
		origins:          deps.EmbedOrigins,
		widgetCache:      map[string]homeWidgetCacheEntry{},
		widgetInflight:   map[string]*homeWidgetFlight{},
		templateFailures: map[string]homeTemplateFailure{},
		actionRunning:    map[string]bool{},
	}
	if uc.lookupEnv == nil {
		uc.lookupEnv = func(string) (string, bool) { return "", false }
	}
	if uc.now == nil {
		uc.now = time.Now
	}
	return uc
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
	// snap.Layout is what the page renders, the last good layout when
	// home.toml does not parse.
	if uc.origins != nil {
		uc.origins.SetHomeEmbedOrigins(snap.Layout.EmbedOrigins())
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
	if uc.versions != nil {
		for gi := range resp.Layout.Groups {
			services := resp.Layout.Groups[gi].Services
			for si := range services {
				services[si].IconVersion = uc.versions.HomeIconVersion(ctx, services[si])
			}
		}
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
