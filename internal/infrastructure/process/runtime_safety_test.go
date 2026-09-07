package process

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeRuntimeSafety(t *testing.T) {
	tests := []struct {
		name        string
		environ     []string
		wantGODEBUG string
		wantChanged bool
	}{
		{name: "missing", environ: []string{"HOME=/tmp"}, wantGODEBUG: "gcshrinkstackoff=1", wantChanged: true},
		{name: "preserves settings", environ: []string{"GODEBUG=gctrace=1"}, wantGODEBUG: "gctrace=1,gcshrinkstackoff=1", wantChanged: true},
		{name: "overrides unsafe value", environ: []string{"GODEBUG=gcshrinkstackoff=0,gctrace=1"}, wantGODEBUG: "gctrace=1,gcshrinkstackoff=1", wantChanged: true},
		{name: "already safe", environ: []string{"GODEBUG=gctrace=1,gcshrinkstackoff=1"}, wantGODEBUG: "gctrace=1,gcshrinkstackoff=1", wantChanged: false},
		{name: "safe first keeps position without exec", environ: []string{"GODEBUG=gcshrinkstackoff=1,gctrace=1"}, wantGODEBUG: "gcshrinkstackoff=1,gctrace=1", wantChanged: false},
		{name: "collapses duplicates", environ: []string{"GODEBUG=gcshrinkstackoff=0,gcshrinkstackoff=1"}, wantGODEBUG: "gcshrinkstackoff=1", wantChanged: true},
		{name: "bare key without value is unsafe", environ: []string{"GODEBUG=gcshrinkstackoff"}, wantGODEBUG: "gcshrinkstackoff=1", wantChanged: true},
		{name: "preserves other debug keys", environ: []string{"GODEBUG=tracebacklabels=0,x509sslcertoverrideplatform=0"}, wantGODEBUG: "tracebacklabels=0,x509sslcertoverrideplatform=0,gcshrinkstackoff=1", wantChanged: true},
		{name: "merges separate duplicate entries", environ: []string{"GODEBUG=gcshrinkstackoff=1", "GODEBUG=gctrace=1"}, wantGODEBUG: "gctrace=1,gcshrinkstackoff=1", wantChanged: true},
		{name: "safe first entry does not hide second entry keys", environ: []string{"GODEBUG=gcshrinkstackoff=1", "GODEBUG=cpu.all=off"}, wantGODEBUG: "cpu.all=off,gcshrinkstackoff=1", wantChanged: true},
		{name: "unsafe first entry preserves second entry keys", environ: []string{"GODEBUG=cpu.all=off", "GODEBUG=gcshrinkstackoff=1,foo=bar"}, wantGODEBUG: "cpu.all=off,foo=bar,gcshrinkstackoff=1", wantChanged: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := MergeRuntimeSafety(tt.environ)
			require.Equal(t, tt.wantChanged, changed)
			require.Equal(t, tt.wantGODEBUG, EnvironmentValue(got, "GODEBUG"))
			if changed {
				// Normalization always collapses to a single trailing
				// entry; no duplicate GODEBUG may survive a change.
				count := 0
				for _, entry := range got {
					if strings.HasPrefix(entry, "GODEBUG=") {
						count++
				}
				}
				require.Equal(t, 1, count, "changed environ must hold exactly one GODEBUG entry")
			}
		})
	}
}

func TestMergeRuntimeSafetyDoesNotMutateInput(t *testing.T) {
	input := []string{"GODEBUG=gctrace=1"}
	got, changed := MergeRuntimeSafety(input)
	require.True(t, changed)
	require.Equal(t, []string{"GODEBUG=gctrace=1"}, input)
	require.Equal(t, "gctrace=1,gcshrinkstackoff=1", EnvironmentValue(got, "GODEBUG"))
}
