package component

import "testing"

func TestInlineGhostSelection(t *testing.T) {
	tests := []struct {
		name           string
		realInput      string
		ghostFullText  string
		selectionStart int
		selectionEnd   int
		hasSelection   bool
		want           bool
	}{
		{
			name:           "selected suffix matches typed prefix",
			realInput:      "goo",
			ghostFullText:  "google.com",
			selectionStart: 3,
			selectionEnd:   10,
			hasSelection:   true,
			want:           true,
		},
		{
			name:           "no selection means no ghost suffix",
			realInput:      "goo",
			ghostFullText:  "google.com",
			selectionStart: 10,
			selectionEnd:   10,
			hasSelection:   false,
			want:           false,
		},
		{
			name:           "collapsed selection should not be treated as ghost",
			realInput:      "goo",
			ghostFullText:  "google.com",
			selectionStart: 10,
			selectionEnd:   10,
			hasSelection:   true,
			want:           false,
		},
		{
			name:           "wrong selection range should not be treated as ghost",
			realInput:      "goo",
			ghostFullText:  "google.com",
			selectionStart: 1,
			selectionEnd:   10,
			hasSelection:   true,
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInlineGhostSelection(
				tt.realInput,
				tt.ghostFullText,
				tt.selectionStart,
				tt.selectionEnd,
				tt.hasSelection,
			)
			if got != tt.want {
				t.Fatalf("isInlineGhostSelection() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAcceptedGhostInput(t *testing.T) {
	tests := []struct {
		name          string
		hasGhost      bool
		ghostFullText string
		wantText      string
		wantAccepted  bool
	}{
		{
			name:          "accepts explicit ghost",
			hasGhost:      true,
			ghostFullText: "google.com",
			wantText:      "google.com",
			wantAccepted:  true,
		},
		{
			name:          "rejects without ghost",
			hasGhost:      false,
			ghostFullText: "google.com",
			wantText:      "",
			wantAccepted:  false,
		},
		{
			name:          "rejects empty ghost text",
			hasGhost:      true,
			ghostFullText: "",
			wantText:      "",
			wantAccepted:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotText, gotAccepted := acceptedGhostInput(tt.hasGhost, tt.ghostFullText)
			if gotText != tt.wantText {
				t.Fatalf("acceptedGhostInput() text = %q, want %q", gotText, tt.wantText)
			}
			if gotAccepted != tt.wantAccepted {
				t.Fatalf("acceptedGhostInput() accepted = %v, want %v", gotAccepted, tt.wantAccepted)
			}
		})
	}
}
