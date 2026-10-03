package component

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bnema/dumber/internal/application/port"
)

func TestJSDialogOriginLabel(t *testing.T) {
	assert.Equal(t, "example.com", jsDialogOriginLabel("https://example.com/a?b=c"))
	assert.Equal(t, "example.com:8080", jsDialogOriginLabel("http://example.com:8080/"))
	assert.Equal(t, "This page", jsDialogOriginLabel(""))
	assert.Equal(t, "This page", jsDialogOriginLabel("not a url"))
	assert.Equal(t, "file:", jsDialogOriginLabel("file:///tmp/x.html"))
	long := jsDialogOriginLabel("https://" + strings.Repeat("a", 500) + ".example.com/")
	assert.LessOrEqual(t, len([]rune(long)), jsDialogMaxHostChars+1)
}

func TestJSDialogOriginLabelKeepsRegistrableDomain(t *testing.T) {
	// A long attacker-controlled prefix must not push the real domain out.
	label := jsDialogOriginLabel("https://" + strings.Repeat("paypal.com.", 20) + "evil.test/")
	assert.True(t, strings.HasPrefix(label, "…"))
	assert.True(t, strings.HasSuffix(label, "evil.test"))
}

func TestBuildJSDialogContentCapsPageText(t *testing.T) {
	huge := strings.Repeat("x", jsDialogMaxTextRunes*3)
	c := buildJSDialogContent(port.JSDialogRequest{Type: port.JSDialogAlert, Message: huge})
	assert.LessOrEqual(t, len([]rune(c.Body)), jsDialogMaxTextRunes+1)
}

func TestBuildJSDialogContentBeforeUnloadUsesFixedText(t *testing.T) {
	c := buildJSDialogContent(port.JSDialogRequest{Type: port.JSDialogBeforeUnload, Origin: "https://example.com", Message: "page text"})
	assert.Contains(t, c.Body, jsDialogBeforeUnloadText)
	assert.NotContains(t, c.Body, "page text")
}

func TestBuildJSDialogContent(t *testing.T) {
	origin := "https://example.com/x"

	alert := buildJSDialogContent(port.JSDialogRequest{Type: port.JSDialogAlert, Origin: origin, Message: "<b>hi</b>"})
	assert.Equal(t, "example.com says:", alert.Heading)
	assert.Equal(t, "<b>hi</b>", alert.Body, "message must stay plain text")
	assert.Equal(t, "OK", alert.OKLabel)
	assert.Empty(t, alert.CancelLabel)
	assert.False(t, alert.ShowInput)

	confirm := buildJSDialogContent(port.JSDialogRequest{Type: port.JSDialogConfirm, Origin: origin})
	assert.Equal(t, "Cancel", confirm.CancelLabel)
	assert.False(t, confirm.ShowInput)

	prompt := buildJSDialogContent(port.JSDialogRequest{Type: port.JSDialogPrompt, Origin: origin})
	assert.True(t, prompt.ShowInput)
	assert.Equal(t, "Cancel", prompt.CancelLabel)

	leave := buildJSDialogContent(port.JSDialogRequest{Type: port.JSDialogBeforeUnload, Origin: origin})
	assert.Equal(t, "Leave page?", leave.Heading)
	assert.Equal(t, "Leave", leave.OKLabel)
	assert.Equal(t, "Stay", leave.CancelLabel)
	assert.Contains(t, leave.Body, "example.com")

	reload := buildJSDialogContent(port.JSDialogRequest{Type: port.JSDialogBeforeUnload, Origin: origin, IsReload: true, Message: "x"})
	assert.Equal(t, "Reload page?", reload.Heading)
}
