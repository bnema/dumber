package systemviews

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// The CEF local network guard trusts an initial about:blank frame under an
// embed only because sandboxed embeds cannot top-navigate (see
// localNetworkScope.trustedFrame).
func TestHomeEmbedSandboxNeverAllowsTopNavigation(t *testing.T) {
	t.Parallel()
	for _, token := range strings.Fields(homeEmbedSandbox) {
		assert.NotContains(t, strings.ToLower(token), "allow-top-navigation", token)
	}

	html := mustRenderComponent(HomeServiceEmbed(homeEmbedView{Name: "Grafana", URL: "http://10.0.0.2:3000/", Height: 240}, true, false))
	assert.Contains(t, html, `sandbox="`+homeEmbedSandbox+`"`)
	assert.NotContains(t, html, "allow-top-navigation")
}
