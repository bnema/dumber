package webkit

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllowsSystemviewBridgeMessage(t *testing.T) {
	const token = "0123456789abcdef0123456789abcdef"
	tests := []struct {
		name, uri, want, got string
		allowed              bool
	}{
		{"top frame of trusted page", "dumb://home", token, token, true},
		{"embedded frame without token", "dumb://home", token, "", false},
		{"embedded frame with guessed token", "dumb://home", token, "deadbeef", false},
		{"token prefix", "dumb://home", token, token[:16], false},
		{"untrusted top page with token", "https://grafana.local/d/x", token, token, false},
		{"webview without token", "dumb://home", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.allowed, allowsSystemviewBridgeMessage(tt.uri, tt.want, tt.got))
		})
	}
}

func TestParseScriptMessageJSONExtractsToken(t *testing.T) {
	msg, token, err := parseScriptMessageJSON([]byte(`{"type":"home_load","payload":{"a":1},"__dumber_bridge_token":"abc"}`))
	require.NoError(t, err)
	assert.Equal(t, "home_load", msg.Type)
	assert.JSONEq(t, `{"a":1}`, string(msg.Payload))
	assert.Equal(t, "abc", token)

	_, token, err = parseScriptMessageJSON([]byte(`{"type":"home_load"}`))
	require.NoError(t, err)
	assert.Empty(t, token)

	_, _, err = parseScriptMessageJSON([]byte(`not json`))
	require.Error(t, err)
}

func TestBuildSystemviewBridgeTokenScript(t *testing.T) {
	script := buildSystemviewBridgeTokenScript("tok'en")
	assert.Contains(t, script, `var token = 'tok\'en';`)
	assert.Contains(t, script, systemviewBridgeTokenField)
	assert.Contains(t, script, "Object.getPrototypeOf(handler)")
	assert.Contains(t, script, "configurable: false")
	guard := strings.Index(script, "location.protocol !== 'dumb:'")
	require.Positive(t, guard, "script must check the dumb: scheme itself")
	assert.Less(t, guard, strings.Index(script, "tok\\'en"), "scheme check must run before the token is defined")
	assert.Contains(t, script, "window.top !== window")
	assert.Equal(t, 1, strings.Count(script, "tok\\'en"), "token must appear once, only in the closure")
}

func TestWebViewSystemviewBridgeTokenAccessors(t *testing.T) {
	wv := &WebView{}
	assert.Empty(t, wv.SystemviewBridgeToken())
	wv.setSystemviewBridgeToken("x")
	assert.Equal(t, "x", wv.SystemviewBridgeToken())
	var nilWV *WebView
	assert.Empty(t, nilWV.SystemviewBridgeToken())
}
