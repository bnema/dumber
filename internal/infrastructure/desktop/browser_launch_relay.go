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
	"regexp"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
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

// ErrSessionRestoreResultUnknown prevents duplicate fallback after ownership may transfer.
var ErrSessionRestoreResultUnknown = errors.New("restore delivered but result unknown")

const sessionRestoreCompletionTimeout = 10 * time.Second

var restoreSessionIDPattern = regexp.MustCompile(`^\d{8}_\d{6}_[0-9a-f]{4}$`)

func validRestoreSessionID(id entity.SessionID) bool {
	if !restoreSessionIDPattern.MatchString(string(id)) {
		return false
	}
	_, err := time.Parse("20060102_150405", string(id)[:15])
	return err == nil
}

type browserLaunchRelay struct {
	ipc       runtimeprofile.IPCPaths
	prepareMu sync.Mutex
	prepared  *browserLaunchRelayListener
	lease     io.Closer
	serving   bool
	// admission is the shared admission/work-lease boundary: a lease is
	// acquired before sending Accepted and released when dispatch settles.
	// Shutdown closes it so losing requests get an error response instead
	// of an acknowledgement. Initialized eagerly; never nil.
	admission *process.AdmissionGate
}

type browserLaunchRequest struct {
	RequestID string              `json:"request_id,omitempty"`
	SessionID entity.SessionID    `json:"session_id,omitempty"`
	URL       string              `json:"url"`
	Instance  string              `json:"instance,omitempty"`
	Action    browserLaunchAction `json:"action,omitempty"`
}

type browserLaunchAction string

const (
	browserLaunchActionOpenExternalURL browserLaunchAction = "open-external-url"
	browserLaunchActionOpenFreshWindow browserLaunchAction = "open-fresh-window"
	browserLaunchActionOpenInstance    browserLaunchAction = "open-instance"
	browserLaunchActionRestoreSession  browserLaunchAction = "restore-session"
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
	Completed bool   `json:"completed,omitempty"`
	Error     string `json:"error,omitempty"`
}

type browserLaunchRelayListener struct {
	listener   *net.UnixListener
	socketPath string
	socketInfo os.FileInfo
	admission  *process.AdmissionGate
	seenMu     sync.Mutex
	seen       map[string]struct{}
	once       sync.Once
	err        error
	closed     atomic.Bool
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

// Compile-time check: the shared gate implements the application boundary.
var _ port.AdmissionBoundary = (*process.AdmissionGate)(nil)

// AdmissionGate returns the shared admission/work-lease boundary.
func (r *browserLaunchRelay) AdmissionGate() *process.AdmissionGate {
	if r == nil || r.admission == nil {
		return process.NewAdmissionGate()
	}
	return r.admission
}

func (r *browserLaunchRelay) DeliverOpenExternalURL(ctx context.Context, url string) (bool, error) {
	return r.deliver(ctx, url, "", "")
}

func (r *browserLaunchRelay) DeliverOpenFreshWindow(ctx context.Context, url string) (bool, error) {
	return r.deliver(ctx, url, "", browserLaunchActionOpenFreshWindow)
}

func (r *browserLaunchRelay) DeliverOpenInstance(ctx context.Context, name, url string) (bool, error) {
	return r.deliver(ctx, url, name, browserLaunchActionOpenInstance)
}

func (r *browserLaunchRelay) deliver(ctx context.Context, url, instance string, action browserLaunchAction) (bool, error) {
	return r.deliverRequest(ctx, browserLaunchRequest{URL: url, Instance: instance, Action: action})
}

func (r *browserLaunchRelay) DeliverRestoreSession(ctx context.Context, id entity.SessionID) (bool, error) {
	return r.deliverRequest(ctx, browserLaunchRequest{Action: browserLaunchActionRestoreSession, SessionID: id})
}

func (r *browserLaunchRelay) deliverRequest(ctx context.Context, request browserLaunchRequest) (bool, error) {
	url := request.URL
	socketPath, err := r.socketPath()
	if err != nil {
		return false, err
	}

	if _, statErr := os.Stat(filepath.Dir(socketPath)); statErr == nil {
		if ownerErr := validateBrowserLaunchSocketDirOwned(socketPath, uint32(os.Geteuid())); ownerErr != nil {
			return false, ownerErr
		}
	} else if !os.IsNotExist(statErr) {
		return false, fmt.Errorf("stat browser launch dir: %w", statErr)
	}

	dialer := net.Dialer{}
	if request.Action == browserLaunchActionRestoreSession {
		dialer.Timeout = browserLaunchIOTimeout
	}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
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
	request.RequestID = requestID
	if request.Action == browserLaunchActionRestoreSession {
		return sendRestoreRequest(ctx, conn, request)
	}
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return false, err
	}

	if err := setBrowserLaunchConnDeadline(ctx, conn); err != nil {
		return false, err
	}

	return receiveLaunchAcknowledgement(ctx, conn, requestID, url, log)
}

