# purego-cef2gtk

GTK4 widget bridge for [`purego-cef`](https://github.com/bnema/purego-cef) accelerated off-screen rendering.

## Status

Early bootstrap. The renderer is Wayland-only and GPU-first:

- CEF accelerated OSR with shared textures/DMABUFs
- GTK4 `GdkDmabufTextureBuilder` + `GtkPicture`/`GtkGraphicsOffload` backend for GTK/GSK rendering
- GTK4 `GtkGLArea` EGL/OpenGL import-and-copy backend as fallback/debug path
- no X11 target
- no `--disable-gpu` or software rendering fallback
- CEF `OnPaint` is treated as a diagnostic/error path, not as a renderer

## Renderer stacks

`NewView()` defaults to the Vulkan GPU stack and fails during view setup if the selected stack cannot be constructed. Use `ResolveRenderStack` to choose a coherent stack explicitly:

```go
plan, err := cef2gtk.ResolveRenderStack(cef2gtk.RenderStackVulkan) // or RenderStackEGL
if err != nil {
    return err
}
cef2gtk.ConfigureRenderStackEnvironment(plan) // call before GTK initialization
cef2gtk.ConfigureCommandLine(commandLine, cef2gtk.CommandLineOptions{RenderStackPlan: plan})
view := cef2gtk.NewViewWithOptions(cef2gtk.ViewOptions{RenderStackPlan: plan})
```

Supported stacks:

- `vulkan`: GDK DMABUF presentation with ANGLE Vulkan and GSK Vulkan.
- `egl`: GtkGLArea presentation with ANGLE GL/EGL and GSK OpenGL.

Environment overrides remain available as diagnostics/backcompat escape hatches:

```sh
PUREGO_CEF2GTK_BACKEND=gdk-dmabuf|glarea
PUREGO_CEF2GTK_ANGLE_BACKEND=vulkan|gl-egl|none
PUREGO_CEF2GTK_OSR_BACKING_SCALE=auto|on|off
```

`PUREGO_CEF2GTK_OSR_BACKING_SCALE=auto` enables the Linux accelerated-OSR
HiDPI compatibility path only when the GTK surface scale is greater than 1. In
that mode CEF receives a device-sized OSR view rect with a 1x screen scale,
because current CEF shared-texture OSR builds can otherwise report a fractional
`device_scale_factor` while still emitting 1x/logical DMABUF frames. Applications
that call `BrowserHost.SetZoomLevel` should divide their page zoom by
`cef2gtk.OSRBackingScaleFactorForScale(float64(view.DeviceScaleFactor()))` before
converting it to CEF's logarithmic zoom level; this keeps the page's CSS viewport
at the GTK logical size while the OSR backing remains device-sized.

Recommended local checks:

```sh
GSK_RENDERER=vulkan PUREGO_CEF2GTK_BACKEND=gdk-dmabuf go run ./examples/simple-browser
GSK_RENDERER=opengl PUREGO_CEF2GTK_BACKEND=glarea PUREGO_CEF2GTK_ANGLE_BACKEND=gl-egl go run ./examples/simple-browser
```

## Usage sketch

```go
plan, err := cef2gtk.ResolveRenderStack(cef2gtk.RenderStackVulkan)
if err != nil {
    return err
}

cef2gtk.ConfigureRenderStackEnvironment(plan) // before GTK initialization

// In cef.App.OnBeforeCommandLineProcessing:
cef2gtk.ConfigureCommandLine(commandLine, cef2gtk.CommandLineOptions{RenderStackPlan: plan})

view := cef2gtk.NewViewWithOptions(cef2gtk.ViewOptions{RenderStackPlan: plan})
window.SetChild(view.Widget())
window.Present()
view.Widget().Realize()
if err := view.PrepareOnGTKThread(); err != nil {
    return err
}

info := cef.NewWindowInfo()
// After the GTK widget/surface is realized, extract its native platform handle
// with the appropriate GTK/platform API and pass that handle as Parent.
cef2gtk.ConfigureWindowInfo(&info, cef2gtk.WindowInfoOptions{Parent: /* realized native handle */})
settings := cef.NewBrowserSettings()
cef2gtk.ConfigureBrowserSettings(&settings, cef2gtk.BrowserSettingsOptions{WindowlessFrameRate: 144})
client := cef.NewClient(myClient{render: view.RenderHandler(cef2gtk.Hooks{})})
cef.BrowserHostCreateBrowser(&info, client, "https://example.com/", &settings, nil, nil)

// In OnAfterCreated(browser cef.Browser), attach the BrowserHost:
_ = view.AttachInput(browser.GetHost(), cef2gtk.InputOptions{Scale: 0})
```

See `examples/simple-browser` for a complete accelerated-only GTK+CEF setup.

## Development

The module depends on the published `github.com/bnema/puregotk` module. During local puregotk development,
you can temporarily add your own `replace github.com/bnema/puregotk => ../puregotk` directive.

### Local smoke execution

The integration smoke suite exercises GTK/EGL context probing, the deterministic DMABUF import-copy
pipeline (synthetic GL texture through FBO render-to-texture and swap), and a build of the example
browser. Smoke tests are skipped by default and require both `WAYLAND_DISPLAY` and the opt-in
environment variable:

```sh
PUREGO_CEF2GTK_CEF_RUNTIME_SMOKE=1 rtk go test ./integration/ -run TestRuntimeSmokeGate -v
```

The probe-import-copy step validates the accelerated frame import/swap path deterministically:
it allocates a real DRM dumb buffer dma-buf through `/dev/dri/card0` (falling back to a synthetic
4×4 GL texture when DRM access is unavailable), fills it with a deterministic 4×4 RGBA pattern,
imports it through `eglImporter.ImportDMABUF` + `glEGLImageTargetTexture2DOES`, and exercises the
`CopyImportedToOwned` FBO render-to-texture pipeline and `DrawTextureToCurrentFramebuffer` swap
path. After the copy pipeline, the result is read back through an FBO and compared with the
original pixel data to verify data integrity. All GL errors, framebuffer completeness, and shader
compilation are checked, providing strict end-to-end coverage of the accelerated EGL import + copy
pipeline.

```sh
rtk make check
```
