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

// performanceEntry represents a performance entry (mark or measure)
type performanceEntry struct {
	name       string
	entryType  string
	startTime  float64
	duration   float64
}

// Install adds the performance object for timing.
func (pm *PerformanceManager) Install() error {
	vm := pm.vm

	performance := vm.NewObject()

	// Storage for entries
	var entries []performanceEntry
	marks := make(map[string]float64)
	measures := make(map[string]performanceEntry)

	// Helper to create entry object
	createEntryObject := func(e performanceEntry) sobek.Value {
		obj := vm.NewObject()
		_ = obj.Set("name", e.name)
		_ = obj.Set("entryType", e.entryType)
		_ = obj.Set("startTime", e.startTime)
		_ = obj.Set("duration", e.duration)
		return obj
	}

	// performance.now() - high resolution timestamp
	_ = performance.Set("now", func(call sobek.FunctionCall) sobek.Value {
		elapsed := time.Since(pm.startTime)
		return vm.ToValue(float64(elapsed.Nanoseconds()) / 1e6) // milliseconds with microsecond precision
	})

	// performance.timeOrigin - when the context started
	_ = performance.Set("timeOrigin", float64(pm.startTime.UnixMilli()))

	// performance.mark() - create a named timestamp
	_ = performance.Set("mark", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Undefined()
		}
		name := call.Arguments[0].String()
		startTime := float64(time.Since(pm.startTime).Nanoseconds()) / 1e6
		marks[name] = startTime

		// Create and store the entry
		entry := performanceEntry{
			name:      name,
			entryType: "mark",
			startTime: startTime,
			duration:  0,
		}
		entries = append(entries, entry)

		return createEntryObject(entry)
	})

	// performance.measure() - measure between two marks
	_ = performance.Set("measure", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Undefined()
		}
		name := call.Arguments[0].String()
		var startTime, endTime float64

		// Handle options object or string arguments
		if len(call.Arguments) > 1 {
			arg1 := call.Arguments[1].Export()
			switch v := arg1.(type) {
			case string:
				// Old API: measure(name, startMark, endMark)
				if t, ok := marks[v]; ok {
					startTime = t
				}
			case map[string]interface{}:
				// New API: measure(name, options)
				if start, ok := v["start"].(string); ok {
					if t, exists := marks[start]; exists {
						startTime = t
					}
				} else if startNum, ok := v["start"].(float64); ok {
					startTime = startNum
				}
				if end, ok := v["end"].(string); ok {
					if t, exists := marks[end]; exists {
						endTime = t
					}
				} else if endNum, ok := v["end"].(float64); ok {
					endTime = endNum
				}
				if dur, ok := v["duration"].(float64); ok {
					endTime = startTime + dur
				}
			}
		}

		if len(call.Arguments) > 2 {
			endName := call.Arguments[2].String()
			if t, ok := marks[endName]; ok {
				endTime = t
			}
		}

		// If no end time specified, use current time
		if endTime == 0 {
			endTime = float64(time.Since(pm.startTime).Nanoseconds()) / 1e6
		}

		entry := performanceEntry{
			name:      name,
			entryType: "measure",
			startTime: startTime,
			duration:  endTime - startTime,
		}
		entries = append(entries, entry)
		measures[name] = entry

		return createEntryObject(entry)
	})

	// performance.clearMarks()
	_ = performance.Set("clearMarks", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			name := call.Arguments[0].String()
			delete(marks, name)
			// Remove from entries
			filtered := entries[:0]
			for _, e := range entries {
				if !(e.entryType == "mark" && e.name == name) {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		} else {
			marks = make(map[string]float64)
			// Remove all marks from entries
			filtered := entries[:0]
			for _, e := range entries {
				if e.entryType != "mark" {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}
		return sobek.Undefined()
	})

	// performance.clearMeasures()
	_ = performance.Set("clearMeasures", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			name := call.Arguments[0].String()
			delete(measures, name)
			// Remove from entries
			filtered := entries[:0]
			for _, e := range entries {
				if !(e.entryType == "measure" && e.name == name) {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		} else {
			measures = make(map[string]performanceEntry)
			// Remove all measures from entries
			filtered := entries[:0]
			for _, e := range entries {
				if e.entryType != "measure" {
					filtered = append(filtered, e)
				}
			}
			entries = filtered
		}
		return sobek.Undefined()
	})

	// performance.clearResourceTimings()
	_ = performance.Set("clearResourceTimings", func(call sobek.FunctionCall) sobek.Value {
		// Remove resource entries (we don't track these, but provide the method)
		return sobek.Undefined()
	})

	// performance.getEntries() - returns all entries
	_ = performance.Set("getEntries", func(call sobek.FunctionCall) sobek.Value {
		result := make([]interface{}, len(entries))
		for i, e := range entries {
			result[i] = createEntryObject(e)
		}
		return vm.ToValue(result)
	})

	// performance.getEntriesByName() - returns entries with matching name
	_ = performance.Set("getEntriesByName", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue([]interface{}{})
		}
		name := call.Arguments[0].String()
		var entryType string
		if len(call.Arguments) > 1 {
			entryType = call.Arguments[1].String()
		}

		var result []interface{}
		for _, e := range entries {
			if e.name == name {
				if entryType == "" || e.entryType == entryType {
					result = append(result, createEntryObject(e))
				}
			}
		}
		if result == nil {
			result = []interface{}{}
		}
		return vm.ToValue(result)
	})

	// performance.getEntriesByType() - returns entries with matching type
	_ = performance.Set("getEntriesByType", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue([]interface{}{})
		}
		entryType := call.Arguments[0].String()

		var result []interface{}
		for _, e := range entries {
			if e.entryType == entryType {
				result = append(result, createEntryObject(e))
			}
		}
		if result == nil {
			result = []interface{}{}
		}
		return vm.ToValue(result)
	})

	// performance.toJSON()
	_ = performance.Set("toJSON", func(call sobek.FunctionCall) sobek.Value {
		result := vm.NewObject()
		_ = result.Set("timeOrigin", float64(pm.startTime.UnixMilli()))
		return result
	})

	vm.Set("performance", performance)
	return nil
}
