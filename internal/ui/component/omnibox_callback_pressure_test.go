package component

import (
	"os"
	"testing"
)

func TestOmniboxCallbackPressure_NoCrashSignal(t *testing.T) {
	if os.Getenv("OMNIBOX_STRESS") != "1" {
		t.Skip("skipping: set OMNIBOX_STRESS=1 to run with GTK stress harness")
	}
	// TODO: wire GTK stress harness and assert no abrupt crash markers
}
