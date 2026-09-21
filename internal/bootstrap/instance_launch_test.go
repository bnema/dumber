package bootstrap

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/stretchr/testify/require"
)

type instanceRelayStub struct {
	port.BrowserLaunchRelay
	err      error
	prepared bool
	lease    io.Closer
}

func (r *instanceRelayStub) Prepare(lease io.Closer) error {
	r.prepared = true
	r.lease = lease
	return r.err
}
func testInstanceProfile(t *testing.T) runtimeprofile.Profile {
	root := t.TempDir()
	return runtimeprofile.Profile{InstanceRoot: root, IPC: runtimeprofile.IPCPaths{BrowserLaunchSocket: filepath.Join(root, "browser-launch.sock")}}
}

func TestRunWithInstanceRelayFailsBeforeEngineStartup(t *testing.T) {
	failure := errors.New("bind failed")
	relay := &instanceRelayStub{err: failure}
	code, err := RunWithInstanceRelay(context.Background(), testInstanceProfile(t), relay, func() int { t.Fatal("must not run"); return 0 })
	require.Equal(t, 1, code)
	require.ErrorIs(t, err, failure)
	require.True(t, relay.prepared)
}
func TestRunWithInstanceRelayOwnsLeaseThroughGUILifetime(t *testing.T) {
	profile := testInstanceProfile(t)
	relay := &instanceRelayStub{}
	code, err := RunWithInstanceRelay(context.Background(), profile, relay, func() int {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, competitorErr := acquireInstanceNamespace(ctx, profile)
		require.Error(t, competitorErr)
		return 0
	})
	require.NoError(t, err)
	require.Zero(t, code)
	lease, err := acquireInstanceNamespace(context.Background(), profile)
	require.NoError(t, err)
	require.NoError(t, lease.Close())
}
