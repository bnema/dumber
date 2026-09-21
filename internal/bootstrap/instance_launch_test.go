package bootstrap

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/stretchr/testify/require"
)

type instanceRelayStub struct {
	port.BrowserLaunchRelay
	err           error
	prepared      bool
	lease         io.Closer
	deliverCalls  int
	deliverAfter  int
	deliveredKind string
}

func (r *instanceRelayStub) deliver(kind string) (bool, error) {
	r.deliveredKind = kind
	r.deliverCalls++
	return r.deliverCalls >= r.deliverAfter, nil
}

func (r *instanceRelayStub) DeliverOpenExternalURL(context.Context, string) (bool, error) {
	return r.deliver("external")
}

func (r *instanceRelayStub) DeliverOpenFreshWindow(context.Context, string) (bool, error) {
	return r.deliver("fresh")
}

func (r *instanceRelayStub) DeliverOpenInstance(context.Context, string, string) (bool, error) {
	return r.deliver("instance")
}

func (r *instanceRelayStub) Prepare(lease io.Closer) error {
	r.prepared = true
	r.lease = lease
	return r.err
}
func testInstanceProfile(t *testing.T) runtimeprofile.Profile {
	t.Helper()
	root := t.TempDir()
	return runtimeprofile.Profile{InstanceRoot: root, IPC: runtimeprofile.IPCPaths{BrowserLaunchSocket: filepath.Join(root, "browser-launch.sock")}}
}

// testColdDefaultProfile mirrors main's default-profile path before
// normalization: InstanceRoot is empty while IPC.RuntimeDir is populated.
func testColdDefaultProfile(t *testing.T) runtimeprofile.Profile {
	t.Helper()
	runtimeDir := t.TempDir()
	return runtimeprofile.Profile{IPC: runtimeprofile.IPCPaths{
		RuntimeDir:          runtimeDir,
		BrowserLaunchSocket: filepath.Join(runtimeDir, "browser-launch.sock"),
	}}
}

func TestRunWithInstanceRelayFailsBeforeEngineStartup(t *testing.T) {
	failure := errors.New("bind failed")
	relay := &instanceRelayStub{err: failure}
	code, err := RunWithInstanceRelay(context.Background(), testInstanceProfile(t), relay, func() int { t.Fatal("must not run"); return 0 })
	require.Equal(t, 1, code)
	require.ErrorIs(t, err, failure)
	require.True(t, relay.prepared)
}
func TestDeliverOrRunWithInstanceRelayPreservesLaunchIntent(t *testing.T) {
	for _, test := range []struct {
		name        string
		instance    string
		freshWindow bool
		want        string
	}{
		{name: "default external", want: "external"},
		{name: "named instance", instance: "work", want: "instance"},
		{name: "fresh window", instance: "work", freshWindow: true, want: "fresh"},
		{name: "fresh window without instance", freshWindow: true, want: "fresh"},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := testInstanceProfile(t)
			owner, err := acquireInstanceNamespace(context.Background(), profile)
			require.NoError(t, err)
			defer owner.Close()
			relay := &instanceRelayStub{deliverAfter: 1}
			code, err := DeliverOrRunWithInstanceRelay(context.Background(), profile, relay, test.instance, "https://example.com", test.freshWindow, func() int {
				t.Fatal("delivered request must not start host")
				return 1
			})
			require.NoError(t, err)
			require.Zero(t, code)
			require.Equal(t, test.want, relay.deliveredKind)
		})
	}
}

func TestDeliverOrRunWithInstanceRelayRetriesWhileHostStarts(t *testing.T) {
	profile := testInstanceProfile(t)
	owner, err := acquireInstanceNamespace(context.Background(), profile)
	require.NoError(t, err)
	defer owner.Close()
	relay := &instanceRelayStub{deliverAfter: 3}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	code, err := DeliverOrRunWithInstanceRelay(ctx, profile, relay, "work", "https://example.com", false, func() int {
		t.Fatal("loser must not start host")
		return 1
	})
	require.NoError(t, err)
	require.Zero(t, code)
	require.Equal(t, 3, relay.deliverCalls)
}

// With a cold default profile no host is running, so the first process must
// normalize InstanceRoot to IPC.RuntimeDir, acquire the lease, prepare the
// relay, and start the host instead of spinning on an empty runtime path. The
// real default cold start carries an empty instance name (main passes
// args.Instance verbatim), so delivery must use the external-URL path.
func TestDeliverOrRunWithInstanceRelayStartsHostForColdDefaultProfile(t *testing.T) {
	profile := testColdDefaultProfile(t)
	relay := &instanceRelayStub{deliverAfter: 1 << 30}
	ran := false

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	code, err := DeliverOrRunWithInstanceRelay(ctx, profile, relay, "", "https://example.com", false, func() int {
		ran = true
		return 0
	})
	require.NoError(t, err)
	require.Zero(t, code)
	require.True(t, ran, "cold default profile must start the host")
	require.True(t, relay.prepared, "host must prepare the relay")
	require.Equal(t, 1, relay.deliverCalls)
	require.Equal(t, "external", relay.deliveredKind, "empty instance name must not use the instance relay")
}

// A permanent acquisition failure (for example a runtime root that cannot be
// created) must return immediately rather than retry until the context ends.
func TestDeliverOrRunWithInstanceRelayReturnsPermanentAcquireError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-a-dir")
	require.NoError(t, os.WriteFile(root, []byte("file"), 0o600))
	profile := runtimeprofile.Profile{
		InstanceRoot: root,
		IPC:          runtimeprofile.IPCPaths{BrowserLaunchSocket: filepath.Join(root, "browser-launch.sock")},
	}
	relay := &instanceRelayStub{deliverAfter: 1 << 30}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	start := time.Now()
	code, err := DeliverOrRunWithInstanceRelay(ctx, profile, relay, "default", "https://example.com", false, func() int {
		t.Fatal("permanent failure must not start the host")
		return 1
	})
	elapsed := time.Since(start)
	require.Error(t, err)
	require.Equal(t, 1, code)
	require.NotErrorIs(t, err, errInstanceNamespaceContended)
	require.ErrorContains(t, err, "create named instance runtime directory")
	require.Less(t, elapsed, instanceElectionSlice, "permanent failure must not retry")
	require.Equal(t, 1, relay.deliverCalls, "delivery is attempted once before election")
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
