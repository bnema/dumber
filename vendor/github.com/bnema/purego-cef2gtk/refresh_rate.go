package cef2gtk

const (
	defaultRefreshRateFallbackFPS = 60
	defaultRefreshRateMinFPS      = 30
	defaultRefreshRateMaxFPS      = 240
)

// RefreshRateOptions configures conversion from a GDK monitor refresh rate to
// a CEF windowless frame-rate. GDK reports refresh rates in milli-Hz.
type RefreshRateOptions struct {
	DefaultFPS int32
	MinFPS     int32
	MaxFPS     int32
}

// WindowlessFrameRateForMonitorRefresh converts a GDK monitor refresh rate
// (milli-Hz) into a CEF OSR windowless frame-rate, with fallback and clamp
// handling for unknown or extreme monitor values.
func WindowlessFrameRateForMonitorRefresh(refreshRateMilliHz int, opts RefreshRateOptions) int32 {
	defaultFPS, minFPS, maxFPS := normalizeRefreshRateOptions(opts)
	if refreshRateMilliHz <= 0 {
		return clampFrameRate(defaultFPS, minFPS, maxFPS)
	}
	fps := int32((refreshRateMilliHz + 500) / 1000)
	return clampFrameRate(fps, minFPS, maxFPS)
}

func normalizeRefreshRateOptions(opts RefreshRateOptions) (defaultFPS, minFPS, maxFPS int32) {
	defaultFPS = opts.DefaultFPS
	if defaultFPS <= 0 {
		defaultFPS = defaultRefreshRateFallbackFPS
	}
	minFPS = opts.MinFPS
	if minFPS <= 0 {
		minFPS = defaultRefreshRateMinFPS
	}
	maxFPS = opts.MaxFPS
	if maxFPS <= 0 {
		maxFPS = defaultRefreshRateMaxFPS
	}
	if maxFPS < minFPS {
		maxFPS = minFPS
	}
	return defaultFPS, minFPS, maxFPS
}

func clampFrameRate(fps, minFPS, maxFPS int32) int32 {
	if fps < minFPS {
		return minFPS
	}
	if fps > maxFPS {
		return maxFPS
	}
	return fps
}
