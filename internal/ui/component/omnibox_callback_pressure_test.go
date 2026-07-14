package component

import "testing"

func TestOmniboxCallbackPressure_NoCrashSignal(t *testing.T) {
	runGTKCallbackLifecycleStress(t)
}
