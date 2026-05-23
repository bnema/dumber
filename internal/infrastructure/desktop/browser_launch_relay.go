package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	neturl "net/url"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/bnema/dumber/internal/logging"
)

const browserLaunchSocketName = "browser-launch.sock"

// Local browser-launch handoff needs enough headroom for a busy browser
// process to accept and acknowledge the request before callers classify the
// handoff as ambiguous.
const browserLaunchIOTimeout = 250 * time.Millisecond

const browserLaunchDirPerm = 0o700

// ErrBrowserLaunchRelayUnconfirmed reports that the relay accepted a launch
// request but the caller did not receive a confirmation response in time.
var ErrBrowserLaunchRelayUnconfirmed = errors.New("browser launch relay did not confirm delivery")

type browserLaunchRelay struct {
	ipc runtimeprofile.IPCPaths
}

type browserLaunchRequest struct {
	URL string `json:"url"`
}

type browserLaunchResponse struct {
	Error string `json:"error,omitempty"`
}

type browserLaunchRelayListener struct {
	listener   *net.UnixListener
	socketPath string
	once       sync.Once
	err        error
}

func NewBrowserLaunchRelay(ipc runtimeprofile.IPCPaths) port.BrowserLaunchRelay {
	return &browserLaunchRelay{ipc: ipc}
}

func (r *browserLaunchRelay) DeliverOpenFreshWindow(ctx context.Context, url string) (bool, error) {
	socketPath, err := r.socketPath()
	if err != nil {
		return false, err
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		if isMissingRelayListener(err) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = conn.Close() }()

	if err := setBrowserLaunchConnDeadline(ctx, conn); err != nil {
		return false, err
	}
	if err := json.NewEncoder(conn).Encode(browserLaunchRequest{URL: url}); err != nil {
		return false, err
	}

	if err := setBrowserLaunchConnDeadline(ctx, conn); err != nil {
		return false, err
	}

	var response browserLaunchResponse
	decoder := json.NewDecoder(conn)
	for {
		if decodeErr := decoder.Decode(&response); decodeErr != nil {
			if isBrowserLaunchReadTimeout(decodeErr) {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return false, ctxErr
				}
				if _, ok := ctx.Deadline(); !ok {
					logging.FromContext(ctx).Warn().
						Str("url_host", safeURLHost(url)).
						Dur("timeout", browserLaunchIOTimeout).
						Msg("browser launch relay response timed out without caller deadline; delivery is unconfirmed")
					return true, ErrBrowserLaunchRelayUnconfirmed
				}
				if deadlineErr := setBrowserLaunchConnDeadline(ctx, conn); deadlineErr != nil {
					return false, deadlineErr
				}
				continue
			}
			return false, decodeErr
		}
		break
	}
	if response.Error != "" {
		return true, errors.New(response.Error)
	}

	return true, nil
}

func isMissingRelayListener(err error) bool {
	return os.IsNotExist(err) || errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ENOENT)
}

func safeURLHost(raw string) string {
	parsed, err := neturl.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}

func isBrowserLaunchReadTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func browserLaunchSocketHasLiveListener(socketPath string) (bool, error) {
	conn, err := net.DialTimeout("unix", socketPath, browserLaunchIOTimeout)
	if err == nil {
		_ = conn.Close()
		return true, nil
	}

	if isMissingRelayListener(err) {
		return false, nil
	}

	return false, fmt.Errorf("check browser launch socket: %w", err)
}

func setBrowserLaunchConnDeadline(ctx context.Context, conn net.Conn) error {
	deadline := time.Now().Add(browserLaunchIOTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}

	return conn.SetDeadline(deadline)
}

func (r *browserLaunchRelay) Listen(ctx context.Context, opener port.BrowserWindowOpener) (io.Closer, error) {
	socketPath, err := r.socketPath()
	if err != nil {
		return nil, err
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(socketPath), browserLaunchDirPerm); mkdirErr != nil {
		return nil, fmt.Errorf("create browser launch dir: %w", mkdirErr)
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, fmt.Errorf("listen browser launch socket: %w", err)
		}

		live, liveErr := browserLaunchSocketHasLiveListener(socketPath)
		if liveErr != nil {
			return nil, liveErr
		}
		if live {
			return nil, errors.New("browser launch relay already running")
		}

		if removeErr := os.Remove(socketPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, fmt.Errorf("remove stale browser launch socket: %w", removeErr)
		}

		listener, err = net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
		if err != nil {
			return nil, fmt.Errorf("listen browser launch socket: %w", err)
		}
	}

	relayListener := &browserLaunchRelayListener{listener: listener, socketPath: socketPath}
	go relayListener.serve(ctx, opener)

	return relayListener, nil
}

func (r *browserLaunchRelay) socketPath() (string, error) {
	if r == nil {
		return "", errors.New("browser launch relay missing IPC paths")
	}
	if r.ipc.BrowserLaunchSocket == "" {
		return "", errors.New("browser launch relay missing browser launch socket path")
	}
	return r.ipc.BrowserLaunchSocket, nil
}

func (l *browserLaunchRelayListener) Close() error {
	l.once.Do(func() {
		if l.listener != nil {
			l.err = l.listener.Close()
		}
		_ = os.Remove(l.socketPath)
	})

	return l.err
}

func (l *browserLaunchRelayListener) serve(ctx context.Context, opener port.BrowserWindowOpener) {
	defer func() { _ = l.Close() }()

	for {
		if err := l.listener.SetDeadline(time.Now().Add(browserLaunchIOTimeout)); err != nil {
			return
		}
		conn, err := l.listener.AcceptUnix()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return
			}
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				if ctx.Err() != nil {
					return
				}
				continue
			}
			continue
		}

		go l.handleConnection(ctx, conn, opener)
	}
}

func (*browserLaunchRelayListener) handleConnection(ctx context.Context, conn *net.UnixConn, opener port.BrowserWindowOpener) {
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(browserLaunchIOTimeout)); err != nil {
		return
	}

	var request browserLaunchRequest
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		return
	}

	if err := conn.SetDeadline(time.Now().Add(browserLaunchIOTimeout)); err != nil {
		return
	}
	if err := json.NewEncoder(conn).Encode(browserLaunchResponse{}); err != nil {
		log := logging.FromContext(ctx)
		log.Warn().Err(err).Str("url", request.URL).Msg("failed to encode browser launch response")
		return
	}

	go func() {
		if err := opener.OpenFreshWindow(ctx, request.URL); err != nil {
			logging.FromContext(ctx).Warn().Err(err).Str("url", request.URL).Msg("failed to open fresh browser window")
		}
	}()
}

var _ port.BrowserLaunchRelay = (*browserLaunchRelay)(nil)
