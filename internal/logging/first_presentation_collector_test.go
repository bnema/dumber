package logging

import (
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
		"DUMBER_CEF_DIR="+runtime,
		"DUMBER_FIRST_PRESENTATION_BIN="+binary,
		"DUMBER_FIRST_PRESENTATION_OUTPUT="+output,
		"DUMBER_FIRST_PRESENTATION_TIMEOUT_SECONDS=1",
	)
	result, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "collector failed: %s", result)

	var artifacts strings.Builder
	for _, name := range append([]string{"metadata.json", "baseline.json"}, runArtifactNames()...) {
		contents, err := os.ReadFile(filepath.Join(output, name))
		require.NoError(t, err)
		artifacts.Write(contents)
	}
	for _, forbidden := range []string{"/home/", temp, "alice", "machine_path", `"time"`} {
		require.NotContainsf(t, artifacts.String(), forbidden, "committed artifact leaked %q", forbidden)
	}
	for _, required := range []string{
		`"measured_source_revision"`,
		`"tag": "v0.8.4"`,
		`"revision": "f217ece342dea3ef2a3f98671fcd16a39ad0037d"`,
	} {
		require.Contains(t, artifacts.String(), required)
	}
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
