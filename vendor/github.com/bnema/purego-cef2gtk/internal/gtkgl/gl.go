package gtkgl

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/bnema/purego"
	"github.com/bnema/purego-cef2gtk/internal/cutil"
)

const (
	glVendor   uint32 = 0x1F00
	glRenderer uint32 = 0x1F01
	glVersion  uint32 = 0x1F02
)

var ErrGLUnavailable = errors.New("opengl unavailable")

type glInfo struct {
	Version  string
	Vendor   string
	Renderer string
}

type glBackend struct {
	glGetString func(uint32) unsafe.Pointer
}

var (
	glLoadOnce sync.Once
	loadedGL   *glBackend
	glLoadErr  error
)

func defaultGLBackend() (*glBackend, error) {
	glLoadOnce.Do(func() {
		loadedGL, glLoadErr = loadGLBackend()
	})
	return loadedGL, glLoadErr
}

func loadGLBackend() (*glBackend, error) {
	var handle uintptr
	var dlErr error
	errMsg := ""
	for _, name := range []string{"libGL.so.1", "libGLESv2.so.2", "libOpenGL.so.0", "libGL.so"} {
		handle, dlErr = purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_LOCAL)
		if dlErr == nil {
			break
		}
		if errMsg != "" {
			errMsg += "; "
		}
		errMsg += name + ": " + dlErr.Error()
	}
	if handle == 0 {
		return nil, fmt.Errorf("%w: %s", ErrGLUnavailable, errMsg)
	}
	initialized := false
	defer func() {
		if !initialized {
			_ = purego.Dlclose(handle)
		}
	}()
	b := &glBackend{}
	sym, err := purego.Dlsym(handle, "glGetString")
	if err != nil {
		return nil, fmt.Errorf("%w: missing glGetString: %w", ErrGLUnavailable, err)
	}
	purego.RegisterFunc(&b.glGetString, sym)
	initialized = true
	return b, nil
}

// currentGLInfo returns OpenGL context information. A valid GL context must
// be current on the calling thread.
func currentGLInfo() (glInfo, error) {
	b, err := defaultGLBackend()
	if err != nil {
		return glInfo{}, err
	}
	version := b.glGetString(glVersion)
	vendor := b.glGetString(glVendor)
	renderer := b.glGetString(glRenderer)
	if version == nil || vendor == nil || renderer == nil {
		return glInfo{}, fmt.Errorf("%w: glGetString requires a current GL context", ErrGLUnavailable)
	}
	return glInfo{
		Version:  cutil.CString(version),
		Vendor:   cutil.CString(vendor),
		Renderer: cutil.CString(renderer),
	}, nil
}
