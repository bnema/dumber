package bootstrap

import "testing"

func TestShouldPreloadLayerShell_WaylandWithoutPreloadReturnsTrue(t *testing.T) {
	env := map[string]string{
		"WAYLAND_DISPLAY": "wayland-1",
		"LD_PRELOAD":      "",
	}

	if !ShouldPreloadLayerShell(env) {
		t.Fatalf("expected preload to be required")
	}
}

func TestFindLayerShellLibrary_UsesLDLibraryPathCandidates(t *testing.T) {
	customCandidate := "/opt/custom/lib/libgtk4-layer-shell.so.0"
	got := FindLayerShellLibrary(map[string]string{
		"LD_LIBRARY_PATH": "/opt/custom/lib",
	}, func(path string) bool {
		return path == customCandidate
	})

	if got != customCandidate {
		t.Fatalf("expected %q, got %q", customCandidate, got)
	}
}

func TestShouldPreloadLayerShell_AlreadyPreloadedReturnsFalse(t *testing.T) {
	env := map[string]string{
		"WAYLAND_DISPLAY": "wayland-1",
		"LD_PRELOAD":      "/usr/lib/libgtk4-layer-shell.so.0",
	}

	if ShouldPreloadLayerShell(env) {
		t.Fatalf("expected preload to be skipped")
	}
}

func TestShouldPreloadLayerShell_NoWaylandReturnsFalse(t *testing.T) {
	if ShouldPreloadLayerShell(map[string]string{}) {
		t.Fatalf("expected preload to be skipped outside Wayland")
	}
}

func TestLayerShellPreloadPresent_DetectsLoadedLibrarySubstring(t *testing.T) {
	env := map[string]string{
		"LD_PRELOAD": "/nix/store/hash-libgtk4-layer-shell.so.0 /tmp/other.so",
	}

	if !LayerShellPreloadPresent(env) {
		t.Fatalf("expected preload presence to be detected")
	}
}

func TestFindLayerShellLibrary_ReturnsFirstExistingCandidate(t *testing.T) {
	got := FindLayerShellLibrary(map[string]string{}, func(path string) bool {
		return path == layerShellLibraryCandidates[1]
	})

	if got != layerShellLibraryCandidates[1] {
		t.Fatalf("expected %q, got %q", layerShellLibraryCandidates[1], got)
	}
}

func TestFindLayerShellLibrary_ReturnsArm64Candidate(t *testing.T) {
	arm64Candidate := "/usr/lib/aarch64-linux-gnu/libgtk4-layer-shell.so.0"
	got := FindLayerShellLibrary(map[string]string{}, func(path string) bool {
		return path == arm64Candidate
	})

	if got != arm64Candidate {
		t.Fatalf("expected %q, got %q", arm64Candidate, got)
	}
}

func TestFindLayerShellLibrary_NoMatchReturnsEmpty(t *testing.T) {
	if got := FindLayerShellLibrary(map[string]string{}, func(string) bool { return false }); got != "" {
		t.Fatalf("expected empty result, got %q", got)
	}
}

func TestLayerShellLibrarySearchPaths_IncludesLDLibraryPathCandidates(t *testing.T) {
	paths := layerShellLibrarySearchPaths(map[string]string{
		"LD_LIBRARY_PATH": "/opt/custom/lib:/nix/store/abc/lib",
	})

	wantCustom := "/opt/custom/lib/libgtk4-layer-shell.so.0"
	wantNix := "/nix/store/abc/lib/libgtk4-layer-shell.so.0"
	if !containsString(paths, wantCustom) {
		t.Fatalf("expected custom search path %q in %#v", wantCustom, paths)
	}
	if !containsString(paths, wantNix) {
		t.Fatalf("expected nix search path %q in %#v", wantNix, paths)
	}
}

func TestLayerShellPreloadEnv_SetsLDPreloadWhenEmpty(t *testing.T) {
	env := LayerShellPreloadEnv(map[string]string{"PATH": "/usr/bin"}, "/usr/lib/libgtk4-layer-shell.so.0")

	if env["LD_PRELOAD"] != "/usr/lib/libgtk4-layer-shell.so.0" {
		t.Fatalf("expected LD_PRELOAD to be set, got %q", env["LD_PRELOAD"])
	}
	if env["PATH"] != "/usr/bin" {
		t.Fatalf("expected unrelated env to be preserved")
	}
}

func TestLayerShellPreloadEnv_PrependsLibraryToExistingLDPreload(t *testing.T) {
	env := LayerShellPreloadEnv(map[string]string{"LD_PRELOAD": "/tmp/other.so"}, "/usr/lib/libgtk4-layer-shell.so.0")

	want := "/usr/lib/libgtk4-layer-shell.so.0 /tmp/other.so"
	if env["LD_PRELOAD"] != want {
		t.Fatalf("expected LD_PRELOAD %q, got %q", want, env["LD_PRELOAD"])
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
