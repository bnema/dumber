package cef

import (
	"reflect"
	"testing"
	"time"
)

func TestSafeChromiumCmdlineFlagsOmitsPathBearingAndUnknownFlags(t *testing.T) {
	cmdline := "/tmp/dumber --type=gpu-process --user-data-dir=/home/user/.config/dumber --use-gl=angle --password=secret --no-sandbox"

	got := safeChromiumCmdlineFlags(cmdline)
	want := []string{"--type=gpu-process", "--use-gl=angle", "--no-sandbox"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("safe flags = %#v, want %#v", got, want)
	}
}

func TestReserveRenderStallProcessDiagnosticsAppliesCooldown(t *testing.T) {
	renderStallProcessDiagnosticsLastUnixNS.Store(0)
	now := time.Unix(100, 0)

	if !reserveRenderStallProcessDiagnostics(now) {
		t.Fatal("first process diagnostic reservation failed")
	}
	if reserveRenderStallProcessDiagnostics(now.Add(renderStallProcessDiagnosticsCooldown / 2)) {
		t.Fatal("process diagnostic reservation inside cooldown succeeded")
	}
	if !reserveRenderStallProcessDiagnostics(now.Add(renderStallProcessDiagnosticsCooldown)) {
		t.Fatal("process diagnostic reservation at cooldown boundary failed")
	}
}
