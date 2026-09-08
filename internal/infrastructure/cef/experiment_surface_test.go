package cef

import (
	purecef "github.com/bnema/purego-cef/cef"
)

// Compile-time pins for the Plan 04 experiment surfaces. These assert the
// selected binding exposes the exact API shape the display-side probes were
// designed against, without initializing CEF. Evaluating a method
// expression performs no call.
//
// P3 cache probe: RequestContextGetGlobalContext,
// BrowserHost.GetRequestContext, RequestContext.GetCachePath.
// P5 DNS experiment: RequestContext.ResolveHost. Note the fire-and-forget
// signature: ResolveHost carries no cancellation handle, so the experiment
// design must recheck selection eligibility before issuance and only ignore
// stale completions afterwards.
var (
	_ func() purecef.RequestContext                                 = purecef.RequestContextGetGlobalContext
	_ func(purecef.BrowserHost) purecef.RequestContext              = purecef.BrowserHost.GetRequestContext
	_ func(purecef.RequestContext) string                           = purecef.RequestContext.GetCachePath
	_ func(purecef.RequestContext, string, purecef.ResolveCallback) = purecef.RequestContext.ResolveHost
)
