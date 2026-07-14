// bridge.go implements portout.CAPI using purego bindings.
// This is one of the handwritten files in the capi package (see also register.go and doc.go).
package capi

import (
	"unsafe"

	"github.com/bnema/purego"
	portout "github.com/bnema/purego-cef/internal/ports/out"
)

// Bridge implements portout.CAPI using purego bindings.
type Bridge struct {
	handle uintptr

	// Handwritten string and string-list functions bound from the CEF shared
	// library. These helpers come from CEF's low-level string headers, not the
	// capi/type header sets parsed by cefgen.
	stringSet        func(*uint16, uintptr, unsafe.Pointer, int32) int32
	stringClear      func(unsafe.Pointer)
	stringFreeFn     func(unsafe.Pointer)
	stringListAlloc  func() uintptr
	stringListAppend func(uintptr, unsafe.Pointer)
	stringListFree   func(uintptr)
	stringListSz     func(uintptr) uintptr
	stringListVal    func(uintptr, uintptr, unsafe.Pointer) int32

	// Core functions bound from CEF shared library (matches out.AppFunctions).
	initialize              func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer, unsafe.Pointer) int32
	shutdown                func()
	doMsgLoopWork           func()
	executeProcess          func(unsafe.Pointer, unsafe.Pointer, unsafe.Pointer) int32
	getExitCode             func() int32
	runMessageLoop          func()
	quitMessageLoop         func()
	setNestableTasksAllowed func(int32)
}

// NewBridge creates a CAPI Bridge, registering all symbols from the handle.
func NewBridge(handle uintptr) *Bridge {
	b := &Bridge{handle: handle}
	Register(handle)
	b.bindStringFuncs(handle)
	b.bindCoreFuncs(handle)
	return b
}

// bindStringFuncs uses purego.RegisterLibFunc (which panics on missing symbols)
// intentionally — these low-level string and string-list functions are
// mandatory in every CEF build.
func (b *Bridge) bindStringFuncs(handle uintptr) {
	purego.RegisterLibFunc(&b.stringSet, handle, "cef_string_utf16_set")
	purego.RegisterLibFunc(&b.stringClear, handle, "cef_string_utf16_clear")
	purego.RegisterLibFunc(&b.stringFreeFn, handle, "cef_string_userfree_utf16_free")
	purego.RegisterLibFunc(&b.stringListAlloc, handle, "cef_string_list_alloc")
	purego.RegisterLibFunc(&b.stringListAppend, handle, "cef_string_list_append")
	purego.RegisterLibFunc(&b.stringListFree, handle, "cef_string_list_free")
	purego.RegisterLibFunc(&b.stringListSz, handle, "cef_string_list_size")
	purego.RegisterLibFunc(&b.stringListVal, handle, "cef_string_list_value")
}

// bindCoreFuncs uses purego.RegisterLibFunc (which panics on missing symbols)
// intentionally — these core lifecycle functions are mandatory in every CEF build.
func (b *Bridge) bindCoreFuncs(handle uintptr) {
	purego.RegisterLibFunc(&b.initialize, handle, "cef_initialize")
	purego.RegisterLibFunc(&b.shutdown, handle, "cef_shutdown")
	purego.RegisterLibFunc(&b.doMsgLoopWork, handle, "cef_do_message_loop_work")
	purego.RegisterLibFunc(&b.executeProcess, handle, "cef_execute_process")
	purego.RegisterLibFunc(&b.getExitCode, handle, "cef_get_exit_code")
	purego.RegisterLibFunc(&b.runMessageLoop, handle, "cef_run_message_loop")
	purego.RegisterLibFunc(&b.quitMessageLoop, handle, "cef_quit_message_loop")
	purego.RegisterLibFunc(&b.setNestableTasksAllowed, handle, "cef_set_nestable_tasks_allowed")
}

func (b *Bridge) Initialize(args, settings, app, sandboxInfo unsafe.Pointer) int32 {
	return b.initialize(args, settings, app, sandboxInfo)
}

func (b *Bridge) Shutdown() {
	b.shutdown()
}

func (b *Bridge) DoMessageLoopWork() {
	b.doMsgLoopWork()
}

func (b *Bridge) ExecuteProcess(args, app, sandboxInfo unsafe.Pointer) int32 {
	return b.executeProcess(args, app, sandboxInfo)
}

func (b *Bridge) GetExitCode() int32 {
	return b.getExitCode()
}

func (b *Bridge) RunMessageLoop() {
	b.runMessageLoop()
}

func (b *Bridge) QuitMessageLoop() {
	b.quitMessageLoop()
}

// SetNestableTasksAllowed adapts the generated port signature (unsafe.Pointer)
// to the actual C signature (int). The port-out template uses unsafe.Pointer
// for all free function params — this is a known limitation.
func (b *Bridge) SetNestableTasksAllowed(allowed unsafe.Pointer) {
	b.setNestableTasksAllowed(int32(uintptr(allowed)))
}

func (b *Bridge) NewCallback(fn any) uintptr {
	return purego.NewCallback(fn)
}

func (b *Bridge) UnrefCallback(cb uintptr) error {
	return purego.UnrefCallback(cb)
}

func (b *Bridge) StringSet(src *uint16, srcLen uintptr, output unsafe.Pointer, cp int32) int32 {
	return b.stringSet(src, srcLen, output, cp)
}

func (b *Bridge) StringClear(s unsafe.Pointer) {
	b.stringClear(s)
}

func (b *Bridge) StringUserfreeFree(s unsafe.Pointer) {
	b.stringFreeFn(s)
}

func (b *Bridge) StringListAlloc() uintptr {
	return b.stringListAlloc()
}

func (b *Bridge) StringListAppend(list uintptr, value unsafe.Pointer) {
	b.stringListAppend(list, value)
}

func (b *Bridge) StringListFree(list uintptr) {
	b.stringListFree(list)
}

func (b *Bridge) StringListSize(list uintptr) uintptr {
	return b.stringListSz(list)
}

func (b *Bridge) StringListValue(list uintptr, index uintptr, value unsafe.Pointer) int32 {
	return b.stringListVal(list, index, value)
}

// Ensure Bridge implements portout.CAPI at compile time.
var _ portout.CAPI = (*Bridge)(nil)
