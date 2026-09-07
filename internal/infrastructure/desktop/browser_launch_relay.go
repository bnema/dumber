package desktop

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"github.com/bnema/dumber/internal/infrastructure/process"
	"github.com/bnema/dumber/internal/infrastructure/runtimeprofile"
	"github.com/bnema/dumber/internal/logging"
	"github.com/rs/zerolog"
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
	// admission is the shared admission/work-lease boundary: a lease is
	// acquired before sending Accepted and released when dispatch settles.
	// Shutdown closes it so losing requests get an error response instead
	// of an acknowledgement. Initialized eagerly; never nil.
	admission *process.AdmissionGate
}

type browserLaunchRequest struct {
	RequestID string              `json:"request_id,omitempty"`
	URL       string              `json:"url"`
	Action    browserLaunchAction `json:"action,omitempty"`
}

type browserLaunchAction string

const (
	browserLaunchActionOpenExternalURL browserLaunchAction = "open-external-url"
	browserLaunchActionOpenFreshWindow browserLaunchAction = "open-fresh-window"
	// browserLaunchActionCloseAllWindows is a diagnostic-only action for the
	// residency harness: it closes every user window through the standard
	// removal path. It is honored only when the OWNING process sets
	// DUMBER_DIAGNOSTIC_WINDOW_CLOSE=1; otherwise it is rejected before
	// acknowledgement. It is never a general unauthenticated shutdown
	// command: the relay socket itself remains owner-only (0700, uid).
	browserLaunchActionCloseAllWindows browserLaunchAction = "close-all-windows"
)

// diagnosticWindowCloseEnvVar gates the diagnostic close action on the
// owning (server) process environment.
const diagnosticWindowCloseEnvVar = "DUMBER_DIAGNOSTIC_WINDOW_CLOSE"

// windowCloseOpener is implemented by openers that support the diagnostic
// close action. It is matched structurally so the port interface is
// unchanged.
type windowCloseOpener interface {
	CloseAllWindows(ctx context.Context) error
}

type browserLaunchResponse struct {
	RequestID string `json:"request_id,omitempty"`
	Accepted  bool   `json:"accepted,omitempty"`
	Error     string `json:"error,omitempty"`
}

type browserLaunchRelayListener struct {
	listener   *net.UnixListener
	socketPath string
	admission  *process.AdmissionGate
	once       sync.Once
	err        error
}

var newBrowserLaunchRequestID = func() string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err == nil {
		return "blr-" + hex.EncodeToString(buf[:])
	}
	return fmt.Sprintf("blr-%d", time.Now().UnixNano())
}

func NewBrowserLaunchRelay(ipc runtimeprofile.IPCPaths) port.BrowserLaunchRelay {
	return &browserLaunchRelay{ipc: ipc, admission: process.NewAdmissionGate()}
}

// AdmissionGateProvider exposes the relay's shared admission boundary
// without extending the port interface or regenerating mocks.
type AdmissionGateProvider interface {
	AdmissionGate() *process.AdmissionGate
}

// AdmissionGate returns the shared admission/work-lease boundary.
func (r *browserLaunchRelay) AdmissionGate() *process.AdmissionGate {
	if r == nil || r.admission == nil {
		return process.NewAdmissionGate()
	}
	return r.admission
}

func (r *browserLaunchRelay) DeliverOpenExternalURL(ctx context.Context, url string) (bool, error) {
	return r.deliver(ctx, url, "")
}

func (r *browserLaunchRelay) DeliverOpenFreshWindow(ctx context.Context, url string) (bool, error) {
	return r.deliver(ctx, url, browserLaunchActionOpenFreshWindow)
}

