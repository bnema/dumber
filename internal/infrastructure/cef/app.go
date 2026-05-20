package cef

import (
	"os"
	"strings"

	purecef "github.com/bnema/purego-cef/cef"
	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/logging"
)

const (
	dumbSchemeName                     = "dumb"
	chromiumEnableFeaturesSwitch       = "enable-features"
	chromiumDisableFeaturesSwitch      = "disable-features"
	chromiumDisableBlinkFeaturesSwitch = "disable-blink-features"
	chromiumRenderNodeOverrideSwitch   = "render-node-override"
)

// Chromium's core Blink runtime flag for the Web Authentication API is
// "WebAuth" (PublicKeyCredential IDL uses RuntimeEnabled=WebAuth). The
// longer "WebAuthentication*" names are separate subfeatures/metrics.
var cefWebAuthnFeaturesDisabledByPolicy = []string{
	"WebAuth",
}

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
	configureCommandLineWithRenderStack(commandLine, cef2gtk.RenderStackPlan{})
}

func configureCommandLineWithRenderStack(commandLine purecef.CommandLine, renderStackPlan cef2gtk.RenderStackPlan) {
	if commandLine == nil {
		return
	}

	// Delegate Wayland accelerated rendering setup to the GTK bridge.
	// Preserves any pre-existing ozone-platform value (e.g. set via
	// CEF command-line args) — the bridge is a no-op when already present.
	cef2gtk.ConfigureCommandLine(commandLine, cef2gtk.CommandLineOptions{RenderStackPlan: renderStackPlan})

	// Enable Chromium's built-in smooth scrolling animation — without this,
	// mouse wheel scroll jumps in discrete steps with no momentum/easing.
	commandLine.AppendSwitch("enable-smooth-scrolling")

	// Allow video autoplay without requiring user gesture — sites like
	// Reddit autoplay muted videos in the feed; without this policy
	// Chromium blocks them, showing an infinite spinner.
	commandLine.AppendSwitchWithValue("autoplay-policy", "no-user-gesture-required")

	configureHardwareVideoDecode(commandLine)
	configureEnvChromiumFlags(commandLine)
	configureRenderNodeOverride(commandLine)
	configureWebAuthnFeaturePolicy(commandLine)
}

func configureHardwareVideoDecode(commandLine purecef.CommandLine) {
	if commandLine == nil || !envBoolEnabled(cefEnableVAAPIEnvVar) {
		return
	}

	appendUniqueCommaSeparatedSwitchValues(commandLine, chromiumEnableFeaturesSwitch,
		"AcceleratedVideoDecoder",
		"AcceleratedVideoEncoder",
		"AcceleratedVideoDecodeLinuxGL",
		"AcceleratedVideoDecodeLinuxZeroCopyGL",
		"VaapiIgnoreDriverChecks",
	)
	appendSwitchIfMissing(commandLine, "ignore-gpu-blocklist")
	appendSwitchIfMissing(commandLine, "enable-zero-copy")
	appendSwitchIfMissing(commandLine, "disable-gpu-driver-bug-workaround")
}

func configureEnvChromiumFlags(commandLine purecef.CommandLine) {
	if commandLine == nil {
		return
	}

	for _, token := range parseChromiumFlagsEnv(os.Getenv(cefChromiumFlagsEnvVar)) {
		applyChromiumFlagToken(commandLine, token)
	}
}

func configureRenderNodeOverride(commandLine purecef.CommandLine) {
	if commandLine == nil {
		return
	}
	value := strings.TrimSpace(os.Getenv(cefRenderNodeEnvVar))
	if value == "" {
		return
	}
	switch strings.ToLower(value) {
	case "auto", "default", "none", "off", "disable", "disabled":
		commandLine.RemoveSwitch(chromiumRenderNodeOverrideSwitch)
		return
	}
	commandLine.RemoveSwitch(chromiumRenderNodeOverrideSwitch)
	commandLine.AppendSwitchWithValue(chromiumRenderNodeOverrideSwitch, value)
}

func parseChromiumFlagsEnv(raw string) []string {
	// Reuse Dumber's command-line tokenizer so developer env flags can contain
	// quoted values without adding a second shell-like parser in the CEF adapter.
	return parseRelaunchCommandLineArgs(raw)
}

