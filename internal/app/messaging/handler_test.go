package messaging

import "testing"

func TestParseIncomingMessageAcceptsNumericWebViewID(t *testing.T) {
	payload := `{"type":"workspace","event":"focus","webviewId":42}`
	msg, err := parseIncomingMessage(payload)
	if err != nil {
		t.Fatalf("unexpected error parsing payload: %v", err)
	}

	if msg.WebViewID != "42" {
		t.Fatalf("expected webviewId to be \"42\", got %q", msg.WebViewID)
	}
}

func TestParseIncomingMessagePreservesStringWebViewID(t *testing.T) {
	payload := `{"type":"workspace","event":"focus","webviewId":"popup-123"}`
	msg, err := parseIncomingMessage(payload)
	if err != nil {
		t.Fatalf("unexpected error parsing payload: %v", err)
	}

	if msg.WebViewID != "popup-123" {
		t.Fatalf("expected webviewId to remain popup-123, got %q", msg.WebViewID)
	}
}
