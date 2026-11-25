package sigstack

import "testing"

func TestNeedsOnStack(t *testing.T) {
	const otherFlags uint64 = 0x04000000 | 0x10000000 // SA_RESTORER | SA_RESTART
	tests := []struct {
		name    string
		handler uintptr
		flags   uint64
		want    bool
	}{
		{"SIG_DFL skipped", sigHandlerDefault, otherFlags, false},
		{"SIG_IGN skipped", sigHandlerIgnore, otherFlags, false},
		{"already onstack skipped", 0x1234, otherFlags | saOnStackFlag, false},
		{"missing onstack repaired", 0x1234, otherFlags, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := needsOnStack(tt.handler, tt.flags); got != tt.want {
				t.Fatalf("needsOnStack(%#x, %#x) = %v, want %v", tt.handler, tt.flags, got, tt.want)
			}
		})
	}
}
