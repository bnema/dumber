package cef

import (
	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/logging"
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
	// Keep the raw Go implementation here. NewApp will wrap it exactly once
	// when GetBrowserProcessHandler is queried by CEF.
	app.bph = &dumberBPH{engine: engine}
	return purecef.NewApp(app)
}

func (a *dumberApp) OnBeforeCommandLineProcessing(processType string, commandLine purecef.CommandLine) {
	log := logging.FromContext(a.engine.ctx)
	cmdline := ""
	if commandLine != nil {
		cmdline = commandLine.GetCommandLineString()
	}
	log.Debug().
		Str("process_type", processType).
		Str("command_line", cmdline).
		Msg("cef: OnBeforeCommandLineProcessing")
}
func (a *dumberApp) OnRegisterCustomSchemes(_ purecef.SchemeRegistrar)       {}
func (a *dumberApp) GetResourceBundleHandler() purecef.ResourceBundleHandler { return nil }
func (a *dumberApp) GetBrowserProcessHandler() purecef.BrowserProcessHandler { return a.bph }
func (a *dumberApp) GetRenderProcessHandler() purecef.RenderProcessHandler   { return nil }

// dumberBPH implements purecef.BrowserProcessHandler. Only
// OnScheduleMessagePumpWork carries real logic; the rest are no-ops.
type dumberBPH struct {
	engine *Engine
}

func (h *dumberBPH) OnRegisterCustomPreferences(_ purecef.PreferencesType, _ purecef.PreferenceRegistrar) {
}
func (h *dumberBPH) OnContextInitialized() {
	h.engine.recordContextInitialized()
}
func (h *dumberBPH) OnBeforeChildProcessLaunch(commandLine purecef.CommandLine) {
	processType := ""
	commandLineString := ""
	useAngle := ""
	ozonePlatform := ""
	if commandLine != nil {
		processType = commandLine.GetSwitchValue("type")
		commandLineString = commandLine.GetCommandLineString()
		useAngle = commandLine.GetSwitchValue("use-angle")
		ozonePlatform = commandLine.GetSwitchValue("ozone-platform")
	}
	h.engine.recordChildProcessLaunch(processType, useAngle, ozonePlatform, commandLineString)
}
func (h *dumberBPH) OnAlreadyRunningAppRelaunch(_ purecef.CommandLine, _ string) int32 { return 0 }
func (h *dumberBPH) GetDefaultClient() purecef.Client                                  { return nil }
func (h *dumberBPH) GetDefaultRequestContextHandler() purecef.RequestContextHandler    { return nil }

// OnScheduleMessagePumpWork is called by CEF when work needs to be done.
// delay_ms <= 0 means "as soon as possible"; > 0 means "after this delay".
func (h *dumberBPH) OnScheduleMessagePumpWork(delayMs int64) {
	h.engine.scheduleMessagePumpWork(delayMs)
}
