package cef

import (
	"testing"

	cef2gtk "github.com/bnema/purego-cef2gtk"
)

func TestClickDiagnosticNamesAreStable(t *testing.T) {
	phases := map[cef2gtk.ClickDiagnosticPhase]string{
		cef2gtk.ClickDiagnosticPressed:   "pressed",
		cef2gtk.ClickDiagnosticReleased:  "released",
		cef2gtk.ClickDiagnosticCancelled: "canceled",
		cef2gtk.ClickDiagnosticForwarded: "forwarded",
		cef2gtk.ClickDiagnosticConsumed:  "consumed",
		cef2gtk.ClickDiagnosticDropped:   "dropped",
	}
	for value, want := range phases {
		if got := clickDiagnosticPhaseName(value); got != want {
			t.Fatalf("phase %d: got %q, want %q", value, got, want)
		}
	}
	if got := clickDiagnosticPhaseName(99); got != "unknown(99)" {
		t.Fatalf("unknown phase: got %q", got)
	}

	reasons := map[cef2gtk.ClickDropReason]string{
		cef2gtk.ClickDropReasonNone:              "none",
		cef2gtk.ClickDropReasonMissingHost:       "missing_host",
		cef2gtk.ClickDropReasonDetached:          "detached",
		cef2gtk.ClickDropReasonMissingInputState: "missing_input_state",
	}
	for value, want := range reasons {
		if got := clickDropReasonName(value); got != want {
			t.Fatalf("reason %d: got %q, want %q", value, got, want)
		}
	}
	if got := clickDropReasonName(99); got != "unknown(99)" {
		t.Fatalf("unknown reason: got %q", got)
	}
}

func TestClickButtonNamesArePrivacySafeAndBounded(t *testing.T) {
	for button, want := range map[uint]string{0: "other", 1: "primary", 2: "middle", 3: "secondary", 8: "other"} {
		if got := clickButtonName(button); got != want {
			t.Fatalf("button %d: got %q, want %q", button, got, want)
		}
	}
}
