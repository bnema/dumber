package cef

import (
	"github.com/bnema/dumber/internal/shared/syncdispatch"
)

// This file owns inertial-scroll cancellation ordering for CEF WebViews.
//
// Threading contract: scrollMotionRetired is safe from any thread (it only
// bumps the library epoch and never touches GTK). GTK cleanup runs either
// inline inside runNavigationWithScrollCancel's ordered GTK operation or via
// runOnGTK-queued epoch-scoped cleanup. Library imports stay inside this
// infrastructure boundary; only the Cef2gtkAdapter seam is extended.

// scrollCancelBridge is the narrow adapter seam for scroll cancellation.
// *Cef2gtkAdapter satisfies it in production; tests inject a recording
// fake. It covers only invalidation and epoch-scoped cleanup.
type scrollCancelBridge interface {
	InvalidateScroll() uint64
	CancelScrollEpoch(epoch uint64) bool
}

// scrollCancelTarget resolves the live adapter for scroll cancellation.
func (wv *WebView) scrollCancelTarget() scrollCancelBridge {
	if wv == nil {
		return nil
	}
	if wv.scrollCancelSeam != nil {
		return wv.scrollCancelSeam
	}
	if wv.viewBridge == nil {
		return nil
	}
	return wv.viewBridge
}

// scrollMotionRetired immediately retires animated scroll motion and release
// eligibility, returning the retired epoch for epoch-scoped GTK cleanup.
// Safe to call from any thread, including during destruction: a destroyed
// or absent bridge reports zero, so no destroyed check is needed here.
func (wv *WebView) scrollMotionRetired() uint64 {
	target := wv.scrollCancelTarget()
	if wv == nil || target == nil {
		return 0
	}
	return target.InvalidateScroll()
}

// cancelScrollEpochOnGTK performs GTK-only cleanup for a retired epoch. It
// never clears a newer session. Call only on the GTK thread.
func (wv *WebView) cancelScrollEpochOnGTK(epoch uint64) {
	if wv == nil || epoch == 0 {
		return
	}
	target := wv.scrollCancelTarget()
	if target == nil {
		return
	}
	target.CancelScrollEpoch(epoch)
}

// invalidateScrollMotion retires motion immediately and queues epoch-scoped
// GTK cleanup through the owning dispatcher. Safe to call from any thread;
// used by lifecycle callbacks (load handlers, visibility) where navigation
// dispatch is not part of the call.
func (wv *WebView) invalidateScrollMotion() uint64 {
	epoch := wv.scrollMotionRetired()
	if epoch == 0 {
		return 0
	}
	target := wv.scrollCancelTarget()
	wv.runOnGTK(func() {
		if target == nil {
			return
		}
		target.CancelScrollEpoch(epoch)
	})
	return epoch
}

// runNavigationWithScrollCancel invalidates scroll motion immediately, then
// performs GTK cleanup followed by the existing navigation dispatch within
// one ordered GTK operation. Calls already on the GTK thread execute inline;
// off-thread calls use the existing bounded synchronous dispatch, which
// skips abandoned queued work so a timed-out dispatch can never navigate
// late. Dispatch errors preserve the command's existing error contract;
// dispatch failure reports the existing GTK-sync error form.
func (wv *WebView) runNavigationWithScrollCancel(label string, dispatch func() error) error {
	if wv.destroyed.Load() {
		return errDestroyed
	}
	epoch := wv.scrollMotionRetired()
	var dispatchErr error
	result := wv.runOnGTKSyncLabel(label, func() {
		wv.cancelScrollEpochOnGTK(epoch)
		dispatchErr = dispatch()
	})
	switch result.Status {
	case syncdispatch.SyncDispatchInline,
		syncdispatch.SyncDispatchCompleted,
		syncdispatch.SyncDispatchCompletedAfterTimeout:
		return dispatchErr
	default:
		return errGTKSyncDispatchIncomplete(label, result)
	}
}
