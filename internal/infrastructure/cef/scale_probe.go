package cef

import (
	"math"
	"strings"
)

const scaleProbeRoundFactor = 1_000_000

type cefScaleProbeMetrics struct {
	SurfaceWidth      int32
	SurfaceHeight     int32
	SurfaceScale      float64
	OSRBackingScale   float64
	UserZoom          float64
	InternalCEFFactor float64
}

func shouldRunCEFScaleProbe(frameURL string, httpStatusCode int32) bool {
	url := strings.ToLower(strings.TrimSpace(frameURL))
	if url == "" || url == "about:blank" {
		return false
	}
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return httpStatusCode >= 200 && httpStatusCode < 400
	}
	return httpStatusCode >= 0
}

func cefScaleProbeSnapshot(wv *WebView) cefScaleProbeMetrics {
	m := cefScaleProbeMetrics{
		SurfaceWidth:      1,
		SurfaceHeight:     1,
		SurfaceScale:      1,
		OSRBackingScale:   1,
		UserZoom:          1,
		InternalCEFFactor: 1,
	}
	if wv == nil {
		return m
	}
	m.UserZoom = normalizeScale(wv.GetZoomLevel())
	m.OSRBackingScale = normalizeScale(wv.osrBackingScaleFactor())
	m.SurfaceScale = normalizeScale(wv.viewBridgeScale())
	m.InternalCEFFactor = m.UserZoom * zoomScaleRatio(m.SurfaceScale, m.OSRBackingScale)
	if wv.viewBridge != nil {
		m.SurfaceWidth, m.SurfaceHeight = wv.viewBridge.Size()
	}
	return m
}

func (m cefScaleProbeMetrics) logFields() map[string]any {
	return map[string]any{
		"surface_width":       m.SurfaceWidth,
		"surface_height":      m.SurfaceHeight,
		"surface_scale":       roundedScale(m.SurfaceScale),
		"osr_backing_scale":   roundedScale(m.OSRBackingScale),
		"user_zoom":           roundedScale(m.UserZoom),
		"internal_cef_factor": roundedScale(m.InternalCEFFactor),
	}
}

func roundedScale(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 1
	}
	return math.Round(v*scaleProbeRoundFactor) / scaleProbeRoundFactor
}

const cefScaleProbeScript = `(function(){
  try {
    var vv = window.visualViewport;
    var probe = document.createElement('div');
    probe.style.cssText = 'position:fixed;left:0;top:0;width:100px;height:100px;visibility:hidden;pointer-events:none;z-index:-1';
    document.documentElement.appendChild(probe);
    var rect = probe.getBoundingClientRect();
    probe.remove();
    var data = {
      dpr: window.devicePixelRatio,
      inner: [window.innerWidth, window.innerHeight],
      outer: [window.outerWidth, window.outerHeight],
      client: [document.documentElement.clientWidth, document.documentElement.clientHeight],
      screen: [window.screen.width, window.screen.height],
      avail: [window.screen.availWidth, window.screen.availHeight],
      visualViewport: vv ? {
        width: vv.width,
        height: vv.height,
        scale: vv.scale,
        offsetLeft: vv.offsetLeft,
        offsetTop: vv.offsetTop,
        pageLeft: vv.pageLeft,
        pageTop: vv.pageTop
      } : null,
      css100Rect: {width: rect.width, height: rect.height, left: rect.left, top: rect.top},
      scroll: [window.scrollX, window.scrollY]
    };
    console.info('[SCALE-PROBE] ' + JSON.stringify(data));
  } catch (e) {
    console.warn('[SCALE-PROBE] error ' + (e && e.message ? e.message : String(e)));
  }
})();`
