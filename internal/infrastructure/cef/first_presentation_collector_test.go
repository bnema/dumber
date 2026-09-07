package cef

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const validFirstPresentationLog = `{"message":"startup_trace: milestone","milestone":"process_entry","t_ms":0,"delta_ms":0,"time":"2026-07-14T09:00:00+02:00","machine_path":"/home/alice/private"}
{"message":"startup_trace: milestone","milestone":"config_complete","t_ms":1,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"cef_library_load_begin","t_ms":2,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"cef_initialized","t_ms":3,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"browser_create_requested","t_ms":4,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"first_accelerated_paint_received","t_ms":5,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"first_dmabuf_texture_swap","t_ms":6,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"first_gtk_presentation","t_ms":7,"delta_ms":1}
{"message":"startup_trace: first presentation","backend":"gdk-dmabuf","incomplete_reason":"","total_ms":7,"host":"alice"}`

const (
	testUpstreamVersion  = "v0.8.5-0.20300102030405-bbd397409ebe"
	testUpstreamRevision = "bbd397409ebed75a5979c1e4566a2ef319f6a484"
	testSourceRevision   = "0123456789abcdef0123456789abcdef01234567"
)

func TestFirstPresentationCollectorNeverRecursivelyDeletesCallerOutput(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	script, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	require.NoError(t, err)
	require.NotContains(t, string(script), "rm -rf \"$output\"")
	require.NotContains(t, string(script), "rm -rf -- \"$output\"")
}

func TestFirstPresentationCollectorHasNoHardcodedRuntimeDefaults(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	script, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	require.NoError(t, err)
	require.NotContains(t, string(script), "cef-147")
	require.NotContains(t, string(script), `"version": "147"`)
	require.NotContains(t, string(script), "go list -m")
	require.NotContains(t, string(script), "go mod download")
	require.NotContains(t, string(script), "-mod=mod")
	require.Contains(t, string(script), "DUMBER_CEF_DIR must be set")
	require.Contains(t, string(script), "cef_runtime_probe.py")
	require.Contains(t, string(script), `CEF_DIR="$selected_cef_dir"`)
}

func TestFirstPresentationCollectorRejectsUnsafeOutputPaths(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := collectorFakeCEFRuntime(t, 150)
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))
	home := filepath.Join(temp, "home")
	require.NoError(t, os.Mkdir(home, 0o755))

	unrelated := filepath.Join(temp, "unrelated")
	require.NoError(t, os.Mkdir(unrelated, 0o755))
	unrelatedSentinel := filepath.Join(unrelated, "keep")
	require.NoError(t, os.WriteFile(unrelatedSentinel, []byte("do not delete"), 0o600))

	safeParent := filepath.Join(temp, "safe")
	require.NoError(t, os.Mkdir(safeParent, 0o755))
	parentSentinel := filepath.Join(safeParent, "keep")
	require.NoError(t, os.WriteFile(parentSentinel, []byte("do not delete"), 0o600))

	external := filepath.Join(temp, "external")
	require.NoError(t, os.Mkdir(external, 0o755))
	externalSentinel := filepath.Join(external, "keep")
	require.NoError(t, os.WriteFile(externalSentinel, []byte("do not delete"), 0o600))
	link := filepath.Join(temp, "link")
	require.NoError(t, os.Symlink(external, link))

	for _, test := range []struct {
		name   string
		output string
		verify func(t *testing.T)
	}{
		{name: "empty", output: ""},
		{name: "root", output: "/"},
		{name: "home", output: home},
		{name: "existing unrelated directory", output: unrelated, verify: func(t *testing.T) {
			requireFileContents(t, unrelatedSentinel, "do not delete")
		}},
		{name: "parent traversal", output: safeParent + "/../escaped", verify: func(t *testing.T) {
			requireFileContents(t, parentSentinel, "do not delete")
			require.NoFileExists(t, filepath.Join(temp, "escaped"))
		}},
		{name: "symlink escape", output: filepath.Join(link, "escaped"), verify: func(t *testing.T) {
			requireFileContents(t, externalSentinel, "do not delete")
			require.NoFileExists(t, filepath.Join(external, "escaped"))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
			// An isolated directory prevents a regression from reaching the tracked
			// default artifact path while this test proves output validation is first.
			cmd.Dir = temp
			cmd.Env = append(os.Environ(),
				"HOME="+home,
				"DISPLAY=:test",
				"DUMBER_CEF_DIR="+runtime,
				"DUMBER_FIRST_PRESENTATION_BIN="+binary,
				"DUMBER_FIRST_PRESENTATION_OUTPUT="+test.output,
			)
			result, err := cmd.CombinedOutput()
			require.Errorf(t, err, "collector accepted unsafe output %q: %s", test.output, result)
			require.Contains(t, string(result), "unsafe output path")
			if test.verify != nil {
				test.verify(t)
			}
		})
	}
}

