package systemviews

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCSSQuoted(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		`plain-id_1`:    `plain-id_1`,
		`a"b\c`:         `a\"b\\c`,
		"line\nbreak":   `line\a break`,
		"cr\rlf\fff":    `cr\d lf\c ff`,
		"nul\x00byte":   "nul�byte",
		`"]; x[y="`:     `\"]; x[y=\"`,
		"home-svc:cam":  "home-svc:cam",
		"emoji 🚀 ok":    "emoji 🚀 ok",
		"\\\"\n":        `\\\"\a `,
		"":              "",
		"tab\tstays":    "tab\tstays",
		"quote\"inline": `quote\"inline`,
	} {
		assert.Equal(t, want, cssQuoted(in), "%q", in)
	}
}

func TestHomeRowLinkMarksSuggestions(t *testing.T) {
	t.Parallel()
	row := homeRow{URL: "https://example.com", Title: "Example"}
	suggestion := mustRenderComponent(homeRowLink(row, true))
	assert.Contains(t, suggestion, `class="sv-home-row sv-home-suggestion"`)
	assert.Contains(t, suggestion, `data-sv-home-suggestion`)
	plain := mustRenderComponent(homeRowLink(row, false))
	assert.Contains(t, plain, `class="sv-home-row"`)
	assert.NotContains(t, plain, `sv-home-suggestion`)
}
