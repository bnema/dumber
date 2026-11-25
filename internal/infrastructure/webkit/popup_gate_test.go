package webkit

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBlockSystemViewPopup(t *testing.T) {
	for target, want := range map[string]bool{
		"dumb://history":                   true,
		"DUMB:config":                      true,
		"https://dumber.invalid/history":   true,
		"https://Dumber.Invalid:443/x":     true,
		"https://dumber.invalid.evil.com/": false,
		"https://example.com/?u=dumb://x":  false,
		"about:blank":                      false,
		"":                                 false,
		"https://example.com/":             false,
	} {
		assert.Equal(t, want, blockSystemViewPopup(target), target)
	}
}
