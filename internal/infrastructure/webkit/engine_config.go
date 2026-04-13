package webkit

import "github.com/bnema/dumber/internal/infrastructure/config"

// WebKitEngineConfig holds all WebKit-specific engine configuration.
// This is the infrastructure-side version (no TOML tags).
type WebKitEngineConfig struct {
	// Skia
	SkiaCPUPaintingThreads int
	SkiaGPUPaintingThreads int
	SkiaEnableCPURendering bool
	// WebKit compositor
	DisableDMABufRenderer  bool
	ForceCompositingMode   bool
	DisableCompositingMode bool
	// GTK/GSK
	GSKRenderer    string
	DisableMipmaps bool
	PreferGL       bool
	// Debug
	ShowFPS                   bool
	SampleMemory              bool
	DebugFrames               bool
	DrawCompositingIndicators bool
	// Privacy
	ITPEnabled bool
	// GStreamer
	ForceVSync          bool
	GLRenderingMode     string
	GStreamerDebugLevel int
	// Runtime
	Prefix string
	// Memory pressure (all fields from config)
	WebProcessMemoryLimitMB                   int
	WebProcessMemoryPollIntervalSec           float64
	WebProcessMemoryConservativeThreshold     float64
	WebProcessMemoryStrictThreshold           float64
	NetworkProcessMemoryLimitMB               int
	NetworkProcessMemoryPollIntervalSec       float64
	NetworkProcessMemoryConservativeThreshold float64
	NetworkProcessMemoryStrictThreshold       float64
}

// EngineConfigFromConfig converts config.WebKitEngineConfig to the infra type.
func EngineConfigFromConfig(cfg config.WebKitEngineConfig) WebKitEngineConfig {
	return WebKitEngineConfig{
		SkiaCPUPaintingThreads:                    cfg.SkiaCPUPaintingThreads,
		SkiaGPUPaintingThreads:                    cfg.SkiaGPUPaintingThreads,
		SkiaEnableCPURendering:                    cfg.SkiaEnableCPURendering,
		DisableDMABufRenderer:                     cfg.DisableDMABufRenderer,
		ForceCompositingMode:                      cfg.ForceCompositingMode,
		DisableCompositingMode:                    cfg.DisableCompositingMode,
		GSKRenderer:                               string(cfg.GSKRenderer),
		DisableMipmaps:                            cfg.DisableMipmaps,
		PreferGL:                                  cfg.PreferGL,
		ShowFPS:                                   cfg.ShowFPS,
		SampleMemory:                              cfg.SampleMemory,
		DebugFrames:                               cfg.DebugFrames,
		DrawCompositingIndicators:                 cfg.DrawCompositingIndicators,
		ITPEnabled:                                cfg.ITPEnabled,
		ForceVSync:                                cfg.ForceVSync,
		GLRenderingMode:                           string(cfg.GLRenderingMode),
		GStreamerDebugLevel:                       cfg.GStreamerDebugLevel,
		Prefix:                                    cfg.Prefix,
		WebProcessMemoryLimitMB:                   cfg.WebProcessMemoryLimitMB,
		WebProcessMemoryPollIntervalSec:           cfg.WebProcessMemoryPollIntervalSec,
		WebProcessMemoryConservativeThreshold:     cfg.WebProcessMemoryConservativeThreshold,
		WebProcessMemoryStrictThreshold:           cfg.WebProcessMemoryStrictThreshold,
		NetworkProcessMemoryLimitMB:               cfg.NetworkProcessMemoryLimitMB,
		NetworkProcessMemoryPollIntervalSec:       cfg.NetworkProcessMemoryPollIntervalSec,
		NetworkProcessMemoryConservativeThreshold: cfg.NetworkProcessMemoryConservativeThreshold,
		NetworkProcessMemoryStrictThreshold:       cfg.NetworkProcessMemoryStrictThreshold,
	}
}
