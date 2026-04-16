package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/domain/entity"
)

const testCaptureEnvFile = "DUMBER_TEST_CAPTURE_ENV_FILE"

const testRootCacheEnvVar = "DUMBER_TEST_ROOT_CACHE_PATH"

type testSessionSpawnEnvironment struct {
	rootDir string
}

func (testSessionSpawnEnvironment) RootCacheEnvVar() string {
	return testRootCacheEnvVar
}

func (e testSessionSpawnEnvironment) SessionRootCachePath(sessionID entity.SessionID) string {
	return filepath.Join(e.rootDir, "sessions", string(sessionID))
}

func TestMain(m *testing.M) {
	if envFile := os.Getenv(testCaptureEnvFile); envFile != "" && len(os.Args) >= 2 && os.Args[1] == "browse" {
		if err := os.WriteFile(envFile, []byte(strings.Join(os.Environ(), "\n")), 0o644); err != nil {
			os.Exit(2)
		}
		os.Exit(0)
	}

	os.Exit(m.Run())
}

func TestSanitizedChildEnv_RemovesLayerShellPreloadOnly(t *testing.T) {
	env := sanitizedChildEnv([]string{
		"PATH=/usr/bin",
		"LD_PRELOAD=/tmp/libgtk4-layer-shell.so.0 /tmp/keep.so",
	})

	if len(env) != 2 {
		t.Fatalf("expected two env entries, got %#v", env)
	}
	if env[0] != "PATH=/usr/bin" {
		t.Fatalf("expected PATH to be preserved, got %#v", env)
	}
	if env[1] != "LD_PRELOAD=/tmp/keep.so" {
		t.Fatalf("expected non-layer-shell preload to remain, got %#v", env)
	}
}

func TestSanitizedChildEnv_DropsEmptyLDPreload(t *testing.T) {
	env := sanitizedChildEnv([]string{"LD_PRELOAD=/tmp/libgtk4-layer-shell.so.0"})
	if len(env) != 0 {
		t.Fatalf("expected layer-shell-only preload to be removed, got %#v", env)
	}
}

func TestSanitizedChildEnv_PreservesColonSeparatedEntries(t *testing.T) {
	env := sanitizedChildEnv([]string{"LD_PRELOAD=a.so:/tmp/libgtk4-layer-shell.so.0:b.so"})
	if len(env) != 1 || env[0] != "LD_PRELOAD=a.so b.so" {
		t.Fatalf("expected unrelated colon-separated entries to remain, got %#v", env)
	}
}

func TestWithoutEnvKeys_RemovesExplicitOverrides(t *testing.T) {
	env := withoutEnvKeys([]string{
		"PATH=/usr/bin",
		RestoreSessionEnvVar + "=old-session",
		testRootCacheEnvVar + "=/tmp/old-root",
	}, RestoreSessionEnvVar, testRootCacheEnvVar)

	if len(env) != 1 || env[0] != "PATH=/usr/bin" {
		t.Fatalf("expected override keys to be removed, got %#v", env)
	}
}

func TestSessionSpawner_SpawnWithSession_PassesRestoreSessionAndEngineOverride(t *testing.T) {
	rootDir := filepath.Join(t.TempDir(), "cef-root")
	spawner := NewSessionSpawner(t.Context(), testSessionSpawnEnvironment{rootDir: rootDir})
	childEnvFile := filepath.Join(t.TempDir(), "child-env.txt")
	t.Setenv(testCaptureEnvFile, childEnvFile)
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("ENV", "")

	sessionID := entity.SessionID("session-123")
	requireNoError(t, spawner.SpawnWithSession(sessionID))

	env := readEnvFile(t, childEnvFile)
	wantRoot := filepath.Join(rootDir, "sessions", string(sessionID))
	if got := env[RestoreSessionEnvVar]; got != string(sessionID) {
		t.Fatalf("restore session env = %q, want %q", got, string(sessionID))
	}
	if got := env[testRootCacheEnvVar]; got != wantRoot {
		t.Fatalf("engine state root env = %q, want %q", got, wantRoot)
	}
}

func readEnvFile(t *testing.T, path string) map[string]string {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(path)
		if err == nil {
			env := make(map[string]string)
			for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
				if line == "" {
					continue
				}
				key, value, ok := strings.Cut(line, "=")
				if ok {
					env[key] = value
				}
			}
			return env
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for child env file %q", path)
	return nil
}

func TestSessionSpawner_SpawnWithSession_OmitsEngineOverrideWhenNotConfigured(t *testing.T) {
	spawner := NewSessionSpawner(t.Context(), nil)
	childEnvFile := filepath.Join(t.TempDir(), "child-env.txt")
	t.Setenv(testCaptureEnvFile, childEnvFile)
	t.Setenv("ENV", "")

	sessionID := entity.SessionID("session-plain")
	requireNoError(t, spawner.SpawnWithSession(sessionID))

	env := readEnvFile(t, childEnvFile)
	if got := env[RestoreSessionEnvVar]; got != string(sessionID) {
		t.Fatalf("restore session env = %q, want %q", got, string(sessionID))
	}
	if got := env[testRootCacheEnvVar]; got != "" {
		t.Fatalf("engine state root env = %q, want empty", got)
	}
}

func requireNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
