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
