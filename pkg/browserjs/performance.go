package browserjs

import (
	"time"

	"github.com/grafana/sobek"
)

// PerformanceManager provides the performance object for timing.
type PerformanceManager struct {
	vm        *sobek.Runtime
	startTime time.Time
}

// NewPerformanceManager creates a new performance manager.
func NewPerformanceManager(vm *sobek.Runtime, startTime time.Time) *PerformanceManager {
	if startTime.IsZero() {
		startTime = time.Now()
	}
	return &PerformanceManager{vm: vm, startTime: startTime}
}

// Install adds the performance object for timing.
func (pm *PerformanceManager) Install() error {
	vm := pm.vm

	performance := vm.NewObject()

	// performance.now() - high resolution timestamp
	_ = performance.Set("now", func(call sobek.FunctionCall) sobek.Value {
		elapsed := time.Since(pm.startTime)
		return vm.ToValue(float64(elapsed.Nanoseconds()) / 1e6) // milliseconds with microsecond precision
	})

	// performance.timeOrigin - when the context started
	_ = performance.Set("timeOrigin", float64(pm.startTime.UnixMilli()))

	// performance.mark() - create a named timestamp
	marks := make(map[string]float64)
	_ = performance.Set("mark", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Undefined()
		}
		name := call.Arguments[0].String()
		marks[name] = float64(time.Since(pm.startTime).Nanoseconds()) / 1e6
		return sobek.Undefined()
	})

	// performance.measure() - measure between two marks
	_ = performance.Set("measure", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Undefined()
		}
		name := call.Arguments[0].String()
		var startTime, endTime float64

		if len(call.Arguments) > 1 {
			startName := call.Arguments[1].String()
			if t, ok := marks[startName]; ok {
				startTime = t
			}
		}

		if len(call.Arguments) > 2 {
			endName := call.Arguments[2].String()
			if t, ok := marks[endName]; ok {
				endTime = t
			}
		} else {
			endTime = float64(time.Since(pm.startTime).Nanoseconds()) / 1e6
		}

		result := vm.NewObject()
		_ = result.Set("name", name)
		_ = result.Set("entryType", "measure")
		_ = result.Set("startTime", startTime)
		_ = result.Set("duration", endTime-startTime)
		return result
	})

	// performance.clearMarks()
	_ = performance.Set("clearMarks", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			delete(marks, call.Arguments[0].String())
		} else {
			marks = make(map[string]float64)
		}
		return sobek.Undefined()
	})

	// performance.getEntries() - simplified
	_ = performance.Set("getEntries", func(call sobek.FunctionCall) sobek.Value {
		return vm.ToValue([]interface{}{})
	})

	_ = performance.Set("getEntriesByName", func(call sobek.FunctionCall) sobek.Value {
		return vm.ToValue([]interface{}{})
	})

	_ = performance.Set("getEntriesByType", func(call sobek.FunctionCall) sobek.Value {
		return vm.ToValue([]interface{}{})
	})

	vm.Set("performance", performance)
	return nil
}
