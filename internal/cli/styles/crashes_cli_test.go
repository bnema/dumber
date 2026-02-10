package styles_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

func TestCrashesCLIRenderer(t *testing.T) {
	theme := styles.NewTheme(config.DefaultConfig())
	r := styles.NewCrashesCLIRenderer(theme)

	out := r.RenderError(errors.New("nope"))
	require.Contains(t, out, "nope")

	h := r.RenderHintList()
	require.Contains(t, h, "dumber crashes")
}