func applyChromiumFlagToken(commandLine purecef.CommandLine, token string) {
	if commandLine == nil {
		return
	}
	token = strings.TrimSpace(token)
	if token == "" || token == "--" {
		return
	}
	name, value, ok := parseChromiumSwitchToken(token)
	if !ok {
		return
	}

	switch name {
	case chromiumEnableFeaturesSwitch, chromiumDisableFeaturesSwitch, chromiumDisableBlinkFeaturesSwitch:
		appendUniqueCommaSeparatedSwitchValues(commandLine, name, strings.Split(value, ",")...)
	default:
		if value == "" {
			appendSwitchIfMissing(commandLine, name)
			return
		}
		commandLine.AppendSwitchWithValue(name, value)
	}
}

func parseChromiumSwitchToken(token string) (name, value string, ok bool) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", "", false
	}

	switch {
	case strings.HasPrefix(token, "--"):
		token = strings.TrimPrefix(token, "--")
	case strings.HasPrefix(token, "-"):
		token = strings.TrimPrefix(token, "-")
	default:
		return "", "", false
	}

	if token == "" || strings.HasPrefix(token, "-") {
		return "", "", false
	}
	name, value, found := strings.Cut(token, "=")
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false
	}
	if !found {
		return name, "", true
	}
	return name, value, true
}

func configureWebAuthnFeaturePolicy(commandLine purecef.CommandLine) {
	if commandLine == nil {
		return
	}
	if cefWebAuthnUnsafeEnabled() {
		removeCommaSeparatedSwitchValues(commandLine, chromiumDisableFeaturesSwitch, cefWebAuthnFeaturesDisabledByPolicy...)
		removeCommaSeparatedSwitchValues(commandLine, chromiumDisableBlinkFeaturesSwitch, cefWebAuthnFeaturesDisabledByPolicy...)
		return
	}
	appendUniqueCommaSeparatedSwitchValues(commandLine, chromiumDisableFeaturesSwitch, cefWebAuthnFeaturesDisabledByPolicy...)
	appendUniqueCommaSeparatedSwitchValues(commandLine, chromiumDisableBlinkFeaturesSwitch, cefWebAuthnFeaturesDisabledByPolicy...)
}

func appendUniqueCommaSeparatedSwitchValues(commandLine purecef.CommandLine, name string, values ...string) {
	if commandLine == nil || len(values) == 0 {
		return
	}

	seen := make(map[string]struct{}, len(values))
	combined := make([]string, 0, len(values))
	for _, value := range strings.Split(commandLine.GetSwitchValue(name), ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		combined = append(combined, value)
	}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		combined = append(combined, value)
	}

	if len(combined) == 0 {
		return
	}
	commandLine.AppendSwitchWithValue(name, strings.Join(combined, ","))
}

func removeCommaSeparatedSwitchValues(commandLine purecef.CommandLine, name string, values ...string) {
	if commandLine == nil || len(values) == 0 {
		return
	}

	existing := commandLine.GetSwitchValue(name)
	if existing == "" {
		return
	}

	removeSet := make(map[string]struct{}, len(values))
	for _, v := range values {
		removeSet[strings.TrimSpace(v)] = struct{}{}
	}

	tokens := strings.Split(existing, ",")
	cleaned := make([]string, 0, len(tokens))
	removed := false
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, ok := removeSet[token]; ok {
			removed = true
			continue
		}
		cleaned = append(cleaned, token)
	}

	if !removed {
		return
	}

	if len(cleaned) == 0 {
		commandLine.RemoveSwitch(name)
		return
	}

	commandLine.AppendSwitchWithValue(name, strings.Join(cleaned, ","))
}

// dumberApp implements purecef.App to provide custom scheme registration,
// command-line configuration, and browser-process lifecycle callbacks.
type dumberApp struct {
	engine *Engine
	bph    purecef.BrowserProcessHandler
	rph    purecef.RenderProcessHandler
}

