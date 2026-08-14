package cef

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogAccessibilityCaptureEnabled_Fields(t *testing.T) {
	var output bytes.Buffer
	logger := zerolog.New(&output).Level(zerolog.InfoLevel)
	ctx := logging.WithContext(context.Background(), logger)

	logAccessibilityCaptureEnabled(ctx, port.WebViewID(9), "/tmp/a11y-capture/webview-9")

	var rec map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &rec))
	assert.Equal(t, "cef: accessibility capture enabled", rec["message"])
	assert.InDelta(t, float64(9), rec["webview_id"], 0)
	assert.Equal(t, "/tmp/a11y-capture/webview-9", rec["capture_dir"])
}
