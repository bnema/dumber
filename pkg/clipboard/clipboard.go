package clipboard

import (
	"fmt"
	"os/exec"
)

// CopyToClipboard copies the given text to the system clipboard.
// It tries wl-copy first (Wayland), then falls back to xclip (X11).
// Returns an error if both fail.
func CopyToClipboard(text string) error {
	if text == "" {
		return fmt.Errorf("cannot copy empty text to clipboard")
	}

	// Try wl-copy first (Wayland)
	if err := tryWlCopy(text); err == nil {
		return nil
	}

	// Fallback to xclip (X11)
	if err := tryXclip(text); err == nil {
		return nil
	}

	return fmt.Errorf("clipboard copy failed: neither wl-copy nor xclip available")
}

// tryWlCopy attempts to copy text using wl-copy (Wayland clipboard)
func tryWlCopy(text string) error {
	cmd := exec.Command("wl-copy")
	cmd.Stdin = nil
	
	// Set up stdin pipe to pass the text
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe for wl-copy: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to start wl-copy: %w", err)
	}

	// Write text to stdin and close it
	_, writeErr := stdin.Write([]byte(text))
	closeErr := stdin.Close()

	// Wait for command to complete
	waitErr := cmd.Wait()

	// Return the first error encountered
	if writeErr != nil {
		return fmt.Errorf("failed to write to wl-copy stdin: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close wl-copy stdin: %w", closeErr)
	}
	if waitErr != nil {
		return fmt.Errorf("wl-copy command failed: %w", waitErr)
	}

	return nil
}

// tryXclip attempts to copy text using xclip (X11 clipboard)
func tryXclip(text string) error {
	cmd := exec.Command("xclip", "-selection", "clipboard")
	
	// Set up stdin pipe to pass the text
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe for xclip: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return fmt.Errorf("failed to start xclip: %w", err)
	}

	// Write text to stdin and close it
	_, writeErr := stdin.Write([]byte(text))
	closeErr := stdin.Close()

	// Wait for command to complete
	waitErr := cmd.Wait()

	// Return the first error encountered
	if writeErr != nil {
		return fmt.Errorf("failed to write to xclip stdin: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close xclip stdin: %w", closeErr)
	}
	if waitErr != nil {
		return fmt.Errorf("xclip command failed: %w", waitErr)
	}

	return nil
}

// IsAvailable checks if clipboard functionality is available on the system
func IsAvailable() bool {
	// Check if wl-copy is available
	if _, err := exec.LookPath("wl-copy"); err == nil {
		return true
	}
	
	// Check if xclip is available
	if _, err := exec.LookPath("xclip"); err == nil {
		return true
	}
	
	return false
}