func (r *browserLaunchRelay) deliver(ctx context.Context, url string, action browserLaunchAction) (bool, error) {
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

	requestID := newBrowserLaunchRequestID()
	log := logging.FromContext(ctx)
	log.Debug().
		Str("request_id", requestID).
		Str("url_host", safeURLHost(url)).
		Msg("browser launch relay delivery started")

	if err := setBrowserLaunchConnDeadline(ctx, conn); err != nil {
		return false, err
	}
	if err := json.NewEncoder(conn).Encode(browserLaunchRequest{RequestID: requestID, URL: url, Action: action}); err != nil {
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
					log.Warn().
						Str("request_id", requestID).
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
	if response.RequestID != "" && response.RequestID != requestID {
		return true, fmt.Errorf("mismatched browser launch relay response request id: got %q, want %q", response.RequestID, requestID)
	}
	if response.Error != "" {
		log.Warn().
			Str("request_id", requestID).
			Str("url_host", safeURLHost(url)).
			Str("relay_error", response.Error).
			Msg("browser launch relay delivery rejected")
		return true, errors.New(response.Error)
	}

	log.Debug().
		Str("request_id", requestID).
		Str("url_host", safeURLHost(url)).
		Bool("accepted", response.Accepted || response.RequestID == "").
		Msg("browser launch relay delivery acknowledged")
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

func validateBrowserLaunchSocketDirOwned(socketPath string, expectedUID uint32) error {
	socketDir := filepath.Dir(socketPath)
	for dir := socketDir; ; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("stat browser launch dir: %w", err)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return nil
		}

		isSocketDir := dir == socketDir
		if stat.Uid != expectedUID {
			if isSocketDir || stat.Uid != 0 {
				return fmt.Errorf("browser launch dir owner mismatch: %s owned by uid %d, want uid %d", dir, stat.Uid, expectedUID)
			}
		}
		if info.Mode().Perm()&0o022 != 0 && (isSocketDir || info.Mode()&os.ModeSticky == 0) {
			return fmt.Errorf("browser launch dir permissions too broad: %s has mode %04o", dir, info.Mode().Perm())
		}
		if parent := filepath.Dir(dir); parent == dir {
			return nil
		}
	}
}

