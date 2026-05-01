package cef

import (
	"context"
	"strings"
	"testing"
	"unsafe"

	purecef "github.com/bnema/purego-cef/cef"
)

type schemeRegistrarStub struct {
	scheme  string
	options int32
	calls   int
}

func (s *schemeRegistrarStub) AddCustomScheme(schemeName string, options int32) int32 {
	s.calls++
	s.scheme = schemeName
	s.options = options
	return 1
}

type relaunchCommandLineStub struct {
	commandLineString string
}

func (c relaunchCommandLineStub) IsValid() bool                      { return true }
func (c relaunchCommandLineStub) IsReadOnly() bool                   { return true }
func (c relaunchCommandLineStub) Copy() purecef.CommandLine          { return c }
func (c relaunchCommandLineStub) InitFromArgv(int32, unsafe.Pointer) {}
func (c relaunchCommandLineStub) InitFromString(string)              {}
func (c relaunchCommandLineStub) Reset()                             {}
func (c relaunchCommandLineStub) GetArgv(purecef.StringList)         {}
func (c relaunchCommandLineStub) GetCommandLineString() string       { return c.commandLineString }
func (c relaunchCommandLineStub) GetProgram() string                 { return "dumber" }
func (c relaunchCommandLineStub) SetProgram(string)                  {}
func (c relaunchCommandLineStub) HasSwitches() bool                  { return false }
func (c relaunchCommandLineStub) HasSwitch(string) bool              { return false }
func (c relaunchCommandLineStub) GetSwitchValue(string) string       { return "" }
func (c relaunchCommandLineStub) GetSwitches(purecef.StringMap)      {}
func (c relaunchCommandLineStub) AppendSwitch(string)                {}
func (c relaunchCommandLineStub) AppendSwitchWithValue(string, string) {
}
func (c relaunchCommandLineStub) HasArguments() bool              { return true }
func (c relaunchCommandLineStub) GetArguments(purecef.StringList) {}
func (c relaunchCommandLineStub) AppendArgument(string) {
}
func (c relaunchCommandLineStub) PrependWrapper(string) {}
func (c relaunchCommandLineStub) RemoveSwitch(string)   {}

type mutableCommandLineStub struct {
	relaunchCommandLineStub
	switches map[string]string
}

func newMutableCommandLineStub() *mutableCommandLineStub {
	return &mutableCommandLineStub{switches: make(map[string]string)}
}

func (c *mutableCommandLineStub) IsReadOnly() bool          { return false }
func (c *mutableCommandLineStub) Copy() purecef.CommandLine { return c }
func (c *mutableCommandLineStub) Reset()                    { c.switches = make(map[string]string) }
func (c *mutableCommandLineStub) HasSwitches() bool         { return len(c.switches) > 0 }
func (c *mutableCommandLineStub) HasSwitch(name string) bool {
	_, ok := c.switches[name]
	return ok
}
func (c *mutableCommandLineStub) GetSwitchValue(name string) string { return c.switches[name] }
func (c *mutableCommandLineStub) AppendSwitch(name string)          { c.switches[name] = "" }
func (c *mutableCommandLineStub) AppendSwitchWithValue(name, value string) {
	c.switches[name] = value
}
func (c *mutableCommandLineStub) HasArguments() bool { return false }
func (c *mutableCommandLineStub) RemoveSwitch(name string) {
	delete(c.switches, name)
}

func TestRegisterDumbScheme(t *testing.T) {
	t.Parallel()

	stub := &schemeRegistrarStub{}
	registerDumbScheme(stub)

	wantOptions := purecef.SchemeOptionsSchemeOptionStandard |
		purecef.SchemeOptionsSchemeOptionSecure |
		purecef.SchemeOptionsSchemeOptionCorsEnabled |
		purecef.SchemeOptionsSchemeOptionCspBypassing |
		purecef.SchemeOptionsSchemeOptionFetchEnabled

	if stub.calls != 1 {
		t.Fatalf("AddCustomScheme call count = %d, want 1", stub.calls)
	}
	if stub.scheme != dumbSchemeName {
		t.Fatalf("scheme = %q, want %q", stub.scheme, dumbSchemeName)
	}
	if stub.options != wantOptions {
		t.Fatalf("options = %d, want %d", stub.options, wantOptions)
	}
}

