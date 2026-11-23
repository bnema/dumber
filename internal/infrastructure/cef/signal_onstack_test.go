package cef

import "testing"

func TestSignalsNeedingOnStack(t *testing.T) {
	const otherFlags uint64 = 0x04000000 | 0x10000000 // SA_RESTORER | SA_RESTART
	tests := []struct {
		name  string
		in    signalHandlerState
		want  bool
		flags uint64
	}{
		{"SIG_DFL skipped", signalHandlerState{Signal: 1, Handler: sigHandlerDefault}, false, 0},
		{"SIG_IGN skipped", signalHandlerState{Signal: 2, Handler: sigHandlerIgnore}, false, 0},
		{"already onstack skipped", signalHandlerState{Signal: 3, Handler: 0x1234, Flags: otherFlags | saOnStackFlag}, false, 0},
		{"missing onstack repaired", signalHandlerState{Signal: 11, Handler: 0x1234, Flags: otherFlags}, true, otherFlags | saOnStackFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := signalsNeedingOnStack([]signalHandlerState{tt.in})
			if !tt.want {
				if len(got) != 0 {
					t.Fatalf("expected no repair, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Signal != tt.in.Signal || got[0].Handler != tt.in.Handler || got[0].Flags != tt.flags {
				t.Fatalf("got %+v, want flags %#x", got, tt.flags)
			}
		})
	}
}