// receiveLaunchAcknowledgement preserves the admission-only contract for URL/window actions.
func receiveLaunchAcknowledgement(ctx context.Context, conn net.Conn, requestID, url string, log *zerolog.Logger) (bool, error) {
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
	r.prepareMu.Lock()
	defer r.prepareMu.Unlock()
	if r.serving {
		return nil, errors.New("browser launch relay already serving")
	}
	listener := r.prepared
	if listener != nil && listener.closed.Load() {
		return nil, errors.New("browser launch relay reservation closed")
	}
	if listener == nil {
		var err error
		listener, err = r.bind(true)
		if err != nil {
			return nil, err
		}
	}
	r.serving = true
	go listener.serve(ctx, opener)
	return listener, nil
}

// Prepare binds without accepting or acknowledging requests until Listen.
// The supplied lease proves exclusive namespace ownership and must outlive
// this relay; stale socket recovery is only safe while that lease is held.
func (r *browserLaunchRelay) Prepare(namespaceLease io.Closer) error {
	r.prepareMu.Lock()
	defer r.prepareMu.Unlock()
	if r.prepared != nil || r.serving {
		return errors.New("browser launch relay already prepared or serving")
	}
	if namespaceLease == nil {
		return errors.New("browser launch relay requires namespace lease")
	}
	listener, err := r.bind(true)
	if err != nil {
		return err
	}
	r.lease = namespaceLease
	r.prepared = listener
	return nil
}

