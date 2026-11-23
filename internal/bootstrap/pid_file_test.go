package bootstrap

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	corelogging "github.com/bnema/dumber/internal/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deadPID is above the Linux pid_max limit (4194304), so no process has it.
const deadPID = 99999999

func TestParsePIDFile(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantPID     int
		wantSession string
		wantErr     bool
	}{
		{name: "v1 one line", raw: "123\n", wantPID: 123},
		{name: "v2 two lines", raw: "123\nabc-session\n", wantPID: 123, wantSession: "abc-session"},
		{name: "unsafe session ignored", raw: "123\n../../etc\n", wantPID: 123},
		{name: "zero pid", raw: "0\nabc\n", wantErr: true},
		{name: "negative pid", raw: "-5\n", wantErr: true},
		{name: "garbage", raw: "not-a-pid\nabc\n", wantErr: true},
		{name: "empty", raw: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pid, sessionID, err := parsePIDFile(tt.raw)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantPID, pid)
			assert.Equal(t, tt.wantSession, sessionID)
		})
	}
}

func writeSessionLog(t *testing.T, logDir, sessionID string, mod time.Time) {
	t.Helper()
	p := filepath.Join(logDir, corelogging.SessionFilename(sessionID))
	require.NoError(t, os.WriteFile(p, []byte(`{"level":"info"}`+"\n"), filePerm))
	require.NoError(t, os.Chtimes(p, mod, mod))
}

func TestDetectPreviousCrash_UsesPIDFileSession(t *testing.T) {
	logDir := t.TempDir()
	now := time.Now()
	writeSessionLog(t, logDir, "crashed", now.Add(-time.Hour))
	writeSessionLog(t, logDir, "other-newer", now.Add(-time.Minute))
	writeSessionLog(t, logDir, "current", now)
	require.NoError(t, os.WriteFile(pidFilePath(logDir), []byte(fmt.Sprintf("%d\ncrashed\n", deadPID)), filePerm))

	reports := detectPreviousCrash(logDir, "current", nil)

	require.Len(t, reports, 1)
	assert.Equal(t, "session_crashed.crash.json", filepath.Base(reports[0]))
	assert.NoFileExists(t, pidFilePath(logDir))
}

func TestDetectPreviousCrash_V1FallbackExcludesCurrentSession(t *testing.T) {
	logDir := t.TempDir()
	now := time.Now()
	writeSessionLog(t, logDir, "previous", now.Add(-time.Hour))
	writeSessionLog(t, logDir, "current", now)
	require.NoError(t, os.WriteFile(pidFilePath(logDir), []byte(fmt.Sprintf("%d\n", deadPID)), filePerm))

	reports := detectPreviousCrash(logDir, "current", nil)

	require.Len(t, reports, 1)
	assert.Equal(t, "session_previous.crash.json", filepath.Base(reports[0]))
}

func TestFindLastSessionLog_ExcludesSession(t *testing.T) {
	logDir := t.TempDir()
	now := time.Now()
	writeSessionLog(t, logDir, "older", now.Add(-time.Hour))
	writeSessionLog(t, logDir, "current", now)

	assert.Equal(t, "session_older.log", filepath.Base(findLastSessionLog(logDir, "current")))
	assert.Equal(t, "session_current.log", filepath.Base(findLastSessionLog(logDir, "")))
}