func requireFileContents(t *testing.T, path, want string) {
	t.Helper()
	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, want, string(contents))
}

func TestFirstPresentationCollectorSanitizesMachineLocalValues(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := collectorFakeCEFRuntime(t, 150)
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

	goBin := collectorFakeGo(t, temp, testUpstreamVersion, testSourceRevision, false)
	manifest := collectorManifest(t, temp, binary, testSourceRevision, testUpstreamVersion, testUpstreamRevision)
	output := filepath.Join(temp, "artifacts")
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"DISPLAY=:test",
		"WAYLAND_DISPLAY=",
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_BUILD_MANIFEST="+manifest,
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+output,
		"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
		"DUMBER_MACHINE_GPU_PROFILE=integrated-gpu",
		"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
	)
	result, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "collector failed: %s", result)

	var artifacts strings.Builder
	for _, name := range append([]string{"metadata.json", "baseline.json"}, runArtifactNames()...) {
		artifactContents, readErr := os.ReadFile(filepath.Join(output, name))
		require.NoError(t, readErr)
		artifacts.Write(artifactContents)
	}
	for _, forbidden := range []string{"/home/", temp, "alice", "machine_path", `"time"`, runtime, "libcef.so"} {
		require.NotContainsf(t, artifacts.String(), forbidden, "committed artifact leaked %q", forbidden)
	}
	for _, required := range []string{
		`"measured_source_revision"`,
		`"version": "` + testUpstreamVersion + `"`,
		`"tag": "v0.8.5"`,
		`"revision": "` + testUpstreamRevision + `"`,
		`"chrome_major": 150`,
		`"libcef_sha256"`,
	} {
		require.Contains(t, artifacts.String(), required)
	}

	var metadata struct {
		Comparison struct {
			OS                string `json:"os"`
			Architecture      string `json:"architecture"`
			DisplayProtocol   string `json:"display_protocol"`
			MachineGPUProfile string `json:"machine_gpu_profile"`
		} `json:"comparison"`
	}
	contents, err := os.ReadFile(filepath.Join(output, "metadata.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(contents, &metadata))
	require.NotEmpty(t, metadata.Comparison.OS)
	require.NotEmpty(t, metadata.Comparison.Architecture)
	require.Equal(t, "x11", metadata.Comparison.DisplayProtocol)
	require.Equal(t, "integrated-gpu", metadata.Comparison.MachineGPUProfile)
}

func TestFirstPresentationCollectorDerivesSelectedImmutableModuleProvenance(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := collectorFakeCEFRuntime(t, 150)
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

	goBin := collectorFakeGo(t, temp, testUpstreamVersion, testSourceRevision, false)
	manifest := collectorManifest(t, temp, binary, testSourceRevision, testUpstreamVersion, testUpstreamRevision)
	output := filepath.Join(temp, "artifacts")
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"DISPLAY=:test",
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_BUILD_MANIFEST="+manifest,
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+output,
		"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
		"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
	)
	result, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "collector failed: %s", result)

	var metadata struct {
		Upstream struct {
			Module   string `json:"module"`
			Version  string `json:"version"`
			Tag      string `json:"tag"`
			Revision string `json:"revision"`
		} `json:"upstream"`
		MeasuredSourceRevision string `json:"measured_source_revision"`
	}
	contents, err := os.ReadFile(filepath.Join(output, "metadata.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(contents, &metadata))
	require.Equal(t, "github.com/bnema/purego-cef2gtk", metadata.Upstream.Module)
	require.Equal(t, testUpstreamVersion, metadata.Upstream.Version)
	require.Equal(t, "v0.8.5", metadata.Upstream.Tag)
	require.Equal(t, testUpstreamRevision, metadata.Upstream.Revision)
	require.Equal(t, testSourceRevision, metadata.MeasuredSourceRevision)

	script, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	require.NoError(t, err)
	require.NotContains(t, string(script), "f217ece342dea3ef2a3f98671fcd16a39ad0037d")
}

func TestFirstPresentationCollectorRejectsNonImmutableModuleProvenanceWithoutLeaks(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)

	for _, test := range []struct {
		name, version string
	}{
		{name: "branch selector", version: "main"},
		{name: "devel version", version: "(devel)"},
		{name: "missing patch", version: "v0.8"},
	} {
		t.Run(test.name, func(t *testing.T) {
			temp := t.TempDir()
			runtime := collectorFakeCEFRuntime(t, 150)
			binary := filepath.Join(temp, "dumber")
			require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

			goBin := collectorFakeGo(t, temp, test.version, testSourceRevision, false)
			manifest := collectorManifest(t, temp, binary, testSourceRevision, testUpstreamVersion, testUpstreamRevision)
			output := filepath.Join(temp, "artifacts")
			cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
			cmd.Dir = repoRoot
			cmd.Env = append(os.Environ(),
				"DISPLAY=:test",
				"DUMBER_CEF_DIR="+runtime,
				"DUMBER_FIRST_PRESENTATION_BIN="+binary,
				"DUMBER_BUILD_MANIFEST="+manifest,
				"DUMBER_FIRST_PRESENTATION_OUTPUT="+output,
				"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
			)
			result, runErr := cmd.CombinedOutput()
			require.Errorf(t, runErr, "collector accepted nonimmutable module provenance: %s", result)
			require.Contains(t, string(result), "immutable module provenance is unavailable")
			require.NotContains(t, string(result), temp)
			require.NoFileExists(t, filepath.Join(output, "metadata.json"))
			for _, name := range runArtifactNames() {
				require.NoFileExists(t, filepath.Join(output, name))
			}
		})
	}
}