func TestParseBrowseURLFromRelaunchCommandLine(t *testing.T) {
	t.Parallel()

	got := parseBrowseURLFromRelaunchCommandLine(relaunchCommandLineStub{
		commandLineString: "dumber browse https://example.com",
	})

	if got != "https://example.com" {
		t.Fatalf("parsed browse url = %q, want %q", got, "https://example.com")
	}
}

func TestDumberBPH_OnAlreadyRunningAppRelaunch_ForwardsBrowseURLAndReturns1(t *testing.T) {
	t.Parallel()

	gotURL := ""
	eng := &Engine{}
	eng.SetAlreadyRunningAppRelaunchHandler(func(url string) {
		gotURL = url
	})

	ret := (&dumberBPH{engine: eng}).OnAlreadyRunningAppRelaunch(relaunchCommandLineStub{
		commandLineString: "dumber browse https://example.com",
	}, "")

	if ret != 1 {
		t.Fatalf("OnAlreadyRunningAppRelaunch returned %d, want 1", ret)
	}
	if gotURL != "https://example.com" {
		t.Fatalf("forwarded browse url = %q, want %q", gotURL, "https://example.com")
	}
}

func TestDumberBPH_OnAlreadyRunningAppRelaunch_DoesNotForwardEmptyBrowseURLAndReturns1(t *testing.T) {
	t.Parallel()

	gotURL := "sentinel"
	eng := &Engine{}
	eng.SetAlreadyRunningAppRelaunchHandler(func(url string) {
		gotURL = url
	})

	ret := (&dumberBPH{engine: eng}).OnAlreadyRunningAppRelaunch(relaunchCommandLineStub{
		commandLineString: "dumber browse",
	}, "")

	if ret != 1 {
		t.Fatalf("OnAlreadyRunningAppRelaunch returned %d, want 1", ret)
	}
	if gotURL != "sentinel" {
		t.Fatalf("forwarded browse url = %q, want %q", gotURL, "sentinel")
	}
}

func TestDumberBPH_OnAlreadyRunningAppRelaunch_DoesNotForwardNonBrowse(t *testing.T) {
	t.Parallel()

	called := false
	eng := &Engine{}
	eng.SetAlreadyRunningAppRelaunchHandler(func(string) {
		called = true
	})

	ret := (&dumberBPH{engine: eng}).OnAlreadyRunningAppRelaunch(relaunchCommandLineStub{
		commandLineString: "dumber",
	}, "")

	if ret != 0 {
		t.Fatalf("OnAlreadyRunningAppRelaunch returned %d, want 0", ret)
	}
	if called {
		t.Fatal("handler should not be called for non-browse relaunch")
	}
}

func TestConfigureCommandLine_AppendsExpectedSwitches(t *testing.T) {
	commandLine := newMutableCommandLineStub()
	configureCommandLine(commandLine)

	if !commandLine.HasSwitch("enable-smooth-scrolling") {
		t.Fatal("expected enable-smooth-scrolling switch to be appended")
	}
	if got := commandLine.GetSwitchValue("autoplay-policy"); got != "no-user-gesture-required" {
		t.Fatalf("autoplay-policy = %q, want %q", got, "no-user-gesture-required")
	}
}

func TestConfigureCommandLine_DisablesWebAuthnByDefault(t *testing.T) {
	t.Setenv(cefEnableWebAuthnUnsafeEnvVar, "")

	commandLine := newMutableCommandLineStub()
	configureCommandLine(commandLine)

	want := "WebAuth"
	if got := commandLine.GetSwitchValue(chromiumDisableFeaturesSwitch); got != want {
		t.Fatalf("disable-features = %q, want %q", got, want)
	}
	if got := commandLine.GetSwitchValue(chromiumDisableBlinkFeaturesSwitch); got != want {
		t.Fatalf("disable-blink-features = %q, want %q", got, want)
	}
}

func TestConfigureCommandLine_WebAuthnUnsafeOptInDoesNotDisableWebAuth(t *testing.T) {
	t.Setenv(cefEnableWebAuthnUnsafeEnvVar, "1")

	commandLine := newMutableCommandLineStub()
	configureCommandLine(commandLine)

	if got := commandLine.GetSwitchValue(chromiumDisableFeaturesSwitch); strings.Contains(got, "WebAuth") {
		t.Fatalf("disable-features = %q, should not contain WebAuth", got)
	}
	if got := commandLine.GetSwitchValue(chromiumDisableBlinkFeaturesSwitch); strings.Contains(got, "WebAuth") {
		t.Fatalf("disable-blink-features = %q, should not contain WebAuth", got)
	}
}

