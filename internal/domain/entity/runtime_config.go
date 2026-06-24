package entity

// EngineHardwareDecodingMode controls engine-facing video hardware acceleration.
type EngineHardwareDecodingMode string

const (
	EngineHardwareDecodingAuto    EngineHardwareDecodingMode = "auto"
	EngineHardwareDecodingForce   EngineHardwareDecodingMode = "force"
	EngineHardwareDecodingDisable EngineHardwareDecodingMode = "disable"
)

// EngineWebContentSettingsPayload is the engine-facing runtime view of web
// content settings that can be applied to newly-created or existing webviews.
type EngineWebContentSettingsPayload struct {
	SansFont                  string
	SerifFont                 string
	MonospaceFont             string
	DefaultFontSize           int
	EnableDevTools            bool
	CaptureConsole            bool
	DrawCompositingIndicators bool
	HardwareDecoding          EngineHardwareDecodingMode
	AutoCopyOnSelection       bool
}

// EngineSettingsPayload is the engine-facing boundary view of runtime config.
// Add fields here only when an engine needs to react to them at runtime.
type EngineSettingsPayload struct {
	DefaultUIScale float64
	WebContent     EngineWebContentSettingsPayload
}

// EngineSettingsUpdate carries a runtime config change to the engine.
type EngineSettingsUpdate struct {
	Settings EngineSettingsPayload
}

type RuntimeConfigSnapshot struct {
	EngineSettings EngineSettingsPayload
	UI             RuntimeUIConfig
}

type RuntimeUIConfig struct {
	DefaultUIScale      float64
	SidebarWidth        int
	Appearance          AppearanceConfig
	Workspace           WorkspaceConfig
	Session             SessionConfig
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
	InitialBehavior   OmniboxInitialBehavior
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