// newDumberApp creates an App whose BrowserProcessHandler handles context
// initialization, child process launch tracking, and scheme registration.
// Returns the raw Go implementation — InitWithApp will wrap it exactly once.
func newDumberApp(engine *Engine) purecef.App {
	app := &dumberApp{engine: engine}
	app.bph = &dumberBPH{engine: engine}
	app.rph = newPopupOpenerRenderProcessHandler()
	return app
}

// NewSubprocessApp returns a lightweight raw App implementation for CEF
// subprocesses re-executed from the main dumber binary. Keep the legacy
// renderer bridge disabled, but allow the minimal popup-opener render handler
// needed to install synthetic opener state before page scripts run.
func NewSubprocessApp() purecef.App {
	return &subprocessApp{rph: newPopupOpenerRenderProcessHandler()}
}

// maxCmdLineLogLen limits logged command line length to avoid leaking sensitive paths.
const maxCmdLineLogLen = 200

func (a *dumberApp) OnBeforeCommandLineProcessing(processType string, commandLine purecef.CommandLine) {
	log := logging.FromContext(a.engine.ctx)
	if commandLine != nil {
		configureCommandLineWithRenderStack(commandLine, a.engine.renderStackPlan)

		if processType == "" {
			if cefWebAuthnUnsafeEnabled() {
				log.Warn().
					Str("env_var", cefEnableWebAuthnUnsafeEnvVar).
					Msg("cef: WebAuthn enabled via unsupported developer override")
			} else {
				log.Info().
					Strs("disabled_features", cefWebAuthnFeaturesDisabledByPolicy).
					Str("override_env_var", cefEnableWebAuthnUnsafeEnvVar).
					Msg("cef: WebAuthn disabled for CEF windowless/Alloy runtime")
			}
		}

		renderNodeOverride := commandLine.GetSwitchValue(chromiumRenderNodeOverrideSwitch)
		cmdline := commandLine.GetCommandLineString()
		if len(cmdline) > maxCmdLineLogLen {
			runes := []rune(cmdline)
			if len(runes) > maxCmdLineLogLen {
				cmdline = string(runes[:maxCmdLineLogLen]) + "…"
			}
		}
		log.Debug().
			Str("process_type", processType).
			Str("render_node_override", renderNodeOverride).
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
func (a *dumberApp) GetRenderProcessHandler() purecef.RenderProcessHandler   { return a.rph }

type subprocessApp struct{ rph purecef.RenderProcessHandler }

func (a *subprocessApp) OnBeforeCommandLineProcessing(_ string, commandLine purecef.CommandLine) {
	configureCommandLine(commandLine)
}
func (a *subprocessApp) OnRegisterCustomSchemes(registrar purecef.SchemeRegistrar) {
	registerDumbScheme(registrar)
}
func (a *subprocessApp) GetResourceBundleHandler() purecef.ResourceBundleHandler { return nil }
func (a *subprocessApp) GetBrowserProcessHandler() purecef.BrowserProcessHandler { return nil }
func (a *subprocessApp) GetRenderProcessHandler() purecef.RenderProcessHandler   { return a.rph }

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
	renderNodeOverride := ""
	if commandLine != nil {
		configureCommandLineWithRenderStack(commandLine, h.engine.renderStackPlan)
		appendSwitchIfMissing(commandLine, "no-zygote")
		processType = commandLine.GetSwitchValue("type")
		commandLineString = commandLine.GetCommandLineString()
		useAngle = commandLine.GetSwitchValue("use-angle")
		ozonePlatform = commandLine.GetSwitchValue("ozone-platform")
		renderNodeOverride = commandLine.GetSwitchValue(chromiumRenderNodeOverrideSwitch)
	}
	h.engine.recordChildProcessLaunch(processType, useAngle, ozonePlatform, renderNodeOverride, commandLineString)
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
func (h *dumberBPH) GetDefaultClient() purecef.RawClient                            { return nil }
func (h *dumberBPH) GetDefaultRequestContextHandler() purecef.RequestContextHandler { return nil }

// OnScheduleMessagePumpWork is a no-op — multi-threaded message loop drives
// its own pump. Required by the BrowserProcessHandler interface.
func (h *dumberBPH) OnScheduleMessagePumpWork(_ int64) {}
