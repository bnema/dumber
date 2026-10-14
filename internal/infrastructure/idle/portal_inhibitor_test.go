package idle

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/infrastructure/idle/mocks"
	"github.com/stretchr/testify/require"
)

const testHandle = "/org/freedesktop/portal/desktop/request/dumber/test"

var errPortal = errors.New("portal error")

// newTestInhibitor returns an inhibitor whose portal answered Version.
func newTestInhibitor(t *testing.T) (*PortalInhibitor, *mocks.MockPortalClient) {
	t.Helper()
	portal := mocks.NewMockPortalClient(t)
	portal.EXPECT().Version().Return(uint32(3), nil).Once()
	return newPortalInhibitor(context.Background(), portal), portal
}

func TestNewPortalInhibitorClosesUnavailablePortal(t *testing.T) {
	portal := mocks.NewMockPortalClient(t)
	portal.EXPECT().Version().Return(uint32(0), errPortal).Once()
	portal.EXPECT().Close().Return(nil).Once()

	p := newPortalInhibitor(context.Background(), portal)
	require.Nil(t, p.portal)

	// Without a portal, the refcount still works and makes no portal call.
	ctx := context.Background()
	require.NoError(t, p.Inhibit(ctx, "video playing"))
	require.True(t, p.IsInhibited())
	require.NoError(t, p.Uninhibit(ctx))
	require.False(t, p.IsInhibited())
	require.NoError(t, p.Close())
}

func TestPortalInhibitorRefcountsOneRequest(t *testing.T) {
	p, portal := newTestInhibitor(t)
	ctx := context.Background()
	portal.EXPECT().Inhibit("video playing").Return(testHandle, nil).Once()

	require.NoError(t, p.Inhibit(ctx, "video playing"))
	require.NoError(t, p.Inhibit(ctx, "second video"))
	require.NoError(t, p.Uninhibit(ctx))
	require.True(t, p.IsInhibited(), "released while one inhibit is still held")

	portal.EXPECT().CloseRequest(testHandle).Return(nil).Once()
	require.NoError(t, p.Uninhibit(ctx))
	require.False(t, p.IsInhibited())

	// Extra Uninhibit calls are no-ops.
	require.NoError(t, p.Uninhibit(ctx))
}

func TestPortalInhibitorIgnoresCloseRequestError(t *testing.T) {
	// A portal that already ended the request answers Close with an error.
	p, portal := newTestInhibitor(t)
	ctx := context.Background()
	portal.EXPECT().Inhibit("video playing").Return(testHandle, nil).Once()
	portal.EXPECT().CloseRequest(testHandle).Return(errPortal).Once()

	require.NoError(t, p.Inhibit(ctx, "video playing"))
	require.NoError(t, p.Uninhibit(ctx))

	// The next inhibition starts a new request.
	portal.EXPECT().Inhibit("next video").Return(testHandle, nil).Once()
	require.NoError(t, p.Inhibit(ctx, "next video"))
}

func TestPortalInhibitorRollsBackFailedInhibit(t *testing.T) {
	p, portal := newTestInhibitor(t)
	ctx := context.Background()
	portal.EXPECT().Inhibit("video playing").Return("", errPortal).Once()

	require.ErrorIs(t, p.Inhibit(ctx, "video playing"), errPortal)
	require.False(t, p.IsInhibited())

	// Nothing to release: no CloseRequest expected.
	require.NoError(t, p.Uninhibit(ctx))
}

func TestPortalInhibitorClose(t *testing.T) {
	tests := []struct {
		name     string
		inhibit  bool
		closeErr error
	}{
		{name: "releases active request", inhibit: true},
		{name: "without active request"},
		{name: "returns connection close error once", closeErr: errPortal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, portal := newTestInhibitor(t)
			if tt.inhibit {
				portal.EXPECT().Inhibit("video playing").Return(testHandle, nil).Once()
				portal.EXPECT().CloseRequest(testHandle).Return(nil).Once()
				require.NoError(t, p.Inhibit(context.Background(), "video playing"))
			}
			portal.EXPECT().Close().Return(tt.closeErr).Once()

			require.ErrorIs(t, p.Close(), tt.closeErr)
			require.False(t, p.IsInhibited())
			// Close is idempotent.
			require.NoError(t, p.Close())
		})
	}
}
