package cef

import (
	"github.com/bnema/purego-cef/internal/capi"
)

// V8ValueCreateDate creates a new V8Value of type Date. |date| is the number of
// microseconds since the Windows epoch (1601-01-01 00:00:00 UTC), matching the
// value returned by V8Value.GetDateValue. This function should only be called
// from within the scope of a render-process-handler, V8 handler or V8 accessor
// callback, or in combination with calling Enter() and Exit() on a stored
// V8Context reference.
//
// Hand-written because cef_v8_value_create_date takes cef_basetime_t by value
// (see skipPublicTypes in cmd/cefgen/internal/emitter/builder.go).
func V8ValueCreateDate(date int64) V8Value {
	ret := capi.CEFV8ValueCreateDate(capi.CEFBasetimeT{Val: date})
	return takeV8Value(ret)
}
