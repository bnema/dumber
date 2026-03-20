package cef

import (
	purecef "github.com/bnema/purego-cef/cef"
)

// dumberApp implements purecef.App to provide a BrowserProcessHandler with
// demand-driven message pump scheduling (OnScheduleMessagePumpWork).
type dumberApp struct {
	engine *Engine
	bph    purecef.BrowserProcessHandler
}

// newDumberApp creates an App whose BrowserProcessHandler drives the engine's
// adaptive message pump.
func newDumberApp(engine *Engine) purecef.App {
	app := &dumberApp{engine: engine}
	app.bph = purecef.NewBrowserProcessHandler(&dumberBPH{engine: engine})
	return purecef.NewApp(app)
}

func (a *dumberApp) OnBeforeCommandLineProcessing(_ string, _ purecef.CommandLine) {}
func (a *dumberApp) OnRegisterCustomSchemes(_ purecef.SchemeRegistrar)             {}
func (a *dumberApp) GetResourceBundleHandler() purecef.ResourceBundleHandler       { return nil }
func (a *dumberApp) GetBrowserProcessHandler() purecef.BrowserProcessHandler       { return a.bph }
func (a *dumberApp) GetRenderProcessHandler() purecef.RenderProcessHandler         { return nil }

// dumberBPH implements purecef.BrowserProcessHandler. Only
// OnScheduleMessagePumpWork carries real logic; the rest are no-ops.
type dumberBPH struct {
	engine *Engine
}

func (h *dumberBPH) OnRegisterCustomPreferences(_ purecef.PreferencesType, _ purecef.PreferenceRegistrar) {
}
func (h *dumberBPH) OnContextInitialized()                                             {}
func (h *dumberBPH) OnBeforeChildProcessLaunch(_ purecef.CommandLine)                  {}
func (h *dumberBPH) OnAlreadyRunningAppRelaunch(_ purecef.CommandLine, _ string) int32 { return 0 }
func (h *dumberBPH) GetDefaultClient() purecef.Client                                  { return nil }
func (h *dumberBPH) GetDefaultRequestContextHandler() purecef.RequestContextHandler    { return nil }

// OnScheduleMessagePumpWork is called by CEF when work needs to be done.
// delay_ms <= 0 means "as soon as possible"; > 0 means "after this delay".
func (h *dumberBPH) OnScheduleMessagePumpWork(delayMs int64) {
	h.engine.scheduleMessagePumpWork(delayMs)
}
