// Package port defines interfaces (ports) for use cases to depend on.
// Concrete browsing-context DTOs live in internal/application/dto.
package port

import "github.com/bnema/dumber/internal/application/dto"

// PaneHostedBrowsingContextCapable is implemented by engines that need an
// explicit transition from a related/native-popup shell into a pane-hosted
// child browsing context path.
type PaneHostedBrowsingContextCapable interface {
	PreparePaneHostedBrowsingContext()
}

// BrowsingContextHostDecisionCapable lets UI coordinators attach the chosen
// host decision to an engine webview so lower-level popup handlers can honor
// pane-vs-native behavior synchronously.
type BrowsingContextHostDecisionCapable interface {
	SetBrowsingContextHostDecision(decision dto.HostDecision)
	BrowsingContextHostDecision() (dto.HostDecision, bool)
}

// NativePopupHostAbortCapable lets the engine-level popup seam abort an
// already-created native popup shell/window when native arming fails.
type NativePopupHostAbortCapable interface {
	SetNativePopupHostAbort(fn func())
	AbortNativePopupHost()
}
