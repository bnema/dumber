package webutil

import (
	"strings"
	"testing"
)

func TestBuildScrollByJS_ContainsRequiredOperations(t *testing.T) {
	tests := []struct {
		name string
		dx   int
		dy   int
	}{
		{"down", 0, 80},
		{"up", 0, -80},
		{"left", -80, 0},
		{"right", 80, 0},
		{"upFast", 0, -320},
		{"downFast", 0, 320},
	}
	required := []struct {
		message  string
		snippets []string
	}{
		{"JS must reference document.activeElement", []string{"activeElement"}},
		{"JS must walk parentElement in a loop", []string{"parentElement"}},
		{"JS must reference document.scrollingElement", []string{"scrollingElement"}},
		{"JS must fall back to window.scrollBy", []string{"window.scrollBy"}},
		{"JS must use getComputedStyle", []string{"getComputedStyle"}},
		{"JS must check overflowY", []string{"overflowY"}},
		{"JS must check overflowX", []string{"overflowX"}},
		{"JS must support smooth element scrolling", []string{"scrollBy({left:dx,top:dy,behavior:'smooth'})"}},
		{"JS must check vertical direction-specific remaining scroll", []string{"dy<0&&el.scrollTop>0", "dy>0&&el.scrollTop<maxTop"}},
		{"JS must check horizontal direction-specific remaining scroll", []string{"dx<0&&el.scrollLeft>0", "dx>0&&el.scrollLeft<maxLeft"}},
		{"JS must check scrollable overflow values", []string{"'scroll'", "'auto'", "'overlay'"}},
		{"JS must wrap in try/catch", []string{"try{", "catch(e)"}},
		{"JS must support smooth window scrolling", []string{"window.scrollBy({left:dx,top:dy,behavior:'smooth'})"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := BuildScrollByJS(tt.dx, tt.dy)
			for _, check := range required {
				for _, snippet := range check.snippets {
					if !strings.Contains(js, snippet) {
						t.Error(check.message)
						break
					}
				}
			}
			if !strings.HasPrefix(js, "(function(){") || !strings.HasSuffix(js, "})()") {
				t.Error("JS must be wrapped in an IIFE")
			}
		})
	}
}

func TestBuildScrollByJS_SpecificDelta(t *testing.T) {
	js := BuildScrollByJS(0, 80)
	if !strings.Contains(js, "var dx=0,dy=80") {
		t.Errorf("expected dy=80 in JS variables, got: %s", js)
	}
	if !strings.Contains(js, "behavior:'smooth'") {
		t.Errorf("expected smooth scroll behavior in JS, got: %s", js)
	}

	js = BuildScrollByJS(-80, 0)
	if !strings.Contains(js, "var dx=-80,dy=0") {
		t.Errorf("expected dx=-80 in JS variables, got: %s", js)
	}
}

func TestBuildScrollByJS_HorizontalAxisCheck(t *testing.T) {
	js := BuildScrollByJS(-80, 0)
	if !strings.Contains(js, "clientWidth") || !strings.Contains(js, "scrollWidth") {
		t.Error("horizontal scroll must check scrollWidth/clientWidth")
	}
}

func TestBuildScrollByJS_VerticalAxisCheck(t *testing.T) {
	js := BuildScrollByJS(0, 80)
	if !strings.Contains(js, "clientHeight") || !strings.Contains(js, "scrollHeight") {
		t.Error("vertical scroll must check scrollHeight/clientHeight")
	}
}

func TestBuildScrollByJS_NegativeDeltaMagnitude(t *testing.T) {
	js := BuildScrollByJS(0, -320)
	if !strings.Contains(js, "var dx=0,dy=-320") {
		t.Errorf("expected dy=-320 in JS variables, got: %s", js)
	}
}

func TestBuildScrollByJS_SyntacticallyValid(t *testing.T) {
	tests := []struct{ dx, dy int }{{0, 80}, {0, -320}, {-80, 0}, {80, 0}, {0, 0}}

	for _, tt := range tests {
		js := BuildScrollByJS(tt.dx, tt.dy)

		depth := 0
		for _, ch := range js {
			switch ch {
			case '(':
				depth++
			case ')':
				depth--
			}
		}
		if depth != 0 {
			t.Errorf("dx=%d,dy=%d: unbalanced parentheses (depth=%d)", tt.dx, tt.dy, depth)
		}

		depth = 0
		for _, ch := range js {
			switch ch {
			case '{':
				depth++
			case '}':
				depth--
			}
		}
		if depth != 0 {
			t.Errorf("dx=%d,dy=%d: unbalanced braces (depth=%d)", tt.dx, tt.dy, depth)
		}
	}
}

func TestBuildScrollByJS_ZeroDeltaSafe(t *testing.T) {
	js := BuildScrollByJS(0, 0)
	if !strings.Contains(js, "var dx=0,dy=0") {
		t.Errorf("expected zero deltas in JS variables, got: %s", js)
	}
	if !strings.Contains(js, "clientHeight") || !strings.Contains(js, "scrollHeight") {
		t.Error("zero-delta script must still include vertical scroll checks")
	}
}
