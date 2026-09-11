package cef

import (
	"strconv"
	"testing"

	purecef "github.com/bnema/purego-cef/cef"
)

func TestWindowlessFrameRatePinnedFollowsTheOverride(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{value: "", want: false},
		{value: "60", want: true},
		{value: " 144 ", want: true},
		{value: "0", want: false},
		{value: "-60", want: false},
		{value: "many", want: false},
		{value: strconv.FormatInt(int64(1)<<40, 10), want: false},
	}
	for _, testCase := range cases {
		t.Run("pin:"+testCase.value, func(t *testing.T) {
			t.Setenv(cefWindowlessFrameRateEnvVar, testCase.value)
			if got := windowlessFrameRatePinned(); got != testCase.want {
				t.Fatalf("windowlessFrameRatePinned(%q) = %v, want %v", testCase.value, got, testCase.want)
			}
		})
	}
}

func TestConfigureNativePopupWindowEnablesExternalBeginFrameByDefault(t *testing.T) {
	// The default is only worth asserting through a real call site: an inverted
	// default shipped before because only the boolean helper was covered.
	cases := []struct {
		value string
		want  int32
	}{
		{value: "", want: 1},
		{value: "1", want: 1},
		{value: "true", want: 1},
		{value: "on", want: 1},
		{value: "random", want: 1},
		{value: "0", want: 0},
		{value: "false", want: 0},
		{value: "off", want: 0},
	}
	for _, testCase := range cases {
		t.Run("call-site:"+testCase.value, func(t *testing.T) {
			t.Setenv(cefExternalBeginFrameEnvVar, testCase.value)
			windowInfo := purecef.NewWindowInfo()
			settings := purecef.NewBrowserSettings()

			configureNativePopupWindow(&windowInfo, &settings, 60)

			if windowInfo.ExternalBeginFrameEnabled != testCase.want {
				t.Fatalf("ExternalBeginFrameEnabled with %q = %d, want %d",
					testCase.value, windowInfo.ExternalBeginFrameEnabled, testCase.want)
			}
		})
	}
}

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
