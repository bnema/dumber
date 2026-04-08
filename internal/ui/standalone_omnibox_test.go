package ui

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/ui/component"
	layoutmocks "github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/bnema/puregotk/v4/layershell"
)

func TestWrapStandaloneOmniboxNavigate_CallsOriginalThenQuit(t *testing.T) {
	var calls []string
	wrapped := wrapStandaloneOmniboxNavigate(func(url string) {
		calls = append(calls, "navigate:"+url)
	}, func() {
		calls = append(calls, "quit")
	})

	wrapped("https://example.com")

	if len(calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %#v", len(calls), calls)
	}
	if calls[0] != "navigate:https://example.com" {
		t.Fatalf("expected navigate call first, got %#v", calls)
	}
	if calls[1] != "quit" {
		t.Fatalf("expected quit call second, got %#v", calls)
	}
}

func TestWrapStandaloneOmniboxNavigate_QuitsWithoutOriginalNavigate(t *testing.T) {
	called := false
	wrapped := wrapStandaloneOmniboxNavigate(nil, func() {
		called = true
	})

	wrapped("https://example.com")

	if !called {
		t.Fatalf("expected quit to be called")
	}
}

func TestStandaloneOmniboxCloseHandler_QuitsApp(t *testing.T) {
	called := false
	onClose := standaloneOmniboxCloseHandler(func() {
		called = true
	})

	onClose()

	if !called {
		t.Fatalf("expected close handler to quit app")
	}
}

func TestStandaloneOmniboxWindowDecorated_LayerShellAppliedReturnsFalse(t *testing.T) {
	if standaloneOmniboxWindowDecorated(true) {
		t.Fatalf("expected layer-shell window to be undecorated")
	}
}

func TestStandaloneOmniboxWindowDecorated_FallbackReturnsTrue(t *testing.T) {
	if !standaloneOmniboxWindowDecorated(false) {
		t.Fatalf("expected fallback window to remain decorated")
	}
}

func TestStandaloneOmniboxWindowResizable_LayerShellAppliedReturnsFalse(t *testing.T) {
	if standaloneOmniboxWindowResizable(true) {
		t.Fatalf("expected layer-shell window to stop keeping stale height allocations")
	}
}

func TestStandaloneOmniboxWindowResizable_FallbackReturnsTrue(t *testing.T) {
	if !standaloneOmniboxWindowResizable(false) {
		t.Fatalf("expected fallback window to remain resizable")
	}
}

func TestStandaloneOmniboxHostLayout_LayerShellUsesNaturalSizingWithoutDefaultSize(t *testing.T) {
	layoutCfg := standaloneOmniboxHostLayout(true)

	if layoutCfg.UseDefaultSize {
		t.Fatalf("expected layer-shell host to avoid fixed default size")
	}
	if layoutCfg.OmniboxSize.FixedWidth != 800 {
		t.Fatalf("expected standalone omnibox fixed width 800, got %d", layoutCfg.OmniboxSize.FixedWidth)
	}
	if !layoutCfg.OmniboxSize.UseFixedTopMargin {
		t.Fatalf("expected standalone omnibox to use a fixed top margin")
	}
	if layoutCfg.OmniboxSize.FixedTopMargin != 0 {
		t.Fatalf("expected standalone omnibox top margin 0, got %d", layoutCfg.OmniboxSize.FixedTopMargin)
	}
}

func TestStandaloneOmniboxHostLayout_FallbackUsesDefaultSize(t *testing.T) {
	layoutCfg := standaloneOmniboxHostLayout(false)

	if !layoutCfg.UseDefaultSize {
		t.Fatalf("expected non-layer-shell host to keep fixed fallback size")
	}
	if layoutCfg.OmniboxSize != standaloneOmniboxSizeConfig() {
		t.Fatalf("expected fallback host to use standalone omnibox sizing, got %#v", layoutCfg.OmniboxSize)
	}
}

func TestAttachStandaloneOmniboxHostWidget_UsesPrimaryChild(t *testing.T) {
	host := layoutmocks.NewMockOverlayWidget(t)
	child := layoutmocks.NewMockWidget(t)

	host.EXPECT().SetChild(child).Once()

	attachStandaloneOmniboxHostWidget(host, child)
}

