package bootstrap

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	for i := range 10 {
		lines = append(lines, fmt.Sprintf("line %d", i))
	}
	require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), filePerm))

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

func TestWriteCrashReport(t *testing.T) {
	logDir := t.TempDir()
	sessionID := "crash-test-1"

	logPath := filepath.Join(logDir, corelogging.SessionFilename(sessionID))
	logBody := `{"level":"info","message":"opening https://example.com/path?a=1&b=2"}` +
		"\n" +
		`{"level":"warn","message":"callback code=abc token=def"}` +
		"\n"
	require.NoError(t, os.WriteFile(logPath, []byte(logBody), filePerm))

	jsonPath, err := writeCrashReport(logDir, sessionID, 12345)
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
	assert.InDelta(t, float64(12345), payload["crashed_pid"], 0)

	tail, ok := payload["session_log_tail_redacted"].([]any)
	require.True(t, ok, "session_log_tail_redacted should be a []any")
	require.GreaterOrEqual(t, len(tail), 2, "expected at least 2 log tail entries")
	first, ok := tail[0].(string)
	require.True(t, ok)
	last, ok := tail[len(tail)-1].(string)
	require.True(t, ok)
	text := first + last
	assert.NotContains(t, text, "?a=1&b=2")
	assert.NotContains(t, text, "code=abc")
	assert.NotContains(t, text, "token=def")
}

func TestWriteCrashReport_EmbedsFatalOutputThenRemovesIt(t *testing.T) {
	logDir := t.TempDir()
	sessionID := "crash-fatal-1"
	crashedPID := 4242

	fatalPath := FatalOutputPath(logDir, crashedPID)
	require.NoError(t, os.MkdirAll(filepath.Dir(fatalPath), dirPerm))
	fatalBody := "fatal error: unexpected signal\nfetching https://example.com/x?token=abc\n"
	require.NoError(t, os.WriteFile(fatalPath, []byte(fatalBody), filePerm))

	jsonPath, err := writeCrashReport(logDir, sessionID, crashedPID)
	require.NoError(t, err)

	raw, err := os.ReadFile(jsonPath)
	require.NoError(t, err)
	var report crashReport
	require.NoError(t, json.Unmarshal(raw, &report))
	assert.Equal(t, fatalPath, report.FatalOutputFile)
	require.Len(t, report.FatalOutput, 2)
	assert.Equal(t, "fatal error: unexpected signal", report.FatalOutput[0])
	assert.NotContains(t, report.FatalOutput[1], "token=abc")

	md, err := os.ReadFile(strings.TrimSuffix(jsonPath, ".json") + ".md")
	require.NoError(t, err)
	assert.Contains(t, string(md), "## Go Fatal Output")
	assert.Contains(t, string(md), "fatal error: unexpected signal")

	assert.NoFileExists(t, fatalPath)
}

func TestRedactSensitiveContent_HomeDir(t *testing.T) {
	tests := []struct {
		name string
		home string
		in   string
		want string
	}{
		{name: "path under home", home: "/home/alice", in: "open /home/alice/.config/x", want: "open ~/.config/x"},
		{name: "home alone", home: "/home/alice/", in: "cwd=/home/alice", want: "cwd=~"},
		{name: "other user prefix kept", home: "/home/alice", in: "/home/alicebob/x", want: "/home/alicebob/x"},
		{name: "root home ignored", home: "/", in: "/etc/passwd", want: "/etc/passwd"},
		{name: "empty home ignored", home: "", in: "/home/alice/x", want: "/home/alice/x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := userHomeDir
			t.Cleanup(func() { userHomeDir = orig })
			userHomeDir = func() (string, error) { return tt.home, nil }
			assert.Equal(t, tt.want, redactSensitiveContent(tt.in))
		})
	}
}
