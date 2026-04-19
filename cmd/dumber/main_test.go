package main

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/bootstrap"
	"github.com/bnema/dumber/internal/infrastructure/colorscheme"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

func TestLaunchModeFromArgs_DetectsStandaloneOmnibox(t *testing.T) {
	mode, _ := launchModeFromArgs([]string{"dumber", "omnibox"})
	if mode != launchModeStandaloneOmnibox {
		t.Fatalf("expected standalone omnibox mode, got %q", mode)
	}
}

func TestLaunchModeFromArgs_DetectsBrowseURL(t *testing.T) {
	mode, browseURL := launchModeFromArgs([]string{"dumber", "browse", "https://example.com"})
	if mode != launchModeBrowse {
		t.Fatalf("expected browse mode, got %q", mode)
	}
	if browseURL != "https://example.com" {
		t.Fatalf("expected browse url to be preserved, got %q", browseURL)
	}
}

func TestLaunchModeFromArgs_BrowseHelpFallsBackToCLI(t *testing.T) {
	mode, browseURL := launchModeFromArgs([]string{"dumber", "browse", "--help"})
	if mode != launchModeCLI {
		t.Fatalf("expected cli mode for browse help, got %q", mode)
	}
	if browseURL != "" {
		t.Fatalf("expected empty browse url for browse help, got %q", browseURL)
	}
}

func TestLaunchModeFromArgs_BrowseExtraPositionalFallsBackToCLI(t *testing.T) {
	mode, browseURL := launchModeFromArgs([]string{"dumber", "browse", "https://example.com", "extra"})
	if mode != launchModeCLI {
		t.Fatalf("expected cli mode for browse extra args, got %q", mode)
	}
	if browseURL != "" {
		t.Fatalf("expected empty browse url for browse extra args, got %q", browseURL)
	}
}

func TestLaunchModeFromArgs_DefaultsToCLI(t *testing.T) {
	mode, browseURL := launchModeFromArgs([]string{"dumber"})
	if mode != launchModeCLI {
		t.Fatalf("expected cli mode, got %q", mode)
	}
	if browseURL != "" {
		t.Fatalf("expected empty browse url, got %q", browseURL)
	}
}

func TestLaunchModeFromArgs_OmniboxHelpFallsBackToCLI(t *testing.T) {
	mode, _ := launchModeFromArgs([]string{"dumber", "omnibox", "--help"})
	if mode != launchModeCLI {
		t.Fatalf("expected cli mode for omnibox help, got %q", mode)
	}
}

func TestLaunchModeFromArgs_OmniboxFlagFallsBackToCLI(t *testing.T) {
	mode, _ := launchModeFromArgs([]string{"dumber", "omnibox", "--bad-flag"})
	if mode != launchModeCLI {
		t.Fatalf("expected cli mode for omnibox flags, got %q", mode)
	}
}

func TestIsCEFSubprocess(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "equals form",
			args: []string{"dumber", "--type=renderer"},
			want: true,
		},
		{
			name: "separate value form",
			args: []string{"dumber", "--type", "renderer"},
			want: true,
		},
		{
			name: "bare type",
			args: []string{"dumber", "--type"},
			want: false,
		},
		{
			name: "type followed by flag",
			args: []string{"dumber", "--type", "--unexpected"},
			want: false,
		},
		{
			name: "no type flag",
			args: []string{"dumber", "--unexpected"},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCEFSubprocess(tc.args); got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}

func TestResolveCurrentExecutable_ReturnsExecutablePath(t *testing.T) {
	got, err := resolveCurrentExecutable(func() (string, error) {
		return "/usr/bin/dumber", nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != "/usr/bin/dumber" {
		t.Fatalf("expected executable path to be preserved, got %q", got)
	}
}

func TestResolveCurrentExecutable_PropagatesError(t *testing.T) {
	wantErr := errors.New("not found")
	_, err := resolveCurrentExecutable(func() (string, error) {
		return "", wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
}

func TestPreInitializeAdwaitaForCEF_InitializesAndMarksDetector(t *testing.T) {
	initResult := &bootstrap.ParallelInitResult{
		AdwaitaDetector: colorscheme.NewAdwaitaDetector(),
	}
	cfg := &config.Config{}
	cfg.Engine.Type = config.EngineTypeCEF

	called := false
	preInitializeAdwaitaForCEF(cfg, initResult, func() {
		called = true
	})

	if !called {
		t.Fatal("expected libadwaita initialization for CEF")
	}
	if !initResult.AdwaitaDetector.Available() {
		t.Fatal("expected adwaita detector to be marked available")
	}
}

func TestPreInitializeAdwaitaForCEF_SkipsNonCEF(t *testing.T) {
	initResult := &bootstrap.ParallelInitResult{
		AdwaitaDetector: colorscheme.NewAdwaitaDetector(),
	}
	cfg := &config.Config{}
	cfg.Engine.Type = config.EngineTypeWebKit

	called := false
	preInitializeAdwaitaForCEF(cfg, initResult, func() {
		called = true
	})

	if called {
		t.Fatal("expected libadwaita initialization to be skipped for non-CEF")
	}
	if initResult.AdwaitaDetector.Available() {
		t.Fatal("expected adwaita detector to remain unavailable")
	}
}

func TestTryForwardBrowseURLToRunningInstance_ReturnsTrueOnRelayHit(t *testing.T) {
	relay := mocks.NewMockBrowserLaunchRelay(t)
	relay.EXPECT().DeliverOpenFreshWindow(context.Background(), "https://example.com").Return(true, nil)

	forwarded, err := tryForwardBrowseURLToRunningInstance(context.Background(), relay, "https://example.com")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !forwarded {
		t.Fatal("expected browse URL to be forwarded")
	}
}

func TestTryForwardBrowseURLToRunningInstance_ReturnsFalseOnRelayMiss(t *testing.T) {
	relay := mocks.NewMockBrowserLaunchRelay(t)
	relay.EXPECT().DeliverOpenFreshWindow(context.Background(), "https://example.com").Return(false, nil)

	forwarded, err := tryForwardBrowseURLToRunningInstance(context.Background(), relay, "https://example.com")

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if forwarded {
		t.Fatal("expected browse URL to remain unforwarded")
	}
}

func TestTryForwardBrowseURLToRunningInstance_PropagatesError(t *testing.T) {
	relay := mocks.NewMockBrowserLaunchRelay(t)
	wantErr := errors.New("relay error")
	relay.EXPECT().DeliverOpenFreshWindow(context.Background(), "https://example.com").Return(false, wantErr)

	forwarded, err := tryForwardBrowseURLToRunningInstance(context.Background(), relay, "https://example.com")

	if !errors.Is(err, wantErr) {
		t.Fatalf("expected error %v, got %v", wantErr, err)
	}
	if forwarded {
		t.Fatal("expected forwarded to be false on error")
	}
}
