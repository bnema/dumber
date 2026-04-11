package cef

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

func TestEngineConfigureDownloads_ReturnsUnsupported(t *testing.T) {
	eng := &Engine{}
	err := eng.ConfigureDownloads(context.Background(), "/tmp", nil, nil)
	require.ErrorIs(t, err, ErrDownloadsUnsupported)
}

func TestWebViewFactoryCreateRelated_ReturnsUnsupported(t *testing.T) {
	factory := &WebViewFactory{}
	wv, err := factory.CreateRelated(context.Background(), port.WebViewID(42))
	require.Nil(t, wv)
	require.ErrorIs(t, err, ErrRelatedWebViewUnsupported)
}