func TestConfigureCommandLine_WebAuthnDisablePreservesExistingDisableFeatures(t *testing.T) {
	commandLine := newMutableCommandLineStub()
	commandLine.AppendSwitchWithValue(chromiumDisableFeaturesSwitch, "ExistingFeature,WebAuth")
	commandLine.AppendSwitchWithValue(chromiumDisableBlinkFeaturesSwitch, "ExistingBlinkFeature,WebAuth")

	configureCommandLine(commandLine)

	if got := commandLine.GetSwitchValue(chromiumDisableFeaturesSwitch); got != "ExistingFeature,WebAuth" {
		t.Fatalf("disable-features = %q, want %q", got, "ExistingFeature,WebAuth")
	}
	if got := commandLine.GetSwitchValue(chromiumDisableBlinkFeaturesSwitch); got != "ExistingBlinkFeature,WebAuth" {
		t.Fatalf("disable-blink-features = %q, want %q", got, "ExistingBlinkFeature,WebAuth")
	}
}

func TestConfigureCommandLine_WebAuthnUnsafeOverrideRemovesExistingWebAuthTokens(t *testing.T) {
	t.Setenv(cefEnableWebAuthnUnsafeEnvVar, "1")

	commandLine := newMutableCommandLineStub()
	commandLine.AppendSwitchWithValue(chromiumDisableFeaturesSwitch, "ExistingFeature,WebAuth,AnotherFeature")
	commandLine.AppendSwitchWithValue(chromiumDisableBlinkFeaturesSwitch, "WebAuth,ExistingBlinkFeature")

	configureCommandLine(commandLine)

	if got := commandLine.GetSwitchValue(chromiumDisableFeaturesSwitch); got != "ExistingFeature,AnotherFeature" {
		t.Fatalf("disable-features = %q, want %q", got, "ExistingFeature,AnotherFeature")
	}
	if got := commandLine.GetSwitchValue(chromiumDisableBlinkFeaturesSwitch); got != "ExistingBlinkFeature" {
		t.Fatalf("disable-blink-features = %q, want %q", got, "ExistingBlinkFeature")
	}
}

func TestConfigureCommandLine_WebAuthnUnsafeOverrideRemovesSwitchWhenOnlyWebAuth(t *testing.T) {
	t.Setenv(cefEnableWebAuthnUnsafeEnvVar, "1")

	commandLine := newMutableCommandLineStub()
	commandLine.AppendSwitchWithValue(chromiumDisableFeaturesSwitch, "WebAuth")

	configureCommandLine(commandLine)

	if commandLine.HasSwitch(chromiumDisableFeaturesSwitch) {
		t.Fatalf("disable-features switch should have been removed entirely, but has value %q", commandLine.GetSwitchValue(chromiumDisableFeaturesSwitch))
	}
}

func TestConfigureCommandLine_SetsOzonePlatformToWayland(t *testing.T) {
	commandLine := newMutableCommandLineStub()
	configureCommandLine(commandLine)

	if got := commandLine.GetSwitchValue("ozone-platform"); got != "wayland" {
		t.Fatalf("ozone-platform = %q, want %q", got, "wayland")
	}

	// Dumber-specific switches must remain present.
	if !commandLine.HasSwitch("enable-smooth-scrolling") {
		t.Fatal("expected enable-smooth-scrolling switch to be present")
	}
	if got := commandLine.GetSwitchValue("autoplay-policy"); got != "no-user-gesture-required" {
		t.Fatalf("autoplay-policy = %q, want %q", got, "no-user-gesture-required")
	}
}

func TestConfigureCommandLine_PreservesExistingOzonePlatform(t *testing.T) {
	commandLine := newMutableCommandLineStub()
	commandLine.AppendSwitchWithValue("ozone-platform", "x11")

	configureCommandLine(commandLine)

	if got := commandLine.GetSwitchValue("ozone-platform"); got != "x11" {
		t.Fatalf("ozone-platform = %q, want %q (bridge preserves existing value)", got, "x11")
	}

	// Dumber-specific switches must remain present.
	if !commandLine.HasSwitch("enable-smooth-scrolling") {
		t.Fatal("expected enable-smooth-scrolling switch to be present")
	}
	if got := commandLine.GetSwitchValue("autoplay-policy"); got != "no-user-gesture-required" {
		t.Fatalf("autoplay-policy = %q, want %q", got, "no-user-gesture-required")
	}
}

