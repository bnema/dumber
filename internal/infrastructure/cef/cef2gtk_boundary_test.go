package cef

import (
	"os/exec"
	"strings"
	"testing"
)

func TestCEF2GTKImportStaysInInfrastructureCEF(t *testing.T) {
	cmd := exec.Command("go", "list", "-mod=mod", "-f", "{{.ImportPath}} {{join .Imports \" \"}}", "./...")
	// Tests in this package run from internal/infrastructure/cef; ../../.. is the
	// repository root for go list ./....
	cmd.Dir = "../../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list imports: %v", err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if !strings.Contains(line, "github.com/bnema/purego-cef2gtk") {
			continue
		}
		if strings.HasPrefix(line, "github.com/bnema/dumber/internal/infrastructure/cef ") {
			continue
		}
		t.Fatalf("purego-cef2gtk imported outside cef infrastructure: %s", line)
	}
}
