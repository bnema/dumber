package cef

import (
	"context"
	"testing"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/usecase"
	"github.com/stretchr/testify/require"
)

func TestEngineConfigureDownloads_StoresHandler(t *testing.T) {
	eng := &Engine{}
	preparer := usecase.NewPrepareDownloadUseCase(nil)

	err := eng.ConfigureDownloads(context.Background(), "/tmp", nil, preparer)

	require.NoError(t, err)
	require.NotNil(t, eng.currentDownloadHandler())
}

func TestEngineConfigureDownloads_NilPreparer(t *testing.T) {
	eng := &Engine{}

	err := eng.ConfigureDownloads(context.Background(), "/tmp", nil, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "preparer is required")
	require.Nil(t, eng.currentDownloadHandler())
}

func TestWebViewFactoryCreateRelated_ReturnsUnsupported(t *testing.T) {
	factory := &WebViewFactory{}
	wv, err := factory.CreateRelated(context.Background(), port.WebViewID(42))
	require.Nil(t, wv)
	require.ErrorIs(t, err, ErrRelatedWebViewUnsupported)
}
