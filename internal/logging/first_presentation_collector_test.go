package logging

import (
	"encoding/json"
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

func TestFirstPresentationCollectorNeverRecursivelyDeletesCallerOutput(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	script, err := os.ReadFile(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	require.NoError(t, err)
	require.NotContains(t, string(script), "rm -rf \"$output\"")
	require.NotContains(t, string(script), "rm -rf -- \"$output\"")
}

func TestFirstPresentationCollectorRejectsUnsafeOutputPaths(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := filepath.Join(temp, "cef-147-runtime")
	require.NoError(t, os.Mkdir(runtime, 0o755))
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
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := filepath.Join(temp, "cef-147-runtime")
	require.NoError(t, os.Mkdir(runtime, 0o755))

	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))

	output := filepath.Join(temp, "artifacts")
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(os.Environ(),
		"DISPLAY=:test",
		"WAYLAND_DISPLAY=",
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+output,
		"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
		"DUMBER_MACHINE_GPU_PROFILE=integrated-gpu",
	)
	result, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "collector failed: %s", result)

	var artifacts strings.Builder
	for _, name := range append([]string{"metadata.json", "baseline.json"}, runArtifactNames()...) {
		artifactContents, readErr := os.ReadFile(filepath.Join(output, name))
		require.NoError(t, readErr)
		artifacts.Write(artifactContents)
	}
	for _, forbidden := range []string{"/home/", temp, "alice", "machine_path", `"time"`} {
		require.NotContainsf(t, artifacts.String(), forbidden, "committed artifact leaked %q", forbidden)
	}
	for _, required := range []string{
		`"measured_source_revision"`,
		`"tag": "v0.8.4-0.20260714143951-2a5b796c8bef"`,
		`"revision": "2a5b796c8befa686b663ecfba4fb00dcd870d539"`,
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

func TestFirstPresentationCollectorDefaultsToXDGStateEvidenceDirectory(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := filepath.Join(temp, "cef-147-runtime")
	require.NoError(t, os.Mkdir(runtime, 0o755))
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))
	stateHome := filepath.Join(temp, "state")

	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(envWithout("DUMBER_FIRST_PRESENTATION_OUTPUT"),
		"DISPLAY=:test",
		"WAYLAND_DISPLAY=",
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
		"DUMBER_MACHINE_GPU_PROFILE=integrated-gpu",
		"XDG_STATE_HOME="+stateHome,
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

func TestFirstPresentationCollectorReadsProvenanceWithScopedGitSafeDirectory(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := filepath.Join(temp, "cef-147-runtime")
	require.NoError(t, os.Mkdir(runtime, 0o755))
	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, collectorTestBinary(validFirstPresentationLog), 0o755))
	gitDir := filepath.Join(temp, "bin")
	require.NoError(t, os.Mkdir(gitDir, 0o755))
	git := filepath.Join(gitDir, "git")
	require.NoError(t, os.WriteFile(git, []byte(`#!/bin/sh
if [ "$1" = "-c" ] && [ "$2" = "safe.directory=$EXPECTED_SAFE_DIRECTORY" ] && [ "$3" = "rev-parse" ] && [ "$4" = "HEAD" ]; then
  echo 0123456789abcdef0123456789abcdef01234567
  exit 0
fi
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
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+output,
		"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
		"EXPECTED_SAFE_DIRECTORY="+repoRoot,
		"PATH="+gitDir+":"+os.Getenv("PATH"),
	)
	result, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "collector failed: %s", result)

	var metadata struct {
		MeasuredSourceRevision string `json:"measured_source_revision"`
	}
	contents, err := os.ReadFile(filepath.Join(output, "metadata.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(contents, &metadata))
	require.Equal(t, "0123456789abcdef0123456789abcdef01234567", metadata.MeasuredSourceRevision)
}

func TestFirstPresentationCollectorRejectsInconsistentTiming(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
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
			runtime := filepath.Join(temp, "cef-147-runtime")
			require.NoError(t, os.Mkdir(runtime, 0o755))
			binary := filepath.Join(temp, "dumber")
			log := strings.Replace(validFirstPresentationLog, test.old, test.new, 1)
			require.NoError(t, os.WriteFile(binary, collectorTestBinary(log), 0o755))

			cmd := exec.Command(filepath.Join(repoRoot, "scripts", "collect_first_presentation.sh"))
			cmd.Dir = repoRoot
			cmd.Env = append(os.Environ(),
				"DISPLAY=:test",
				"DUMBER_CEF_DIR="+runtime,
				"DUMBER_FIRST_PRESENTATION_BIN="+binary,
				"DUMBER_FIRST_PRESENTATION_OUTPUT="+filepath.Join(temp, "artifacts"),
				"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
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
