package cef

import "testing"

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
}

func TestExternalBeginFrameIsEnabledByDefault(t *testing.T) {
	t.Setenv(cefExternalBeginFrameEnvVar, "")
	if !externalBeginFrameEnabled() {
		t.Fatal("external begin frame should be enabled by default")
	}

	for _, value := range []string{"0", "false", "no", "off", "OFF"} {
		t.Setenv(cefExternalBeginFrameEnvVar, value)
		if externalBeginFrameEnabled() {
			t.Fatalf("external begin frame enabled for %q, want disabled", value)
		}
	}

	for _, value := range []string{"1", "true", "yes", "on"} {
		t.Setenv(cefExternalBeginFrameEnvVar, value)
		if !externalBeginFrameEnabled() {
			t.Fatalf("external begin frame disabled for %q, want enabled", value)
		}
	}
}
