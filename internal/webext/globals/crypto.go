package globals

import (
	"crypto/rand"
	"fmt"

	"github.com/bnema/dumber/pkg/webcrypto"
	"github.com/grafana/sobek"
)

// installCrypto adds crypto.getRandomValues, crypto.randomUUID, and crypto.subtle.
func (bg *BrowserGlobals) installCrypto() error {
	vm := bg.vm
	subtle := webcrypto.NewSubtleCrypto()

	crypto := vm.NewObject()

	// crypto.getRandomValues
	_ = crypto.Set("getRandomValues", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 1 {
			return sobek.Undefined()
		}
		arr := call.Arguments[0].Export()
		switch v := arr.(type) {
		case []byte:
			_, _ = rand.Read(v)
		case []int8:
			buf := make([]byte, len(v))
			_, _ = rand.Read(buf)
			for i, b := range buf {
				v[i] = int8(b)
			}
		case []uint16:
			buf := make([]byte, len(v)*2)
			_, _ = rand.Read(buf)
			for i := range v {
				v[i] = uint16(buf[i*2]) | uint16(buf[i*2+1])<<8
			}
		case []int16:
			buf := make([]byte, len(v)*2)
			_, _ = rand.Read(buf)
			for i := range v {
				v[i] = int16(uint16(buf[i*2]) | uint16(buf[i*2+1])<<8)
			}
		case []uint32:
			buf := make([]byte, len(v)*4)
			_, _ = rand.Read(buf)
			for i := range v {
				v[i] = uint32(buf[i*4]) | uint32(buf[i*4+1])<<8 |
					uint32(buf[i*4+2])<<16 | uint32(buf[i*4+3])<<24
			}
		case []int32:
			buf := make([]byte, len(v)*4)
			_, _ = rand.Read(buf)
			for i := range v {
				v[i] = int32(uint32(buf[i*4]) | uint32(buf[i*4+1])<<8 |
					uint32(buf[i*4+2])<<16 | uint32(buf[i*4+3])<<24)
			}
		}
		return call.Arguments[0]
	})

	// crypto.randomUUID
	_ = crypto.Set("randomUUID", func(sobek.FunctionCall) sobek.Value {
		uuid := make([]byte, 16)
		_, _ = rand.Read(uuid)
		uuid[6] = (uuid[6] & 0x0f) | 0x40 // Version 4
		uuid[8] = (uuid[8] & 0x3f) | 0x80 // Variant
		return vm.ToValue(fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
			uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16]))
	})

	// crypto.subtle
	subtleObj := vm.NewObject()

	// Helper to create a resolved promise
	resolveWith := func(val any) sobek.Value {
		promise, resolve, _ := vm.NewPromise()
		go func() {
			resolve(vm.ToValue(val))
		}()
		return vm.ToValue(promise)
	}

	// Helper to create a rejected promise
	rejectWith := func(err error) sobek.Value {
		promise, _, reject := vm.NewPromise()
		go func() {
			reject(vm.ToValue(err.Error()))
		}()
		return vm.ToValue(promise)
	}

	// Helper to convert JS algorithm to Go params
	toAlgorithm := func(v sobek.Value) any {
		if v == nil || sobek.IsUndefined(v) || sobek.IsNull(v) {
			return nil
		}
		return v.Export()
	}

	// Helper to get bytes from JS value
	toBytes := func(v sobek.Value) []byte {
		if v == nil {
			return nil
		}
		switch d := v.Export().(type) {
		case []byte:
			return d
		case sobek.ArrayBuffer:
			return d.Bytes()
		case string:
			return []byte(d)
		default:
			return nil
		}
	}

	// Helper to convert CryptoKey to JS object
	keyToJS := func(key *webcrypto.CryptoKey) *sobek.Object {
		obj := vm.NewObject()
		_ = obj.Set("type", string(key.Type))
		_ = obj.Set("extractable", key.Extractable)
		_ = obj.Set("algorithm", map[string]any{"name": key.Algorithm.Name})
		usages := make([]string, len(key.Usages))
		for i, u := range key.Usages {
			usages[i] = string(u)
		}
		_ = obj.Set("usages", usages)
		// Store the actual key in a hidden property
		_ = obj.Set("_handle", key)
		return obj
	}

	// Helper to get CryptoKey from JS object
	keyFromJS := func(v sobek.Value) *webcrypto.CryptoKey {
		if v == nil {
			return nil
		}
		obj := v.ToObject(vm)
		handle := obj.Get("_handle")
		if handle == nil {
			return nil
		}
		if key, ok := handle.Export().(*webcrypto.CryptoKey); ok {
			return key
		}
		return nil
	}

	// Helper to convert usages
	toUsages := func(v sobek.Value) []webcrypto.KeyUsage {
		if v == nil || sobek.IsUndefined(v) {
			return nil
		}
		arr := v.Export()
		switch a := arr.(type) {
		case []any:
			usages := make([]webcrypto.KeyUsage, len(a))
			for i, u := range a {
				usages[i] = webcrypto.KeyUsage(fmt.Sprint(u))
			}
			return usages
		case []string:
			usages := make([]webcrypto.KeyUsage, len(a))
			for i, u := range a {
				usages[i] = webcrypto.KeyUsage(u)
			}
			return usages
		}
		return nil
	}

	// subtle.digest
	_ = subtleObj.Set("digest", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 2 {
			return rejectWith(fmt.Errorf("digest requires 2 arguments"))
		}
		alg := toAlgorithm(call.Arguments[0])
		data := toBytes(call.Arguments[1])
		if data == nil {
			return rejectWith(fmt.Errorf("invalid data"))
		}

		result, err := subtle.Digest(alg, data)
		if err != nil {
			return rejectWith(err)
		}
		return resolveWith(vm.NewArrayBuffer(result))
	})

	// subtle.encrypt
	_ = subtleObj.Set("encrypt", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 3 {
			return rejectWith(fmt.Errorf("encrypt requires 3 arguments"))
		}
		alg := toAlgorithm(call.Arguments[0])
		key := keyFromJS(call.Arguments[1])
		data := toBytes(call.Arguments[2])

		if key == nil {
			return rejectWith(fmt.Errorf("invalid key"))
		}

		result, err := subtle.Encrypt(alg, key, data)
		if err != nil {
			return rejectWith(err)
		}
		return resolveWith(vm.NewArrayBuffer(result))
	})

	// subtle.decrypt
	_ = subtleObj.Set("decrypt", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 3 {
			return rejectWith(fmt.Errorf("decrypt requires 3 arguments"))
		}
		alg := toAlgorithm(call.Arguments[0])
		key := keyFromJS(call.Arguments[1])
		data := toBytes(call.Arguments[2])

		if key == nil {
			return rejectWith(fmt.Errorf("invalid key"))
		}

		result, err := subtle.Decrypt(alg, key, data)
		if err != nil {
			return rejectWith(err)
		}
		return resolveWith(vm.NewArrayBuffer(result))
	})

	// subtle.sign
	_ = subtleObj.Set("sign", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 3 {
			return rejectWith(fmt.Errorf("sign requires 3 arguments"))
		}
		alg := toAlgorithm(call.Arguments[0])
		key := keyFromJS(call.Arguments[1])
		data := toBytes(call.Arguments[2])

		if key == nil {
			return rejectWith(fmt.Errorf("invalid key"))
		}

		result, err := subtle.Sign(alg, key, data)
		if err != nil {
			return rejectWith(err)
		}
		return resolveWith(vm.NewArrayBuffer(result))
	})

	// subtle.verify
	_ = subtleObj.Set("verify", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 4 {
			return rejectWith(fmt.Errorf("verify requires 4 arguments"))
		}
		alg := toAlgorithm(call.Arguments[0])
		key := keyFromJS(call.Arguments[1])
		signature := toBytes(call.Arguments[2])
		data := toBytes(call.Arguments[3])

		if key == nil {
			return rejectWith(fmt.Errorf("invalid key"))
		}

		result, err := subtle.Verify(alg, key, signature, data)
		if err != nil {
			return rejectWith(err)
		}
		return resolveWith(result)
	})

	// subtle.generateKey
	_ = subtleObj.Set("generateKey", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 3 {
			return rejectWith(fmt.Errorf("generateKey requires 3 arguments"))
		}
		alg := toAlgorithm(call.Arguments[0])
		extractable := call.Arguments[1].ToBoolean()
		usages := toUsages(call.Arguments[2])

		result, err := subtle.GenerateKey(alg, extractable, usages)
		if err != nil {
			return rejectWith(err)
		}

		switch r := result.(type) {
		case *webcrypto.CryptoKey:
			return resolveWith(keyToJS(r))
		case *webcrypto.CryptoKeyPair:
			pair := vm.NewObject()
			_ = pair.Set("publicKey", keyToJS(r.PublicKey))
			_ = pair.Set("privateKey", keyToJS(r.PrivateKey))
			return resolveWith(pair)
		default:
			return rejectWith(fmt.Errorf("unexpected result type"))
		}
	})

	// subtle.importKey
	_ = subtleObj.Set("importKey", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 5 {
			return rejectWith(fmt.Errorf("importKey requires 5 arguments"))
		}
		format := call.Arguments[0].String()

		var keyData any
		switch format {
		case "raw":
			keyData = toBytes(call.Arguments[1])
		case "jwk":
			keyData = call.Arguments[1].Export()
		default:
			keyData = toBytes(call.Arguments[1])
		}

		alg := toAlgorithm(call.Arguments[2])
		extractable := call.Arguments[3].ToBoolean()
		usages := toUsages(call.Arguments[4])

		result, err := subtle.ImportKey(format, keyData, alg, extractable, usages)
		if err != nil {
			return rejectWith(err)
		}
		return resolveWith(keyToJS(result))
	})

	// subtle.exportKey
	_ = subtleObj.Set("exportKey", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 2 {
			return rejectWith(fmt.Errorf("exportKey requires 2 arguments"))
		}
		format := call.Arguments[0].String()
		key := keyFromJS(call.Arguments[1])

		if key == nil {
			return rejectWith(fmt.Errorf("invalid key"))
		}

		result, err := subtle.ExportKey(format, key)
		if err != nil {
			return rejectWith(err)
		}

		// Convert result based on format
		switch format {
		case "raw", "spki", "pkcs8":
			if data, ok := result.([]byte); ok {
				return resolveWith(vm.NewArrayBuffer(data))
			}
		case "jwk":
			return resolveWith(result)
		}
		return resolveWith(result)
	})

	// subtle.deriveBits
	_ = subtleObj.Set("deriveBits", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 3 {
			return rejectWith(fmt.Errorf("deriveBits requires 3 arguments"))
		}
		alg := toAlgorithm(call.Arguments[0])
		baseKey := keyFromJS(call.Arguments[1])
		length := int(call.Arguments[2].ToInteger())

		if baseKey == nil {
			return rejectWith(fmt.Errorf("invalid key"))
		}

		result, err := subtle.DeriveBits(alg, baseKey, length)
		if err != nil {
			return rejectWith(err)
		}
		return resolveWith(vm.NewArrayBuffer(result))
	})

	// subtle.deriveKey
	_ = subtleObj.Set("deriveKey", func(call sobek.FunctionCall) sobek.Value {
		if len(call.Arguments) < 5 {
			return rejectWith(fmt.Errorf("deriveKey requires 5 arguments"))
		}
		alg := toAlgorithm(call.Arguments[0])
		baseKey := keyFromJS(call.Arguments[1])
		derivedKeyType := toAlgorithm(call.Arguments[2])
		extractable := call.Arguments[3].ToBoolean()
		usages := toUsages(call.Arguments[4])

		if baseKey == nil {
			return rejectWith(fmt.Errorf("invalid key"))
		}

		result, err := subtle.DeriveKey(alg, baseKey, derivedKeyType, extractable, usages)
		if err != nil {
			return rejectWith(err)
		}
		return resolveWith(keyToJS(result))
	})

	// subtle.wrapKey (stub)
	_ = subtleObj.Set("wrapKey", func(call sobek.FunctionCall) sobek.Value {
		return rejectWith(fmt.Errorf("wrapKey not implemented"))
	})

	// subtle.unwrapKey (stub)
	_ = subtleObj.Set("unwrapKey", func(call sobek.FunctionCall) sobek.Value {
		return rejectWith(fmt.Errorf("unwrapKey not implemented"))
	})

	_ = crypto.Set("subtle", subtleObj)
	vm.Set("crypto", crypto)
	return nil
}
