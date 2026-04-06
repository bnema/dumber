package ui

import (
	"context"
	"os"
	"strings"

	"github.com/bnema/dumber/internal/bootstrap"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/layout"
	"github.com/bnema/puregotk/v4/adw"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/glib"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/bnema/puregotk/v4/layershell"
)

var layerShellLibraryAvailable = func() bool {
	if bootstrap.LayerShellPreloadPresent(bootstrap.CurrentEnvMap()) {
		return true
	}

	return bootstrap.LayerShellLibraryPath() != ""
}

const (
	standaloneOmniboxNamespace = AppID + ".omnibox"
	standaloneOmniboxTitle     = "Dumber Omnibox"
	standaloneOmniboxWidth     = 1200
	standaloneOmniboxHeight    = 200
	standaloneOmniboxWindowCSS = "standalone-omnibox-window"
)

type StandaloneOmniboxWindowConfig struct {
	Namespace string
	Title     string
	Width     int
	Height    int
}

type StandaloneOmniboxRuntime struct {
	OmniboxCfg component.OmniboxConfig
	ApplyTheme func(display *gdk.Display)
}

type standaloneOmniboxHostLayoutConfig struct {
	UseDefaultSize bool
	OmniboxSize    component.ModalSizeConfig
}

var (
	layerShellIsSupported   = layershell.IsSupported
	layerShellInitForWindow = func(window *gtk.ApplicationWindow) {
		layershell.InitForWindow(&window.Window)
	}
	layerShellSetLayer = func(window *gtk.ApplicationWindow, layer layershell.Layer) {
		layershell.SetLayer(&window.Window, layer)
	}
	layerShellSetExclusiveZone = func(window *gtk.ApplicationWindow, zone int) {
		layershell.SetExclusiveZone(&window.Window, zone)
	}
	layerShellSetNamespace = func(window *gtk.ApplicationWindow, namespace *string) {
		layershell.SetNamespace(&window.Window, namespace)
	}
	layerShellSetKeyboardMode = func(window *gtk.ApplicationWindow, mode layershell.KeyboardMode) {
		layershell.SetKeyboardMode(&window.Window, mode)
	}
)

func DefaultStandaloneOmniboxWindowConfig() StandaloneOmniboxWindowConfig {
	return StandaloneOmniboxWindowConfig{
		Namespace: standaloneOmniboxNamespace,
		Title:     standaloneOmniboxTitle,
		Width:     standaloneOmniboxWidth,
		Height:    standaloneOmniboxHeight,
	}
}

func standaloneOmniboxArgv(args []string) []string {
	if len(args) == 0 {
		return []string{"dumber-omnibox"}
	}

	filtered := []string{args[0]}
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "omnibox" {
			continue
		}
		if strings.HasPrefix(arg, "-test.") {
			if !strings.Contains(arg, "=") && goTestFlagConsumesNextArg(arg) && i+1 < len(args) {
				i++
			}
			continue
		}
		filtered = append(filtered, arg)
	}

	return filtered
}

func goTestFlagConsumesNextArg(arg string) bool {
	switch arg {
	case "-test.bench", "-test.blockprofile", "-test.count", "-test.coverprofile",
		"-test.cpu", "-test.cpuprofile", "-test.fuzz", "-test.fuzzcachedir",
		"-test.fuzzminimizetime", "-test.fuzztime", "-test.list", "-test.memprofile",
		"-test.memprofilerate", "-test.mutexprofile", "-test.mutexprofilefraction",
		"-test.outputdir", "-test.parallel", "-test.run", "-test.shuffle",
		"-test.skip", "-test.timeout", "-test.trace":
		return true
	default:
		return false
	}
}

func wrapStandaloneOmniboxNavigate(original func(string), quit func()) func(string) {
	return func(url string) {
		if original != nil {
			original(url)
		}
		if quit != nil {
			quit()
		}
	}
}

func standaloneOmniboxCloseHandler(quit func()) func() {
	return func() {
		if quit != nil {
			quit()
		}
	}
}

func standaloneOmniboxWindowDecorated(layerShellApplied bool) bool {
	return !layerShellApplied
}

func standaloneOmniboxWindowResizable(layerShellApplied bool) bool {
	return !layerShellApplied
}

func standaloneOmniboxHostLayout(layerShellApplied bool) standaloneOmniboxHostLayoutConfig {
	return standaloneOmniboxHostLayoutConfig{
		UseDefaultSize: !layerShellApplied,
		OmniboxSize:    standaloneOmniboxSizeConfig(),
	}
}

func attachStandaloneOmniboxHostWidget(host layout.OverlayWidget, omniboxWidget layout.Widget) {
	if host == nil {
		return
	}

	host.SetChild(omniboxWidget)
}

func standaloneOmniboxSizeConfig() component.ModalSizeConfig {
	return component.ResolveModalSizeConfig(component.ModalSizeConfig{
		FixedWidth:        component.OmniboxSizeDefaults.MaxWidth,
		FixedTopMargin:    0,
		UseFixedTopMargin: true,
	}, component.OmniboxSizeDefaults)
}

