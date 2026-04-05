package cef

import (
	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/logging"
)

const dumbSchemeName = "dumb"

func dumbSchemeOptions() int32 {
	return int32(purecef.SchemeOptionsSchemeOptionStandard |
		purecef.SchemeOptionsSchemeOptionSecure |
		purecef.SchemeOptionsSchemeOptionCorsEnabled |
		purecef.SchemeOptionsSchemeOptionCspBypassing |
		purecef.SchemeOptionsSchemeOptionFetchEnabled)
}

func registerDumbScheme(registrar purecef.SchemeRegistrar) {
	if registrar == nil {
		return
	}
	registrar.AddCustomScheme(dumbSchemeName, dumbSchemeOptions())
}

func configureCommandLine(commandLine purecef.CommandLine) {
	if commandLine == nil {
		return
	}

	// Enable Chromium's built-in smooth scrolling animation — without this,
	// mouse wheel scroll jumps in discrete steps with no momentum/easing.
	commandLine.AppendSwitch("enable-smooth-scrolling")

	// Allow video autoplay without requiring user gesture — sites like
	// Reddit autoplay muted videos in the feed; without this policy
	// Chromium blocks them, showing an infinite spinner.
	commandLine.AppendSwitchWithValue("autoplay-policy", "no-user-gesture-required")
}

// dumberApp implements purecef.App to provide custom scheme registration,
// command-line configuration, and browser-process lifecycle callbacks.
type dumberApp struct {
	engine *Engine
	bph    purecef.BrowserProcessHandler
}

// newDumberApp creates an App whose BrowserProcessHandler handles context
// initialization, child process launch tracking, and scheme registration.
// Returns the raw Go implementation — InitWithApp will wrap it exactly once.
func newDumberApp(engine *Engine) purecef.App {
	app := &dumberApp{engine: engine}
	app.bph = &dumberBPH{engine: engine}
	return app
}

// NewSubprocessApp returns a lightweight App for helper processes so CEF sees
// the same custom scheme registration in renderer/GPU/utility processes.
func NewSubprocessApp() purecef.App {
	return purecef.NewApp(&subprocessApp{})
}

// maxCmdLineLogLen limits logged command line length to avoid leaking sensitive paths.
const maxCmdLineLogLen = 200

func (a *dumberApp) OnBeforeCommandLineProcessing(processType string, commandLine purecef.CommandLine) {
	log := logging.FromContext(a.engine.ctx)
	if commandLine != nil {
		configureCommandLine(commandLine)

		cmdline := commandLine.GetCommandLineString()
		if len(cmdline) > maxCmdLineLogLen {
			runes := []rune(cmdline)
			if len(runes) > maxCmdLineLogLen {
				cmdline = string(runes[:maxCmdLineLogLen]) + "…"
			}
		}
		log.Debug().
			Str("process_type", processType).
			Str("command_line", cmdline).
			Msg("cef: OnBeforeCommandLineProcessing")
	}
}
func (a *dumberApp) OnRegisterCustomSchemes(registrar purecef.SchemeRegistrar) {
	registerDumbScheme(registrar)
	logging.FromContext(a.engine.ctx).Debug().Msg("cef: registered dumb:// custom scheme")
}
func (a *dumberApp) GetResourceBundleHandler() purecef.ResourceBundleHandler { return nil }
func (a *dumberApp) GetBrowserProcessHandler() purecef.BrowserProcessHandler { return a.bph }
func (a *dumberApp) GetRenderProcessHandler() purecef.RenderProcessHandler   { return nil }

type subprocessApp struct{}

func (a *subprocessApp) OnBeforeCommandLineProcessing(_ string, commandLine purecef.CommandLine) {
	configureCommandLine(commandLine)
}
func (a *subprocessApp) OnRegisterCustomSchemes(registrar purecef.SchemeRegistrar) {
	registerDumbScheme(registrar)
}
func (a *subprocessApp) GetResourceBundleHandler() purecef.ResourceBundleHandler { return nil }
func (a *subprocessApp) GetBrowserProcessHandler() purecef.BrowserProcessHandler { return nil }
func (a *subprocessApp) GetRenderProcessHandler() purecef.RenderProcessHandler   { return nil }

// dumberBPH implements purecef.BrowserProcessHandler for context initialization,
// child process launch tracking, and diagnostic logging.
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

// OnScheduleMessagePumpWork is a no-op — multi-threaded message loop drives
// its own pump. Required by the BrowserProcessHandler interface.
func (h *dumberBPH) OnScheduleMessagePumpWork(_ int64) {}