func TestStandaloneOmniboxSizeConfig_UsesFixedGeometry(t *testing.T) {
	cfg := standaloneOmniboxSizeConfig()

	if cfg != (component.ModalSizeConfig{
		WidthPct:          component.OmniboxSizeDefaults.WidthPct,
		MaxWidth:          component.OmniboxSizeDefaults.MaxWidth,
		TopMarginPct:      component.OmniboxSizeDefaults.TopMarginPct,
		FallbackWidth:     component.OmniboxSizeDefaults.FallbackWidth,
		FallbackHeight:    component.OmniboxSizeDefaults.FallbackHeight,
		FixedWidth:        800,
		FixedTopMargin:    0,
		UseFixedTopMargin: true,
	}) {
		t.Fatalf("unexpected standalone omnibox size config: %#v", cfg)
	}
}

func TestStandaloneWindowConfig_Defaults(t *testing.T) {
	cfg := DefaultStandaloneOmniboxWindowConfig()
	if cfg.Namespace == "" {
		t.Fatalf("expected namespace")
	}
	if cfg.Title == "" {
		t.Fatalf("expected title")
	}
}

func TestStandaloneOmniboxArgv_RemovesGoTestFlags(t *testing.T) {
	argv := standaloneOmniboxArgv([]string{"/tmp/testbin", "-test.v", "-test.run", "TestFoo", "--gtk-debug"})

	if len(argv) != 2 {
		t.Fatalf("expected 2 args, got %d: %#v", len(argv), argv)
	}
	if argv[0] != "/tmp/testbin" {
		t.Fatalf("expected binary path to be preserved, got %q", argv[0])
	}
	if argv[1] != "--gtk-debug" {
		t.Fatalf("expected non-test arg to be preserved, got %#v", argv)
	}
}

func TestStandaloneOmniboxArgv_RemovesOnlyLeadingOmniboxSubcommand(t *testing.T) {
	argv := standaloneOmniboxArgv([]string{"/tmp/testbin", "omnibox", "https://example.com/omnibox", "omnibox"})

	if len(argv) != 3 {
		t.Fatalf("expected 3 args, got %d: %#v", len(argv), argv)
	}
	if argv[0] != "/tmp/testbin" {
		t.Fatalf("expected binary path to be preserved, got %q", argv[0])
	}
	if argv[1] != "https://example.com/omnibox" {
		t.Fatalf("expected non-subcommand omnibox value to be preserved, got %#v", argv)
	}
	if argv[2] != "omnibox" {
		t.Fatalf("expected later omnibox arg to be preserved, got %#v", argv)
	}
}