func TestFirstPresentationCollectorRequiresExplicitRuntimeDir(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	env := envWithout("DUMBER_CEF_DIR")
	env = append(env,
		"DISPLAY=:test",
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+filepath.Join(temp, "artifacts"),
	)
	cmd.Env = env
	result, err := cmd.CombinedOutput()
	require.Errorf(t, err, "collector accepted missing runtime dir: %s", result)
	require.Contains(t, string(result), "DUMBER_CEF_DIR must be set")
}

func TestFirstPresentationCollectorIgnoresConflictingParentCEFDir(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := collectorFakeCEFRuntime(t, 150)
	bogus := filepath.Join(temp, "bogus-runtime")
	require.NoError(t, os.Mkdir(bogus, 0o755))
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

	goBin := collectorFakeGo(t, temp, testUpstreamVersion, testSourceRevision, false)
	manifest := collectorManifest(t, temp, binary, testSourceRevision, testUpstreamVersion, testUpstreamRevision)
	output := filepath.Join(temp, "artifacts")
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"DISPLAY=:test",
		"CEF_DIR="+bogus,
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_BUILD_MANIFEST="+manifest,
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+output,
		"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
		"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
	)
	result, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "collector failed with conflicting parent CEF_DIR: %s", result)
	require.FileExists(t, filepath.Join(output, "metadata.json"))
}

