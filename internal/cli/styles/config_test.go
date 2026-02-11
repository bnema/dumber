package styles_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

func TestConfigRenderer_RenderOpening(t *testing.T) {
	theme := styles.NewTheme(config.DefaultConfig())
	r := styles.NewConfigRenderer(theme)

	out := r.RenderOpening("/tmp/dumber/config.toml", "vim")
	require.Contains(t, out, "Opening")
	require.Contains(t, out, "config.toml")
	require.Contains(t, out, "vim")
}