func TestActivateStandaloneOmnibox_RetainsObjectsUntilReleased(t *testing.T) {
	originalBuildHost := buildStandaloneOmniboxHostFn
	originalNewWindow := newStandaloneOmniboxWindow
	originalConfigureWindow := configureStandaloneOmniboxWindowFn
	originalPresentWindow := presentStandaloneOmniboxWindow
	originalLogHostAllocation := logStandaloneOmniboxHostAllocationFn
	originalShowOmnibox := showStandaloneOmniboxFn
	t.Cleanup(func() {
		buildStandaloneOmniboxHostFn = originalBuildHost
		newStandaloneOmniboxWindow = originalNewWindow
		configureStandaloneOmniboxWindowFn = originalConfigureWindow
		presentStandaloneOmniboxWindow = originalPresentWindow
		logStandaloneOmniboxHostAllocationFn = originalLogHostAllocation
		showStandaloneOmniboxFn = originalShowOmnibox
	})

	expectedHost := &gtk.Widget{}
	expectedOmnibox := &component.Omnibox{}
	expectedWindow := &gtk.ApplicationWindow{}
	refs := &standaloneOmniboxActivationRetention{}
	runtimeCfg := &StandaloneOmniboxRuntime{}
	windowCfg := DefaultStandaloneOmniboxWindowConfig()

	buildStandaloneOmniboxHostFn = func(_ context.Context, _ *StandaloneOmniboxRuntime, _ *gtk.Application) (*gtk.Widget, *component.Omnibox) {
		return expectedHost, expectedOmnibox
	}
	newStandaloneOmniboxWindow = func(_ *gtk.Application) *gtk.ApplicationWindow {
		return expectedWindow
	}
	configureStandaloneOmniboxWindowFn = func(window *gtk.ApplicationWindow, cfg StandaloneOmniboxWindowConfig, child *gtk.Widget) {
		if window != expectedWindow || child != expectedHost || cfg != windowCfg {
			t.Fatalf("unexpected configure arguments: window=%p child=%p cfg=%#v", window, child, cfg)
		}
	}
	presentStandaloneOmniboxWindow = func(window *gtk.ApplicationWindow) {
		if window != expectedWindow {
			t.Fatalf("unexpected window presented: %p", window)
		}
	}
	logStandaloneOmniboxHostAllocationFn = func(_ context.Context, host *gtk.Widget) {
		if host != expectedHost {
			t.Fatalf("unexpected host logged: %p", host)
		}
	}
	showStandaloneOmniboxFn = func(_ context.Context, omnibox *component.Omnibox) {
		if omnibox != expectedOmnibox {
			t.Fatalf("unexpected omnibox shown: %p", omnibox)
		}
		if refs.window != expectedWindow || refs.host != expectedHost || refs.omnibox != expectedOmnibox {
			t.Fatalf("expected standalone activation refs to retain GTK objects during show, got %#v", refs)
		}
	}

	activateStandaloneOmnibox(context.Background(), runtimeCfg, nil, windowCfg, refs)

	if refs.window != expectedWindow || refs.host != expectedHost || refs.omnibox != expectedOmnibox {
		t.Fatalf("expected standalone activation refs to retain GTK objects, got %#v", refs)
	}

	refs.release()

	if refs.window != nil || refs.host != nil || refs.omnibox != nil {
		t.Fatalf("expected standalone activation refs to release GTK objects, got %#v", refs)
	}
}

func TestTryInitLayerShell_NilWindowReturnsFalse(t *testing.T) {
	if tryInitLayerShell(nil, DefaultStandaloneOmniboxWindowConfig()) {
		t.Fatalf("expected nil window to skip layer-shell")
	}
}

func TestTryInitLayerShell_UnsupportedReturnsFalse(t *testing.T) {
	originalLibraryAvailable := layerShellLibraryAvailable
	original := layerShellIsSupported
	layerShellLibraryAvailable = func() bool { return true }
	layerShellIsSupported = func() bool { return false }
	t.Cleanup(func() {
		layerShellLibraryAvailable = originalLibraryAvailable
		layerShellIsSupported = original
	})

	if tryInitLayerShell(&gtk.ApplicationWindow{}, DefaultStandaloneOmniboxWindowConfig()) {
		t.Fatalf("expected unsupported layer-shell to skip initialization")
	}
}

func TestTryInitLayerShell_MissingLibraryReturnsFalseWithoutSymbolCalls(t *testing.T) {
	originalLibraryAvailable := layerShellLibraryAvailable
	originalIsSupported := layerShellIsSupported
	layerShellLibraryAvailable = func() bool { return false }
	layerShellIsSupported = func() bool {
		t.Fatalf("expected layer-shell support check to be skipped when library is missing")
		return true
	}
	t.Cleanup(func() {
		layerShellLibraryAvailable = originalLibraryAvailable
		layerShellIsSupported = originalIsSupported
	})

	if tryInitLayerShell(&gtk.ApplicationWindow{}, DefaultStandaloneOmniboxWindowConfig()) {
		t.Fatalf("expected missing library to skip layer-shell initialization")
	}
}

