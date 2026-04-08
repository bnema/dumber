package desktop

import "testing"

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
