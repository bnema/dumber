package browserjs

import (
	"testing"

	"github.com/grafana/sobek"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestURLManager_URL(t *testing.T) {
	vm := sobek.New()
	um := NewURLManager(vm)
	require.NoError(t, um.Install())

	val, err := vm.RunString(`
		var url = new URL("https://example.com:8080/path?query=value#hash");
		JSON.stringify({
			protocol: url.protocol,
			hostname: url.hostname,
			port: url.port,
			pathname: url.pathname,
			search: url.search,
			hash: url.hash,
			origin: url.origin
		});
	`)
	require.NoError(t, err)

	expected := `{"protocol":"https:","hostname":"example.com","port":"8080","pathname":"/path","search":"?query=value","hash":"#hash","origin":"https://example.com:8080"}`
	assert.Equal(t, expected, val.String())
}

func TestURLManager_URLSearchParams(t *testing.T) {
	vm := sobek.New()
	um := NewURLManager(vm)
	require.NoError(t, um.Install())

	val, err := vm.RunString(`
		var params = new URLSearchParams("foo=1&bar=2");
		params.append("baz", "3");
		params.toString();
	`)
	require.NoError(t, err)
	assert.Equal(t, "foo=1&bar=2&baz=3", val.String())
}

func TestURLManager_URLWithBase(t *testing.T) {
	vm := sobek.New()
	um := NewURLManager(vm)
	require.NoError(t, um.Install())

	val, err := vm.RunString(`
		var url = new URL("/path", "https://example.com");
		url.href;
	`)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/path", val.String())
}

func TestURLManager_SpecialSchemes(t *testing.T) {
	vm := sobek.New()
	um := NewURLManager(vm)
	require.NoError(t, um.Install())

	val, err := vm.RunString(`
		var url = new URL("about:blank");
		url.protocol + " " + url.origin;
	`)
	require.NoError(t, err)
	assert.Equal(t, "about: null", val.String())
}
