package webkit

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestJSCSuspendResumeSignal(t *testing.T) {
	tests := []struct {
		env  string
		want int
	}{
		{"", int(unix.SIGUSR1)},
		{"abc", int(unix.SIGUSR1)},
		{"12", 12},
		{" 34x", 34},
		{"-", int(unix.SIGUSR1)},
	}
	for _, tt := range tests {
		if got := jscSuspendResumeSignal(tt.env); got != tt.want {
			t.Errorf("jscSuspendResumeSignal(%q) = %d, want %d", tt.env, got, tt.want)
		}
	}
}
