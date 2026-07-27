package main

import "testing"

func TestRuntimeSafetyEnvironmentDisablesStackShrinkingBeforeStartup(t *testing.T) {
	tests := []struct {
		name        string
		environ     []string
		wantGODEBUG string
		wantReexec  bool
	}{
		{name: "missing", environ: []string{"HOME=/tmp"}, wantGODEBUG: "gcshrinkstackoff=1", wantReexec: true},
		{name: "preserves settings", environ: []string{"GODEBUG=gctrace=1"}, wantGODEBUG: "gctrace=1,gcshrinkstackoff=1", wantReexec: true},
		{name: "overrides unsafe value", environ: []string{"GODEBUG=gcshrinkstackoff=0,gctrace=1"}, wantGODEBUG: "gctrace=1,gcshrinkstackoff=1", wantReexec: true},
		{name: "already safe", environ: []string{"GODEBUG=gctrace=1,gcshrinkstackoff=1"}, wantGODEBUG: "gctrace=1,gcshrinkstackoff=1", wantReexec: false},
		{name: "forged marker is ignored", environ: []string{"DUMBER_RUNTIME_SAFETY_REEXEC=1", "GODEBUG=gcshrinkstackoff=0"}, wantGODEBUG: "gcshrinkstackoff=1", wantReexec: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, reexec := runtimeSafetyEnvironment(tt.environ)
			if reexec != tt.wantReexec {
				t.Fatalf("reexec = %v, want %v", reexec, tt.wantReexec)
			}
			if value := environmentValue(got, "GODEBUG"); value != tt.wantGODEBUG {
				t.Fatalf("GODEBUG = %q, want %q", value, tt.wantGODEBUG)
			}
		})
	}
}
