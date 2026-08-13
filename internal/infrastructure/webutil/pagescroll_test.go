package webutil

import (
	"strings"
	"testing"
)

func TestBuildPageScrollByJS_TargetsViewportCenterInsteadOfActiveElement(t *testing.T) {
	js := BuildPageScrollByJS(0, 80)

	if !strings.Contains(js, "doc.elementFromPoint(window.innerWidth/2,window.innerHeight/2)") {
		t.Fatalf("page scroll must resolve its initial target from the viewport center, got: %s", js)
	}
	if strings.Contains(js, "activeElement") {
		t.Fatalf("page scroll must not use the legacy active-element target, got: %s", js)
	}
}

func TestBuildPageScrollByJS_ContainsRequiredOperations(t *testing.T) {
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
		{"JS must resolve the viewport-center element", []string{"elementFromPoint", "window.innerWidth/2", "window.innerHeight/2"}},
		{"JS must walk parentElement in a loop", []string{"parentElement"}},
		{"JS must reference document.scrollingElement", []string{"scrollingElement"}},
		{"JS must fall back to window.scrollBy", []string{"window.scrollBy"}},
		{"JS must use getComputedStyle", []string{"getComputedStyle"}},
		{"JS must check overflowY", []string{"overflowY"}},
		{"JS must check overflowX", []string{"overflowX"}},
		{"JS must use direct element scroll offsets", []string{"scrollLeft=beforeLeft+dx", "scrollTop=beforeTop+dy"}},
		{"JS must check vertical direction-specific remaining scroll", []string{"dy<0&&el.scrollTop>0", "dy>0&&el.scrollTop<maxTop"}},
		{"JS must check horizontal direction-specific remaining scroll", []string{"dx<0&&el.scrollLeft>0", "dx>0&&el.scrollLeft<maxLeft"}},
		{"JS must check scrollable overflow values", []string{"'scroll'", "'auto'", "'overlay'"}},
		{"JS must wrap in try/catch", []string{"try{", "catch(e)"}},
		{"JS must support immediate window scrolling", []string{"window.scrollBy(dx,dy)"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := BuildPageScrollByJS(tt.dx, tt.dy)
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

func TestBuildPageScrollByJS_SpecificDelta(t *testing.T) {
	js := BuildPageScrollByJS(0, 80)
	if !strings.Contains(js, "var dx=0,dy=80") {
		t.Errorf("expected dy=80 in JS variables, got: %s", js)
	}
	if strings.Contains(js, "requestAnimationFrame") {
		t.Errorf("expected immediate fallback scroll step without requestAnimationFrame, got: %s", js)
	}

	js = BuildPageScrollByJS(-80, 0)
	if !strings.Contains(js, "var dx=-80,dy=0") {
		t.Errorf("expected dx=-80 in JS variables, got: %s", js)
	}
}

func TestBuildPageScrollByJS_HorizontalAxisCheck(t *testing.T) {
	js := BuildPageScrollByJS(-80, 0)
	if !strings.Contains(js, "clientWidth") || !strings.Contains(js, "scrollWidth") {
		t.Error("horizontal scroll must check scrollWidth/clientWidth")
	}
}

func TestBuildPageScrollByJS_VerticalAxisCheck(t *testing.T) {
	js := BuildPageScrollByJS(0, 80)
	if !strings.Contains(js, "clientHeight") || !strings.Contains(js, "scrollHeight") {
		t.Error("vertical scroll must check scrollHeight/clientHeight")
	}
}

func TestBuildPageScrollByJS_NegativeDeltaMagnitude(t *testing.T) {
	js := BuildPageScrollByJS(0, -320)
	if !strings.Contains(js, "var dx=0,dy=-320") {
		t.Errorf("expected dy=-320 in JS variables, got: %s", js)
	}
}

func TestBuildPageScrollByJS_SyntacticallyValid(t *testing.T) {
	tests := []struct{ dx, dy int }{{0, 80}, {0, -320}, {-80, 0}, {80, 0}, {0, 0}}

	for _, tt := range tests {
		js := BuildPageScrollByJS(tt.dx, tt.dy)

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

func TestBuildPageScrollByJS_ZeroDeltaSafe(t *testing.T) {
	js := BuildPageScrollByJS(0, 0)
	if !strings.Contains(js, "var dx=0,dy=0") {
		t.Errorf("expected zero deltas in JS variables, got: %s", js)
	}
	if !strings.Contains(js, "clientHeight") || !strings.Contains(js, "scrollHeight") {
		t.Error("zero-delta script must still include vertical scroll checks")
	}
}
