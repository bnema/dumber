package egl

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/bnema/purego"
	"github.com/bnema/purego-cef2gtk/internal/cutil"
)

var ErrUnavailable = errors.New("egl unavailable")

type backend interface {
	currentDisplay() Display
	queryString(Display, int32) string
}

type dynamicBackend struct {
	// The libEGL handle is intentionally kept open for the process lifetime
	// because registered purego function pointers remain backed by it.
	handle uintptr

	eglGetCurrentDisplay func() uintptr
	eglQueryString       func(uintptr, int32) unsafe.Pointer
	eglGetError          func() uint32
	eglCreateImageKHR    func(display uintptr, context uintptr, target uint32, buffer uintptr, attrs *Attribute) uintptr
	eglDestroyImageKHR   func(display uintptr, image uintptr) uint32
}

var (
	loadOnce sync.Once
	loaded   backend
	loadErr  error
)

func defaultBackend() (backend, error) {
	loadOnce.Do(func() {
		loaded, loadErr = loadDynamicBackend()
	})
	return loaded, loadErr
}

func loadDynamicBackend() (ret backend, retErr error) {
	var handle uintptr
	errMsg := ""
	for _, name := range []string{"libEGL.so.1", "libEGL.so"} {
		var err error
		handle, err = purego.Dlopen(name, purego.RTLD_LAZY|purego.RTLD_LOCAL)
		if err == nil {
			break
		}
		if errMsg != "" {
			errMsg += "; "
		}
		errMsg += name + ": " + err.Error()
	}
	if handle == 0 {
		return nil, fmt.Errorf("%w: %s", ErrUnavailable, errMsg)
	}
	defer func() {
		if retErr != nil {
			_ = purego.Dlclose(handle)
		}
	}()

	b := &dynamicBackend{handle: handle}
	if err := registerLibFunc(&b.eglGetCurrentDisplay, handle, "eglGetCurrentDisplay"); err != nil {
		return nil, err
	}
	if err := registerLibFunc(&b.eglQueryString, handle, "eglQueryString"); err != nil {
		return nil, err
	}
	if err := registerLibFunc(&b.eglGetError, handle, "eglGetError"); err != nil {
		return nil, err
	}
	if err := registerEGLProc(&b.eglCreateImageKHR, handle, "eglCreateImageKHR"); err != nil {
		return nil, err
	}
	if err := registerEGLProc(&b.eglDestroyImageKHR, handle, "eglDestroyImageKHR"); err != nil {
		return nil, err
	}
	return b, nil
}

func registerLibFunc(fptr any, handle uintptr, name string) error {
	sym, err := purego.Dlsym(handle, name)
	if err != nil {
		return fmt.Errorf("%w: missing %s: %w", ErrUnavailable, name, err)
	}
	purego.RegisterFunc(fptr, sym)
	return nil
}

func (b *dynamicBackend) currentDisplay() Display {
	return Display(b.eglGetCurrentDisplay())
}

func (b *dynamicBackend) queryString(d Display, name int32) string {
	ptr := b.eglQueryString(uintptr(d), name)
	s := cutil.CString(ptr)
	return string([]byte(s))
}
