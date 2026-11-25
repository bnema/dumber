package port

import (
	"context"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/domain/entity"
)

// HomeConfigSnapshot is the result of loading home.toml.
//
// Layout is always usable: the parsed file, the last valid layout when the
// file is broken, or entity.DefaultHomeLayout when none was ever valid.
type HomeConfigSnapshot struct {
	Layout           entity.HomeLayout
	Path             string
	Exists           bool
	ParseError       *dto.HomeParseError
	ValidationErrors []entity.HomeValidationError
	// ContentHash is the SHA-256 of the home.toml bytes, or zero when the
	// file is missing or unreadable. Equal hashes mean an unchanged layout.
	ContentHash [32]byte
}

// HomeConfigSource loads the user's home.toml.
type HomeConfigSource interface {
	// Load reads and validates home.toml. Parse and validation problems are
	// reported in the snapshot; the error is only set for I/O failures.
	Load(ctx context.Context) (HomeConfigSnapshot, error)
	// Path returns the absolute path of home.toml.
	Path() string
	// EnsureExists writes a commented starter home.toml if it is missing.
	EnsureExists() (created bool, err error)
}

// FileOpener opens a local file with the user's default application.
type FileOpener interface {
	OpenFile(ctx context.Context, path string) error
}

// HomeSitesHistory is the history access needed by the home page widgets.
type HomeSitesHistory interface {
	GetRecent(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error)
	GetMostVisitedWithin(ctx context.Context, limit, maxAgeDays int) ([]*entity.HistoryEntry, error)
}

// HomePage serves the dumb://home messages.
type HomePage interface {
	LoadHome(ctx context.Context) (*dto.HomeLoadResponse, error)
	HomeSites(ctx context.Context, req dto.HomeSitesRequest) (*dto.HomeSitesResponse, error)
	OpenHomeConfig(ctx context.Context) (*dto.HomeOpenConfigResponse, error)
	FetchWidgetData(ctx context.Context, serviceID string) (*dto.HomeWidgetDataResponse, error)
	RunHomeAction(ctx context.Context, req dto.HomeActionRequest) (*dto.HomeActionResponse, error)
}

// HomeTemplateRenderer renders an api widget template to sanitized HTML.
// Errors carry only the template parse or execution message.
type HomeTemplateRenderer interface {
	Render(tmpl string, data any) (string, error)
}
