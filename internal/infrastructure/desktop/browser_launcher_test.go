package desktop

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserLauncher_LaunchURL_ReturnsWithoutSpawningWhenRelayDelivers(t *testing.T) {
	relay := mocks.NewMockBrowserLaunchRelay(t)
	relay.EXPECT().DeliverOpenFreshWindow(context.Background(), "https://example.com").Return(true, nil)

	launcher := NewBrowserLauncher(relay)
	spawned := false
	launcher.resolveExecutablePath = func() (string, error) {
		t.Fatal("expected executable path resolution to be skipped")
		return "", nil
	}
	launcher.startDetachedProcess = func(_ *exec.Cmd) error {
		spawned = true
		return nil
	}

	launcher.LaunchURL(context.Background(), "https://example.com")

	assert.False(t, spawned)
}

func TestBrowserLauncher_LaunchURL_FallsBackToSpawnWhenRelayMisses(t *testing.T) {
	relay := mocks.NewMockBrowserLaunchRelay(t)
	relay.EXPECT().DeliverOpenFreshWindow(context.Background(), "https://example.com").Return(false, nil)

	launcher := NewBrowserLauncher(relay)
	launcher.resolveExecutablePath = func() (string, error) {
		return "/usr/bin/dumber", nil
	}

	var gotCmd *exec.Cmd
	launcher.startDetachedProcess = func(cmd *exec.Cmd) error {
		gotCmd = cmd
		return nil
	}

	launcher.LaunchURL(context.Background(), "https://example.com")

	require.NotNil(t, gotCmd)
	assert.Equal(t, "/usr/bin/dumber", gotCmd.Path)
	assert.Equal(t, []string{"/usr/bin/dumber", "browse", "https://example.com"}, gotCmd.Args)
}

func TestBrowserLauncher_LaunchURL_ReportsRelayErrorWhenDelivered(t *testing.T) {
	relay := mocks.NewMockBrowserLaunchRelay(t)
	wantErr := errors.New("relay exploded")
	relay.EXPECT().DeliverOpenFreshWindow(context.Background(), "https://example.com").Return(true, wantErr)

	launcher := NewBrowserLauncher(relay)
	launcher.resolveExecutablePath = func() (string, error) {
		t.Fatal("expected executable path resolution to be skipped")
		return "", nil
	}
	launcher.startDetachedProcess = func(_ *exec.Cmd) error {
		t.Fatal("expected launch to stop after relay delivery")
		return nil
	}

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = oldStderr
	})

	launcher.LaunchURL(context.Background(), "https://example.com")

	require.NoError(t, w.Close())
	output, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Contains(t, string(output), wantErr.Error())
}
