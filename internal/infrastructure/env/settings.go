package env

// RenderingSettings holds all rendering environment variable settings.
// This was previously port.RenderingEnvSettings but moved here because
// these settings are engine-specific (WebKit/GTK/GStreamer).
type RenderingSettings struct {
	// GStreamer
	ForceVSync          bool
	GLRenderingMode     string
	GStreamerDebugLevel int
	// WebKit compositor
	DisableDMABufRenderer  bool
	ForceCompositingMode   bool
	DisableCompositingMode bool
	// GTK/GSK
	GSKRenderer    string
	DisableMipmaps bool
	PreferGL       bool
	// Debug
	ShowFPS      bool
	SampleMemory bool
	DebugFrames  bool
	// Skia
	SkiaCPUPaintingThreads int
	SkiaGPUPaintingThreads int
	SkiaEnableCPURendering bool
}
