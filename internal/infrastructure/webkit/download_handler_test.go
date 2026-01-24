package webkit

import (
	"testing"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/stretchr/testify/assert"
)

func TestNewDownloadHandler(t *testing.T) {
	prepareUC := usecase.NewPrepareDownloadUseCase(nil)

	t.Run("creates handler with path and nil event handler", func(t *testing.T) {
		handler := NewDownloadHandler("/tmp/downloads", nil, prepareUC)

		assert.NotNil(t, handler)
		assert.Equal(t, "/tmp/downloads", handler.downloadPath)
		assert.Nil(t, handler.eventHandler)
		assert.NotNil(t, handler.prepareDownloadUC)
	})

	t.Run("creates handler with custom path", func(t *testing.T) {
		handler := NewDownloadHandler("/custom/path", nil, prepareUC)

		assert.Equal(t, "/custom/path", handler.downloadPath)
	})
}

func TestDownloadHandler_SetDownloadPath(t *testing.T) {
	prepareUC := usecase.NewPrepareDownloadUseCase(nil)
	handler := NewDownloadHandler("/initial/path", nil, prepareUC)

	handler.SetDownloadPath("/new/path")

	assert.Equal(t, "/new/path", handler.downloadPath)
}

func TestURIResponseAdapter(t *testing.T) {
	t.Run("handles nil response", func(t *testing.T) {
		adapter := &uriResponseAdapter{resp: nil}

		assert.Empty(t, adapter.GetMimeType())
		assert.Empty(t, adapter.GetSuggestedFilename())
		assert.Empty(t, adapter.GetUri())
	})
}
