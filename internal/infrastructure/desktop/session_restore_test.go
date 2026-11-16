package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/process"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestSessionRestoreLauncherDeliveryAndFallback(t *testing.T) {
	for _, scenario := range []string{"delivered", "cold", "failure", "unconfirmed"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			relay := portmocks.NewMockSessionRestoreRelay(t)
			delivered := scenario == "delivered" || scenario == "unconfirmed"
			var deliveryErr error
			if scenario == "failure" || scenario == "unconfirmed" {
				deliveryErr = errors.New("relay unavailable")
			}
			relay.EXPECT().DeliverRestoreSession(ctx, entity.SessionID("saved")).Return(delivered, deliveryErr).Once()
			started := false
			launcher := &sessionRestoreLauncher{relay: relay, start: func(id entity.SessionID) error {
				started = true
				require.Equal(t, entity.SessionID("saved"), id)
				return nil
			}}
			cold, err := launcher.RestoreSession(ctx, "saved")
			require.Equal(t, scenario == "cold", cold)
			if deliveryErr != nil {
				require.ErrorContains(t, err, "relay unavailable")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, scenario == "cold", started)
		})
	}
}

// Combine generated boundary mocks without hand-writing port behavior.
type restoreTestOpener struct {
	*portmocks.MockBrowserWindowOpener
	*portmocks.MockSessionRestoreOpener
}

func newRestoreTestOpener(t *testing.T) restoreTestOpener {
	return restoreTestOpener{portmocks.NewMockBrowserWindowOpener(t), portmocks.NewMockSessionRestoreOpener(t)}
}
func awaitRestoreTest[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for restore test event")
		var zero T
		return zero
	}
}

func TestBrowserLaunchRelayCarriesRestoreIDToProfileHost(t *testing.T) {
	root := shortTempDir(t)
	ipc := runtimeprofile.IPCPaths{RuntimeDir: root, BrowserLaunchSocket: filepath.Join(root, "browser-launch.sock")}
	relay := NewBrowserLaunchRelay(ipc)
	opener := newRestoreTestOpener(t)
	opener.MockSessionRestoreOpener.EXPECT().RestoreSession(mock.Anything, entity.SessionID("20251217_205106_a7b3")).Return(nil).Once()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	listener, err := relay.Listen(ctx, opener)
	require.NoError(t, err)
	defer listener.Close()
	delivered, err := relay.(port.SessionRestoreRelay).DeliverRestoreSession(ctx, "20251217_205106_a7b3")
	require.NoError(t, err)
	require.True(t, delivered)
}

func TestBrowserLaunchRelayRejectsUnsupportedRestore(t *testing.T) {
	root := shortTempDir(t)
	relay := NewBrowserLaunchRelay(runtimeprofile.IPCPaths{RuntimeDir: root, BrowserLaunchSocket: filepath.Join(root, "browser-launch.sock")})
	ctx := context.Background()
	listener, err := relay.Listen(ctx, portmocks.NewMockBrowserWindowOpener(t))
	require.NoError(t, err)
	defer listener.Close()
	delivered, err := relay.(port.SessionRestoreRelay).DeliverRestoreSession(ctx, "saved")
	require.True(t, delivered)
	require.ErrorContains(t, err, "unsupported")
}

func TestRestoreRelayReturnsHostFailureAfterCompletion(t *testing.T) {
	root := shortTempDir(t)
	relay := NewBrowserLaunchRelay(runtimeprofile.IPCPaths{RuntimeDir: root, BrowserLaunchSocket: filepath.Join(root, "browser-launch.sock")})
	entered, finish := make(chan struct{}), make(chan struct{})
	opener := newRestoreTestOpener(t)
	opener.MockSessionRestoreOpener.EXPECT().RestoreSession(mock.Anything, mock.Anything).RunAndReturn(func(context.Context, entity.SessionID) error {
		close(entered)
		awaitRestoreTest(t, finish)
		return errors.New("snapshot unavailable")
	}).Once()
	listener, err := relay.Listen(context.Background(), opener)
	require.NoError(t, err)
	defer listener.Close()
	result := make(chan error, 1)
	go func() {
		delivered, err := relay.(port.SessionRestoreRelay).DeliverRestoreSession(context.Background(), "20251217_205106_a7b3")
		if !delivered {
			result <- errors.New("not delivered")
			return
		}
		result <- err
	}()
	awaitRestoreTest(t, entered)
	select {
	case <-result:
		t.Fatal("restore acknowledged before completion")
	default:
	}
	close(finish)
	require.ErrorContains(t, awaitRestoreTest(t, result), "snapshot unavailable")
}

func TestRestoreRelayRejectsMalformedSessionIDs(t *testing.T) {
	root := shortTempDir(t)
	relay := NewBrowserLaunchRelay(runtimeprofile.IPCPaths{RuntimeDir: root, BrowserLaunchSocket: filepath.Join(root, "browser-launch.sock")})
	opener := newRestoreTestOpener(t)
	listener, err := relay.Listen(context.Background(), opener)
	require.NoError(t, err)
	defer listener.Close()
	for _, id := range []entity.SessionID{"", "saved", "../session", "20251317_205106_a7b3", "20251217_205106_zzzz", "20251217_205106_a7b3\n"} {
		t.Run(string(id), func(t *testing.T) {
			delivered, err := relay.(port.SessionRestoreRelay).DeliverRestoreSession(context.Background(), id)
			require.True(t, delivered)
			require.ErrorContains(t, err, "invalid")
		})
	}
}

