package component

import "testing"

func TestSignExtendCInt(t *testing.T) {
	for in, want := range map[int]int{
		0:          0,
		3:          3,
		4294967295: -1, // C -1 read without sign extension
		-1:         -1,
	} {
		if got := signExtendCInt(in); got != want {
			t.Errorf("signExtendCInt(%d) = %d, want %d", in, got, want)
		}
	}
}
