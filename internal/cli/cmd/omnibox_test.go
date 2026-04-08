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

func TestRootCommand_BrowseCommandRejectsExtraArgs(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"browse"})
	if err != nil {
		t.Fatalf("expected browse command to be registered: %v", err)
	}
	if cmd == nil || cmd.Name() != "browse" {
		t.Fatalf("expected browse command, got %#v", cmd)
	}
	if cmd.Args == nil {
		t.Fatal("expected browse command to define argument validation")
	}
	if err := cmd.Args(cmd, []string{"https://example.com", "extra"}); err == nil {
		t.Fatal("expected browse command to reject extra args")
	}
}
