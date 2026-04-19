package webkit

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDownloadHandler(t *testing.T) {
	prepareUC := usecase.NewPrepareDownloadUseCase(nil)

	t.Run("creates handler with path and nil event handler", func(t *testing.T) {
		handler := NewDownloadHandler("/tmp/downloads", nil, prepareUC)

		assert.NotNil(t, handler)
		require.NotNil(t, handler.runtime)
	})

	t.Run("creates handler with custom path", func(t *testing.T) {
		handler := NewDownloadHandler("/custom/path", nil, prepareUC)

		require.NotNil(t, handler.runtime)
	})
}

func TestDownloadHandler_SetDownloadPath(t *testing.T) {
	prepareUC := usecase.NewPrepareDownloadUseCase(nil)
	handler := NewDownloadHandler("/initial/path", nil, prepareUC)

	newPath := t.TempDir()
	handler.SetDownloadPath(newPath)

	output, err := handler.runtime.ResolveDestination(context.Background(), "artifact.pdf", nil)

	require.NoError(t, err)
	assert.Equal(t, filepath.Join(newPath, "artifact.pdf"), output.DestinationPath)
}

func TestURIResponseAdapter(t *testing.T) {
	t.Run("handles nil response", func(t *testing.T) {
		adapter := &uriResponseAdapter{resp: nil}

		assert.Empty(t, adapter.GetMimeType())
		assert.Empty(t, adapter.GetSuggestedFilename())
		assert.Empty(t, adapter.GetUri())
	})
}
