package cef

import (
	purecef "github.com/bnema/purego-cef/cef"
	"testing"
)

func TestPopupAllocationPreservesCEFGeometry(t *testing.T) {
	for _, rect := range []purecef.Rect{
		{X: 12, Y: 650, Width: 300, Height: 900},
		{X: -20, Y: -400, Width: 300, Height: 900},
		{X: 0, Y: 0, Width: 1600, Height: 1200},
	} {
		got, valid := popupAllocation(rect)
		if !valid || got.X != rect.X || got.Y != rect.Y || got.Width != rect.Width || got.Height != rect.Height {
			t.Fatalf("allocation(%+v) = %+v, %v", rect, got, valid)
		}
	}
	for _, rect := range []purecef.Rect{{}, {Width: -1, Height: 20}, {Width: 20, Height: 0}} {
		if _, valid := popupAllocation(rect); valid {
			t.Fatalf("accepted invalid rect %+v", rect)
		}
	}
}
