package url

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSystemviewPageResolvesAliases(t *testing.T) {
	t.Parallel()
	for host, want := range map[string]string{"history": "history", "Favorites": "favorites", "config": "config", "settings": "config"} {
		page, ok := SystemviewPage(host)
		assert.True(t, ok, host)
		assert.Equal(t, want, page, host)
	}
	_, ok := SystemviewPage("error")
	assert.False(t, ok)
}

func TestIsSystemviewPageURL(t *testing.T) {
	t.Parallel()
	assert.True(t, IsSystemviewPageURL("dumb://history"))
	assert.True(t, IsSystemviewPageURL("dumb://config/"))
	assert.True(t, IsSystemviewPageURL("dumb://settings"))
	assert.False(t, IsSystemviewPageURL("dumb://history/crash"))
	assert.False(t, IsSystemviewPageURL("dumb://error"))
	assert.False(t, IsSystemviewPageURL("dumb://api/favicon"))
	assert.False(t, IsSystemviewPageURL("https://dumber.invalid/history"))
}

func TestSystemviewTitle(t *testing.T) {
	t.Parallel()
	title, ok := SystemviewTitle("dumb://settings")
	assert.True(t, ok)
	assert.Equal(t, "Settings", title)
	_, ok = SystemviewTitle("https://example.com")
	assert.False(t, ok)
}
