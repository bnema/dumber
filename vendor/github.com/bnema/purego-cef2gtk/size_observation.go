package cef2gtk

import "math"

const (
	sizeTickStableFrames = 2
	sizeTickMaxFrames    = 8
)

type sizeObservationSample struct {
	width  int32
	height int32
	scale  float64
}

type sizeObservationStrategyConfig struct {
	widgetNotifyDetails       []string
	surfaceSizeNotifyDetails  []string
	surfaceScaleNotifyDetails []string
	useGLAreaResize           bool
	useSurfaceLayout          bool
}

func sizeObservationStrategy(hasGLArea bool) sizeObservationStrategyConfig {
	cfg := sizeObservationStrategyConfig{
		widgetNotifyDetails:       []string{"scale-factor"},
		surfaceScaleNotifyDetails: []string{"scale", "scale-factor"},
		useGLAreaResize:           hasGLArea,
		useSurfaceLayout:          !hasGLArea,
	}
	if !hasGLArea {
		cfg.surfaceSizeNotifyDetails = []string{"width", "height"}
	}
	return cfg
}

func shouldEmitSizeHooks(sizeChanged, scaleChanged bool) bool {
	return sizeChanged || scaleChanged
}

type sizeTickSettler struct {
	initialized   bool
	totalFrames   int
	stableFrames  int
	lastWidth     int32
	lastHeight    int32
	lastScaleBits uint64
}

func (s *sizeTickSettler) Reset() {
	if s == nil {
		return
	}
	*s = sizeTickSettler{}
}

func (s *sizeTickSettler) Next(width, height int32, scale float64) bool {
	if s == nil {
		return false
	}
	scaleBits := math.Float64bits(normalizeDeviceScale(scale))
	if !s.initialized {
		s.initialized = true
		s.totalFrames = 1
		s.lastWidth = width
		s.lastHeight = height
		s.lastScaleBits = scaleBits
		return true
	}

	s.totalFrames++
	same := s.lastWidth == width && s.lastHeight == height && s.lastScaleBits == scaleBits
	s.lastWidth = width
	s.lastHeight = height
	s.lastScaleBits = scaleBits
	if same {
		s.stableFrames++
	} else {
		s.stableFrames = 0
	}
	if s.stableFrames >= sizeTickStableFrames {
		return false
	}
	if s.totalFrames >= sizeTickMaxFrames {
		return false
	}
	return true
}