func TestFirstPresentationCollectorRejectsProbeFailure(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := filepath.Join(temp, "empty-runtime")
	require.NoError(t, os.Mkdir(runtime, 0o755))
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

	goBin := collectorFakeGo(t, temp, testUpstreamVersion, testSourceRevision, false)
	manifest := collectorManifest(t, temp, binary, testSourceRevision, testUpstreamVersion, testUpstreamRevision)
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"DISPLAY=:test",
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_BUILD_MANIFEST="+manifest,
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+filepath.Join(temp, "artifacts"),
		"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
	)
	result, err := cmd.CombinedOutput()
	require.Errorf(t, err, "collector accepted missing runtime library: %s", result)
	require.Contains(t, string(result), "CEF runtime probe failed")
}

func TestFirstPresentationCollectorRejectsUnsupportedRuntime(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := collectorFakeCEFRuntime(t, 140)
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

	goBin := collectorFakeGo(t, temp, testUpstreamVersion, testSourceRevision, false)
	manifest := collectorManifest(t, temp, binary, testSourceRevision, testUpstreamVersion, testUpstreamRevision)
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"DISPLAY=:test",
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_BUILD_MANIFEST="+manifest,
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+filepath.Join(temp, "artifacts"),
		"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
	)
	result, err := cmd.CombinedOutput()
	require.Errorf(t, err, "collector accepted unsupported runtime: %s", result)
	require.Contains(t, string(result), "unsupported CEF runtime")
}

func TestFirstPresentationCollectorRequiresManifestWithoutEmbeddedVCS(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := collectorFakeCEFRuntime(t, 150)
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

	goBin := collectorFakeGo(t, temp, testUpstreamVersion, "", false)
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"DISPLAY=:test",
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_BUILD_MANIFEST="+filepath.Join(temp, "missing-manifest.json"),
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+filepath.Join(temp, "artifacts"),
		"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
	)
	result, err := cmd.CombinedOutput()
	require.Errorf(t, err, "collector accepted binary without VCS or manifest: %s", result)
	require.Contains(t, string(result), "build manifest is required")
}

func TestFirstPresentationCollectorRejectsManifestMismatch(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)

	for _, test := range []struct {
		name           string
		sourceRevision string
		depVersion     string
		depRevision    string
		mutateBinary   bool
	}{
		{name: "binary checkout mismatch", sourceRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", depVersion: testUpstreamVersion, depRevision: testUpstreamRevision},
		{name: "dependency version mismatch", sourceRevision: testSourceRevision, depVersion: "v0.9.3", depRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		{name: "dependency revision mismatch", sourceRevision: testSourceRevision, depVersion: testUpstreamVersion, depRevision: "cccccccccccccccccccccccccccccccccccccccc"},
	} {
		t.Run(test.name, func(t *testing.T) {
			temp := t.TempDir()
			runtime := collectorFakeCEFRuntime(t, 150)
			binary := filepath.Join(temp, "dumber")
			require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

			goBin := collectorFakeGo(t, temp, testUpstreamVersion, testSourceRevision, false)
			manifest := collectorManifest(t, temp, binary, test.sourceRevision, test.depVersion, test.depRevision)
			if test.mutateBinary {
				require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog+"extra"), 0o755))
			}
			cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
			cmd.Dir = repoRoot
			cmd.Env = append(os.Environ(),
				"DISPLAY=:test",
				"DUMBER_CEF_DIR="+runtime,
				"DUMBER_FIRST_PRESENTATION_BIN="+binary,
				"DUMBER_BUILD_MANIFEST="+manifest,
				"DUMBER_FIRST_PRESENTATION_OUTPUT="+filepath.Join(temp, "artifacts"),
				"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
			)
			result, err := cmd.CombinedOutput()
			require.Errorf(t, err, "collector accepted mismatched manifest: %s", result)
			require.Contains(t, string(result), "build manifest does not match")
		})
	}
}

func TestFirstPresentationCollectorRejectsReplacementContamination(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := collectorFakeCEFRuntime(t, 150)
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

	goBin := collectorFakeGo(t, temp, testUpstreamVersion, testSourceRevision, true)
	manifest := collectorManifest(t, temp, binary, testSourceRevision, testUpstreamVersion, testUpstreamRevision)
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"DISPLAY=:test",
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_BUILD_MANIFEST="+manifest,
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+filepath.Join(temp, "artifacts"),
		"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
	)
	result, err := cmd.CombinedOutput()
	require.Errorf(t, err, "collector accepted replacement contamination: %s", result)
	require.Contains(t, string(result), "immutable module provenance is unavailable")
}

