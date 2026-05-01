package cef

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	cef2gtk "github.com/bnema/purego-cef2gtk"

	"github.com/bnema/dumber/internal/logging"
)

const (
	cef2gtkProfileEnv         = "DUMBER_CEF2GTK_PROFILE"
	cef2gtkProfileIntervalEnv = "DUMBER_CEF2GTK_PROFILE_INTERVAL"
	cef2gtkProfileOutputEnv   = "DUMBER_CEF2GTK_PROFILE_OUTPUT"
)

var errProfileWriterClosed = errors.New("cef2gtk profile writer is closed")

type lockedProfileWriter struct {
	mu   sync.Mutex
	file *os.File
}

func (w *lockedProfileWriter) Write(p []byte) (int, error) {
	if w == nil {
		return 0, errProfileWriterClosed
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return 0, errProfileWriterClosed
	}
	return w.file.Write(p)
}

func (w *lockedProfileWriter) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func cef2gtkProfileEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(cef2gtkProfileEnv)))
	if v == "" {
		return false
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	switch v {
	case "1", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func cef2gtkProfileInterval() time.Duration {
	v := strings.TrimSpace(os.Getenv(cef2gtkProfileIntervalEnv))
	if v == "" {
		return time.Second
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return time.Second
	}
	return d
}

type cef2gtkProfileRecord struct {
	WebViewID uint64                  `json:"webview_id"`
	Snapshot  cef2gtk.ProfileSnapshot `json:"snapshot"`
}

func (e *Engine) cef2gtkProfileOptions(wv *WebView) cef2gtk.ProfileOptions {
	if e == nil || wv == nil || !cef2gtkProfileEnabled() {
		return cef2gtk.ProfileOptions{}
	}
	path := e.cef2gtkProfilePath()
	if path == "" {
		return cef2gtk.ProfileOptions{}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logging.FromContext(e.ctx).Warn().Err(err).Str("path", path).Msg("cef2gtk: failed to create profile log directory")
		return cef2gtk.ProfileOptions{}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		logging.FromContext(e.ctx).Warn().Err(err).Str("path", path).Msg("cef2gtk: failed to open profile log")
		return cef2gtk.ProfileOptions{}
	}
	writer := &lockedProfileWriter{file: file}
	wv.profileCleanup = func() { _ = writer.Close() }
	logging.FromContext(e.ctx).Info().Str("profile_log", path).Uint64("webview_id", uint64(wv.id)).Msg("cef2gtk profiling enabled")
	return cef2gtk.ProfileOptions{
		Enabled:  true,
		Interval: cef2gtkProfileInterval(),
		OnSnapshot: func(s cef2gtk.ProfileSnapshot) {
			wv.recordCEF2GTKProfileSnapshot(s)
			record := cef2gtkProfileRecord{WebViewID: uint64(wv.id), Snapshot: s}
			if b, err := json.Marshal(record); err == nil {
				_, _ = writer.Write(append(b, '\n'))
			}
			logging.FromContext(e.ctx).Debug().
				Uint64("webview_id", uint64(wv.id)).
				Uint64("frames_received", s.FramesReceived).
				Uint64("frames_rendered", s.FramesRendered).
				Uint32("gc_delta", s.GC.NumGCDelta).
				Msg("cef2gtk profile snapshot")
		},
	}
}

func (wv *WebView) recordCEF2GTKProfileSnapshot(snapshot cef2gtk.ProfileSnapshot) {
	if wv == nil {
		return
	}
	wv.profileMu.Lock()
	wv.previousProfileSnapshot = wv.latestProfileSnapshot
	wv.latestProfileSnapshot = snapshot
	wv.latestProfileSnapshotAt = snapshot.Time
	wv.profileMu.Unlock()
}

func (wv *WebView) latestCEF2GTKProfileSnapshot(now time.Time) (cef2gtk.ProfileSnapshot, time.Duration, bool) {
	if wv == nil {
		return cef2gtk.ProfileSnapshot{}, 0, false
	}
	wv.profileMu.Lock()
	snapshot := wv.latestProfileSnapshot
	snapshotAt := wv.latestProfileSnapshotAt
	wv.profileMu.Unlock()
	if snapshotAt.IsZero() {
		return cef2gtk.ProfileSnapshot{}, 0, false
	}
	age := now.Sub(snapshotAt)
	if age < 0 {
		age = 0
	}
	return snapshot, age, true
}

func (e *Engine) cef2gtkProfilePath() string {
	if explicit := strings.TrimSpace(os.Getenv(cef2gtkProfileOutputEnv)); explicit != "" {
		return explicit
	}
	metadata, ok := logging.SessionMetadataFromContext(e.ctx)
	if !ok || metadata.ID == "" {
		if e.profileLogDir == "" {
			return ""
		}
		return filepath.Join(e.profileLogDir, "cef2gtk_profile.jsonl")
	}
	logDir := e.profileLogDir
	if metadata.LogPath != "" {
		logDir = filepath.Dir(metadata.LogPath)
	}
	if logDir == "" {
		return ""
	}
	return filepath.Join(logDir, fmt.Sprintf("session_%s_cef2gtk_profile.jsonl", metadata.ID))
}
