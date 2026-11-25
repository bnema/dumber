package content

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
)

func TestBuildUserCSSScript_EscapesCSS(t *testing.T) {
	t.Parallel()
	css := "body{content:\"</style><script>x</script>\\n\"}\n`${a}`"
	script, err := buildUserCSSScript(css)
	require.NoError(t, err)

	encoded, err := json.Marshal(css)
	require.NoError(t, err)
	assert.Contains(t, script, "var cssText = "+string(encoded)+";")
	assert.NotContains(t, script, "</script>")
	assert.Contains(t, script, "head.appendChild(style)")
}

func TestBuildUserCSSScript_EmptyRemovesElement(t *testing.T) {
	t.Parallel()
	script, err := buildUserCSSScript("")
	require.NoError(t, err)
	assert.Contains(t, script, `var cssText = "";`)
	assert.Contains(t, script, "style.remove()")
}

func TestApplySystemViewUserCSS_AppliesAfterThemeOnLaterCommit(t *testing.T) {
	t.Parallel()
	c := newSystemViewCoordinator(map[entity.PaneID]port.WebView{})
	ctx := context.Background()

	// user.css arrives before the theme at startup; theme must still apply first.
	c.ApplySystemViewUserCSS(ctx, "body{outline:5px solid red}")
	c.BroadcastToSystemViews(ctx, "other", "O1")
	c.ApplyWebUIThemeToAll(ctx, true, "body{}")

	wv := &systemViewWebViewStub{uri: "dumb://history"}
	c.applySystemViewStateOnCommit(ctx, "a", wv)

	require.Len(t, wv.scripts, 3)
	themeScript, err := buildWebUIThemeScript(true, "body{}")
	require.NoError(t, err)
	userScript, err := buildUserCSSScript("body{outline:5px solid red}")
	require.NoError(t, err)
	assert.Equal(t, []string{themeScript, "O1", userScript}, wv.scripts)
}
