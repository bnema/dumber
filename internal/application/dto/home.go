package dto

import "bnema.dev/dumber/internal/domain/entity"

// HomeLoadResponse is the payload of the home_load message.
//
// Layout is always usable: the parsed home.toml, the last valid one when the
// file is broken, or the built-in default when the file does not exist.
// Layout is redacted (entity.HomeLayout.RedactedForPage) before it is sent.
type HomeLoadResponse struct {
	Layout           entity.HomeLayout            `json:"layout"`
	ConfigPath       string                       `json:"config_path"`
	ConfigExists     bool                         `json:"config_exists"`
	ParseError       *HomeParseError              `json:"parse_error,omitempty"`
	ValidationErrors []entity.HomeValidationError `json:"validation_errors,omitempty"`
	SearchEngine     string                       `json:"search_engine"`
}

// HomeParseError locates a TOML syntax or decoding error in home.toml.
type HomeParseError struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

// HomeSitesKind selects the history list for the top_sites and recent widgets.
type HomeSitesKind string

const (
	HomeSitesTop    HomeSitesKind = "top"
	HomeSitesRecent HomeSitesKind = "recent"
)

// HomeSitesRequest is the payload of the home_sites message.
type HomeSitesRequest struct {
	Kind  HomeSitesKind `json:"kind"`
	Limit int           `json:"limit"`
}

// HomeSite is one row of the top_sites or recent widget.
type HomeSite struct {
	URL        string `json:"url"`
	Title      string `json:"title"`
	VisitCount int64  `json:"visit_count"`
}

// HomeSitesResponse is the payload returned by home_sites.
type HomeSitesResponse struct {
	Sites []HomeSite `json:"sites"`
}

// HomeOpenConfigResponse is returned by home_open_config.
type HomeOpenConfigResponse struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

// HomeChangedEvent is the window event the native side dispatches on system
// view pages when a watched home file changes.
const HomeChangedEvent = "dumber:systemview-changed"

// HomeChangedTopic is the detail.topic value for home.toml changes.
const HomeChangedTopic = "home"