func TestSafeLayerShellSupported_PreloadedLibrarySkipsPathProbe(t *testing.T) {
	originalLibraryAvailable := layerShellLibraryAvailable
	originalIsSupported := layerShellIsSupported
	t.Setenv("LD_PRELOAD", "/custom/libgtk4-layer-shell.so.0")
	layerShellLibraryAvailable = func() bool {
		t.Fatalf("expected path probe to be skipped when layer-shell is already preloaded")
		return false
	}
	layerShellIsSupported = func() bool { return true }
	t.Cleanup(func() {
		layerShellLibraryAvailable = originalLibraryAvailable
		layerShellIsSupported = originalIsSupported
	})

	if !safeLayerShellSupported() {
		t.Fatalf("expected preloaded layer-shell library to be treated as supported")
	}
}

func TestTryInitLayerShell_SupportedConfiguresWindow(t *testing.T) {
	originalLibraryAvailable := layerShellLibraryAvailable
	originalIsSupported := layerShellIsSupported
	originalInitForWindow := layerShellInitForWindow
	originalSetLayer := layerShellSetLayer
	originalSetExclusiveZone := layerShellSetExclusiveZone
	originalSetNamespace := layerShellSetNamespace
	originalSetKeyboardMode := layerShellSetKeyboardMode
	t.Cleanup(func() {
		layerShellLibraryAvailable = originalLibraryAvailable
		layerShellIsSupported = originalIsSupported
		layerShellInitForWindow = originalInitForWindow
		layerShellSetLayer = originalSetLayer
		layerShellSetExclusiveZone = originalSetExclusiveZone
		layerShellSetNamespace = originalSetNamespace
		layerShellSetKeyboardMode = originalSetKeyboardMode
	})

	window := &gtk.ApplicationWindow{}
	cfg := DefaultStandaloneOmniboxWindowConfig()
	var callOrder []string

	layerShellLibraryAvailable = func() bool {
		callOrder = append(callOrder, "library")
		return true
	}
	layerShellIsSupported = func() bool {
		callOrder = append(callOrder, "supported")
		return true
	}
	layerShellInitForWindow = func(got *gtk.ApplicationWindow) {
		if got != window {
			t.Fatalf("expected init window %p, got %p", window, got)
		}
		callOrder = append(callOrder, "init")
	}
	layerShellSetLayer = func(got *gtk.ApplicationWindow, layer layershell.Layer) {
		if got != window {
			t.Fatalf("expected layer window %p, got %p", window, got)
		}
		if layer != layershell.LayerOverlayValue {
			t.Fatalf("expected overlay layer, got %v", layer)
		}
		callOrder = append(callOrder, "layer")
	}
	layerShellSetExclusiveZone = func(got *gtk.ApplicationWindow, zone int) {
		if got != window {
			t.Fatalf("expected exclusive zone window %p, got %p", window, got)
		}
		if zone != 0 {
			t.Fatalf("expected exclusive zone 0, got %d", zone)
		}
		callOrder = append(callOrder, "zone")
	}
	layerShellSetNamespace = func(got *gtk.ApplicationWindow, namespace *string) {
		if got != window {
			t.Fatalf("expected namespace window %p, got %p", window, got)
		}
		if namespace == nil || *namespace != cfg.Namespace {
			t.Fatalf("expected namespace %q, got %#v", cfg.Namespace, namespace)
		}
		callOrder = append(callOrder, "namespace")
	}
	layerShellSetKeyboardMode = func(got *gtk.ApplicationWindow, mode layershell.KeyboardMode) {
		if got != window {
			t.Fatalf("expected keyboard mode window %p, got %p", window, got)
		}
		if mode != layershell.KeyboardModeExclusiveValue {
			t.Fatalf("expected exclusive keyboard mode, got %v", mode)
		}
		callOrder = append(callOrder, "keyboard")
	}

	if !tryInitLayerShell(window, cfg) {
		t.Fatalf("expected supported layer-shell to initialize")
	}

	wantOrder := []string{"library", "supported", "init", "layer", "zone", "namespace", "keyboard"}
	if len(callOrder) != len(wantOrder) {
		t.Fatalf("expected %d calls, got %d: %#v", len(wantOrder), len(callOrder), callOrder)
	}
	for i := range wantOrder {
		if callOrder[i] != wantOrder[i] {
			t.Fatalf("expected call %d to be %q, got %q", i, wantOrder[i], callOrder[i])
		}
	}
}
