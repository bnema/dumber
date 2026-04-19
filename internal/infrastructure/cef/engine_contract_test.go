package cef

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/stretchr/testify/require"
)

func TestEngineConfigureDownloads_StoresHandler(t *testing.T) {
	eng := &Engine{}
	preparer := usecase.NewPrepareDownloadUseCase(nil)

	err := eng.ConfigureDownloads(context.Background(), "/tmp", nil, preparer)

	require.NoError(t, err)
	require.NotNil(t, eng.currentDownloadHandler())
}

func TestWebViewFactoryCreateRelated_ReturnsUnsupported(t *testing.T) {
	factory := &WebViewFactory{}
	wv, err := factory.CreateRelated(context.Background(), port.WebViewID(42))
	require.Nil(t, wv)
	require.ErrorIs(t, err, ErrRelatedWebViewUnsupported)
}
