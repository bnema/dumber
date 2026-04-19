package cef

import (
	"strings"

	purecef "github.com/bnema/purego-cef/cef"

	"github.com/bnema/dumber/internal/logging"
)

const dumbSchemeName = "dumb"

func dumbSchemeOptions() int32 {
	return purecef.SchemeOptionsSchemeOptionStandard |
		purecef.SchemeOptionsSchemeOptionSecure |
		purecef.SchemeOptionsSchemeOptionCorsEnabled |
		purecef.SchemeOptionsSchemeOptionCspBypassing |
		purecef.SchemeOptionsSchemeOptionFetchEnabled
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

// NewSubprocessApp returns a lightweight raw App implementation for CEF
// subprocesses re-executed from the main dumber binary. Keep subprocesses free
// of custom render handlers for now: the renderer bridge introduced a startup
// regression where OSR pages stopped producing their first paint.
func NewSubprocessApp() purecef.App {
	return &subprocessApp{}
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
		appendSwitchIfMissing(commandLine, "no-zygote")
		processType = commandLine.GetSwitchValue("type")
		commandLineString = commandLine.GetCommandLineString()
		useAngle = commandLine.GetSwitchValue("use-angle")
		ozonePlatform = commandLine.GetSwitchValue("ozone-platform")
	}
	h.engine.recordChildProcessLaunch(processType, useAngle, ozonePlatform, commandLineString)
}

func appendSwitchIfMissing(commandLine purecef.CommandLine, name string) {
	if commandLine == nil || commandLine.HasSwitch(name) {
		return
	}
	commandLine.AppendSwitch(name)
}

func parseRelaunchCommandLineArgs(commandLine string) []string {
	args := make([]string, 0, 4)
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		args = append(args, current.String())
		current.Reset()
	}

	for _, r := range commandLine {
		switch {
		case escaped:
			current.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			current.WriteRune(r)
		}
	}

	if escaped {
		current.WriteRune('\\')
	}
	flush()

	return args
}

func parseBrowseRelaunchCommandLine(commandLine purecef.CommandLine) (string, bool) {
	if commandLine == nil {
		return "", false
	}

	args := parseRelaunchCommandLineArgs(commandLine.GetCommandLineString())
	if len(args) < 3 || args[1] != "browse" || args[2] == "" {
		return "", false
	}
	return args[2], true
}

func isBrowseRelaunchCommandLine(commandLine purecef.CommandLine) bool {
	if commandLine == nil {
		return false
	}

	args := parseRelaunchCommandLineArgs(commandLine.GetCommandLineString())
	return len(args) >= 2 && args[1] == "browse"
}

func parseBrowseURLFromRelaunchCommandLine(commandLine purecef.CommandLine) string {
	browseURL, _ := parseBrowseRelaunchCommandLine(commandLine)
	return browseURL
}

func (h *dumberBPH) OnAlreadyRunningAppRelaunch(commandLine purecef.CommandLine, _ string) int32 {
	if browseURL, ok := parseBrowseRelaunchCommandLine(commandLine); ok {
		if h != nil && h.engine != nil {
			if handler := h.engine.alreadyRunningAppRelaunchCallback(); handler != nil {
				handler(browseURL)
			}
		}
		return 1
	}
	if isBrowseRelaunchCommandLine(commandLine) {
		return 1
	}

	return 0
}
func (h *dumberBPH) GetDefaultClient() purecef.Client                               { return nil }
func (h *dumberBPH) GetDefaultRequestContextHandler() purecef.RequestContextHandler { return nil }

// OnScheduleMessagePumpWork is a no-op — multi-threaded message loop drives
// its own pump. Required by the BrowserProcessHandler interface.
func (h *dumberBPH) OnScheduleMessagePumpWork(_ int64) {}
