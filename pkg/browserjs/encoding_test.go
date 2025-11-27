package browserjs

import (
	"testing"

	"github.com/grafana/sobek"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodingManager_TextEncoder(t *testing.T) {
	vm := sobek.New()
	em := NewEncodingManager(vm)
	require.NoError(t, em.Install())

	// Test basic encoding
	val, err := vm.RunString(`
		var encoder = new TextEncoder();
		var result = encoder.encode("hello");
		Array.from(result);
	`)
	require.NoError(t, err)

	exported := val.Export()
	arr, ok := exported.([]interface{})
	require.True(t, ok)
	assert.Len(t, arr, 5) // "hello" = 5 bytes
}

func TestEncodingManager_TextDecoder(t *testing.T) {
	vm := sobek.New()
	em := NewEncodingManager(vm)
	require.NoError(t, em.Install())

	// Test basic decoding
	val, err := vm.RunString(`
		var decoder = new TextDecoder();
		decoder.decode(new Uint8Array([104, 101, 108, 108, 111]));
	`)
	require.NoError(t, err)
	assert.Equal(t, "hello", val.String())
}

func TestEncodingManager_Btoa(t *testing.T) {
	vm := sobek.New()
	em := NewEncodingManager(vm)
	require.NoError(t, em.Install())

	val, err := vm.RunString(`btoa("hello")`)
	require.NoError(t, err)
	assert.Equal(t, "aGVsbG8=", val.String())
}

func TestEncodingManager_Atob(t *testing.T) {
	vm := sobek.New()
	em := NewEncodingManager(vm)
	require.NoError(t, em.Install())

	val, err := vm.RunString(`atob("aGVsbG8=")`)
	require.NoError(t, err)
	assert.Equal(t, "hello", val.String())
}

func TestEncodingManager_AtobBtoaRoundtrip(t *testing.T) {
	vm := sobek.New()
	em := NewEncodingManager(vm)
	require.NoError(t, em.Install())

	val, err := vm.RunString(`atob(btoa("test string"))`)
	require.NoError(t, err)
	assert.Equal(t, "test string", val.String())
}

func TestEncodingManager_TextEncoderEncodeInto(t *testing.T) {
	vm := sobek.New()
	em := NewEncodingManager(vm)
	require.NoError(t, em.Install())

	t.Run("encodes string into Uint8Array", func(t *testing.T) {
		val, err := vm.RunString(`
			var encoder = new TextEncoder();
			var dest = new Uint8Array(10);
			var result = encoder.encodeInto("hello", dest);
			({ read: result.read, written: result.written });
		`)
		require.NoError(t, err)

		obj := val.ToObject(vm)
		assert.Equal(t, int64(5), obj.Get("read").ToInteger())
		assert.Equal(t, int64(5), obj.Get("written").ToInteger())
	})

	t.Run("truncates when destination too small", func(t *testing.T) {
		val, err := vm.RunString(`
			var encoder = new TextEncoder();
			var dest = new Uint8Array(3);
			var result = encoder.encodeInto("hello", dest);
			({ read: result.read, written: result.written });
		`)
		require.NoError(t, err)

		obj := val.ToObject(vm)
		assert.Equal(t, int64(3), obj.Get("read").ToInteger())
		assert.Equal(t, int64(3), obj.Get("written").ToInteger())
	})

	t.Run("handles empty string", func(t *testing.T) {
		val, err := vm.RunString(`
			var encoder = new TextEncoder();
			var dest = new Uint8Array(10);
			var result = encoder.encodeInto("", dest);
			({ read: result.read, written: result.written });
		`)
		require.NoError(t, err)

		obj := val.ToObject(vm)
		assert.Equal(t, int64(0), obj.Get("read").ToInteger())
		assert.Equal(t, int64(0), obj.Get("written").ToInteger())
	})

	t.Run("handles missing arguments", func(t *testing.T) {
		val, err := vm.RunString(`
			var encoder = new TextEncoder();
			var result = encoder.encodeInto();
			({ read: result.read, written: result.written });
		`)
		require.NoError(t, err)

		obj := val.ToObject(vm)
		assert.Equal(t, int64(0), obj.Get("read").ToInteger())
		assert.Equal(t, int64(0), obj.Get("written").ToInteger())
	})

	t.Run("encodes UTF-8 multi-byte characters", func(t *testing.T) {
		val, err := vm.RunString(`
			var encoder = new TextEncoder();
			var dest = new Uint8Array(10);
			var result = encoder.encodeInto("日本", dest);  // 2 characters, 6 bytes in UTF-8
			({ read: result.read, written: result.written });
		`)
		require.NoError(t, err)

		obj := val.ToObject(vm)
		// 2 code units in UTF-16
		assert.Equal(t, int64(2), obj.Get("read").ToInteger())
		// 6 bytes in UTF-8
		assert.Equal(t, int64(6), obj.Get("written").ToInteger())
	})
}