func TestRestoreCompletionRequiresExplicitResult(t *testing.T) {
	for _, scenario := range []string{"old host ACK", "connection drop", "deadline"} {
		t.Run(scenario, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			ctx := context.Background()
			if scenario == "deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer cancel()
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer server.Close()
				if scenario == "old host ACK" {
					_ = json.NewEncoder(server).Encode(browserLaunchResponse{RequestID: "request", Accepted: true})
				}
			}()
			delivered, err := receiveRestoreCompletion(ctx, client, "request")
			require.True(t, delivered)
			require.ErrorIs(t, err, ErrSessionRestoreResultUnknown)
			awaitRestoreTest(t, done)
		})
	}
}

func TestRestoreHostTimeoutRetainsLeaseUntilWorkerFinishes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client, server := net.Pipe()
		defer client.Close()
		defer server.Close()
		release := make(chan struct{})
		gate := process.NewAdmissionGate()
		require.True(t, gate.Acquire())
		opener := portmocks.NewMockSessionRestoreOpener(t)
		opener.EXPECT().RestoreSession(mock.Anything, mock.Anything).RunAndReturn(func(context.Context, entity.SessionID) error { <-release; return nil }).Once()
		done := make(chan struct{})
		go func() {
			defer close(done)
			completeSessionRestore(context.Background(), server, browserLaunchRequest{RequestID: "request", SessionID: "20251217_205106_a7b3"}, opener, gate)
		}()
		var response browserLaunchResponse
		require.NoError(t, json.NewDecoder(client).Decode(&response))
		require.Equal(t, ErrSessionRestoreResultUnknown.Error(), response.Error)
		require.False(t, response.Completed)
		require.Equal(t, 1, gate.Active(), "timed-out response must retain admitted work")
		close(release)
		synctest.Wait()
		require.Zero(t, gate.Active(), "worker completion releases the lease")
		<-done
	})
}

func TestRestoreCompletionKeepsHostContextAlive(t *testing.T) {
	hostCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	require.NoError(t, client.SetDeadline(time.Now().Add(2*time.Second)))
	opener := portmocks.NewMockSessionRestoreOpener(t)
	var retained context.Context
	opener.EXPECT().RestoreSession(hostCtx, entity.SessionID("20251217_205106_a7b3")).RunAndReturn(func(ctx context.Context, _ entity.SessionID) error {
		retained = ctx
		return nil
	})
	gate := process.NewAdmissionGate()
	require.True(t, gate.Acquire())
	done := make(chan struct{})
	go func() {
		defer close(done)
		completeSessionRestore(hostCtx, server, browserLaunchRequest{RequestID: "request", SessionID: "20251217_205106_a7b3"}, opener, gate)
	}()
	var response browserLaunchResponse
	require.NoError(t, json.NewDecoder(client).Decode(&response))
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("completion did not finish")
	}
	require.True(t, response.Completed)
	require.Empty(t, response.Error)
	require.NotNil(t, retained)
	require.NoError(t, retained.Err(), "persistent UI context must outlive completion")
	cancel()
	require.ErrorIs(t, retained.Err(), context.Canceled, "host still owns the context lifetime")
}

func TestRestoreDuplicateWhileInProgressDoesNotDispatchTwice(t *testing.T) {
	root := shortTempDir(t)
	relay := NewBrowserLaunchRelay(runtimeprofile.IPCPaths{RuntimeDir: root, BrowserLaunchSocket: filepath.Join(root, "browser-launch.sock")}).(*browserLaunchRelay)
	entered, release := make(chan struct{}), make(chan struct{})
	opener := newRestoreTestOpener(t)
	opener.MockSessionRestoreOpener.EXPECT().RestoreSession(mock.Anything, mock.Anything).RunAndReturn(func(context.Context, entity.SessionID) error {
		close(entered)
		awaitRestoreTest(t, release)
		return nil
	}).Once()
	listener, err := relay.Listen(context.Background(), opener)
	require.NoError(t, err)
	defer listener.Close()
	request := browserLaunchRequest{RequestID: "duplicate", Action: browserLaunchActionRestoreSession, SessionID: "20251217_205106_a7b3"}
	first, err := net.Dial("unix", filepath.Join(root, "browser-launch.sock"))
	require.NoError(t, err)
	defer first.Close()
	require.NoError(t, first.SetDeadline(time.Now().Add(2*time.Second)))
	require.NoError(t, json.NewEncoder(first).Encode(request))
	awaitRestoreTest(t, entered)
	second, err := net.Dial("unix", filepath.Join(root, "browser-launch.sock"))
	require.NoError(t, err)
	defer second.Close()
	require.NoError(t, second.SetDeadline(time.Now().Add(2*time.Second)))
	require.NoError(t, json.NewEncoder(second).Encode(request))
	var duplicate browserLaunchResponse
	require.NoError(t, json.NewDecoder(second).Decode(&duplicate))
	require.Equal(t, ErrSessionRestoreResultUnknown.Error(), duplicate.Error)
	require.False(t, duplicate.Completed)
	require.Equal(t, 1, relay.admission.Active())
	close(release)
	var completed browserLaunchResponse
	require.NoError(t, json.NewDecoder(first).Decode(&completed))
	require.True(t, completed.Completed)
	require.Empty(t, completed.Error)
}
