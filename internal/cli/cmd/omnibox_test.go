package cmd

import "testing"

func TestRootCommand_RegistersOmniboxCommand(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"omnibox"})
	if err != nil {
		t.Fatalf("expected omnibox command to be registered: %v", err)
	}
	if cmd == nil || cmd.Name() != "omnibox" {
		t.Fatalf("expected omnibox command, got %#v", cmd)
	}
}
