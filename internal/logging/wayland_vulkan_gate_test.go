package logging

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWaylandVulkanGateRejectsIncompleteLifecycleAndGPU1002(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	for _, tc := range []struct {
		name string
		log  string
		want string
	}{
		{name: "missing startup milestone", log: strings.Replace(gateLog(), `echo '{"message":"startup_trace: milestone","milestone":"process_entry","t_ms":0,"delta_ms":0}'`, "", 1), want: "incomplete Wayland Vulkan lifecycle"},
		{name: "out of order startup milestone", log: strings.Replace(gateLog(), `"milestone":"config_complete"`, `"milestone":"cef_initialized"`, 1), want: "incomplete Wayland Vulkan lifecycle"},
		{name: "missing dmabuf import", log: gateLog("dmabuf"), want: "incomplete Wayland Vulkan lifecycle"},
		{name: "missing vulkan renderer", log: gateLog("vulkan"), want: "incomplete Wayland Vulkan lifecycle"},
		{name: "missing resize", log: gateLog("resize"), want: "incomplete Wayland Vulkan lifecycle"},
		{name: "missing OnBeforeClose", log: gateLog("before-close"), want: "incomplete Wayland Vulkan lifecycle"},
		{name: "missing Engine.Close", log: gateLog("engine-close"), want: "incomplete Wayland Vulkan lifecycle"},
		{name: "GPU error 1002", log: gateLog() + `{"message":"GPU process error 1002"}` + "\n", want: "CEF GPU error 1002 (blocked)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			output, result := runGate(t, repoRoot, tc.log, 5)
			require.Error(t, result)
			require.Contains(t, string(output), tc.want)
		})
	}
}

func TestWaylandVulkanGateProducesOnlySanitizedPairedJSON(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	output, result := runGate(t, repoRoot, gateLog(), 5)
	require.NoErrorf(t, result, "gate failed: %s", output)

	root := gateOutput(t, string(output))
	for _, side := range []string{"baseline", "candidate"} {
		entries, err := os.ReadDir(filepath.Join(root, side))
		require.NoError(t, err)
		require.Len(t, entries, 7)
		for _, entry := range entries {
			contents, readErr := os.ReadFile(filepath.Join(root, side, entry.Name()))
			require.NoError(t, readErr)
			var value any
			require.NoError(t, json.Unmarshal(contents, &value), "artifact %s is not JSON", entry.Name())
			for _, forbidden := range []string{"/home/", "private-host", "raw-log"} {
				require.NotContains(t, string(contents), forbidden)
			}
		}
	}
	require.FileExists(t, filepath.Join(root, "result.json"))
}

func TestWaylandVulkanGateRejectsUnsafeOrReusedOutput(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	temp := t.TempDir()
	unsafe := filepath.Join(repoRoot, "unsafe-gate-artifacts")
	binary := gateBinary(t, temp, gateLog())
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "wayland_vulkan_gate.sh"))
	cmd.Dir = repoRoot
	cmd.Env = gateEnv(repoRoot, binary, unsafe)
	result, err := cmd.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(result), "output must be external")
}

func TestCompareFirstPresentationRejectsUnpairedProvenanceAndRegression(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	root := t.TempDir()
	baseline, candidate := filepath.Join(root, "baseline"), filepath.Join(root, "candidate")
	writeComparisonEvidence(t, baseline, []int{10, 10, 10, 10, 10}, "intel-integrated")
	writeComparisonEvidence(t, candidate, []int{10, 10, 10, 10, 12}, "intel-integrated")
	cmd := exec.Command("python3", filepath.Join(repoRoot, "scripts", "compare_first_presentation.py"), baseline, candidate)
	result, err := cmd.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(result), "candidate p95 regression exceeds 10%")

	require.NoError(t, os.RemoveAll(candidate))
	writeComparisonEvidence(t, candidate, []int{12, 12, 12, 12, 12}, "intel-integrated")
	cmd = exec.Command("python3", filepath.Join(repoRoot, "scripts", "compare_first_presentation.py"), baseline, candidate)
	result, err = cmd.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(result), "candidate median regression exceeds 10%")

	require.NoError(t, os.RemoveAll(candidate))
	writeComparisonEvidence(t, candidate, []int{10, 10, 10, 10, 10}, "amd")
	cmd = exec.Command("python3", filepath.Join(repoRoot, "scripts", "compare_first_presentation.py"), baseline, candidate)
	result, err = cmd.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(result), "unpaired provenance")
}

func TestWaylandVulkanGateFailsTimeout(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	output, result := runGate(t, repoRoot, "sleep 2", 1)
	require.Error(t, result)
	require.Contains(t, string(output), "timed out")
}

