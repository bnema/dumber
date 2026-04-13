package cef

import "testing"

func TestExternalBeginFrameEnabledIsOptIn(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "1", value: "1", want: true},
		{name: "true", value: "true", want: true},
		{name: "yes", value: "yes", want: true},
		{name: "on", value: "on", want: true},
		{name: "TRUE", value: "TRUE", want: true},
		{name: "  true  ", value: "  true  ", want: true},
		{name: "", value: "", want: false},
		{name: "false", value: "false", want: false},
		{name: "0", value: "0", want: false},
		{name: "no", value: "no", want: false},
		{name: "off", value: "off", want: false},
		{name: "random", value: "random", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(cefExternalBeginFrameEnvVar, tt.value)

			if got := envBoolEnabled(cefExternalBeginFrameEnvVar); got != tt.want {
				t.Fatalf("envBoolEnabled(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
