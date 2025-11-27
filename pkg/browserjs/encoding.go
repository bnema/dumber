package browserjs

import (
	"encoding/base64"
	"fmt"
	"unicode/utf8"

	"github.com/grafana/sobek"
)

// decodeRuneInString wraps utf8.DecodeRuneInString for cleaner code
func decodeRuneInString(s string) (rune, int) {
	return utf8.DecodeRuneInString(s)
}

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

			if len(call.Arguments) < 2 {
				_ = result.Set("read", 0)
				_ = result.Set("written", 0)
				return result
			}

			str := call.Arguments[0].String()
			destObj := call.Arguments[1].ToObject(vm)

			// Get the length of the destination buffer
			lengthVal := destObj.Get("length")
			if lengthVal == nil || sobek.IsUndefined(lengthVal) {
				_ = result.Set("read", 0)
				_ = result.Set("written", 0)
				return result
			}
			destLen := int(lengthVal.ToInteger())

			// Convert string to UTF-8 bytes
			strBytes := []byte(str)

			// Calculate how many bytes we can write
			bytesToWrite := len(strBytes)
			if bytesToWrite > destLen {
				bytesToWrite = destLen
			}

			// Find the last complete UTF-8 character boundary
			// to avoid writing partial multi-byte characters
			if bytesToWrite > 0 && bytesToWrite < len(strBytes) {
				for bytesToWrite > 0 {
					// Check if this is a valid UTF-8 continuation boundary
					b := strBytes[bytesToWrite]
					// UTF-8 continuation bytes start with 10xxxxxx
					if (b & 0xC0) != 0x80 {
						break
					}
					bytesToWrite--
				}
			}

			// Write bytes to destination array
			for i := 0; i < bytesToWrite; i++ {
				destObj.Set(fmt.Sprintf("%d", i), int(strBytes[i]))
			}

			// Count UTF-16 code units read
			// In Go, string length is bytes but we need to count runes for UTF-16 compatibility
			readBytes := strBytes[:bytesToWrite]
			runeCount := 0
			for len(readBytes) > 0 {
				r, size := decodeRuneInString(string(readBytes))
				if r > 0xFFFF {
					// Surrogate pair in UTF-16
					runeCount += 2
				} else {
					runeCount++
				}
				readBytes = readBytes[size:]
			}

			_ = result.Set("read", runeCount)
			_ = result.Set("written", bytesToWrite)
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
