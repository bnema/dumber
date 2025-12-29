package entity

import (
	"testing"
)

func TestHasAccents(t *testing.T) {
	tests := []struct {
		name string
		char rune
		want bool
	}{
		{"lowercase e", 'e', true},
		{"uppercase E", 'E', true},
		{"lowercase a", 'a', true},
		{"uppercase A", 'A', true},
		{"lowercase c", 'c', true},
		{"lowercase n", 'n', true},
		{"lowercase x (no accents)", 'x', false},
		{"uppercase X (no accents)", 'X', false},
		{"digit 1 (no accents)", '1', false},
		{"space (no accents)", ' ', false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HasAccents(tt.char)
			if got != tt.want {
				t.Errorf("HasAccents(%q) = %v, want %v", tt.char, got, tt.want)
			}
		})
	}
}

func TestGetAccents(t *testing.T) {
	tests := []struct {
		name      string
		char      rune
		uppercase bool
		wantLen   int
		wantFirst rune
	}{
		{"lowercase e", 'e', false, 6, 'è'},
		{"uppercase E", 'E', true, 6, 'È'},
		{"lowercase a", 'a', false, 7, 'à'},
		{"uppercase A", 'A', true, 7, 'À'},
		{"lowercase c", 'c', false, 3, 'ç'},
		{"uppercase C with shift", 'c', true, 3, 'Ç'},
		{"no accents x", 'x', false, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAccents(tt.char, tt.uppercase)

			if len(got) != tt.wantLen {
				t.Errorf("GetAccents(%q, %v) length = %d, want %d", tt.char, tt.uppercase, len(got), tt.wantLen)
			}

			if tt.wantLen > 0 && got[0] != tt.wantFirst {
				t.Errorf("GetAccents(%q, %v)[0] = %q, want %q", tt.char, tt.uppercase, got[0], tt.wantFirst)
			}
		})
	}
}

func TestGetAccents_UppercaseFromLowercase(t *testing.T) {
	// When passing lowercase char with uppercase=true, should return uppercase accents
	uppercase := GetAccents('e', true)
	lowercase := GetAccents('e', false)

	if len(uppercase) == 0 {
		t.Fatal("expected accents for 'e' with uppercase=true")
	}

	if len(uppercase) != len(lowercase) {
		t.Fatalf("uppercase and lowercase accent counts differ: %d vs %d", len(uppercase), len(lowercase))
	}

	// Verify each uppercase accent is different from the corresponding lowercase accent
	for i, upper := range uppercase {
		lower := lowercase[i]
		if upper == lower {
			t.Errorf("accent at index %d is same for uppercase and lowercase: %q", i, upper)
		}
	}

	// Verify first accent is È (uppercase of è)
	if uppercase[0] != 'È' {
		t.Errorf("expected first uppercase accent to be È, got %q", uppercase[0])
	}
}

func TestAccentMapCoverage(t *testing.T) {
	// Verify all expected characters have accents
	expectedChars := []rune{'a', 'c', 'e', 'i', 'n', 'o', 's', 'u', 'y'}

	for _, char := range expectedChars {
		if !HasAccents(char) {
			t.Errorf("expected %q to have accents", char)
		}

		accents := GetAccents(char, false)
		if len(accents) == 0 {
			t.Errorf("expected %q to have at least one accent variant", char)
		}
	}
}
