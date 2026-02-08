package bootstrap

import (
	"encoding/json"
	"fmt"
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
	t.Run("url_query_params", func(t *testing.T) {
		line := "redirect to https://example.com/cb?code=abc123&token=secret#frag"
		redacted := redactSensitiveContent(line)
		assert.Contains(t, redacted, "https://example.com/cb")
		assert.NotContains(t, redacted, "code=abc123")
		assert.NotContains(t, redacted, "token=secret")
		assert.NotContains(t, redacted, "#frag")
	})

	t.Run("json_secret_fields", func(t *testing.T) {
		line := `{"token":"eyJhbGciOi","access_token":"sk-abc123","password":"hunter2"}`
		redacted := redactSensitiveContent(line)
		assert.NotContains(t, redacted, "eyJhbGciOi")
		assert.NotContains(t, redacted, "sk-abc123")
		assert.NotContains(t, redacted, "hunter2")
		assert.Contains(t, redacted, `"token":"[REDACTED]"`)
		assert.Contains(t, redacted, `"access_token":"[REDACTED]"`)
		assert.Contains(t, redacted, `"password":"[REDACTED]"`)
	})

	t.Run("key_value_params", func(t *testing.T) {
		line := "callback received password=secret123 token=abc456"
		redacted := redactSensitiveContent(line)
		assert.NotContains(t, redacted, "secret123")
		assert.NotContains(t, redacted, "abc456")
		assert.Contains(t, redacted, "password=[REDACTED]")
		assert.Contains(t, redacted, "token=[REDACTED]")
	})

	t.Run("authorization_headers", func(t *testing.T) {
		line := "Authorization: Bearer eyJhbGciOiJSUz"
		redacted := redactSensitiveContent(line)
		assert.NotContains(t, redacted, "eyJhbGciOiJSUz")
		assert.Contains(t, redacted, "[REDACTED]")

		line2 := "auth: basic dXNlcjpwYXNz"
		redacted2 := redactSensitiveContent(line2)
		assert.NotContains(t, redacted2, "dXNlcjpwYXNz")
		assert.Contains(t, redacted2, "[REDACTED]")
	})
}

func TestReadRedactedLogTail_RingBuffer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	var lines []string
	for i := 0; i < 10; i++ {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), markerFilePerm))

	t.Run("fewer_lines_than_requested", func(t *testing.T) {
		result := readRedactedLogTail(path, 20)
		assert.Len(t, result, 10)
		assert.Equal(t, "line 0", result[0])
		assert.Equal(t, "line 9", result[9])
	})

	t.Run("exact_tail", func(t *testing.T) {
		result := readRedactedLogTail(path, 3)
		assert.Len(t, result, 3)
		assert.Equal(t, "line 7", result[0])
		assert.Equal(t, "line 8", result[1])
		assert.Equal(t, "line 9", result[2])
	})
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

	classificationRaw, ok := payload["classification"].(map[string]any)
	require.True(t, ok, "classification should be a map[string]any")
	classValue, ok := classificationRaw["class"].(string)
	require.True(t, ok, "classification.class should be a string")
	assert.Equal(t, string(SessionExitMainProcessCrashOrAbrupt), classValue)

	tail, ok := payload["session_log_tail_redacted"].([]any)
	require.True(t, ok, "session_log_tail_redacted should be a []any")
	require.GreaterOrEqual(t, len(tail), 2, "expected at least 2 log tail entries")
	first, ok := tail[0].(string)
	require.True(t, ok, "first tail entry should be a string")
	last, ok := tail[len(tail)-1].(string)
	require.True(t, ok, "last tail entry should be a string")
	text := first + last
	assert.NotContains(t, text, "?a=1&b=2")
	assert.NotContains(t, text, "code=abc")
	assert.NotContains(t, text, "token=def")
}
