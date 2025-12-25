package updater

import (
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		// Equal versions
		{"equal simple", "1.0.0", "1.0.0", 0},
		{"equal two parts", "1.0", "1.0", 0},
		{"equal one part", "1", "1", 0},

		// v1 < v2 (should update)
		{"major less", "1.0.0", "2.0.0", -1},
		{"minor less", "1.1.0", "1.2.0", -1},
		{"patch less", "1.1.1", "1.1.2", -1},
		{"complex less", "0.20.1", "0.21.0", -1},
		{"real world less", "0.20.1", "0.20.2", -1},

		// v1 > v2 (no update needed)
		{"major greater", "2.0.0", "1.0.0", 1},
		{"minor greater", "1.2.0", "1.1.0", 1},
		{"patch greater", "1.1.2", "1.1.1", 1},

		// Pre-release versions (suffix stripped)
		{"prerelease equal", "1.0.0-alpha", "1.0.0", 0},
		{"prerelease less", "1.0.0-alpha", "1.0.1", -1},
		{"prerelease greater", "1.0.1-beta", "1.0.0", 1},

		// Partial versions
		{"partial v1", "1", "1.0.0", 0},
		{"partial v2", "1.0.0", "1", 0},
		{"partial less", "1", "2.0.0", -1},
		{"partial greater", "2", "1.0.0", 1},

		// Edge cases
		{"zero versions", "0.0.0", "0.0.0", 0},
		{"zero less", "0.0.0", "0.0.1", -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestGetArchName(t *testing.T) {
	// This test just ensures the function doesn't panic and returns something.
	arch := getArchName()
	if arch == "" {
		t.Error("getArchName() returned empty string")
	}
}
