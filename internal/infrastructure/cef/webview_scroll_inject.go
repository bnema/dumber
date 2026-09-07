package cef

// This file owns inertial-scroll injection for keyboard-driven page scrolls
// (Vim Mode J/K, arrows). A scroll request becomes one engine impulse per
// press or held-key step: repeats join the live wheel burst and the tail
// coasts after release, so key scrolling feels like wheel scrolling.
//
// Threading contract: InjectScroll hops to the GTK thread internally and is
// safe from any thread. Key release sends nothing on purpose: canceling
// the burst would kill the coast, so CancelPageScroll only drains the
// legacy JS queue (which stays empty while injection succeeds).

// scrollInjectBridge is the narrow adapter seam for scroll injection.
// *Cef2gtkAdapter satisfies it in production; tests inject a recording
// fake. It covers only impulse acceptance, mirroring scrollCancelBridge.
type scrollInjectBridge interface {
	InjectScroll(dx, dy float64) bool
}

// scrollInjectTarget resolves the live adapter for scroll injection.
func (wv *WebView) scrollInjectTarget() scrollInjectBridge {
	if wv == nil {
		return nil
	}
	if wv.scrollInjectSeam != nil {
		return wv.scrollInjectSeam
	}
	if wv.viewBridge == nil {
		return nil
	}
	return wv.viewBridge
}

// injectPageScroll forwards one key-scroll step to the inertial engine.
// It reports whether the engine accepted the impulse; callers fall back to
// the legacy queue when it does not (smoothing disabled, no bridge).
func (wv *WebView) injectPageScroll(deltaX, deltaY int32) bool {
	if wv == nil || !wv.inputConfig.ScrollWheelSmoothing {
		return false
	}
	target := wv.scrollInjectTarget()
	if target == nil {
		return false
	}
	return target.InjectScroll(float64(deltaX), float64(deltaY))
}