func runGate(t *testing.T, repoRoot, log string, timeout int) ([]byte, error) {
	t.Helper()
	temp := t.TempDir()
	binary := gateBinary(t, temp, log)
	output := filepath.Join(temp, "evidence")
	cmd := exec.Command(filepath.Join(repoRoot, "scripts", "wayland_vulkan_gate.sh"))
	cmd.Dir = repoRoot
	cmd.Env = append(gateEnv(repoRoot, binary, output), "DUMBER_WAYLAND_VULKAN_TIMEOUT_SECONDS="+strconv.Itoa(timeout))
	return cmd.CombinedOutput()
}

func gateEnv(repoRoot, binary, output string) []string {
	runtime := filepath.Join(filepath.Dir(binary), "cef")
	_ = os.Mkdir(runtime, 0o755)
	return append(os.Environ(),
		"WAYLAND_DISPLAY=wayland-test", "XDG_RUNTIME_DIR=/tmp", "DUMBER_CEF_DIR="+runtime,
		"DUMBER_WAYLAND_VULKAN_BASELINE_BIN="+binary, "DUMBER_WAYLAND_VULKAN_CANDIDATE_BIN="+binary,
		"DUMBER_WAYLAND_VULKAN_BASELINE_SOURCE="+repoRoot, "DUMBER_WAYLAND_VULKAN_CANDIDATE_SOURCE="+repoRoot,
		"DUMBER_WAYLAND_VULKAN_OUTPUT="+output, "DUMBER_MACHINE_GPU_PROFILE=intel-integrated",
	)
}

func gateBinary(t *testing.T, directory, log string) string {
	t.Helper()
	path := filepath.Join(directory, "dumber")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+log+"\n"), 0o755))
	return path
}

func gateLog(without ...string) string {
	skip := map[string]bool{}
	for _, value := range without {
		skip[value] = true
	}
	var lines []string
	milestones := []string{"process_entry", "config_complete", "cef_library_load_begin", "cef_initialized", "browser_create_requested", "first_accelerated_paint_received", "first_dmabuf_texture_swap", "first_gtk_presentation"}
	for index, milestone := range milestones {
		delta := 1
		if index == 0 {
			delta = 0
		}
		lines = append(lines, `echo '{"message":"startup_trace: milestone","milestone":"`+milestone+`","t_ms":`+strconv.Itoa(index)+`,"delta_ms":`+strconv.Itoa(delta)+`}'`)
	}
	lines = append(lines, `echo '{"message":"startup_trace: first presentation","backend":"gdk-dmabuf","incomplete_reason":"","total_ms":7}'`)
	if !skip["dmabuf"] {
		lines = append(lines, `echo '{"message":"wayland_vulkan_gate: dmabuf import","backend":"gdk-dmabuf"}'`)
	}
	if !skip["vulkan"] {
		lines = append(lines, `echo '{"message":"wayland_vulkan_gate: vulkan renderer","renderer":"vulkan"}'`)
	}
	if !skip["resize"] {
		lines = append(lines, `echo '{"message":"wayland_vulkan_gate: resize"}'`)
	}
	if !skip["before-close"] {
		lines = append(lines, `echo '{"message":"wayland_vulkan_gate: OnBeforeClose"}'`)
	}
	if !skip["engine-close"] {
		lines = append(lines, `echo '{"message":"wayland_vulkan_gate: Engine.Close"}'`)
	}
	return strings.Join(lines, "\n")
}

func gateOutput(t *testing.T, output string) string {
	t.Helper()
	prefix := "wayland-vulkan gate artifacts: "
	index := strings.LastIndex(output, prefix)
	require.NotEqual(t, -1, index)
	return strings.TrimSpace(output[index+len(prefix):])
}

func writeComparisonEvidence(t *testing.T, root string, values []int, profile string) {
	t.Helper()
	require.NoError(t, os.Mkdir(root, 0o755))
	metadata := map[string]any{"upstream": map[string]any{"revision": "2a5b796c8befa686b663ecfba4fb00dcd870d539"}, "runtime": map[string]any{"version": "147"}, "comparison": map[string]any{"machine_gpu_profile": profile}, "render_configuration": map[string]any{"renderer": "vulkan"}}
	writeJSON(t, filepath.Join(root, "metadata.json"), metadata)
	for i, value := range values {
		name := "run-" + strconv.FormatInt(int64(i+1), 10)
		if i < 9 {
			name = "run-0" + strconv.Itoa(i+1)
		}
		writeJSON(t, filepath.Join(root, name+".json"), map[string]any{"run": name, "valid": true, "summary": map[string]any{"total_ms": value}})
	}
	ordered := append([]int(nil), values...)
	// Inputs are already ordered in this test; five makes median index 2 and p95 index 4.
	writeJSON(t, filepath.Join(root, "baseline.json"), map[string]any{"runs": 5, "total_ms": map[string]any{"min": ordered[0], "median": ordered[2], "max": ordered[4], "p95": ordered[4]}})
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	contents, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, contents, 0o600))
}