func TestConfigureCommandLine_EnableVAAPIEnvAppendsHardwareDecodeFlags(t *testing.T) {
	t.Setenv(cefEnableVAAPIEnvVar, "1")

	commandLine := newMutableCommandLineStub()
	commandLine.AppendSwitchWithValue(chromiumEnableFeaturesSwitch, "ExistingFeature")

	configureCommandLine(commandLine)

	gotFeatures := commandLine.GetSwitchValue(chromiumEnableFeaturesSwitch)
	for _, feature := range []string{
		"ExistingFeature",
		"AcceleratedVideoDecoder",
		"AcceleratedVideoEncoder",
		"AcceleratedVideoDecodeLinuxGL",
		"AcceleratedVideoDecodeLinuxZeroCopyGL",
		"VaapiIgnoreDriverChecks",
	} {
		if !strings.Contains(gotFeatures, feature) {
			t.Fatalf("enable-features = %q, missing %q", gotFeatures, feature)
		}
	}
	if !commandLine.HasSwitch("ignore-gpu-blocklist") {
		t.Fatal("expected ignore-gpu-blocklist switch")
	}
	if !commandLine.HasSwitch("enable-zero-copy") {
		t.Fatal("expected enable-zero-copy switch")
	}
	if !commandLine.HasSwitch("disable-gpu-driver-bug-workaround") {
		t.Fatal("expected disable-gpu-driver-bug-workaround switch")
	}
}

func TestConfigureCommandLine_ChromiumFlagsEnvAppendsSwitches(t *testing.T) {
	t.Setenv(cefChromiumFlagsEnvVar, `--enable-features=VaapiVideoDecoder,VaapiIgnoreDriverChecks --ignore-gpu-blocklist --autoplay-policy=document-user-activation-required ignored-arg`)

	commandLine := newMutableCommandLineStub()
	commandLine.AppendSwitchWithValue(chromiumEnableFeaturesSwitch, "ExistingFeature")

	configureCommandLine(commandLine)

	if got := commandLine.GetSwitchValue(chromiumEnableFeaturesSwitch); got != "ExistingFeature,VaapiVideoDecoder,VaapiIgnoreDriverChecks" {
		t.Fatalf("enable-features = %q, want env features appended", got)
	}
	if !commandLine.HasSwitch("ignore-gpu-blocklist") {
		t.Fatal("expected ignore-gpu-blocklist switch")
	}
	if got := commandLine.GetSwitchValue("autoplay-policy"); got != "document-user-activation-required" {
		t.Fatalf("autoplay-policy = %q, want env override", got)
	}
	if commandLine.HasSwitch("ignored-arg") {
		t.Fatal("non-switch env token should be ignored")
	}
}

func TestParseChromiumSwitchToken(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		token     string
		wantName  string
		wantValue string
		wantOK    bool
	}{
		{name: "long switch with value", token: "--enable-features=VaapiVideoDecoder", wantName: "enable-features", wantValue: "VaapiVideoDecoder", wantOK: true},
		{name: "long switch without value", token: "--ignore-gpu-blocklist", wantName: "ignore-gpu-blocklist", wantOK: true},
		{name: "single dash switch", token: "-enable-zero-copy", wantName: "enable-zero-copy", wantOK: true},
		{name: "empty value", token: "--flag=", wantName: "flag", wantOK: true},
		{name: "embedded equals", token: "--flag=a=b", wantName: "flag", wantValue: "a=b", wantOK: true},
		{name: "non switch", token: "youtube.com", wantOK: false},
		{name: "sentinel", token: "--", wantOK: false},
		{name: "empty", token: " ", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotValue, gotOK := parseChromiumSwitchToken(tt.token)
			if gotName != tt.wantName || gotValue != tt.wantValue || gotOK != tt.wantOK {
				t.Fatalf("parseChromiumSwitchToken(%q) = (%q, %q, %v), want (%q, %q, %v)", tt.token, gotName, gotValue, gotOK, tt.wantName, tt.wantValue, tt.wantOK)
			}
		})
	}
}

