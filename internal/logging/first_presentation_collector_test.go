package logging

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFirstPresentationCollectorSanitizesMachineLocalValues(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	runtime := filepath.Join(temp, "cef-147-runtime")
	require.NoError(t, os.Mkdir(runtime, 0o755))

	binary := filepath.Join(temp, "dumber")
	require.NoError(t, os.WriteFile(binary, []byte(`#!/bin/sh
cat <<'EOF'
{"message":"startup_trace: milestone","milestone":"process_entry","t_ms":0,"delta_ms":0,"time":"2026-07-14T09:00:00+02:00","machine_path":"/home/alice/private"}
{"message":"startup_trace: milestone","milestone":"config_complete","t_ms":1,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"cef_library_load_begin","t_ms":2,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"cef_initialized","t_ms":3,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"browser_create_requested","t_ms":4,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"first_accelerated_paint_received","t_ms":5,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"first_dmabuf_texture_swap","t_ms":6,"delta_ms":1}
{"message":"startup_trace: milestone","milestone":"first_gtk_presentation","t_ms":7,"delta_ms":1}
{"message":"startup_trace: first presentation","backend":"gdk-dmabuf","incomplete_reason":"","total_ms":7,"host":"alice"}
EOF
exit 139
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
}

func runArtifactNames() []string {
	return []string{"run-01.json", "run-02.json", "run-03.json", "run-04.json", "run-05.json"}
}
