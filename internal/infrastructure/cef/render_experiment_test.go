package cef

import (
	"strconv"
	"testing"
)

func TestResolveWindowlessFrameRatePrefersAPinnedRate(t *testing.T) {
	t.Setenv(cefWindowlessFrameRateEnvVar, "")

	rate, adaptive := resolveWindowlessFrameRate(0, true)
	if rate != defaultCEFWindowlessFrameRate || !adaptive {
		t.Fatalf("unpinned resolve = (%d, %v), want (%d, true)", rate, adaptive, defaultCEFWindowlessFrameRate)
	}

	rate, adaptive = resolveWindowlessFrameRate(90, true)
	if rate != 90 || !adaptive {
		t.Fatalf("configured resolve = (%d, %v), want (90, true)", rate, adaptive)
	}

	rate, adaptive = resolveWindowlessFrameRate(90, false)
	if rate != 90 || adaptive {
		t.Fatalf("non-adaptive resolve = (%d, %v), want (90, false)", rate, adaptive)
	}

	t.Setenv(cefWindowlessFrameRateEnvVar, "144")
	rate, adaptive = resolveWindowlessFrameRate(0, true)
	if rate != 144 || adaptive {
		t.Fatalf("pinned resolve = (%d, %v), want (144, false)", rate, adaptive)
	}

	t.Setenv(cefWindowlessFrameRateEnvVar, "not-a-rate")
	rate, adaptive = resolveWindowlessFrameRate(0, true)
	if rate != defaultCEFWindowlessFrameRate || !adaptive {
		t.Fatalf("unparsable pin resolve = (%d, %v), want the configured fallback", rate, adaptive)
	}

	t.Setenv(cefWindowlessFrameRateEnvVar, "0")
	rate, adaptive = resolveWindowlessFrameRate(60, true)
	if rate != 60 || !adaptive {
		t.Fatalf("zero pin resolve = (%d, %v), want (60, true)", rate, adaptive)
	}

	t.Setenv(cefWindowlessFrameRateEnvVar, "-60")
	rate, adaptive = resolveWindowlessFrameRate(60, true)
	if rate != 60 || !adaptive {
		t.Fatalf("negative pin resolve = (%d, %v), want (60, true)", rate, adaptive)
	}

	overflow := strconv.FormatInt(int64(1)<<40, 10)
	t.Setenv(cefWindowlessFrameRateEnvVar, overflow)
	rate, adaptive = resolveWindowlessFrameRate(60, true)
	if rate != 60 || !adaptive {
		t.Fatalf("overflowing pin resolve = (%d, %v), want (60, true)", rate, adaptive)
	}
}
