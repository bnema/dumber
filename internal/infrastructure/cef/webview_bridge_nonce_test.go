package cef

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewBridgeNonce_ReturnsEmptyOnRandomFailure(t *testing.T) {
	oldBridgeNonceRandom := bridgeNonceRandom
	bridgeNonceRandom = func([]byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}
	defer func() {
		bridgeNonceRandom = oldBridgeNonceRandom
	}()

	require.Empty(t, newBridgeNonce())
}

func TestWebViewRotateBridgeNonce_PreservesExistingNonceOnRandomFailure(t *testing.T) {
	oldBridgeNonceRandom := bridgeNonceRandom
	bridgeNonceRandom = func([]byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}
	defer func() {
		bridgeNonceRandom = oldBridgeNonceRandom
	}()

	wv := &WebView{bridgeNonce: "existing-nonce"}
	require.Empty(t, wv.rotateBridgeNonce())

	wv.mu.RLock()
	defer wv.mu.RUnlock()
	require.Equal(t, "existing-nonce", wv.bridgeNonce)
}

func TestWebViewRotateBridgeNonce_ReplacesNonceOnSuccess(t *testing.T) {
	oldBridgeNonceRandom := bridgeNonceRandom
	bridgeNonceRandom = func(buf []byte) (int, error) {
		for i := range buf {
			buf[i] = byte(i + 1)
		}
		return len(buf), nil
	}
	defer func() {
		bridgeNonceRandom = oldBridgeNonceRandom
	}()

	wv := &WebView{bridgeNonce: "old-nonce"}
	require.Equal(t, "0102030405060708090a0b0c0d0e0f10", wv.rotateBridgeNonce())

	wv.mu.RLock()
	defer wv.mu.RUnlock()
	require.Equal(t, "0102030405060708090a0b0c0d0e0f10", wv.bridgeNonce)
}

func TestWebViewEnsureBridgeNonce_ReusesExistingNonce(t *testing.T) {
	wv := &WebView{bridgeNonce: "existing-nonce"}
	require.Equal(t, "existing-nonce", wv.ensureBridgeNonce())
}
