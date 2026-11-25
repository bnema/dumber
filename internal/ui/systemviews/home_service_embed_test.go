package systemviews

import (
	"testing"

	"bnema.dev/dumber/internal/domain/entity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func embedService(rawURL string, height int) entity.HomeService {
	return entity.HomeService{ID: "cam", Name: "Cam cam", Description: "front door", Widget: &entity.HomeServiceWidget{
		Type: entity.HomeServiceWidgetEmbed, URL: rawURL, Height: height,
	}}
}

// embedPushes returns every markup pushed into the cam embed region.
func embedPushes(dom *treeDOM) []string {
	var pushes []string
	for _, update := range dom.regions() {
		for _, region := range update {
			if region.Name == homeServiceEmbedRegion("cam") {
				pushes = append(pushes, region.HTML)
			}
		}
	}
	return pushes
}

func newEmbedApp(t *testing.T, home *homeStore) (*App, *treeDOM) {
	t.Helper()
	dom := newTreeDOM()
	app := newHomeApp(t, dom, home)
	t.Cleanup(app.Close)
	return app, dom
}

func TestHomeEmbedURL(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]string{
		"https://grafana.lan/d/x?kiosk":     "https://grafana.lan/d/x?kiosk",
		"HTTP://192.168.1.4:3000/":          "http://192.168.1.4:3000/",
		"https://user:pass@cam.lan/live":    "https://cam.lan/live",
		"  https://cam.lan  ":               "https://cam.lan",
		"/history":                          "",
		"history":                           "",
		"//cam.lan/":                        "",
		"dumb://history":                    "",
		"javascript:alert(1)":               "",
		"data:text/html,<script>x</script>": "",
		"file:///etc/passwd":                "",
		"https://dumber.invalid/history":    "",
		"https://DUMBER.INVALID./config":    "",
		"https://api.dumber.invalid/":       "",
		"http://ｄｕｍｂｅｒ.invalid/":            "",
		"http://dumber。invalid/":            "",
		"http://dumber%2Einvalid/":          "",
		"https://":                          "",
	} {
		got, ok := homeEmbedURL(raw)
		assert.Equal(t, want != "", ok, raw)
		assert.Equal(t, want, got, raw)
	}
}

func TestHomeEmbedRendersSandboxedIframe(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: homeLayoutWith(embedService("https://cam.lan/live", 420))})
	app, dom := newEmbedApp(t, home)
	assert.Contains(t, app.renderedHTML, `data-sv-region="home-svc-embed:cam"`)
	pushes := embedPushes(dom)
	require.Len(t, pushes, 1)
	assert.Equal(t,
		`<iframe class="sv-home-embed" src="https://cam.lan/live" height="420" title="Cam cam" sandbox="allow-scripts allow-same-origin allow-forms allow-popups" referrerpolicy="no-referrer" loading="lazy"></iframe>`,
		pushes[0])
}

func TestHomeEmbedRejectedURLShowsNote(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: homeLayoutWith(embedService("https://dumber.invalid/config", 0))})
	_, dom := newEmbedApp(t, home)
	pushes := embedPushes(dom)
	require.Len(t, pushes, 1)
	assert.NotContains(t, pushes[0], "<iframe")
	assert.Contains(t, pushes[0], "http(s)")
}

func TestHomeEmbedKeptAcrossReloads(t *testing.T) {
	t.Parallel()
	home := newHomeStore(t, &homeStore{load: homeLayoutWith(embedService("https://cam.lan/live", 0), entity.HomeService{ID: "other", Name: "Other"})})
	app, dom := newEmbedApp(t, home)
	require.Len(t, embedPushes(dom), 1)
	mounts := dom.mountCount()

	// Same layout: nothing touches the iframe.
	reloadHome(t, app)
	// The embed service's own description and a sibling change: still untouched.
	home.load.Layout.Groups[0].Services[0].Description = "back door"
	home.load.Layout.Groups[0].Services[1].Description = "changed"
	reloadHome(t, app)
	assert.Len(t, embedPushes(dom), 1, "the iframe node is kept")
	assert.Equal(t, mounts, dom.mountCount(), "no remount")

	// A new height replaces the iframe.
	home.load.Layout.Groups[0].Services[0].Widget.Height = 500
	reloadHome(t, app)
	pushes := embedPushes(dom)
	require.Len(t, pushes, 2)
	assert.Contains(t, pushes[1], `height="500"`)

	// Dropping the embed empties the region.
	home.load.Layout.Groups[0].Services[0].Widget = nil
	reloadHome(t, app)
	pushes = embedPushes(dom)
	require.Len(t, pushes, 3)
	assert.Empty(t, pushes[2])
}
