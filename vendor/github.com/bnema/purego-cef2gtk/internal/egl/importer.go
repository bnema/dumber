package egl

import (
	"fmt"
	"runtime"

	"github.com/bnema/purego"

	"github.com/bnema/purego-cef2gtk/internal/dmabuf"
)

const (
	noContext uintptr = 0
	noImage   uintptr = 0

	success              uint32 = 0x3000
	badAlloc             uint32 = 0x3003
	badAttribute         uint32 = 0x3004
	badContext           uint32 = 0x3006
	badCurrentSurface    uint32 = 0x3007
	badDisplay           uint32 = 0x3008
	badMatch             uint32 = 0x3009
	badParameter         uint32 = 0x300C
	badAccess            uint32 = 0x3002
	badNativePixmap      uint32 = 0x300A
	badNativeWindow      uint32 = 0x300B
	contextLost          uint32 = 0x300E
	targetLinuxDMABUFExt uint32 = 0x3270
)

// Image is an EGLImageKHR handle. Zero is EGL_NO_IMAGE_KHR.
type Image uintptr

func (i Image) Valid() bool { return i != Image(noImage) }

// Importer imports callback-scoped DMABUF frames into EGLImageKHR handles.
type Importer struct {
	display    Display
	extensions Extensions
	create     func(display uintptr, context uintptr, target uint32, buffer uintptr, attrs *Attribute) uintptr
	destroy    func(display uintptr, image uintptr) uint32
	getError   func() uint32
}

// NewImporterFromCurrentDisplay creates an EGL DMABUF importer for the current
// thread's EGL display. Callers must invoke it while the GTK GL context is
// current on the GTK thread.
func NewImporterFromCurrentDisplay(required ...string) (*Importer, error) {
	b, err := defaultBackend()
	if err != nil {
		return nil, err
	}
	info, err := currentDisplayInfo(b)
	if err != nil {
		return nil, err
	}
	required = append([]string{ExtensionDMABUFImport}, required...)
	if err := info.RequireExtensions(required...); err != nil {
		return nil, err
	}
	db, ok := b.(*dynamicBackend)
	if !ok || db.eglCreateImageKHR == nil || db.eglDestroyImageKHR == nil || db.eglGetError == nil {
		return nil, fmt.Errorf("%w: EGL image import functions unavailable", ErrUnavailable)
	}
	return &Importer{display: info.Display, extensions: info.Extensions, create: db.eglCreateImageKHR, destroy: db.eglDestroyImageKHR, getError: db.eglGetError}, nil
}

// Close is a no-op retained for lifecycle symmetry with GL importers. Importer
// does not own EGLImages beyond the callback-scoped handles destroyed by Destroy.
func (i *Importer) Close() error { return nil }

// ImportDMABUF imports frame into an EGLImageKHR. The returned image remains
// callback-scoped and must be destroyed before returning from the paint callback.
func (i *Importer) ImportDMABUF(frame dmabuf.BorrowedFrame) (Image, error) {
	if i == nil || !i.display.Valid() || i.create == nil {
		return 0, fmt.Errorf("%w: EGL importer not initialized", ErrUnavailable)
	}
	attrs, err := DMABUFImageAttributes(frame, i.extensions)
	if err != nil {
		return 0, err
	}
	image := Image(i.create(uintptr(i.display), noContext, targetLinuxDMABUFExt, 0, &attrs[0]))
	if !image.Valid() {
		return 0, fmt.Errorf("eglCreateImageKHR DMABUF import failed: %s", i.errorName())
	}
	return image, nil
}

// Destroy releases image with eglDestroyImageKHR and reports EGL errors.
func (i *Importer) Destroy(image Image) error {
	if !image.Valid() {
		return nil
	}
	if i == nil || !i.display.Valid() || i.destroy == nil {
		return fmt.Errorf("%w: EGL importer not initialized", ErrUnavailable)
	}
	if ok := i.destroy(uintptr(i.display), uintptr(image)); ok == 0 {
		return fmt.Errorf("eglDestroyImageKHR failed: %s", i.errorName())
	}
	return nil
}

func (i *Importer) errorName() string {
	if i == nil || i.getError == nil {
		return "unknown EGL error"
	}
	return ErrorName(i.getError())
}

// ErrorName returns a stable name for an EGL error code.
func ErrorName(code uint32) string {
	switch code {
	case success:
		return "EGL_SUCCESS"
	case badAccess:
		return "EGL_BAD_ACCESS"
	case badAlloc:
		return "EGL_BAD_ALLOC"
	case badAttribute:
		return "EGL_BAD_ATTRIBUTE"
	case badContext:
		return "EGL_BAD_CONTEXT"
	case badCurrentSurface:
		return "EGL_BAD_CURRENT_SURFACE"
	case badDisplay:
		return "EGL_BAD_DISPLAY"
	case badMatch:
		return "EGL_BAD_MATCH"
	case badNativePixmap:
		return "EGL_BAD_NATIVE_PIXMAP"
	case badNativeWindow:
		return "EGL_BAD_NATIVE_WINDOW"
	case badParameter:
		return "EGL_BAD_PARAMETER"
	case contextLost:
		return "EGL_CONTEXT_LOST"
	default:
		return fmt.Sprintf("0x%x", code)
	}
}

func eglProc(handle uintptr, name string) uintptr {
	if sym, err := purego.Dlsym(handle, name); err == nil {
		return sym
	}
	var getProcAddress func(*byte) uintptr
	sym, err := purego.Dlsym(handle, "eglGetProcAddress")
	if err != nil {
		return 0
	}
	purego.RegisterFunc(&getProcAddress, sym)
	nameBytes := append([]byte(name), 0)
	addr := getProcAddress(&nameBytes[0])
	runtime.KeepAlive(nameBytes)
	return addr
}

func registerEGLProc(fptr any, handle uintptr, name string) error {
	sym := eglProc(handle, name)
	if sym == 0 {
		return fmt.Errorf("%w: missing %s", ErrUnavailable, name)
	}
	purego.RegisterFunc(fptr, sym)
	return nil
}