func tryInitLayerShell(window *gtk.ApplicationWindow, cfg StandaloneOmniboxWindowConfig) bool {
	if window == nil {
		return false
	}
	if !safeLayerShellSupported() {
		return false
	}

	layerShellInitForWindow(window)
	layerShellSetLayer(window, layershell.LayerOverlayValue)
	layerShellSetExclusiveZone(window, 0)
	layerShellSetNamespace(window, &cfg.Namespace)
	layerShellSetKeyboardMode(window, layershell.KeyboardModeExclusiveValue)
	return true
}

func safeLayerShellSupported() (supported bool) {
	defer func() {
		if recover() != nil {
			supported = false
		}
	}()

	if bootstrap.LayerShellPreloadPresent(bootstrap.CurrentEnvMap()) || layerShellLibraryAvailable() {
		return layerShellIsSupported()
	}

	return false
}

func configureStandaloneOmniboxWindow(window *gtk.ApplicationWindow, windowCfg StandaloneOmniboxWindowConfig, child *gtk.Widget) {
	if window == nil {
		return
	}

	title := windowCfg.Title
	window.SetTitle(&title)
	layerShellApplied := tryInitLayerShell(window, windowCfg)
	hostLayout := standaloneOmniboxHostLayout(layerShellApplied)
	if hostLayout.UseDefaultSize {
		window.SetDefaultSize(windowCfg.Width, windowCfg.Height)
	}
	if layerShellApplied {
		window.AddCssClass(standaloneOmniboxWindowCSS)
	}
	window.SetDecorated(standaloneOmniboxWindowDecorated(layerShellApplied))
	window.SetResizable(standaloneOmniboxWindowResizable(layerShellApplied))
	window.SetChild(child)
}

func buildStandaloneOmniboxHost(
	ctx context.Context,
	runtimeCfg *StandaloneOmniboxRuntime,
	gtkApp *gtk.Application,
) (*gtk.Widget, *component.Omnibox) {
	factory := layout.NewGtkWidgetFactory()
	overlay := factory.NewOverlay()
	hostLayout := standaloneOmniboxHostLayout(false)
	if overlay == nil {
		logging.FromContext(ctx).Error().Msg("failed to create standalone omnibox host widgets")
		return nil, nil
	}

	cfg := runtimeCfg.OmniboxCfg
	cfg.OnNavigate = wrapStandaloneOmniboxNavigate(cfg.OnNavigate, gtkApp.Quit)
	cfg.SizeConfig = hostLayout.OmniboxSize

	omnibox := component.NewOmnibox(ctx, cfg)
	if omnibox == nil {
		logging.FromContext(ctx).Error().Msg("failed to create standalone omnibox")
		return nil, nil
	}

	overlay.SetVisible(true)
	omnibox.SetParentOverlay(overlay)
	omnibox.SetOnClose(standaloneOmniboxCloseHandler(gtkApp.Quit))

	omniboxWidget := omnibox.WidgetAsLayout(factory)
	if omniboxWidget == nil {
		logging.FromContext(ctx).Error().Msg("failed to wrap standalone omnibox widget")
		return nil, nil
	}

	attachStandaloneOmniboxHostWidget(overlay, omniboxWidget)
	logging.FromContext(ctx).Debug().Msg("standalone omnibox host configured with primary child")
	return overlay.GtkWidget(), omnibox
}

func logStandaloneOmniboxHostAllocation(ctx context.Context, host *gtk.Widget) {
	if host == nil {
		return
	}

	var cb glib.SourceFunc = func(uintptr) bool {
		logging.FromContext(ctx).Debug().
			Int("width", host.GetAllocatedWidth()).
			Int("height", host.GetAllocatedHeight()).
			Msg("standalone omnibox host allocation")
		return false
	}
	glib.IdleAdd(&cb, 0)
}

func RunStandaloneOmnibox(ctx context.Context, runtimeCfg *StandaloneOmniboxRuntime) int {
	if runtimeCfg == nil {
		logging.FromContext(ctx).Error().Msg("standalone omnibox runtime not configured")
		return 1
	}

	adw.Init()
	windowCfg := DefaultStandaloneOmniboxWindowConfig()
	appID := windowCfg.Namespace
	gtkApp := gtk.NewApplication(&appID, gtkApplicationFlags())
	if gtkApp == nil {
		logging.FromContext(ctx).Error().Msg("failed to create standalone omnibox application")
		return 1
	}
	defer gtkApp.Unref()

	activateCb := func(_ gio.Application) {
		child, omnibox := buildStandaloneOmniboxHost(ctx, runtimeCfg, gtkApp)
		if child == nil || omnibox == nil {
			gtkApp.Quit()
			return
		}

		window := gtk.NewApplicationWindow(gtkApp)
		if window == nil {
			logging.FromContext(ctx).Error().Msg("failed to create standalone omnibox window")
			gtkApp.Quit()
			return
		}

		configureStandaloneOmniboxWindow(window, windowCfg, child)
		if display := window.GetDisplay(); display != nil && runtimeCfg.ApplyTheme != nil {
			runtimeCfg.ApplyTheme(display)
		}
		window.Present()
		logStandaloneOmniboxHostAllocation(ctx, child)
		omnibox.Show(ctx, "")
	}
	gtkApp.ConnectActivate(&activateCb)

	argv := standaloneOmniboxArgv(os.Args)
	return gtkApp.Run(len(argv), argv)
}
