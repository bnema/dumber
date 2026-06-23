package port

import "github.com/bnema/dumber/internal/domain/entity"

type RuntimeConfigProvider interface {
	Current() RuntimeConfigSnapshot
	Watch() error
	OnChange(func(RuntimeConfigSnapshot))
}

type RuntimeConfigSnapshot struct {
	EngineSettings EngineSettingsPayload
	UI             RuntimeUIConfig
}

type RuntimeUIConfig struct {
	DefaultUIScale      float64
	SidebarWidth        int
	Appearance          entity.AppearanceConfig
	Workspace           entity.WorkspaceConfig
	Session             entity.SessionConfig
	Clipboard           RuntimeClipboardConfig
	SearchShortcuts     map[string]RuntimeSearchShortcut
	DefaultSearchEngine string
	Omnibox             RuntimeOmniboxConfig
	Update              RuntimeUpdateConfig
	Downloads           RuntimeDownloadsConfig
}

type RuntimeClipboardConfig struct {
	AutoCopyOnSelection bool
}

type RuntimeSearchShortcut struct {
	URL         string
	Description string
}

type RuntimeOmniboxConfig struct {
	InitialBehavior   entity.OmniboxInitialBehavior
	MostVisitedDays   int
	AutoOpenOnNewPane bool
}

type RuntimeUpdateConfig struct {
	EnableOnStartup     bool
	AutoDownload        bool
	NotifyOnNewSettings bool
}

type RuntimeDownloadsConfig struct {
	Path string
}
