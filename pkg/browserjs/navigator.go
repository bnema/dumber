package browserjs

import "github.com/grafana/sobek"

// NavigatorManager provides the navigator object.
type NavigatorManager struct {
	vm    *sobek.Runtime
	tasks chan func()
}

// NewNavigatorManager creates a new navigator manager.
func NewNavigatorManager(vm *sobek.Runtime, tasks chan func()) *NavigatorManager {
	return &NavigatorManager{vm: vm, tasks: tasks}
}

// Install adds the navigator object.
func (nm *NavigatorManager) Install() error {
	vm := nm.vm

	navigator := vm.NewObject()
	_ = navigator.Set("userAgent", "Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0")
	_ = navigator.Set("language", "en-US")
	_ = navigator.Set("languages", []string{"en-US", "en"})
	_ = navigator.Set("platform", "Linux x86_64")
	_ = navigator.Set("vendor", "")
	_ = navigator.Set("onLine", true)
	_ = navigator.Set("cookieEnabled", true)
	_ = navigator.Set("doNotTrack", "1")
	_ = navigator.Set("hardwareConcurrency", 4)
	_ = navigator.Set("maxTouchPoints", 0)
	_ = navigator.Set("webdriver", false)

	// navigator.clipboard (minimal)
	clipboard := vm.NewObject()
	_ = clipboard.Set("writeText", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			if nm.tasks != nil {
				nm.tasks <- func() { _ = resolve(sobek.Undefined()) }
			} else {
				_ = resolve(sobek.Undefined())
			}
		}()
		return vm.ToValue(promise)
	})
	_ = clipboard.Set("readText", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			if nm.tasks != nil {
				nm.tasks <- func() { _ = resolve(vm.ToValue("")) }
			} else {
				_ = resolve(vm.ToValue(""))
			}
		}()
		return vm.ToValue(promise)
	})
	_ = navigator.Set("clipboard", clipboard)

	// navigator.permissions (minimal)
	permissions := vm.NewObject()
	_ = permissions.Set("query", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			run := func() {
				result := vm.NewObject()
				_ = result.Set("state", "granted")
				_ = resolve(result)
			}
			if nm.tasks != nil {
				nm.tasks <- run
			} else {
				run()
			}
		}()
		return vm.ToValue(promise)
	})
	_ = navigator.Set("permissions", permissions)

	// navigator.storage (minimal)
	storage := vm.NewObject()
	_ = storage.Set("estimate", func(call sobek.FunctionCall) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			run := func() {
				result := vm.NewObject()
				_ = result.Set("quota", 1073741824) // 1GB
				_ = result.Set("usage", 0)
				_ = resolve(result)
			}
			if nm.tasks != nil {
				nm.tasks <- run
			} else {
				run()
			}
		}()
		return vm.ToValue(promise)
	})
	_ = navigator.Set("storage", storage)

	vm.Set("navigator", navigator)
	return nil
}
