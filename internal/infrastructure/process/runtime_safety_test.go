package process

import (
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := MergeRuntimeSafety(tt.environ)
			require.Equal(t, tt.wantChanged, changed)
			require.Equal(t, tt.wantGODEBUG, EnvironmentValue(got, "GODEBUG"))
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