// collectorFakeGo fakes `go version -m <binary>` for the measured binary.
// It never consults the current checkout module graph.
func collectorFakeGo(t *testing.T, temp, depVersion, vcsRevision string, replacement bool) string {
	t.Helper()
	binDir := filepath.Join(temp, "gobin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	goBin := filepath.Join(binDir, "go")
	depLine := fmt.Sprintf("dep\tgithub.com/bnema/purego-cef2gtk\t%s", depVersion)
	if replacement {
		depLine += " => ./local-override"
	}
	buildLine := ""
	if vcsRevision != "" {
		buildLine = fmt.Sprintf("build\tvcs.revision=%s", vcsRevision)
	}
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "version" ] && [ "$2" = "-m" ]; then
  printf '%%s\n' 'path\tgithub.com/bnema/dumber/cmd/dumber' 'mod\tgithub.com/bnema/dumber\t(devel)' '%s' '%s'
  exit 0
fi
exit 1
`, depLine, buildLine)
	require.NoError(t, os.WriteFile(goBin, []byte(script), 0o755))
	return goBin
}

func collectorManifest(t *testing.T, temp, binary, sourceRevision, depVersion, depRevision string) string {
	t.Helper()
	contents, err := os.ReadFile(binary)
	require.NoError(t, err)
	sum := sha256.Sum256(contents)
	manifest := map[string]any{
		"binary_sha256":   hex.EncodeToString(sum[:]),
		"source_revision": sourceRevision,
		"modules": map[string]any{
			"github.com/bnema/purego-cef2gtk": map[string]string{
				"version":  depVersion,
				"revision": depRevision,
			},
		},
	}
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	path := filepath.Join(temp, "build-manifest.json")
	require.NoError(t, os.WriteFile(path, raw, 0o600))
	return path
}

// collectorFakeCEFRuntime builds a tiny shared library exposing the
// header-verified cef_version_info(int) entries with a controlled Chrome
// major. It exercises the real ctypes probe path without copying a full
// multi-hundred-megabyte runtime per test.
func collectorFakeCEFRuntime(t *testing.T, chromeMajor int) string {
	t.Helper()
	runtime := t.TempDir()
	source := fmt.Sprintf(`int cef_version_info(int entry) {
  switch (entry) {
    case 0: return 150;
    case 1: return 0;
    case 2: return 0;
    case 3: return 0;
    case 4: return %d;
    case 5: return 0;
    case 6: return 0;
    case 7: return 0;
    default: return 0;
  }
}
`, chromeMajor)
	src := filepath.Join(runtime, "fake_cef.c")
	require.NoError(t, os.WriteFile(src, []byte(source), 0o600))
	lib := filepath.Join(runtime, "libcef.so")
	cmd := exec.Command("gcc", "-shared", "-fPIC", "-o", lib, src)
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "failed to build fake libcef: %s", out)
	return runtime
}

func TestFirstPresentationCollectorDefaultsToXDGStateEvidenceDirectory(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := collectorFakeCEFRuntime(t, 150)
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))
	stateHome := filepath.Join(temp, "state")
	goBin := collectorFakeGo(t, temp, testUpstreamVersion, testSourceRevision, false)
	manifest := collectorManifest(t, temp, binary, testSourceRevision, testUpstreamVersion, testUpstreamRevision)

	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(envWithout("DUMBER_FIRST_PRESENTATION_OUTPUT"),
		"DISPLAY=:test",
		"WAYLAND_DISPLAY=",
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_BUILD_MANIFEST="+manifest,
		"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
		"DUMBER_MACHINE_GPU_PROFILE=integrated-gpu",
		"XDG_STATE_HOME="+stateHome,
		"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
	)
	result, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "collector failed: %s", result)

	evidenceRoot := filepath.Join(stateHome, "dumber", "roadmap-evidence")
	entries, err := os.ReadDir(evidenceRoot)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.True(t, entries[0].IsDir())
	require.FileExists(t, filepath.Join(evidenceRoot, entries[0].Name(), "metadata.json"))
	require.NotContains(t, string(result), filepath.Join(repoRoot, "phase1"))
}

func envWithout(name string) []string {
	prefix := name + "="
	var environment []string
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, prefix) {
			environment = append(environment, entry)
		}
	}
	return environment
}

func TestFirstPresentationCollectorBindsBinaryNotCheckoutGit(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := collectorFakeCEFRuntime(t, 150)
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))
	goBin := collectorFakeGo(t, temp, testUpstreamVersion, testSourceRevision, false)
	manifest := collectorManifest(t, temp, binary, testSourceRevision, testUpstreamVersion, testUpstreamRevision)
	gitDir := filepath.Join(temp, "gitbin")
	require.NoError(t, os.Mkdir(gitDir, 0o755))
	git := filepath.Join(gitDir, "git")
	require.NoError(t, os.WriteFile(git, []byte(`#!/bin/sh
echo "fatal: detected dubious ownership in repository" >&2
exit 128
`), 0o755))

	output := filepath.Join(temp, "artifacts")
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"DISPLAY=:test",
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_BUILD_MANIFEST="+manifest,
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+output,
		"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
		"PATH="+gitDir+":"+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
	)
	result, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "collector depends on checkout git: %s", result)

	var metadata struct {
		MeasuredSourceRevision string `json:"measured_source_revision"`
	}
	contents, err := os.ReadFile(filepath.Join(output, "metadata.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(contents, &metadata))
	require.Equal(t, testSourceRevision, metadata.MeasuredSourceRevision)
}

func TestFirstPresentationCollectorRejectsInconsistentTiming(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)

	for _, test := range []struct {
		name string
		old  string
		new  string
	}{
		{name: "non-integer delta", old: `"delta_ms":1}`, new: `"delta_ms":"1"}`},
		{name: "incorrect delta", old: `"delta_ms":1}`, new: `"delta_ms":2}`},
		{name: "non-integer summary total", old: `"total_ms":7`, new: `"total_ms":7.5`},
		{name: "incorrect summary total", old: `"total_ms":7`, new: `"total_ms":8`},
	} {
		t.Run(test.name, func(t *testing.T) {
			temp := t.TempDir()
			runtime := collectorFakeCEFRuntime(t, 150)
			binary := filepath.Join(temp, "dumber")
			log := strings.Replace(validFirstPresentationLog, test.old, test.new, 1)
			require.NoError(t, os.WriteFile(binary, collectorTestBinary(log), 0o755))
			goBin := collectorFakeGo(t, temp, testUpstreamVersion, testSourceRevision, false)
			manifest := collectorManifest(t, temp, binary, testSourceRevision, testUpstreamVersion, testUpstreamRevision)

			cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
			cmd.Dir = repoRoot
			cmd.Env = append(os.Environ(),
				"DISPLAY=:test",
				"DUMBER_CEF_DIR="+runtime,
				"DUMBER_FIRST_PRESENTATION_BIN="+binary,
				"DUMBER_BUILD_MANIFEST="+manifest,
				"DUMBER_FIRST_PRESENTATION_OUTPUT="+filepath.Join(temp, "artifacts"),
				"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
				"PATH="+filepath.Dir(goBin)+":"+os.Getenv("PATH"),
			)
			result, err := cmd.CombinedOutput()
			require.Errorf(t, err, "collector accepted malformed timing artifact: %s", result)
			require.Contains(t, string(result), "invalid or incomplete non-DMABUF timeline")
		})
	}
}

func collectorTestBinary(log string) []byte {
	return []byte("#!/bin/sh\ncat <<'EOF'\n" + log + "\nEOF\nexit 139\n")
}

func runArtifactNames() []string {
	return []string{"run-01.json", "run-02.json", "run-03.json", "run-04.json", "run-05.json"}
}