func (r *browserLaunchRelay) Listen(ctx context.Context, opener port.BrowserWindowOpener) (io.Closer, error) {
	socketPath, err := r.socketPath()
	if err != nil {
		return nil, err
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(socketPath), browserLaunchDirPerm); mkdirErr != nil {
		return nil, fmt.Errorf("create browser launch dir: %w", mkdirErr)
	}
	if ownerErr := validateBrowserLaunchSocketDirOwned(socketPath, uint32(os.Geteuid())); ownerErr != nil {
		return nil, ownerErr
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

		if ownerErr := validateBrowserLaunchSocketDirOwned(socketPath, uint32(os.Geteuid())); ownerErr != nil {
			return nil, ownerErr
		}

		if removeErr := os.Remove(socketPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return nil, fmt.Errorf("remove stale browser launch socket: %w", removeErr)
		}

		listener, err = net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
		if err != nil {
			return nil, fmt.Errorf("listen browser launch socket: %w", err)
		}
	}

	relayListener := &browserLaunchRelayListener{listener: listener, socketPath: socketPath, admission: r.AdmissionGate()}
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

// admitRelayConnection acquires one admission lease for the decoded request
// and enforces pre-acknowledgement eligibility. It returns nil after sending
// an error response (closed admission or unauthorized diagnostic action);
// the caller must return without acknowledging. An admitted lease is released
// by the caller on early failure and by dispatch completion otherwise.
func admitRelayConnection(
	conn *net.UnixConn,
	configured *process.AdmissionGate,
	request browserLaunchRequest,
	requestID string,
	log *zerolog.Logger,
) *process.AdmissionGate {
	gate := configured
	if gate == nil {
		gate = process.NewAdmissionGate()
	}
	refuse := func(errorBody string) *process.AdmissionGate {
		if err := conn.SetDeadline(time.Now().Add(browserLaunchIOTimeout)); err != nil {
			return nil
		}
		_ = json.NewEncoder(conn).Encode(browserLaunchResponse{RequestID: requestID, Error: errorBody})
		return nil
	}
	if !gate.Acquire() {
		log.Debug().
			Str("request_id", requestID).
			Msg("browser launch relay refused request after admission closed")
		return refuse("shutting down")
	}
	// The diagnostic close action is rejected before acknowledgement
	// unless the owning process explicitly enables it.
	if request.Action == browserLaunchActionCloseAllWindows && os.Getenv(diagnosticWindowCloseEnvVar) != "1" {
		gate.Release()
		log.Warn().
			Str("request_id", requestID).
			Msg("browser launch relay rejected diagnostic close without owner opt-in")
		return refuse("diagnostic close not enabled")
	}
	return gate
}

func (l *browserLaunchRelayListener) handleConnection(ctx context.Context, conn *net.UnixConn, opener port.BrowserWindowOpener) {
	defer func() { _ = conn.Close() }()
	log := logging.FromContext(ctx)
	if err := conn.SetDeadline(time.Now().Add(browserLaunchIOTimeout)); err != nil {
		return
	}

	var request browserLaunchRequest
	if err := json.NewDecoder(conn).Decode(&request); err != nil {
		return
	}
	requestID := request.RequestID
	if requestID == "" {
		requestID = newBrowserLaunchRequestID()
	}
	log.Debug().
		Str("request_id", requestID).
		Str("url_host", safeURLHost(request.URL)).
		Msg("browser launch relay request received")

	// Admit before acknowledgement: losing requests receive the existing
	// error response here, never an acknowledgement for work that will not
	// run. Acknowledgement still precedes expensive UI dispatch below, and
	// unconfirmed delivery keeps its no-duplicate-fallback semantics.
	gate := admitRelayConnection(conn, l.admission, request, requestID, log)
	if gate == nil {
		return
	}

	if err := conn.SetDeadline(time.Now().Add(browserLaunchIOTimeout)); err != nil {
		gate.Release()
		return
	}
	if err := json.NewEncoder(conn).Encode(browserLaunchResponse{RequestID: requestID, Accepted: true}); err != nil {
		log.Warn().Err(err).
			Str("request_id", requestID).
			Str("url_host", safeURLHost(request.URL)).
			Msg("failed to encode browser launch response")
		gate.Release()
		return
	}
	log.Debug().
		Str("request_id", requestID).
		Str("url_host", safeURLHost(request.URL)).
		Msg("browser launch relay request accepted")

	go func() {
		// The lease spans acknowledgement through dispatch completion or
		// definitive factory/dispatch failure. Factory/dispatch failure
		// retains the documented existing failure semantics and never
		// triggers duplicate fallback.
		defer gate.Release()
		if opener == nil {
			log.Warn().
				Str("request_id", requestID).
				Str("url_host", safeURLHost(request.URL)).
				Msg("browser launch relay accepted request without opener")
			return
		}
		started := time.Now()
		log.Debug().
			Str("request_id", requestID).
			Str("url_host", safeURLHost(request.URL)).
			Msg("browser launch relay calling browser URL opener")
		var err error
		switch request.Action {
		case "", browserLaunchActionOpenExternalURL:
			err = opener.OpenExternalURL(ctx, request.URL)
		case browserLaunchActionOpenFreshWindow:
			err = opener.OpenFreshWindow(ctx, request.URL)
		case browserLaunchActionCloseAllWindows:
			if closer, ok := opener.(windowCloseOpener); ok {
				err = closer.CloseAllWindows(ctx)
			} else {
				err = errors.New("browser launch relay opener does not support diagnostic close")
			}
		default:
			log.Warn().
				Str("request_id", requestID).
				Str("url_host", safeURLHost(request.URL)).
				Str("action", string(request.Action)).
				Msg("browser launch relay rejected unknown action")
			return
		}
		if err != nil {
			log.Warn().Err(err).
				Str("request_id", requestID).
				Str("url_host", safeURLHost(request.URL)).
				Dur("elapsed", time.Since(started)).
				Msg("browser launch relay browser URL opener failed")
			return
		}
		log.Debug().
			Str("request_id", requestID).
			Str("url_host", safeURLHost(request.URL)).
			Dur("elapsed", time.Since(started)).
			Msg("browser launch relay browser URL opener returned success")
	}()
}

var _ port.BrowserLaunchRelay = (*browserLaunchRelay)(nil)
