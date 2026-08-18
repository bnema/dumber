package webkit

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewEditableFocusBridgeToken_UsesRandomHex(t *testing.T) {
	old := editableFocusTokenRand
	t.Cleanup(func() { editableFocusTokenRand = old })
	editableFocusTokenRand = func(b []byte) (int, error) {
		for i := range b {
			b[i] = byte(i + 1)
		}
		return len(b), nil
	}

	token := newEditableFocusBridgeToken()
	require.Equal(t, "0102030405060708090a0b0c0d0e0f10", token)
}

func TestNewEditableFocusBridgeToken_RandFailureReturnsEmpty(t *testing.T) {
	old := editableFocusTokenRand
	t.Cleanup(func() { editableFocusTokenRand = old })
	editableFocusTokenRand = func([]byte) (int, error) {
		return 0, errors.New("entropy exhausted")
	}

	token := newEditableFocusBridgeToken()
	require.Empty(t, token)
}

func TestEditableFocusBridgeToken_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	wv := &WebView{}
	wv.setEditableFocusBridgeToken("initial-token")

	var wg sync.WaitGroup
	for range 32 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 200 {
				_ = wv.EditableFocusBridgeToken()
				_ = wv.matchesEditableFocusBridgeToken("initial-token")
			}
		}()
		go func() {
			defer wg.Done()
			for i := range 200 {
				wv.setEditableFocusBridgeToken(newEditableFocusBridgeToken())
				if i%17 == 0 {
					wv.setEditableFocusBridgeToken("initial-token")
				}
			}
		}()
	}
	wg.Wait()
}

func TestMatchesEditableFocusBridgeToken_RejectsEmpty(t *testing.T) {
	wv := &WebView{}
	wv.setEditableFocusBridgeToken("")
	require.False(t, wv.matchesEditableFocusBridgeToken(""))
	require.False(t, wv.matchesEditableFocusBridgeToken("anything"))

	wv.setEditableFocusBridgeToken("secret")
	require.True(t, wv.matchesEditableFocusBridgeToken("secret"))
	require.False(t, wv.matchesEditableFocusBridgeToken("other"))
	require.False(t, wv.matchesEditableFocusBridgeToken(""))
}