func (r *browserLaunchRelay) bind(recoverStale bool) (*browserLaunchRelayListener, error) {
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
		if !recoverStale || !errors.Is(err, syscall.EADDRINUSE) {
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

	// Go's UnixListener otherwise unlinks by pathname on Close, which can
	// remove a successor socket created after this listener was closed.
	listener.SetUnlinkOnClose(false)
	socketInfo, err := os.Lstat(socketPath)
	if err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("stat bound browser launch socket: %w", err)
	}
	return &browserLaunchRelayListener{
		listener:   listener,
		socketPath: socketPath,
		socketInfo: socketInfo,
		admission:  r.AdmissionGate(),
		seen:       make(map[string]struct{}),
	}, nil
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
		l.closed.Store(true)
		if l.listener != nil {
			l.err = l.listener.Close()
		}
		// Remove only the directory entry that still names this listener.
		// A concurrently-created successor owns a different inode and must live.
		if current, err := os.Lstat(l.socketPath); err == nil && l.socketInfo != nil && os.SameFile(current, l.socketInfo) {
			l.err = errors.Join(l.err, os.Remove(l.socketPath))
		} else if err != nil && !os.IsNotExist(err) {
			l.err = errors.Join(l.err, err)
		}
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
	if request.Action == browserLaunchActionRestoreSession {
		_, supported := opener.(port.SessionRestoreOpener)
		if !supported || !validRestoreSessionID(request.SessionID) {
			_ = json.NewEncoder(conn).Encode(browserLaunchResponse{
				RequestID: request.RequestID, Error: "invalid or unsupported session restore request",
			})
			return
		}
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

	l.seenMu.Lock()
	_, duplicate := l.seen[requestID]
	if !duplicate {
		l.seen[requestID] = struct{}{}
	}
	l.seenMu.Unlock()
	if request.Action == browserLaunchActionRestoreSession {
		if duplicate {
			gate.Release()
			_ = json.NewEncoder(conn).Encode(browserLaunchResponse{RequestID: requestID, Error: ErrSessionRestoreResultUnknown.Error()})
			return
		}
		completeSessionRestore(ctx, conn, request, opener.(port.SessionRestoreOpener), gate)
		return
	}
	if duplicate {
		defer gate.Release()
		_ = json.NewEncoder(conn).Encode(browserLaunchResponse{RequestID: requestID, Accepted: true})
		return
	}

	if err := conn.SetDeadline(time.Now().Add(browserLaunchIOTimeout)); err != nil {
		gate.Release()
		return
	}
	ackErr := json.NewEncoder(conn).Encode(browserLaunchResponse{RequestID: requestID, Accepted: true})
	if ackErr != nil {
		// Decode plus admission transfers ownership of the request to the host.
		// Dispatch must not depend on the client remaining alive for the ACK.
		log.Warn().Err(ackErr).
			Str("request_id", requestID).
			Str("url_host", safeURLHost(request.URL)).
			Msg("failed to encode browser launch response; dispatching retained request")
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
		dispatchBrowserLaunch(ctx, request, requestID, opener, gate, log)
	}()
}

// dispatchBrowserLaunch performs the admitted request's UI dispatch on behalf
// of handleConnection, holding the admission lease until it completes. It
// retains the documented failure semantics: a definitive factory or dispatch
// failure only logs and releases the lease, never triggering duplicate
// fallback.
func dispatchBrowserLaunch(
	ctx context.Context,
	request browserLaunchRequest,
	requestID string,
	opener port.BrowserWindowOpener,
	gate *process.AdmissionGate,
	log *zerolog.Logger,
) {
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
	case browserLaunchActionOpenInstance:
		instanceOpener, ok := opener.(port.BrowserInstanceOpener)
		if !ok {
			err = errors.New("browser launch relay opener does not support named instances")
		} else {
			err = instanceOpener.OpenInstance(ctx, request.Instance, request.URL)
		}
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
}

var _ port.BrowserLaunchRelay = (*browserLaunchRelay)(nil)

// receiveRestoreCompletion requires an explicit completion marker: older hosts'
// admission ACKs cannot be mistaken for a successful restoration.
func receiveRestoreCompletion(ctx context.Context, conn net.Conn, requestID string) (bool, error) {
	deadline := time.Now().Add(sessionRestoreCompletionTimeout + time.Second)
	if caller, ok := ctx.Deadline(); ok && caller.Before(deadline) {
		deadline = caller
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return true, fmt.Errorf("%w: %w", ErrSessionRestoreResultUnknown, err)
	}
	var response browserLaunchResponse
	if err := json.NewDecoder(conn).Decode(&response); err != nil {
		return true, fmt.Errorf("%w: %w", ErrSessionRestoreResultUnknown, err)
	}
	if response.RequestID != requestID {
		return true, fmt.Errorf("%w: mismatched response", ErrSessionRestoreResultUnknown)
	}
	if response.Error == ErrSessionRestoreResultUnknown.Error() {
		return true, ErrSessionRestoreResultUnknown
	}
	if response.Error != "" {
		return true, errors.New(response.Error)
	}
	if !response.Completed {
		return true, fmt.Errorf("%w: host did not confirm restore completion", ErrSessionRestoreResultUnknown)
	}
	return true, nil
}

// completeSessionRestore bounds the response wait, not ownership: admitted work
// retains its lease until it settles even when the client has disconnected.
func completeSessionRestore(
	ctx context.Context, conn net.Conn, request browserLaunchRequest,
	opener port.SessionRestoreOpener, gate *process.AdmissionGate,
) {
	result := make(chan error, 1)
	timer := time.NewTimer(sessionRestoreCompletionTimeout)
	defer timer.Stop()
	// UI callbacks retain this context; its lifetime belongs to the host, not
	// to the client's completion wait.
	go func() { defer gate.Release(); result <- opener.RestoreSession(ctx, request.SessionID) }()
	response := browserLaunchResponse{RequestID: request.RequestID, Accepted: true}
	select {
	case err := <-result:
		response.Completed = true
		if err != nil {
			response.Error = err.Error()
		}
	case <-timer.C:
		response.Error = ErrSessionRestoreResultUnknown.Error()
	case <-ctx.Done():
		response.Error = ErrSessionRestoreResultUnknown.Error()
	}
	_ = conn.SetWriteDeadline(time.Now().Add(browserLaunchIOTimeout))
	_ = json.NewEncoder(conn).Encode(response)
}

func sendRestoreRequest(ctx context.Context, conn net.Conn, request browserLaunchRequest) (bool, error) {
	if err := json.NewEncoder(conn).Encode(request); err != nil {
		return true, fmt.Errorf("%w: %w", ErrSessionRestoreResultUnknown, err)
	}
	return receiveRestoreCompletion(ctx, conn, request.RequestID)
}
