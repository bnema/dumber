package browserjs

import (
	"fmt"
	"strings"
	"time"

	"github.com/grafana/sobek"
)

// ConsoleManager provides the console object for logging.
type ConsoleManager struct {
	vm        *sobek.Runtime
	logger    Logger
	startTime time.Time
}

// NewConsoleManager creates a new console manager.
func NewConsoleManager(vm *sobek.Runtime, logger Logger, startTime time.Time) *ConsoleManager {
	if startTime.IsZero() {
		startTime = time.Now()
	}
	return &ConsoleManager{
		vm:        vm,
		logger:    logger,
		startTime: startTime,
	}
}

// Install adds the console object for logging.
func (cm *ConsoleManager) Install() error {
	vm := cm.vm

	console := vm.NewObject()

	formatArgs := func(args []sobek.Value) string {
		parts := make([]string, len(args))
		for i, arg := range args {
			parts[i] = fmt.Sprintf("%v", arg.Export())
		}
		return strings.Join(parts, " ")
	}

	log := func(level string, args ...any) {
		if cm.logger != nil {
			cm.logger.Log(level, args...)
		}
	}

	_ = console.Set("log", func(call sobek.FunctionCall) sobek.Value {
		log("log", formatArgs(call.Arguments))
		return sobek.Undefined()
	})

	_ = console.Set("info", func(call sobek.FunctionCall) sobek.Value {
		log("info", formatArgs(call.Arguments))
		return sobek.Undefined()
	})

	_ = console.Set("warn", func(call sobek.FunctionCall) sobek.Value {
		log("warn", formatArgs(call.Arguments))
		return sobek.Undefined()
	})

	_ = console.Set("error", func(call sobek.FunctionCall) sobek.Value {
		log("error", formatArgs(call.Arguments))
		return sobek.Undefined()
	})

	_ = console.Set("debug", func(call sobek.FunctionCall) sobek.Value {
		log("debug", formatArgs(call.Arguments))
		return sobek.Undefined()
	})

	_ = console.Set("trace", func(call sobek.FunctionCall) sobek.Value {
		log("trace", formatArgs(call.Arguments))
		return sobek.Undefined()
	})

	_ = console.Set("assert", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 && !call.Arguments[0].ToBoolean() {
			msg := "Assertion failed"
			if len(call.Arguments) > 1 {
				msg = formatArgs(call.Arguments[1:])
			}
			log("assert", msg)
		}
		return sobek.Undefined()
	})

	_ = console.Set("dir", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			log("dir", fmt.Sprintf("%+v", call.Arguments[0].Export()))
		}
		return sobek.Undefined()
	})

	_ = console.Set("table", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			log("table", fmt.Sprintf("%+v", call.Arguments[0].Export()))
		}
		return sobek.Undefined()
	})

	// Grouping (simplified - just indentation in logs)
	_ = console.Set("group", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			log("group", formatArgs(call.Arguments))
		}
		return sobek.Undefined()
	})

	_ = console.Set("groupCollapsed", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) > 0 {
			log("group", formatArgs(call.Arguments))
		}
		return sobek.Undefined()
	})

	_ = console.Set("groupEnd", func(call sobek.FunctionCall) sobek.Value {
		return sobek.Undefined()
	})

	// Timing
	timers := make(map[string]time.Time)

	_ = console.Set("time", func(call sobek.FunctionCall) sobek.Value {
		label := "default"
		if len(call.Arguments) > 0 {
			label = call.Arguments[0].String()
		}
		timers[label] = time.Now()
		return sobek.Undefined()
	})

	_ = console.Set("timeEnd", func(call sobek.FunctionCall) sobek.Value {
		label := "default"
		if len(call.Arguments) > 0 {
			label = call.Arguments[0].String()
		}
		if start, ok := timers[label]; ok {
			elapsed := time.Since(start).Milliseconds()
			log("time", fmt.Sprintf("%s: %dms", label, elapsed))
			delete(timers, label)
		}
		return sobek.Undefined()
	})

	_ = console.Set("timeLog", func(call sobek.FunctionCall) sobek.Value {
		label := "default"
		if len(call.Arguments) > 0 {
			label = call.Arguments[0].String()
		}
		if start, ok := timers[label]; ok {
			elapsed := time.Since(start).Milliseconds()
			log("time", fmt.Sprintf("%s: %dms", label, elapsed))
		}
		return sobek.Undefined()
	})

	// Counting
	counters := make(map[string]int)

	_ = console.Set("count", func(call sobek.FunctionCall) sobek.Value {
		label := "default"
		if len(call.Arguments) > 0 {
			label = call.Arguments[0].String()
		}
		counters[label]++
		log("count", fmt.Sprintf("%s: %d", label, counters[label]))
		return sobek.Undefined()
	})

	_ = console.Set("countReset", func(call sobek.FunctionCall) sobek.Value {
		label := "default"
		if len(call.Arguments) > 0 {
			label = call.Arguments[0].String()
		}
		counters[label] = 0
		return sobek.Undefined()
	})

	_ = console.Set("clear", func(call sobek.FunctionCall) sobek.Value {
		return sobek.Undefined()
	})

	vm.Set("console", console)
	return nil
}
