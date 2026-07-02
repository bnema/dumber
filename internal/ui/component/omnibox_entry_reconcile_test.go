package component

import "testing"

// reconcileEntryState decides how row-selected handlers resync the entry
// buffer with the realInput/ghostSuffix shadow. The entry buffer is the source
// of truth: search-changed is debounced (~150ms), so realInput lags behind
// what the user actually typed. The old force-restore behavior rewrote the
// buffer to the stale realInput, deleting characters typed during the
// debounce window (fast typing dropped letters).
func TestReconcileEntryState(t *testing.T) {
	tests := []struct {
		name          string
		buffer        string
		realInput     string
		ghostSuffix   string
		wantRealInput string
		wantRewrite   bool
	}{
		{
			name:          "ghost visible and untouched: strip it from the buffer",
			buffer:        "google.com",
			realInput:     "goo",
			ghostSuffix:   "gle.com",
			wantRealInput: "goo",
			wantRewrite:   true,
		},
		{
			name:          "buffer in sync, no ghost: nothing to do",
			buffer:        "goo",
			realInput:     "goo",
			ghostSuffix:   "",
			wantRealInput: "goo",
			wantRewrite:   false,
		},
		{
			name:          "user typed ahead of debounced realInput: buffer wins, never rewrite",
			buffer:        "bne",
			realInput:     "bn",
			ghostSuffix:   "",
			wantRealInput: "bne",
			wantRewrite:   false,
		},
		{
			name:          "user typed over the selected ghost suffix: buffer wins",
			buffer:        "goog",
			realInput:     "goo",
			ghostSuffix:   "gle.com",
			wantRealInput: "goog",
			wantRewrite:   false,
		},
		{
			name:          "stale ghost flag but buffer already shows real input only",
			buffer:        "goo",
			realInput:     "goo",
			ghostSuffix:   "gle.com",
			wantRealInput: "goo",
			wantRewrite:   false,
		},
		{
			name:          "user cleared the entry: buffer wins",
			buffer:        "",
			realInput:     "goo",
			ghostSuffix:   "",
			wantRealInput: "",
			wantRewrite:   false,
		},
		{
			name:          "empty ghost with empty input stays empty",
			buffer:        "",
			realInput:     "",
			ghostSuffix:   "",
			wantRealInput: "",
			wantRewrite:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotRealInput, gotRewrite := reconcileEntryState(tt.buffer, tt.realInput, tt.ghostSuffix)
			if gotRealInput != tt.wantRealInput {
				t.Errorf("realInput = %q, want %q", gotRealInput, tt.wantRealInput)
			}
			if gotRewrite != tt.wantRewrite {
				t.Errorf("rewrite = %v, want %v", gotRewrite, tt.wantRewrite)
			}
		})
	}
}
