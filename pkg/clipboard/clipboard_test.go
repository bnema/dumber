package clipboard

import (
	"os/exec"
	"testing"
)

func TestCopyToClipboard(t *testing.T) {
	// Check if either wl-copy or xclip is available
	if !IsAvailable() {
		t.Skip("Neither wl-copy nor xclip is available, skipping clipboard tests")
	}

	testText := "https://example.com/test-url"
	
	err := CopyToClipboard(testText)
	if err != nil {
		t.Errorf("CopyToClipboard failed: %v", err)
	}
}

func TestCopyToClipboard_EmptyString(t *testing.T) {
	err := CopyToClipboard("")
	if err == nil {
		t.Error("Expected error when copying empty string, got nil")
	}
}

func TestIsAvailable(t *testing.T) {
	available := IsAvailable()
	
	// Check if the result matches the actual availability
	wlCopyExists := false
	xclipExists := false
	
	if _, err := exec.LookPath("wl-copy"); err == nil {
		wlCopyExists = true
	}
	
	if _, err := exec.LookPath("xclip"); err == nil {
		xclipExists = true
	}
	
	expected := wlCopyExists || xclipExists
	
	if available != expected {
		t.Errorf("IsAvailable() = %v, expected %v", available, expected)
	}
}

func TestTryWlCopy(t *testing.T) {
	if _, err := exec.LookPath("wl-copy"); err != nil {
		t.Skip("wl-copy not available, skipping test")
	}
	
	err := tryWlCopy("test content")
	if err != nil {
		t.Errorf("tryWlCopy failed: %v", err)
	}
}

func TestTryXclip(t *testing.T) {
	if _, err := exec.LookPath("xclip"); err != nil {
		t.Skip("xclip not available, skipping test")
	}
	
	err := tryXclip("test content")
	if err != nil {
		t.Errorf("tryXclip failed: %v", err)
	}
}