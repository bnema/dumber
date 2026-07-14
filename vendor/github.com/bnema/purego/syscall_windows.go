// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2022 The Ebitengine Authors

package purego

import (
	"errors"
	"reflect"
	"sync"
	"syscall"
	"unsafe"
)

var syscall15XABI0 uintptr

func syscall_syscall15X(fn, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14, a15 uintptr) (r1, r2, err uintptr) {
	r1, r2, errno := syscall.Syscall15(fn, 15, a1, a2, a3, a4, a5, a6, a7, a8, a9, a10, a11, a12, a13, a14, a15)
	return r1, r2, uintptr(errno)
}

// NewCallback converts a Go function to a function pointer conforming to the stdcall calling convention.
// This is useful when interoperating with Windows code requiring callbacks. The argument is expected to be a
// function with one uintptr-sized result. The function must not have arguments with size larger than the
// size of uintptr. Only a limited number of callbacks may be created in a single Go process, and any memory
// allocated for these callbacks is never released. Between NewCallback and NewCallbackCDecl, at least 1024
// callbacks can always be created. Although this function is similiar to the darwin version it may act
// differently.
func NewCallback(fn any) uintptr {
	isCDecl := false
	ty := reflect.TypeOf(fn)
	for i := 0; i < ty.NumIn(); i++ {
		in := ty.In(i)
		if !in.AssignableTo(reflect.TypeOf(CDecl{})) {
			continue
		}
		if i != 0 {
			panic("purego: CDecl must be the first argument")
		}
		isCDecl = true
	}
	if isCDecl {
		return syscall.NewCallbackCDecl(fn)
	}
	return syscall.NewCallback(fn)
}

// NewCallbackFnPtr converts a Go function pointer to a Windows callback and reuses an existing callback when possible.
func NewCallbackFnPtr(fnPtr any) uintptr {
	val := reflect.ValueOf(fnPtr)
	if val.IsNil() {
		panic("purego: function must not be nil")
	}
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Func {
		panic("purego: the type must be a function pointer but was not")
	}
	if addr, ok := getCallbackByFnPtr(val); ok {
		return addr
	}
	addr := NewCallback(val.Elem().Interface())
	cbs.lock.Lock()
	cbs.knownFnPtr[val.Pointer()] = addr
	cbs.lock.Unlock()
	return addr
}

// UnrefCallback is unsupported on Windows because runtime callback slots cannot be reclaimed.
func UnrefCallback(cb uintptr) error {
	if cb == 0 {
		return errors.New("callback not found")
	}
	return errors.New("purego: unreferencing callbacks is unsupported on windows")
}

// UnrefCallbackFnPtr is unsupported on Windows because runtime callback slots cannot be reclaimed.
func UnrefCallbackFnPtr(fnPtr any) error {
	val := reflect.ValueOf(fnPtr)
	if val.IsNil() {
		panic("purego: function must not be nil")
	}
	if val.Kind() != reflect.Ptr || val.Elem().Kind() != reflect.Func {
		panic("purego: the type must be a function pointer but was not")
	}
	return errors.New("purego: unreferencing callbacks is unsupported on windows")
}

// maxCb is the maximum number of tracked Windows callback function pointers.
const maxCB = 1024

var cbs = struct {
	lock       sync.RWMutex
	knownFnPtr map[uintptr]uintptr
}{
	knownFnPtr: make(map[uintptr]uintptr, maxCB),
}

func getCallbackByFnPtr(val reflect.Value) (uintptr, bool) {
	cbs.lock.RLock()
	defer cbs.lock.RUnlock()
	addr, ok := cbs.knownFnPtr[val.Pointer()]
	return addr, ok
}

func loadSymbol(handle uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(handle), name)
}

// callbackMaxFrame is a stub definition.
const callbackMaxFrame = 0

func callbackArgFromStack(argsBase unsafe.Pointer, stackSlot int, stackByteOffset *uintptr, inType reflect.Type) reflect.Value {
	panic("purego: callbackArgFromStack should not be called on windows")
}