func TestAppendUniqueCommaSeparatedSwitchValues(t *testing.T) {
	t.Run("trims whitespace and skips empty existing values", func(t *testing.T) {
		commandLine := newMutableCommandLineStub()
		commandLine.AppendSwitchWithValue("disable-features", " , ExistingFeature , ")

		appendUniqueCommaSeparatedSwitchValues(commandLine, "disable-features", "FeatureA")

		want := "ExistingFeature,FeatureA"
		if got := commandLine.GetSwitchValue("disable-features"); got != want {
			t.Fatalf("disable-features = %q, want %q", got, want)
		}
	})

	t.Run("deduplicates existing and appended values", func(t *testing.T) {
		commandLine := newMutableCommandLineStub()
		commandLine.AppendSwitchWithValue("disable-features", "FeatureA,FeatureA")

		appendUniqueCommaSeparatedSwitchValues(commandLine, "disable-features", "FeatureA", "FeatureB")

		want := "FeatureA,FeatureB"
		if got := commandLine.GetSwitchValue("disable-features"); got != want {
			t.Fatalf("disable-features = %q, want %q", got, want)
		}
	})

	t.Run("empty appended values are no-op", func(t *testing.T) {
		commandLine := newMutableCommandLineStub()
		commandLine.AppendSwitchWithValue("disable-features", "ExistingFeature")

		appendUniqueCommaSeparatedSwitchValues(commandLine, "disable-features")

		if got := commandLine.GetSwitchValue("disable-features"); got != "ExistingFeature" {
			t.Fatalf("disable-features = %q, want ExistingFeature", got)
		}
	})

	t.Run("nil command line is no-op", func(_ *testing.T) {
		appendUniqueCommaSeparatedSwitchValues(nil, "disable-features", "FeatureA")
	})
}

func TestDumberBPH_OnBeforeChildProcessLaunch_AppendsNoZygote(t *testing.T) {
	commandLine := newMutableCommandLineStub()
	commandLine.AppendSwitchWithValue("type", "renderer")

	(&dumberBPH{engine: &Engine{ctx: context.Background()}}).OnBeforeChildProcessLaunch(commandLine)

	if !commandLine.HasSwitch("no-zygote") {
		t.Fatal("expected no-zygote switch to be appended for child processes")
	}
}

func TestDumberBPH_OnBeforeChildProcessLaunch_AppliesRenderingAndHardwareDecodeFlags(t *testing.T) {
	t.Setenv(cef2gtkAngleBackendVar, "vulkan")
	t.Setenv(cefEnableVAAPIEnvVar, "1")

	commandLine := newMutableCommandLineStub()
	commandLine.AppendSwitchWithValue("type", "gpu-process")
	commandLine.AppendSwitchWithValue("use-angle", "gl-egl")

	(&dumberBPH{engine: &Engine{ctx: context.Background()}}).OnBeforeChildProcessLaunch(commandLine)

	if got := commandLine.GetSwitchValue("use-angle"); got != "vulkan" {
		t.Fatalf("use-angle = %q, want vulkan", got)
	}
	if got := commandLine.GetSwitchValue(chromiumEnableFeaturesSwitch); !strings.Contains(got, "AcceleratedVideoDecoder") || !strings.Contains(got, "VulkanFromANGLE") {
		t.Fatalf("enable-features = %q, want hardware decode and Vulkan ANGLE features", got)
	}
	if !commandLine.HasSwitch("ignore-gpu-blocklist") {
		t.Fatal("expected ignore-gpu-blocklist switch")
	}
	if !commandLine.HasSwitch("enable-zero-copy") {
		t.Fatal("expected enable-zero-copy switch")
	}
	if !commandLine.HasSwitch("no-zygote") {
		t.Fatal("expected no-zygote switch")
	}
}

func TestDumberBPH_OnBeforeChildProcessLaunch_DisablesWebAuthnByDefault(t *testing.T) {
	t.Setenv(cefEnableWebAuthnUnsafeEnvVar, "")

	commandLine := newMutableCommandLineStub()
	commandLine.AppendSwitchWithValue("type", "renderer")

	(&dumberBPH{engine: &Engine{ctx: context.Background()}}).OnBeforeChildProcessLaunch(commandLine)

	want := "WebAuth"
	if got := commandLine.GetSwitchValue(chromiumDisableFeaturesSwitch); got != want {
		t.Fatalf("disable-features = %q, want %q", got, want)
	}
	if got := commandLine.GetSwitchValue(chromiumDisableBlinkFeaturesSwitch); got != want {
		t.Fatalf("disable-blink-features = %q, want %q", got, want)
	}
	if !commandLine.HasSwitch("no-zygote") {
		t.Fatal("expected no-zygote switch to be appended")
	}
}
