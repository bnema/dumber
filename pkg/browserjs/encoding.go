package browserjs

import (
	"encoding/base64"
	"fmt"

	"github.com/grafana/sobek"
)

// EncodingManager provides TextEncoder, TextDecoder, atob, btoa APIs.
type EncodingManager struct {
	vm *sobek.Runtime
}

// NewEncodingManager creates a new encoding manager.
func NewEncodingManager(vm *sobek.Runtime) *EncodingManager {
	return &EncodingManager{vm: vm}
}

// Install registers encoding globals on the VM.
func (em *EncodingManager) Install() error {
	vm := em.vm

	// TextEncoder constructor
	textEncoderCtor := func(call sobek.ConstructorCall) *sobek.Object {
		encoder := call.This
		_ = encoder.Set("encoding", "utf-8")
		_ = encoder.Set("encode", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) < 1 {
				return vm.ToValue([]byte{})
			}
			str := call.Arguments[0].String()
			return vm.ToValue([]byte(str))
		})
		_ = encoder.Set("encodeInto", func(call sobek.FunctionCall) sobek.Value {
			result := vm.NewObject()
			_ = result.Set("read", 0)
			_ = result.Set("written", 0)
			return result
		})
		return nil
	}
	vm.Set("TextEncoder", textEncoderCtor)

	// TextDecoder constructor
	textDecoderCtor := func(call sobek.ConstructorCall) *sobek.Object {
		encoding := "utf-8"
		if len(call.Arguments) > 0 {
			encoding = call.Arguments[0].String()
		}
		decoder := call.This
		_ = decoder.Set("encoding", encoding)
		_ = decoder.Set("fatal", false)
		_ = decoder.Set("ignoreBOM", false)
		_ = decoder.Set("decode", func(call sobek.FunctionCall) sobek.Value {
			if len(call.Arguments) < 1 {
				return vm.ToValue("")
			}
			arg := call.Arguments[0].Export()
			var bytes []byte
			switch v := arg.(type) {
			case []byte:
				bytes = v
			case []interface{}:
				bytes = make([]byte, len(v))
				for i, b := range v {
					if num, ok := b.(int64); ok {
						bytes[i] = byte(num)
					} else if num, ok := b.(float64); ok {
						bytes[i] = byte(num)
					}
				}
			default:
				return vm.ToValue("")
			}
			return vm.ToValue(string(bytes))
		})
		return nil
	}
	vm.Set("TextDecoder", textDecoderCtor)

	// atob - decode base64
	vm.Set("atob", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue("")
		}
		encoded := call.Arguments[0].String()
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			panic(vm.ToValue(fmt.Sprintf("InvalidCharacterError: %v", err)))
		}
		return vm.ToValue(string(decoded))
	})

	// btoa - encode to base64
	vm.Set("btoa", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return vm.ToValue("")
		}
		str := call.Arguments[0].String()
		encoded := base64.StdEncoding.EncodeToString([]byte(str))
		return vm.ToValue(encoded)
	})

	return nil
}
