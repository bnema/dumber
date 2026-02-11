package styles_test

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/bnema/dumber/internal/cli/styles"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

func TestSessionsCLIRenderer(t *testing.T) {
	theme := styles.NewTheme(config.DefaultConfig())
	r := styles.NewSessionsCLIRenderer(theme)

	out := r.RenderEmptyList()
	require.Contains(t, out, "No saved sessions found.")

	now := time.Now().UTC()
	items := []entity.SessionInfo{
		{
			Session:   &entity.Session{ID: entity.SessionID("20260210_120000_abcd")},
			TabCount:  3,
			PaneCount: 5,
			IsCurrent: true,
			UpdatedAt: now,
		},
	}
	out = r.RenderList(items, 20)
	require.Contains(t, out, "Sessions")
	require.Contains(t, out, "20260210_120000_abcd")
	require.Contains(t, out, "3 tabs")
	require.Contains(t, out, "5 panes")

	errOut := r.RenderError(errors.New("boom"))
	require.Contains(t, errOut, "boom")
}
