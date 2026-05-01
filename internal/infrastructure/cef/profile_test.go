package cef

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

func TestCEF2GTKProfilePathUsesSessionLogDirectory(t *testing.T) {
	t.Setenv(cef2gtkProfileEnv, "1")
	logDir := t.TempDir()
	ctx := logging.WithSessionMetadata(context.Background(), "20260430_120000_abcd", filepath.Join(logDir, "session_20260430_120000_abcd.log"))
	eng := &Engine{ctx: ctx, profileLogDir: filepath.Join(t.TempDir(), "fallback")}

	got := eng.cef2gtkProfilePath()
	want := filepath.Join(logDir, "session_20260430_120000_abcd_cef2gtk_profile.jsonl")
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestCEF2GTKProfilePathUsesEngineProfileLogDirFallback(t *testing.T) {
	profileLogDir := t.TempDir()
	eng := &Engine{ctx: context.Background(), profileLogDir: profileLogDir}

	got := eng.cef2gtkProfilePath()
	want := filepath.Join(profileLogDir, "cef2gtk_profile.jsonl")
	if got != want {
		t.Fatalf("path = %q, want fallback output", got)
	}
}

func TestCEF2GTKProfilePathAllowsExplicitOutput(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "profile.jsonl")
	t.Setenv(cef2gtkProfileOutputEnv, explicit)
	eng := &Engine{ctx: context.Background()}

	if got := eng.cef2gtkProfilePath(); got != explicit {
		t.Fatalf("path = %q, want explicit output", got)
	}
}

func TestCEF2GTKProfileOptionsDisabledByDefault(t *testing.T) {
	eng := &Engine{ctx: context.Background()}
	wv := &WebView{id: port.WebViewID(7)}

	if opts := eng.cef2gtkProfileOptions(wv); opts.Enabled {
		t.Fatal("profile options enabled without env")
	}
}

func TestLockedProfileWriterWriteReturnsErrorWhenClosed(t *testing.T) {
	var nilWriter *lockedProfileWriter
	if _, err := nilWriter.Write([]byte("x")); !errors.Is(err, errProfileWriterClosed) {
		t.Fatalf("nil writer error = %v, want %v", err, errProfileWriterClosed)
	}

	writer := &lockedProfileWriter{}
	if _, err := writer.Write([]byte("x")); !errors.Is(err, errProfileWriterClosed) {
		t.Fatalf("closed writer error = %v, want %v", err, errProfileWriterClosed)
	}
}
