package gtkutil

import "testing"

func TestNormalizeScale(t *testing.T) {
	if got := NormalizeScale(0); got != 1 {
		t.Fatalf("NormalizeScale(0) = %v, want 1", got)
	}
	if got := NormalizeScale(1.2); got != 1.2 {
		t.Fatalf("NormalizeScale(1.2) = %v, want 1.2", got)
	}
}

func TestDeviceLogicalConversions(t *testing.T) {
	if got := DeviceToLogical(320, 1.25); got != 256 {
		t.Fatalf("DeviceToLogical(320, 1.25) = %d, want 256", got)
	}
}
