package bootstrap

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	corelogging "github.com/bnema/dumber/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedactSensitiveContent(t *testing.T) {
	line := "redirect to https://example.com/cb?code=abc123&token=secret#frag"
	redacted := redactSensitiveContent(line)

	assert.Contains(t, redacted, "https://example.com/cb")
	assert.NotContains(t, redacted, "code=abc123")
	assert.NotContains(t, redacted, "token=secret")
	assert.NotContains(t, redacted, "#frag")
}

func TestWriteUnexpectedCloseReport(t *testing.T) {
	lockDir := t.TempDir()
	sessionID := "crash-test-1"
	startedAt := time.Date(2026, 2, 8, 10, 0, 0, 0, time.UTC)

	require.NoError(t, writeStartupMarker(lockDir, sessionID, startedAt))
	_, err := markAbruptExits(lockDir, startedAt.Add(3*time.Minute), nil)
	require.NoError(t, err)

	logPath := filepath.Join(lockDir, corelogging.SessionFilename(sessionID))
	logBody := `{"level":"info","message":"opening https://example.com/path?a=1&b=2"}` +
		"\n" +
		`{"level":"warn","message":"callback code=abc token=def"}` +
		"\n"
	require.NoError(t, os.WriteFile(logPath, []byte(logBody), markerFilePerm))

	jsonPath, err := writeUnexpectedCloseReport(lockDir, sessionID)
	require.NoError(t, err)
	require.NotEmpty(t, jsonPath)
	assert.FileExists(t, jsonPath)

	mdPath := strings.TrimSuffix(jsonPath, ".json") + ".md"
	assert.FileExists(t, mdPath)

	raw, err := os.ReadFile(jsonPath)
	require.NoError(t, err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	assert.Equal(t, sessionID, payload["session_id"])

	classification := payload["classification"].(map[string]any)
	classValue, _ := classification["class"].(string)
	assert.Equal(t, string(SessionExitMainProcessCrashOrAbrupt), classValue)

	tail := payload["session_log_tail_redacted"].([]any)
	require.NotEmpty(t, tail)
	text := tail[0].(string) + tail[len(tail)-1].(string)
	assert.NotContains(t, text, "?a=1&b=2")
	assert.NotContains(t, text, "code=abc")
	assert.NotContains(t, text, "token=def")
}
