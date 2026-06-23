package ui

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	repomocks "github.com/bnema/dumber/internal/domain/repository/mocks"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/shared/syncdispatch"
	"github.com/bnema/dumber/internal/ui/component"
	"github.com/bnema/dumber/internal/ui/coordinator"
	contentcoord "github.com/bnema/dumber/internal/ui/coordinator/content"
	"github.com/bnema/dumber/internal/ui/dispatcher"
	"github.com/bnema/dumber/internal/ui/input"
	"github.com/bnema/dumber/internal/ui/layout"
	layoutmocks "github.com/bnema/dumber/internal/ui/layout/mocks"
	"github.com/bnema/dumber/internal/ui/window"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/bnema/puregotk/v4/gio"
	"github.com/bnema/puregotk/v4/gtk"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type testBrowserLaunchRelay struct {
	listenCalls int
	closer      *testCloser
}

func (r *testBrowserLaunchRelay) DeliverOpenFreshWindow(context.Context, string) (bool, error) {
	return false, nil
}

func (r *testBrowserLaunchRelay) Listen(_ context.Context, opener port.BrowserWindowOpener) (io.Closer, error) {
	r.listenCalls++
	_ = opener
	return r.closer, nil
}

type testCloser struct {
	closed bool
}

func (c *testCloser) Close() error {
	c.closed = true
	return nil
}

func testPathIsSocket(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode()&os.ModeSocket != 0
}

func mockSessionStateRepoWithSnapshot(t *testing.T, sessionID entity.SessionID, state *entity.SessionState) *repomocks.MockSessionStateRepository {
	t.Helper()
	repo := repomocks.NewMockSessionStateRepository(t)
	repo.EXPECT().
		GetSnapshot(mock.Anything, sessionID).
		Return(state, nil).
		Once()
	return repo
}

func mockZoomRepo(t *testing.T) *repomocks.MockZoomRepository {
	t.Helper()
	repo := repomocks.NewMockZoomRepository(t)
	repo.EXPECT().
		Get(mock.Anything, mock.Anything).
		Return((*entity.ZoomLevel)(nil), nil).
		Maybe()
	repo.EXPECT().
		Set(mock.Anything, mock.Anything).
		Return(nil).
		Maybe()
	return repo
}

func testGDKBackendAllows(backend string) bool {
	configured := os.Getenv("GDK_BACKEND")
	if configured == "" {
		return true
	}
	for _, candidate := range strings.Split(configured, ",") {
		if strings.TrimSpace(candidate) == backend {
			return true
		}
	}
	return false
}

func testHasUsableWaylandDisplay() bool {
	if !testGDKBackendAllows("wayland") {
		return false
	}
	waylandDisplay := os.Getenv("WAYLAND_DISPLAY")
	if waylandDisplay == "" {
		return false
	}
	candidates := []string{waylandDisplay}
	if runtimeDir := os.Getenv("XDG_RUNTIME_DIR"); runtimeDir != "" && !filepath.IsAbs(waylandDisplay) {
		candidates = append([]string{filepath.Join(runtimeDir, waylandDisplay)}, candidates...)
	}
	for _, candidate := range candidates {
		if testPathIsSocket(candidate) {
			return true
		}
	}
	return false
}

func testHasUsableX11Display() bool {
	if !testGDKBackendAllows("x11") {
		return false
	}
	display := os.Getenv("DISPLAY")
	if display == "" {
		return false
	}
	if strings.HasPrefix(display, ":") {
		displayNum := strings.TrimPrefix(display, ":")
		displayNum = strings.SplitN(displayNum, ".", 2)[0]
		if displayNum == "" {
			return false
		}
		x11SocketCandidates := []string{filepath.Join(os.TempDir(), ".X11-unix", "X"+displayNum)}
		if fallback := "/tmp/.X11-unix/X" + displayNum; fallback != x11SocketCandidates[0] {
			x11SocketCandidates = append(x11SocketCandidates, fallback)
		}
		for _, candidate := range x11SocketCandidates {
			if testPathIsSocket(candidate) {
				return true
			}
		}
		return false
	}
	return true // TCP display
}

func testHasUsableGTKDisplay() bool {
	return testHasUsableWaylandDisplay() || testHasUsableX11Display()
}

// testHasX11Auth checks whether X11 authorization is available for the current
// DISPLAY before any GTK/Adwaita initialization. It is a pure function that
// inspects env vars and the .Xauthority file only; no GTK calls.
func testHasX11Auth() bool {
	display := os.Getenv("DISPLAY")
	if display == "" || !strings.HasPrefix(display, ":") {
		// No DISPLAY or TCP-style display; can't check locally.
		// Let GTK try and fall through to gdk.DisplayGetDefault().
		return true
	}

	displayNum := strings.TrimPrefix(display, ":")
	displayNum = strings.SplitN(displayNum, ".", 2)[0]
	if displayNum == "" {
		return false
	}

	// Find the X authority file.
	authFile := os.Getenv("XAUTHORITY")
	if authFile == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false
		}
		authFile = filepath.Join(home, ".Xauthority")
	}

	if _, err := os.Stat(authFile); err != nil {
		return false // No authority file; X11 auth will fail.
	}

	// Parse the .Xauthority file to look for a matching display entry.
	// Format: sequence of {family(2B), addrLen(2B), addr, numLen(2B), num, nameLen(2B), name, dataLen(2B), data}
	// All multi-byte values are big-endian (network byte order).
	data, err := os.ReadFile(authFile)
	if err != nil {
		return false
	}

	return testXauthorityHasDisplay(data, displayNum)
}

func testXauthorityHasDisplay(data []byte, displayNum string) bool {
	for len(data) > 0 {
		if len(data) < 2 {
			return false
		}
		data = data[2:] // family, unused

		_, rest, ok := testReadXauthorityField(data) // address
		if !ok {
			return false
		}
		num, rest, ok := testReadXauthorityField(rest)
		if !ok {
			return false
		}
		name, rest, ok := testReadXauthorityField(rest)
		if !ok {
			return false
		}
		cookie, rest, ok := testReadXauthorityField(rest)
		if !ok {
			return false
		}
		if string(num) == displayNum && len(name) > 0 && len(cookie) > 0 {
			return true
		}
		data = rest
	}
	return false
}

func testReadXauthorityField(data []byte) ([]byte, []byte, bool) {
	if len(data) < 2 {
		return nil, nil, false
	}
	fieldLen := int(binary.BigEndian.Uint16(data[:2]))
	data = data[2:]
	if len(data) < fieldLen {
		return nil, nil, false
	}
	return data[:fieldLen], data[fieldLen:], true
}

// testNeedsX11Auth reports whether GTK may need the X11 backend and therefore
// requires X11 authorization before GTK initialization. Wayland-only backend
// configuration never needs X11 auth; X11 is required when it is the only usable
// allowed backend.
func testNeedsX11Auth() bool {
	if !testGDKBackendAllows("x11") {
		return false
	}
	return !testGDKBackendAllows("wayland") || !testHasUsableWaylandDisplay()
}

func testCreateUnixSocket(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = listener.Close()
		_ = os.Remove(path)
	})
}

func testUnusedX11DisplayNumber(t *testing.T) string {
	t.Helper()
	for display := 90; display < 300; display++ {
		displayNum := strconv.Itoa(display)
		if !testPathIsSocket("/tmp/.X11-unix/X" + displayNum) {
			return displayNum
		}
	}
	t.Fatal("could not find an unused X11 display number")
	return ""
}

func testCreateX11Socket(t *testing.T, displayNum string) {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)
	testCreateUnixSocket(t, filepath.Join(tmpDir, ".X11-unix", "X"+displayNum))
}

func requireGTKDisplayApp(t *testing.T) *gtk.Application {
	t.Helper()
	hasWayland := testHasUsableWaylandDisplay()
	hasX11 := testHasUsableX11Display()
	if !hasWayland && !hasX11 {
		t.Skip("GTK display not available (no DISPLAY/WAYLAND_DISPLAY socket)")
	}
	// Only require X11 auth when X11 is the only usable allowed backend.
	if testNeedsX11Auth() && !testHasX11Auth() {
		t.Skip("GTK display not available (X11 authorization unavailable)")
	}
	EnsureAdwaitaInitialized()
	if gdk.DisplayGetDefault() == nil {
		t.Skip("GTK display not available (gdk.DisplayGetDefault returned nil)")
	}
	appID := AppID
	gtkApp := gtk.NewApplication(&appID, gio.GApplicationNonUniqueValue)
	if gtkApp == nil {
		t.Fatal("gtk application creation failed")
	}
	return gtkApp
}

func Test_testHasUsableGTKDisplay_PureProbe(t *testing.T) {
	t.Setenv("GDK_BACKEND", "")

	t.Run("no display env vars", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "")
		if got := testHasUsableGTKDisplay(); got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want false", got)
		}
	})

	t.Run("DISPLAY numeric without socket", func(t *testing.T) {
		displayNum := testUnusedX11DisplayNumber(t)
		t.Setenv("DISPLAY", ":"+displayNum)
		t.Setenv("WAYLAND_DISPLAY", "")
		if got := testHasUsableGTKDisplay(); got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want false", got)
		}
	})

	t.Run("DISPLAY with screen without socket", func(t *testing.T) {
		displayNum := testUnusedX11DisplayNumber(t)
		t.Setenv("DISPLAY", ":"+displayNum+".0")
		t.Setenv("WAYLAND_DISPLAY", "")
		if got := testHasUsableGTKDisplay(); got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want false", got)
		}
	})

	t.Run("DISPLAY colon only no number", func(t *testing.T) {
		t.Setenv("DISPLAY", ":")
		t.Setenv("WAYLAND_DISPLAY", "")
		if got := testHasUsableGTKDisplay(); got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want false for empty display number", got)
		}
	})

	t.Run("DISPLAY TCP always passes socket check", func(t *testing.T) {
		t.Setenv("DISPLAY", "remote-host:0")
		t.Setenv("WAYLAND_DISPLAY", "")
		if got := testHasUsableGTKDisplay(); !got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want true for TCP display", got)
		}
	})

	t.Run("DISPLAY with existing socket", func(t *testing.T) {
		displayNum := testUnusedX11DisplayNumber(t)
		tmpDir := t.TempDir()
		t.Setenv("TMPDIR", tmpDir)
		t.Setenv("DISPLAY", ":"+displayNum)
		t.Setenv("WAYLAND_DISPLAY", "")
		testCreateUnixSocket(t, filepath.Join(tmpDir, ".X11-unix", "X"+displayNum))
		if got := testHasUsableGTKDisplay(); !got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want true with existing X11 socket", got)
		}
	})

	t.Run("WAYLAND_DISPLAY with existing socket", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "wayland-1")
		tmpDir := t.TempDir()
		socketPath := filepath.Join(tmpDir, "wayland-1")
		testCreateUnixSocket(t, socketPath)
		// XDG_RUNTIME_DIR controls where Wayland sockets are looked up
		t.Setenv("XDG_RUNTIME_DIR", tmpDir)
		if got := testHasUsableGTKDisplay(); !got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want true with existing wayland socket", got)
		}
	})

	t.Run("WAYLAND_DISPLAY without socket", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("WAYLAND_DISPLAY", "wayland-nonexistent")
		t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
		if got := testHasUsableGTKDisplay(); got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want false without wayland socket", got)
		}
	})

	t.Run("WAYLAND_DISPLAY absolute path with existing socket", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		socketPath := filepath.Join(t.TempDir(), "wayland.sock")
		testCreateUnixSocket(t, socketPath)
		t.Setenv("WAYLAND_DISPLAY", socketPath)
		t.Setenv("XDG_RUNTIME_DIR", "/nonexistent") // Should be ignored for absolute paths
		if got := testHasUsableGTKDisplay(); !got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want true with absolute wayland socket path", got)
		}
	})
}

func Test_testHasX11Auth_PureProbe(t *testing.T) {
	t.Run("DISPLAY unset", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		t.Setenv("XAUTHORITY", "")
		if got := testHasX11Auth(); !got {
			t.Errorf("testHasX11Auth() = %v, want true when DISPLAY unset", got)
		}
	})

	t.Run("DISPLAY TCP remote", func(t *testing.T) {
		t.Setenv("DISPLAY", "remote-host:0")
		t.Setenv("XAUTHORITY", "")
		if got := testHasX11Auth(); !got {
			t.Errorf("testHasX11Auth() = %v, want true for TCP display", got)
		}
	})

	t.Run("DISPLAY local no auth file", func(t *testing.T) {
		t.Setenv("DISPLAY", ":0")
		t.Setenv("XAUTHORITY", "")
		// Use a temp home dir with no .Xauthority
		t.Setenv("HOME", t.TempDir())
		if got := testHasX11Auth(); got {
			t.Errorf("testHasX11Auth() = %v, want false without auth file", got)
		}
	})

	t.Run("DISPLAY local XAUTHORITY env missing file", func(t *testing.T) {
		t.Setenv("DISPLAY", ":0")
		t.Setenv("XAUTHORITY", "/nonexistent/.Xauthority")
		if got := testHasX11Auth(); got {
			t.Errorf("testHasX11Auth() = %v, want false when XAUTHORITY file missing", got)
		}
	})

	t.Run("DISPLAY local XAUTHORITY with matching entry", func(t *testing.T) {
		t.Setenv("DISPLAY", ":1")
		authFile := filepath.Join(t.TempDir(), ".Xauthority")
		createXauthFile(t, authFile, "1", "MIT-MAGIC-COOKIE-1", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10})
		t.Setenv("XAUTHORITY", authFile)
		if got := testHasX11Auth(); !got {
			t.Errorf("testHasX11Auth() = %v, want true with matching auth entry", got)
		}
	})

	t.Run("DISPLAY local XAUTHORITY with non-matching entry", func(t *testing.T) {
		t.Setenv("DISPLAY", ":2")
		authFile := filepath.Join(t.TempDir(), ".Xauthority")
		createXauthFile(t, authFile, "0", "MIT-MAGIC-COOKIE-1", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10})
		t.Setenv("XAUTHORITY", authFile)
		if got := testHasX11Auth(); got {
			t.Errorf("testHasX11Auth() = %v, want false with non-matching auth entry", got)
		}
	})

	t.Run("DISPLAY local with screen number", func(t *testing.T) {
		t.Setenv("DISPLAY", ":0.0")
		authFile := filepath.Join(t.TempDir(), ".Xauthority")
		createXauthFile(t, authFile, "0", "MIT-MAGIC-COOKIE-1", []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10})
		t.Setenv("XAUTHORITY", authFile)
		if got := testHasX11Auth(); !got {
			t.Errorf("testHasX11Auth() = %v, want true with screen suffix stripped", got)
		}
	})

	t.Run("DISPLAY colon only no number defaults to false", func(t *testing.T) {
		t.Setenv("DISPLAY", ":")
		t.Setenv("XAUTHORITY", "")
		if got := testHasX11Auth(); got {
			t.Errorf("testHasX11Auth() = %v, want false for empty display number", got)
		}
	})
}

// writeXauthRecord appends one local-family .Xauthority record to buf with the given fields.
// If name or data is empty, the length byte is written as zero but the field
// data is omitted, producing a truncated record suitable for malformed tests.
func writeXauthRecord(buf *bytes.Buffer, num, name string, data []byte) {
	// family (2 bytes, big-endian)
	familyBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(familyBytes, 256)
	buf.Write(familyBytes)

	// address
	addrLen := make([]byte, 2)
	addr := "host"
	binary.BigEndian.PutUint16(addrLen, uint16(len(addr)))
	buf.Write(addrLen)
	buf.WriteString(addr)

	// number
	numLen := make([]byte, 2)
	binary.BigEndian.PutUint16(numLen, uint16(len(num)))
	buf.Write(numLen)
	buf.WriteString(num)

	// name
	nameLen := make([]byte, 2)
	binary.BigEndian.PutUint16(nameLen, uint16(len(name)))
	buf.Write(nameLen)
	if name != "" {
		buf.WriteString(name)
	}

	// data
	dLen := make([]byte, 2)
	binary.BigEndian.PutUint16(dLen, uint16(len(data)))
	buf.Write(dLen)
	if len(data) > 0 {
		buf.Write(data)
	}
}

func Test_testHasX11Auth_MalformedRecords(t *testing.T) {
	t.Run("matching display with empty name field", func(t *testing.T) {
		t.Setenv("DISPLAY", ":1")
		authFile := filepath.Join(t.TempDir(), ".Xauthority")
		var buf bytes.Buffer
		writeXauthRecord(&buf, "1", "", []byte{0x01, 0x02, 0x03, 0x04})
		if err := os.WriteFile(authFile, buf.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XAUTHORITY", authFile)
		if got := testHasX11Auth(); got {
			t.Errorf("testHasX11Auth() = %v, want false for matching display with empty name", got)
		}
	})

	t.Run("matching display with empty data field", func(t *testing.T) {
		t.Setenv("DISPLAY", ":1")
		authFile := filepath.Join(t.TempDir(), ".Xauthority")
		var buf bytes.Buffer
		writeXauthRecord(&buf, "1", "MIT-MAGIC-COOKIE-1", nil)
		if err := os.WriteFile(authFile, buf.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XAUTHORITY", authFile)
		if got := testHasX11Auth(); got {
			t.Errorf("testHasX11Auth() = %v, want false for matching display with empty data", got)
		}
	})

	t.Run("truncated record after display number", func(t *testing.T) {
		t.Setenv("DISPLAY", ":1")
		authFile := filepath.Join(t.TempDir(), ".Xauthority")
		var buf bytes.Buffer
		family := make([]byte, 2)
		binary.BigEndian.PutUint16(family, 256)
		buf.Write(family)
		addr := "host"
		addrLen := make([]byte, 2)
		binary.BigEndian.PutUint16(addrLen, uint16(len(addr)))
		buf.Write(addrLen)
		buf.WriteString(addr)
		num := "1"
		numLen := make([]byte, 2)
		binary.BigEndian.PutUint16(numLen, uint16(len(num)))
		buf.Write(numLen)
		buf.WriteString(num)
		// Record ends here — no name or data fields follow.
		if err := os.WriteFile(authFile, buf.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XAUTHORITY", authFile)
		if got := testHasX11Auth(); got {
			t.Errorf("testHasX11Auth() = %v, want false for truncated record", got)
		}
	})

	t.Run("multiple entries with one matching and valid", func(t *testing.T) {
		t.Setenv("DISPLAY", ":3")
		authFile := filepath.Join(t.TempDir(), ".Xauthority")
		var buf bytes.Buffer
		writeXauthRecord(&buf, "0", "MIT-MAGIC-COOKIE-1", []byte{0x01, 0x02, 0x03, 0x04})
		writeXauthRecord(&buf, "1", "MIT-MAGIC-COOKIE-1", []byte{0x05, 0x06, 0x07, 0x08})
		writeXauthRecord(&buf, "3", "MIT-MAGIC-COOKIE-1", []byte{0x09, 0x0a, 0x0b, 0x0c})
		if err := os.WriteFile(authFile, buf.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XAUTHORITY", authFile)
		if got := testHasX11Auth(); !got {
			t.Errorf("testHasX11Auth() = %v, want true for third entry matching display :3", got)
		}
	})

	t.Run("invalid matching entry does not hide later valid match", func(t *testing.T) {
		t.Setenv("DISPLAY", ":3")
		authFile := filepath.Join(t.TempDir(), ".Xauthority")
		var buf bytes.Buffer
		writeXauthRecord(&buf, "3", "", []byte{0x01, 0x02, 0x03, 0x04})
		writeXauthRecord(&buf, "3", "MIT-MAGIC-COOKIE-1", []byte{0x09, 0x0a, 0x0b, 0x0c})
		if err := os.WriteFile(authFile, buf.Bytes(), 0644); err != nil {
			t.Fatal(err)
		}
		t.Setenv("XAUTHORITY", authFile)
		if got := testHasX11Auth(); !got {
			t.Errorf("testHasX11Auth() = %v, want true for later valid matching display", got)
		}
	})
}

func Test_testHasUsableWaylandDisplay_PureProbe(t *testing.T) {
	t.Setenv("GDK_BACKEND", "")

	t.Run("no WAYLAND_DISPLAY", func(t *testing.T) {
		t.Setenv("WAYLAND_DISPLAY", "")
		if got := testHasUsableWaylandDisplay(); got {
			t.Errorf("testHasUsableWaylandDisplay() = %v, want false", got)
		}
	})

	t.Run("WAYLAND_DISPLAY with existing socket", func(t *testing.T) {
		t.Setenv("WAYLAND_DISPLAY", "wayland-1")
		tmpDir := t.TempDir()
		socketPath := filepath.Join(tmpDir, "wayland-1")
		testCreateUnixSocket(t, socketPath)
		t.Setenv("XDG_RUNTIME_DIR", tmpDir)
		if got := testHasUsableWaylandDisplay(); !got {
			t.Errorf("testHasUsableWaylandDisplay() = %v, want true with existing wayland socket", got)
		}
	})

	t.Run("WAYLAND_DISPLAY absolute path", func(t *testing.T) {
		socketPath := filepath.Join(t.TempDir(), "wayland.sock")
		testCreateUnixSocket(t, socketPath)
		t.Setenv("WAYLAND_DISPLAY", socketPath)
		t.Setenv("XDG_RUNTIME_DIR", "/nonexistent")
		if got := testHasUsableWaylandDisplay(); !got {
			t.Errorf("testHasUsableWaylandDisplay() = %v, want true with absolute path", got)
		}
	})
}

func Test_testHasUsableX11Display_PureProbe(t *testing.T) {
	t.Setenv("GDK_BACKEND", "")

	t.Run("no DISPLAY", func(t *testing.T) {
		t.Setenv("DISPLAY", "")
		if got := testHasUsableX11Display(); got {
			t.Errorf("testHasUsableX11Display() = %v, want false", got)
		}
	})

	t.Run("DISPLAY numeric without socket", func(t *testing.T) {
		displayNum := testUnusedX11DisplayNumber(t)
		t.Setenv("DISPLAY", ":"+displayNum)
		if got := testHasUsableX11Display(); got {
			t.Errorf("testHasUsableX11Display() = %v, want false", got)
		}
	})

	t.Run("DISPLAY TCP always passes socket check", func(t *testing.T) {
		t.Setenv("DISPLAY", "remote-host:0")
		if got := testHasUsableX11Display(); !got {
			t.Errorf("testHasUsableX11Display() = %v, want true for TCP display", got)
		}
	})
}

func Test_testHasUsableGTKDisplay_GDKBackendSelection(t *testing.T) {
	t.Run("GDK_BACKEND=x11 ignores Wayland-only socket", func(t *testing.T) {
		t.Setenv("GDK_BACKEND", "x11")
		t.Setenv("WAYLAND_DISPLAY", "wayland-1")
		tmpDir := t.TempDir()
		testCreateUnixSocket(t, filepath.Join(tmpDir, "wayland-1"))
		t.Setenv("XDG_RUNTIME_DIR", tmpDir)
		t.Setenv("DISPLAY", ":"+testUnusedX11DisplayNumber(t))
		if got := testHasUsableGTKDisplay(); got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want false when x11 is forced without an X11 socket", got)
		}
	})

	t.Run("GDK_BACKEND=wayland ignores X11-only socket", func(t *testing.T) {
		displayNum := testUnusedX11DisplayNumber(t)
		t.Setenv("GDK_BACKEND", "wayland")
		t.Setenv("DISPLAY", ":"+displayNum)
		t.Setenv("WAYLAND_DISPLAY", "wayland-missing")
		t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
		testCreateX11Socket(t, displayNum)
		if got := testHasUsableGTKDisplay(); got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want false when wayland is forced without a Wayland socket", got)
		}
	})

	t.Run("GDK_BACKEND=wayland,x11 allows X11 fallback", func(t *testing.T) {
		displayNum := testUnusedX11DisplayNumber(t)
		t.Setenv("GDK_BACKEND", "wayland,x11")
		t.Setenv("DISPLAY", ":"+displayNum)
		t.Setenv("WAYLAND_DISPLAY", "wayland-missing")
		t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
		testCreateX11Socket(t, displayNum)
		if got := testHasUsableGTKDisplay(); !got {
			t.Errorf("testHasUsableGTKDisplay() = %v, want true with X11 fallback socket", got)
		}
		if got := testNeedsX11Auth(); !got {
			t.Errorf("testNeedsX11Auth() = %v, want true when X11 is the only usable backend", got)
		}
	})
}

func Test_testNeedsX11Auth_PureProbe(t *testing.T) {
	t.Run("Wayland available forces no X11 auth", func(t *testing.T) {
		t.Setenv("GDK_BACKEND", "")
		t.Setenv("WAYLAND_DISPLAY", "wayland-1")
		tmpDir := t.TempDir()
		socketPath := filepath.Join(tmpDir, "wayland-1")
		testCreateUnixSocket(t, socketPath)
		t.Setenv("XDG_RUNTIME_DIR", tmpDir)
		t.Setenv("DISPLAY", ":0")
		if got := testNeedsX11Auth(); got {
			t.Errorf("testNeedsX11Auth() = %v, want false when Wayland is available and GDK_BACKEND is unset", got)
		}
	})

	t.Run("GDK_BACKEND=x11 forces X11 auth even with Wayland", func(t *testing.T) {
		t.Setenv("GDK_BACKEND", "x11")
		t.Setenv("WAYLAND_DISPLAY", "wayland-1")
		tmpDir := t.TempDir()
		socketPath := filepath.Join(tmpDir, "wayland-1")
		testCreateUnixSocket(t, socketPath)
		t.Setenv("XDG_RUNTIME_DIR", tmpDir)
		if got := testNeedsX11Auth(); !got {
			t.Errorf("testNeedsX11Auth() = %v, want true when GDK_BACKEND=x11", got)
		}
	})

	t.Run("No Wayland fallback needs X11 auth", func(t *testing.T) {
		t.Setenv("GDK_BACKEND", "")
		t.Setenv("WAYLAND_DISPLAY", "")
		t.Setenv("DISPLAY", ":0")
		if got := testNeedsX11Auth(); !got {
			t.Errorf("testNeedsX11Auth() = %v, want true when only X11 is available", got)
		}
	})
}

// createXauthFile writes a minimal .Xauthority file with one entry.
// family=256 (FamilyLocal), address=hostname, number=displayNum, name=proto, data=cookie.
func createXauthFile(t *testing.T, path, displayNum, proto string, cookie []byte) {
	t.Helper()

	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}

	var buf bytes.Buffer
	// family (2 bytes, big-endian) - 256 = FamilyLocal
	family := make([]byte, 2)
	binary.BigEndian.PutUint16(family, 256)
	buf.Write(family)

	// address length + address (hostname)
	addrLen := make([]byte, 2)
	binary.BigEndian.PutUint16(addrLen, uint16(len(hostname)))
	buf.Write(addrLen)
	buf.WriteString(hostname)

	// number length + number (display number)
	numLen := make([]byte, 2)
	binary.BigEndian.PutUint16(numLen, uint16(len(displayNum)))
	buf.Write(numLen)
	buf.WriteString(displayNum)

	// name length + name (auth protocol)
	nameLen := make([]byte, 2)
	binary.BigEndian.PutUint16(nameLen, uint16(len(proto)))
	buf.Write(nameLen)
	buf.WriteString(proto)

	// data length + data (auth cookie)
	dataLen := make([]byte, 2)
	binary.BigEndian.PutUint16(dataLen, uint16(len(cookie)))
	buf.Write(dataLen)
	buf.Write(cookie)

	if err := os.WriteFile(path, buf.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
}

// setDependencyField uses reflection to reach unexported dependency fields for tests.
// Keep this test-only so production visibility stays narrow.
func setDependencyField(t *testing.T, deps *Dependencies, field string, value any) {
	t.Helper()

	rv := reflect.ValueOf(deps).Elem()
	fv := rv.FieldByName(field)
	if !fv.IsValid() {
		t.Fatalf("Dependencies missing %s field", field)
	}
	fv.Set(reflect.ValueOf(value))
}

// windowForTabCount uses reflection to inspect unexported app state in tests.
// Keep this test-only so production visibility stays narrow.
func windowForTabCount(t *testing.T, app *App) int {
	t.Helper()

	rv := reflect.ValueOf(app).Elem()
	fv := rv.FieldByName("windowForTab")
	if !fv.IsValid() {
		t.Fatalf("App missing windowForTab field")
	}
	return fv.Len()
}

func assertWindowOwnershipInvariant(t *testing.T, app *App) {
	t.Helper()

	seen := make(map[entity.TabID]string)
	for windowID, bw := range app.browserWindows {
		if bw == nil {
			continue
		}
		if bw.tabs == nil {
			t.Errorf("browser window %s has nil tabs", windowID)
			continue
		}
		for _, tab := range bw.tabs.Tabs {
			if tab == nil {
				continue
			}
			if previous, exists := seen[tab.ID]; exists {
				t.Errorf("tab %s is owned by both %s and %s", tab.ID, previous, windowID)
				continue
			}
			seen[tab.ID] = windowID
			if got := app.windowForTab[tab.ID]; got != bw {
				t.Errorf("windowForTab[%s] = %p, want owner window %s (%p)", tab.ID, got, windowID, bw)
			}
		}
	}
	for tabID, bw := range app.windowForTab {
		if bw == nil || bw.tabs == nil || bw.tabs.Find(tabID) == nil {
			t.Errorf("windowForTab[%s] points to stale or non-owning window %p", tabID, bw)
		}
	}
}

// tabCoordinatorMainWindowPtr uses reflection to inspect unexported coordinator state in tests.
// Keep this test-only so production visibility stays narrow.
func tabCoordinatorMainWindowPtr(t *testing.T, tc *coordinator.TabCoordinator) uintptr {
	t.Helper()

	rv := reflect.ValueOf(tc).Elem()
	fv := rv.FieldByName("mainWindow")
	if !fv.IsValid() {
		t.Fatalf("TabCoordinator missing mainWindow field")
	}
	return fv.Pointer()
}

func setWindowTabBar(t *testing.T, mw *window.MainWindow, tabBar *component.TabBar) {
	t.Helper()

	rv := reflect.ValueOf(mw).Elem()
	fv := rv.FieldByName("tabBar")
	if !fv.IsValid() {
		t.Fatalf("MainWindow missing tabBar field")
	}
	reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().Set(reflect.ValueOf(tabBar))
}

func omniboxNavigateCallbackForTest(t *testing.T, omnibox *component.Omnibox) func(context.Context, string) error {
	t.Helper()
	if omnibox == nil {
		t.Fatal("omnibox is nil")
	}

	rv := reflect.ValueOf(omnibox).Elem()
	fv := rv.FieldByName("onNavigate")
	if !fv.IsValid() {
		t.Fatal("Omnibox missing onNavigate field")
	}
	cb, ok := reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().Interface().(func(context.Context, string) error)
	if !ok || cb == nil {
		t.Fatal("omnibox onNavigate callback is nil or has unexpected type")
	}
	return cb
}

func newTestTabBarShell(t *testing.T, tabIDs ...entity.TabID) *component.TabBar {
	t.Helper()

	tabBar := &component.TabBar{}
	buttons := make(map[entity.TabID]*component.TabButton, len(tabIDs))
	for _, tabID := range tabIDs {
		buttons[tabID] = &component.TabButton{}
	}

	rv := reflect.ValueOf(tabBar).Elem()
	fv := rv.FieldByName("buttons")
	if !fv.IsValid() {
		t.Fatalf("TabBar missing buttons field")
	}
	reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().Set(reflect.ValueOf(buttons))

	return tabBar
}

func windowTabBarActiveID(t *testing.T, mw *window.MainWindow) entity.TabID {
	t.Helper()
	if mw == nil || mw.TabBar() == nil {
		return ""
	}
	return mw.TabBar().ActiveTabID()
}

func windowTitle(t *testing.T, mw *window.MainWindow) string {
	t.Helper()
	if mw == nil || mw.Window() == nil {
		return ""
	}
	return mw.Window().GetTitle()
}

func windowTabBarVisible(t *testing.T, mw *window.MainWindow) bool {
	t.Helper()
	if mw == nil || mw.TabBar() == nil || mw.TabBar().Box() == nil {
		return false
	}
	return mw.TabBar().Box().GetVisible()
}

func assertWindowTabBarAutoHidden(t *testing.T, mw *window.MainWindow, wantHidden bool) {
	t.Helper()
	if mw == nil || mw.TabBar() == nil || mw.TabBar().Box() == nil {
		t.Fatal("missing tab bar")
	}
	box := mw.TabBar().Box()
	if got := box.GetVisible(); !got {
		t.Fatalf("tab bar visible = %v, want true so allocation is preserved", got)
	}
	if wantHidden {
		if got := box.GetOpacity(); got != 0.0 {
			t.Fatalf("tab bar opacity = %v, want 0", got)
		}
		if got := box.GetCanTarget(); got {
			t.Fatalf("tab bar can target = %v, want false while auto-hidden", got)
		}
		return
	}
	if got := box.GetOpacity(); got != 1.0 {
		t.Fatalf("tab bar opacity = %v, want 1", got)
	}
	if got := box.GetCanTarget(); !got {
		t.Fatalf("tab bar can target = %v, want true while shown", got)
	}
}

func stackedViewOnActivateIsNil(t *testing.T, sv *layout.StackedView) bool {
	t.Helper()
	if sv == nil {
		return true
	}

	rv := reflect.ValueOf(sv).Elem()
	fv := rv.FieldByName("onActivate")
	if !fv.IsValid() {
		t.Fatalf("StackedView missing onActivate field")
	}
	return reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().IsNil()
}

func newTestShellToaster(t *testing.T) (*component.Toaster, *layoutmocks.MockBoxWidget, *layoutmocks.MockLabelWidget) {
	t.Helper()

	factory := layoutmocks.NewMockWidgetFactory(t)
	box := layoutmocks.NewMockBoxWidget(t)
	label := layoutmocks.NewMockLabelWidget(t)

	factory.EXPECT().NewBox(layout.OrientationHorizontal, 0).Return(box).Once()
	factory.EXPECT().NewLabel("").Return(label).Once()
	box.EXPECT().AddCssClass("toast").Once()
	box.EXPECT().AddCssClass("toast-info").Once()
	box.EXPECT().SetHalign(gtk.AlignStartValue).Twice()
	box.EXPECT().SetValign(gtk.AlignStartValue).Twice()
	box.EXPECT().SetHexpand(false).Once()
	box.EXPECT().SetVexpand(false).Once()
	box.EXPECT().SetCanTarget(false).Once()
	box.EXPECT().SetCanFocus(false).Once()
	box.EXPECT().SetVisible(false).Once()
	box.EXPECT().Append(label).Once()
	label.EXPECT().SetCanTarget(false).Once()
	label.EXPECT().SetCanFocus(false).Once()

	return component.NewToaster(factory), box, label
}

func TestApp_ShowFilterStatusUsesLastFocusedBrowserWindowToaster(t *testing.T) {
	ctx := context.Background()
	toaster, box, label := newTestShellToaster(t)
	box.EXPECT().SetVisible(true).Once()
	label.EXPECT().SetText("Ad blocker loading").Once()

	bw := &browserWindow{id: "window-1", appToaster: toaster}
	app := &App{
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
	}

	app.showFilterStatus(ctx, port.FilterStatus{State: port.FilterStateLoading, Message: "Ad blocker loading"})
}

func TestApp_CheckConfigMigrationUsesLastFocusedBrowserWindowToaster(t *testing.T) {
	ctx := context.Background()
	toaster, box, label := newTestShellToaster(t)
	box.EXPECT().SetVisible(true).Once()
	label.EXPECT().SetText("Config has 1 new settings. Run 'dumber config migrate'").Once()

	migrator := portmocks.NewMockConfigMigrator(t)
	migrator.EXPECT().CheckMigration().Return(&port.MigrationResult{MissingKeys: []string{"update.notify_on_new_settings"}}, nil).Once()

	bw := &browserWindow{id: "window-1", appToaster: toaster}
	cfg := &config.Config{
		Update: config.UpdateConfig{NotifyOnNewSettings: true},
	}
	app := &App{
		deps: &Dependencies{
			MigrationChecker: migrator,
		},
		runtimeConfig:       runtimeConfigStateForTest(cfg),
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
	}

	app.checkConfigMigration(ctx)
}

func TestApp_FinalizeActivationStartsBrowserLaunchRelayOnceAndClosesOnShutdown(t *testing.T) {
	relay := &testBrowserLaunchRelay{closer: &testCloser{}}
	deps := &Dependencies{}
	setDependencyField(t, deps, "BrowserLaunchRelay", relay)

	app := &App{deps: deps, cancel: func(error) {}}

	app.finalizeActivation(context.Background())
	app.finalizeActivation(context.Background())

	if relay.listenCalls != 1 {
		t.Fatalf("Listen calls = %d, want 1", relay.listenCalls)
	}

	app.onShutdown(context.Background())

	if !relay.closer.closed {
		t.Fatalf("relay listener closer was not closed")
	}
}

func TestApp_GetWindowSnapshotStateReturnsUnavailableWhenMainThreadDispatchTimesOut(t *testing.T) {
	app := &App{
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{},
		dispatchOnMainThread: func(label string, fn func()) syncdispatch.SyncDispatchResult {
			if label != "ui.snapshot_window_state" {
				t.Fatalf("dispatch label = %q, want ui.snapshot_window_state", label)
			}
			return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchTimedOut, Elapsed: 5 * time.Millisecond}
		},
	}

	windows, activeWindowIndex := app.GetWindowSnapshotState()

	require.Nil(t, windows)
	require.Equal(t, -1, activeWindowIndex)
}

func TestApp_OpenFreshWindowReportsMainThreadDispatchTimeout(t *testing.T) {
	var factoryCalls int
	app := &App{
		dispatchOnMainThread: func(label string, fn func()) syncdispatch.SyncDispatchResult {
			return syncdispatch.SyncDispatchResult{Label: label, Status: syncdispatch.SyncDispatchTimedOut, Elapsed: 5 * time.Millisecond}
		},
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			factoryCalls++
			return &browserWindow{id: "window-timeout", tabs: entity.NewTabList()}, nil
		},
	}

	err := app.OpenFreshWindow(context.Background(), "https://example.com")

	if err == nil {
		t.Fatal("OpenFreshWindow returned nil error, want dispatch timeout")
	}
	if !strings.Contains(err.Error(), "main thread dispatch did not complete") {
		t.Fatalf("OpenFreshWindow error = %q, want dispatch timeout", err.Error())
	}
	if factoryCalls != 0 {
		t.Fatalf("browserWindowFactory calls = %d, want 0", factoryCalls)
	}
}

func TestApp_OpenFreshWindowRecordsTabOwnership(t *testing.T) {
	existingTab := entity.NewTab(entity.TabID("existing-tab"), entity.WorkspaceID("existing-workspace"), entity.NewPane(entity.PaneID("existing-pane")))
	existingTabs := entity.NewTabList()
	existingTabs.Add(existingTab)
	existingTabs.SetActive(existingTab.ID)
	existingWindow := &browserWindow{id: "existing-window", tabs: existingTabs}
	created := &browserWindow{id: "window-1", tabs: entity.NewTabList()}
	app := &App{
		tabs:           entity.NewTabList(),
		tabsUC:         usecase.NewManageTabsUseCase(func() string { return "id-1" }),
		browserWindows: map[string]*browserWindow{existingWindow.id: existingWindow},
		windowForTab:   map[entity.TabID]*browserWindow{existingTab.ID: existingWindow},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{entity.TabID("id-1"): &component.WorkspaceView{}},
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			return created, nil
		},
	}

	if err := app.OpenFreshWindow(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("OpenFreshWindow returned error: %v", err)
	}
	if got := windowForTabCount(t, app); got != 2 {
		t.Fatalf("windowForTab length = %d, want 2 (existing + new tab)", got)
	}
	assertWindowOwnershipInvariant(t, app)
}

func TestApp_RestoreSessionHandlesEmptyWindowSnapshotAsSuccess(t *testing.T) {
	ctx := context.Background()
	sessionID := entity.SessionID("session-empty-windows")
	staleTab := entity.NewTab(entity.TabID("stale-tab"), entity.WorkspaceID("stale-workspace"), entity.NewPane(entity.PaneID("stale-pane")))
	staleTabs := entity.NewTabList()
	staleTabs.Add(staleTab)
	staleWindow := &browserWindow{id: "window-stale", tabs: staleTabs}
	state := entity.SnapshotFromWindowTabLists(sessionID, []entity.WindowTabListState{}, 0, time.Unix(123, 0))

	app := &App{
		deps: &Dependencies{
			SessionStateRepo: mockSessionStateRepoWithSnapshot(t, sessionID, state),
		},
		runtimeConfig:       runtimeConfigStateForTest(&config.Config{}),
		tabs:                entity.NewTabList(),
		browserWindows:      map[string]*browserWindow{staleWindow.id: staleWindow},
		lastFocusedWindowID: staleWindow.id,
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{staleTab.ID: {}},
		windowForTab:        map[entity.TabID]*browserWindow{staleTab.ID: staleWindow},
	}
	app.tabs.Add(staleTab)

	err := app.restoreSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("restoreSession returned error for valid v2 empty window snapshot: %v", err)
	}
	// Stale state should be cleared and stale window should have an empty tab list.
	if _, ok := app.workspaceViews[staleTab.ID]; ok {
		t.Fatal("stale workspace view was not cleared")
	}
	if _, ok := app.windowForTab[staleTab.ID]; ok {
		t.Fatal("stale windowForTab entry was not cleared")
	}
	if got := app.tabs.Count(); got != 0 {
		t.Fatalf("app.tabs.Count() = %d, want 0 after empty restore", got)
	}
	if got := staleWindow.tabs.Count(); got != 0 {
		t.Fatalf("staleWindow.tabs.Count() = %d, want 0 after empty restore", got)
	}
}

func TestApp_CreateInitialTabNoFallbackAfterEmptyWindowRestore(t *testing.T) {
	ctx := context.Background()
	sessionID := entity.SessionID("session-empty-windows-fallback")
	state := entity.SnapshotFromWindowTabLists(sessionID, []entity.WindowTabListState{}, 0, time.Unix(123, 0))
	tabsUC := usecase.NewManageTabsUseCase(func() string { return "fallback-tab" })
	windowTabs := entity.NewTabList()
	bw := &browserWindow{id: "window-1", tabs: windowTabs}
	app := &App{
		deps: &Dependencies{
			RestoreSessionID: string(sessionID),
			InitialURL:       "https://fallback.example",
			SessionStateRepo: mockSessionStateRepoWithSnapshot(t, sessionID, state),
		},
		runtimeConfig:       runtimeConfigStateForTest(&config.Config{}),
		tabs:                entity.NewTabList(),
		tabsUC:              tabsUC,
		tabCoord:            coordinator.NewTabCoordinator(ctx, coordinator.TabCoordinatorConfig{TabsUC: tabsUC, Tabs: entity.NewTabList()}),
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
	}

	app.createInitialTab(ctx)

	// For valid v2 empty-window restore, no fallback tab should be created.
	if created := windowTabs.Find(entity.TabID("fallback-tab")); created != nil {
		t.Fatal("fallback tab was created despite valid v2 empty-window restore")
	}
	if got := app.tabs.Count(); got != 0 {
		t.Fatalf("app.tabs.Count() = %d, want 0 after empty restore", got)
	}
}

func TestApp_RestoreSessionClearsStaleUIStateBeforeApplyingRestoredTabs(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	staleTab := entity.NewTab(entity.TabID("stale-tab"), entity.WorkspaceID("stale-workspace"), entity.NewPane(entity.PaneID("stale-pane")))
	staleWindow := &browserWindow{id: "window-stale", mainWindow: mainWindow}
	staleSessionKey := floatingSessionKey{tabID: staleTab.ID, sessionID: "profile-stale"}
	restoredSessionID := entity.SessionID("session-restore")
	restoredTabs := entity.NewTabList()
	restoredTabs.Add(entity.NewTab(entity.TabID("restored-tab"), entity.WorkspaceID("restored-workspace"), entity.NewPane(entity.PaneID("restored-pane"))))

	app := &App{
		deps: &Dependencies{
			SessionStateRepo: mockSessionStateRepoWithSnapshot(t, restoredSessionID, entity.SnapshotFromTabList(restoredSessionID, restoredTabs)),
		},
		runtimeConfig:  runtimeConfigStateForTest(&config.Config{}),
		mainWindow:     mainWindow,
		widgetFactory:  layout.NewGtkWidgetFactory(),
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{staleWindow.id: staleWindow},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{staleTab.ID: &component.WorkspaceView{}},
		windowForTab:   map[entity.TabID]*browserWindow{staleTab.ID: staleWindow},
		floatingSessions: map[floatingSessionKey]*floatingWorkspaceSession{
			staleSessionKey: {},
		},
	}
	app.tabs.Add(staleTab)
	mainWindow.TabBar().AddTab(staleTab)

	if err := app.restoreSession(context.Background(), restoredSessionID); err != nil {
		t.Fatalf("restoreSession returned error: %v", err)
	}

	if got := app.tabs.Count(); got != 1 {
		t.Fatalf("tabs.Count() = %d, want 1", got)
	}
	if got := mainWindow.TabBar().Count(); got != 1 {
		t.Fatalf("tab bar count = %d, want 1", got)
	}
	if _, ok := app.workspaceViews[staleTab.ID]; ok {
		t.Fatalf("stale workspace view was not removed")
	}
	if _, ok := app.windowForTab[staleTab.ID]; ok {
		t.Fatalf("stale windowForTab entry was not removed")
	}
	if _, ok := app.floatingSessions[staleSessionKey]; ok {
		t.Fatalf("stale floating session was not removed")
	}
	assertWindowOwnershipInvariant(t, app)
}

func TestApp_RestoreSessionWiresStackedPaneTitleBarCallbacks(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	stackedSessionID := entity.SessionID("session-stacked-restore")
	stackedTabs := entity.NewTabList()
	stackedTabs.Add(entity.NewTab(entity.TabID("restored-tab"), entity.WorkspaceID("restored-workspace"), entity.NewPane(entity.PaneID("restored-pane-1"))))
	stackedTabs.Tabs[0].Workspace = &entity.Workspace{
		ID: entity.WorkspaceID("restored-workspace"),
		Root: &entity.PaneNode{
			ID:        "restored-stack-root",
			IsStacked: true,
			Children: []*entity.PaneNode{
				{ID: "restored-child-1", Pane: entity.NewPane(entity.PaneID("restored-pane-1"))},
				{ID: "restored-child-2", Pane: entity.NewPane(entity.PaneID("restored-pane-2"))},
			},
			ActiveStackIndex: 0,
		},
		ActivePaneID: entity.PaneID("restored-pane-1"),
	}

	app := &App{
		deps: &Dependencies{
			SessionStateRepo: mockSessionStateRepoWithSnapshot(t, stackedSessionID, entity.SnapshotFromTabList(stackedSessionID, stackedTabs)),
		},
		runtimeConfig:  runtimeConfigStateForTest(&config.Config{}),
		mainWindow:     mainWindow,
		widgetFactory:  layout.NewGtkWidgetFactory(),
		contentCoord:   &contentcoord.Coordinator{},
		wsCoord:        coordinator.NewWorkspaceCoordinator(context.Background(), coordinator.WorkspaceCoordinatorConfig{ContentCoord: &contentcoord.Coordinator{}}),
		tabs:           entity.NewTabList(),
		browserWindows: map[string]*browserWindow{},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{},
		windowForTab:   map[entity.TabID]*browserWindow{},
	}

	if err := app.restoreSession(context.Background(), stackedSessionID); err != nil {
		t.Fatalf("restoreSession returned error: %v", err)
	}

	restoredTab := app.tabs.ActiveTab()
	if restoredTab == nil {
		t.Fatal("restored active tab missing")
	}
	wv := app.workspaceViews[restoredTab.ID]
	if wv == nil {
		t.Fatal("restored workspace view missing")
	}
	tr := wv.TreeRenderer()
	if tr == nil {
		t.Fatal("restored tree renderer missing")
	}
	if restoredTab.Workspace == nil || restoredTab.Workspace.Root == nil || len(restoredTab.Workspace.Root.Children) == 0 {
		t.Fatal("restored stacked workspace missing children")
	}
	stackedView := tr.GetStackedViewForPane(string(restoredTab.Workspace.Root.Children[0].Pane.ID))
	if stackedView == nil {
		t.Fatal("restored stacked view missing")
	}
	if stackedViewOnActivateIsNil(t, stackedView) {
		t.Fatal("restored stacked view onActivate callback is nil")
	}
}

func TestApp_RemoveBrowserWindowRebindsPromotedTabCoordinatorWindow(t *testing.T) {
	oldWindow := &browserWindow{id: "window-1", mainWindow: &window.MainWindow{}}
	newWindow := &browserWindow{id: "window-2", mainWindow: &window.MainWindow{}}
	tc := coordinator.NewTabCoordinator(context.Background(), coordinator.TabCoordinatorConfig{
		MainWindow: oldWindow.mainWindow,
	})
	app := &App{
		mainWindow:     oldWindow.mainWindow,
		browserWindows: map[string]*browserWindow{oldWindow.id: oldWindow, newWindow.id: newWindow},
		tabCoord:       tc,
	}

	app.removeBrowserWindow(oldWindow.id)

	if app.mainWindow != newWindow.mainWindow {
		t.Fatalf("mainWindow = %p, want %p", app.mainWindow, newWindow.mainWindow)
	}
	if got := tabCoordinatorMainWindowPtr(t, tc); got != reflect.ValueOf(newWindow.mainWindow).Pointer() {
		t.Fatalf("tab coordinator mainWindow = %x, want %x", got, reflect.ValueOf(newWindow.mainWindow).Pointer())
	}
}

func TestApp_OpenFreshWindowRollsBackOnTabCreationFailure(t *testing.T) {
	created := &browserWindow{id: "window-1", tabs: entity.NewTabList()}
	originalWindow := &window.MainWindow{}
	tabBar := &component.TabBar{}
	setWindowTabBar(t, originalWindow, tabBar)
	existingTabID := entity.TabID("existing-tab")
	staleTabID := entity.TabID("stale-tab")
	tabBar.SetActive(staleTabID)

	existingTab := entity.NewTab(existingTabID, entity.WorkspaceID("existing-workspace"), entity.NewPane(entity.PaneID("existing-pane")))
	existingTabs := entity.NewTabList()
	existingTabs.Add(existingTab)
	existingTabs.SetActive(existingTab.ID)
	existingWindow := &browserWindow{id: "existing-window", tabs: existingTabs, mainWindow: originalWindow}

	app := &App{
		tabs:           entity.NewTabList(),
		tabsUC:         usecase.NewManageTabsUseCase(func() string { return "id-1" }),
		browserWindows: map[string]*browserWindow{existingWindow.id: existingWindow},
		windowForTab:   map[entity.TabID]*browserWindow{existingTab.ID: existingWindow},
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			return created, nil
		},
		mainWindow: originalWindow,
	}
	app.tabCoord = coordinator.NewTabCoordinator(context.Background(), coordinator.TabCoordinatorConfig{
		TabsUC:     app.tabsUC,
		Tabs:       entity.NewTabList(),
		MainWindow: &window.MainWindow{},
	})

	if err := app.OpenFreshWindow(context.Background(), "https://example.com/fail"); err == nil {
		t.Fatalf("OpenFreshWindow = nil error, want failure")
	}
	if got := len(app.browserWindows); got != 1 {
		t.Fatalf("browserWindows length = %d, want 1 (existing window only)", got)
	}
	if app.browserWindows[existingWindow.id] == nil {
		t.Fatal("existing window should remain after rollback")
	}
	if app.browserWindows["window-1"] != nil {
		t.Fatal("created window should be removed after rollback")
	}
	if got := windowForTabCount(t, app); got != 1 {
		t.Fatalf("windowForTab length = %d, want 1 (existing tab only)", got)
	}
	if got := windowTabBarActiveID(t, originalWindow); got != staleTabID {
		t.Fatalf("tab bar active tab = %q, want %q (original window unchanged)", got, staleTabID)
	}
}

func TestApp_OpenFreshWindowTargetsNewWindowTabBar(t *testing.T) {
	existingTabID := entity.TabID("existing-tab")
	createdTabID := entity.TabID("id-1")
	oldWindow := &window.MainWindow{}
	newWindow := &window.MainWindow{}
	setWindowTabBar(t, oldWindow, newTestTabBarShell(t))
	setWindowTabBar(t, newWindow, newTestTabBarShell(t, createdTabID))
	tabs := entity.NewTabList()
	tabsUC := usecase.NewManageTabsUseCase(func() string { return string(createdTabID) })

	existingTab := entity.NewTab(existingTabID, entity.WorkspaceID("existing-workspace"), entity.NewPane(entity.PaneID("existing-pane")))
	existingTabs := entity.NewTabList()
	existingTabs.Add(existingTab)
	existingTabs.SetActive(existingTab.ID)
	existingWindow := &browserWindow{id: "existing-window", tabs: existingTabs, mainWindow: oldWindow}

	app := &App{
		tabs:           tabs,
		tabsUC:         tabsUC,
		browserWindows: map[string]*browserWindow{existingWindow.id: existingWindow},
		windowForTab:   map[entity.TabID]*browserWindow{existingTab.ID: existingWindow},
		browserWindowFactory: func(context.Context, string) (*browserWindow, error) {
			return &browserWindow{id: "window-1", mainWindow: newWindow}, nil
		},
		mainWindow: oldWindow,
		tabCoord: coordinator.NewTabCoordinator(context.Background(), coordinator.TabCoordinatorConfig{
			TabsUC:                  tabsUC,
			Tabs:                    tabs,
			MainWindow:              oldWindow,
			HideTabBarWhenSingleTab: true,
		}),
		workspaceViews: make(map[entity.TabID]*component.WorkspaceView),
	}
	app.tabCoord.SetOnTabCreated(func(ctx context.Context, target coordinator.TabTarget, tab *entity.Tab) {
		app.workspaceViews[tab.ID] = &component.WorkspaceView{}
	})

	if err := app.OpenFreshWindow(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("OpenFreshWindow returned error: %v", err)
	}

	gotCreatedTabID := app.tabs.ActiveTabID
	if gotCreatedTabID == "" {
		t.Fatalf("created tab id = %q, want non-empty", gotCreatedTabID)
	}
	if got := windowTabBarActiveID(t, newWindow); got != gotCreatedTabID {
		t.Fatalf("new window tab bar active tab = %q, want %q", got, gotCreatedTabID)
	}
	if got := windowTabBarActiveID(t, oldWindow); got != "" {
		t.Fatalf("old window tab bar active tab = %q, want empty", got)
	}
	// The lightweight test tab bar shell has no GTK box, so allocation state is
	// covered by TestApp_UpdateBrowserWindowTabBarVisibilityAutoHidesSingleTabButPreservesAllocation.
}

func TestApp_ActivateBrowserWindowSwitchesActiveWorkspace(t *testing.T) {
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	ws1 := &component.WorkspaceView{}
	ws2 := &component.WorkspaceView{}
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs, mainWindow: &window.MainWindow{}}
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs, mainWindow: &window.MainWindow{}}
	kh := &input.KeyboardHandler{}
	gs := &input.GlobalShortcutHandler{}
	second.keyboardHandler = kh
	second.globalShortcutHandler = gs
	app := &App{
		tabs:           tabs,
		mainWindow:     first.mainWindow,
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:   map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{tab1.ID: ws1, tab2.ID: ws2},
	}

	app.activateBrowserWindow(second)

	if app.mainWindow != second.mainWindow {
		t.Fatalf("mainWindow = %p, want %p", app.mainWindow, second.mainWindow)
	}
	if second.tabs.ActiveTabID != tab2.ID {
		t.Fatalf("active tab = %q, want %q", second.tabs.ActiveTabID, tab2.ID)
	}
	if app.activeWorkspaceView() != ws2 {
		t.Fatalf("active workspace view = %p, want %p", app.activeWorkspaceView(), ws2)
	}
	if app.keyboardHandler != kh {
		t.Fatalf("keyboardHandler = %p, want %p", app.keyboardHandler, kh)
	}
	if app.globalShortcutHandler != gs {
		t.Fatalf("globalShortcutHandler = %p, want %p", app.globalShortcutHandler, gs)
	}
}

func TestApp_BrowserWindowForPaneFindsOwningWindow(t *testing.T) {
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	second := &browserWindow{id: "window-2", tabs: secondTabs}
	app := &App{
		tabs:           tabs,
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:   map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
	}

	got := app.browserWindowForPane(entity.PaneID("pane-2"))

	if got != second {
		t.Fatalf("browserWindowForPane = %p, want %p", got, second)
	}
}

func TestApp_OwnerOrLastFocusedBrowserWindowPrefersPaneOwner(t *testing.T) {
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	second := &browserWindow{id: "window-2", tabs: secondTabs}
	app := &App{
		tabs:                tabs,
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		lastFocusedWindowID: first.id,
	}

	got := app.ownerOrLastFocusedBrowserWindow("", entity.PaneID("pane-2"))

	if got != second {
		t.Fatalf("ownerOrLastFocusedBrowserWindow = %p, want %p", got, second)
	}
}

func TestApp_HandlePaneWindowTitleChangedTargetsOwningWindow(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	firstMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("first window creation failed: %v", err)
	}
	defer firstMainWindow.Destroy()
	secondMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("second window creation failed: %v", err)
	}
	defer secondMainWindow.Destroy()

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow, tabs: firstTabs}
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow, tabs: secondTabs}
	app := &App{
		tabs:                tabs,
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		lastFocusedWindowID: first.id,
	}

	app.handlePaneWindowTitleChanged(entity.PaneID("pane-2"), "Pane Two")

	if got := windowTitle(t, firstMainWindow); got != "Dumber" {
		t.Fatalf("first window title = %q, want %q", got, "Dumber")
	}
	if got := windowTitle(t, secondMainWindow); got != "Pane Two - Dumber" {
		t.Fatalf("second window title = %q, want %q", got, "Pane Two - Dumber")
	}
}

func TestApp_ActivateBrowserWindowResyncsTitleFromBackgroundPane(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	firstMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("first window creation failed: %v", err)
	}
	defer firstMainWindow.Destroy()
	secondMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("second window creation failed: %v", err)
	}
	defer secondMainWindow.Destroy()

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow, tabs: firstTabs}
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow, tabs: secondTabs}
	contentCoord := &contentcoord.Coordinator{}
	contentTitles := reflect.ValueOf(contentCoord).Elem().FieldByName("paneTitles")
	reflect.NewAt(contentTitles.Type(), unsafe.Pointer(contentTitles.UnsafeAddr())).Elem().Set(reflect.ValueOf(map[entity.PaneID]string{tab2.Workspace.ActivePaneID: "Pane Two"}))
	app := &App{
		tabs:           tabs,
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:   map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		contentCoord:   contentCoord,
	}

	app.handlePaneWindowTitleChanged(tab2.Workspace.ActivePaneID, "Pane Two")
	secondMainWindow.SetTitle("Dumber")

	app.activateBrowserWindow(second)

	if got := windowTitle(t, secondMainWindow); got != "Pane Two - Dumber" {
		t.Fatalf("second window title = %q, want %q", got, "Pane Two - Dumber")
	}
	if got := windowTitle(t, firstMainWindow); got != "Dumber" {
		t.Fatalf("first window title = %q, want %q", got, "Dumber")
	}
}

func TestApp_HandlePaneFullscreenChangedTargetsOwningWindow(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	firstMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("first window creation failed: %v", err)
	}
	defer firstMainWindow.Destroy()
	secondMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("second window creation failed: %v", err)
	}
	defer secondMainWindow.Destroy()

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	secondMainWindow.TabBar().AddTab(tab1)
	secondMainWindow.TabBar().AddTab(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow, tabs: firstTabs}
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow, tabs: secondTabs}
	app := &App{
		tabs:                tabs,
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		lastFocusedWindowID: first.id,
	}

	app.handlePaneFullscreenChanged(entity.PaneID("pane-2"), true)

	if windowTabBarVisible(t, firstMainWindow) != true {
		t.Fatalf("first window tab bar visibility changed unexpectedly")
	}
	if got := windowTabBarVisible(t, secondMainWindow); got {
		t.Fatalf("second window tab bar visible = %v, want false", got)
	}

	app.handlePaneFullscreenChanged(entity.PaneID("pane-2"), false)

	if got := windowTabBarVisible(t, secondMainWindow); !got {
		t.Fatalf("second window tab bar visible = %v, want true", got)
	}
}

func TestApp_HandlePaneFullscreenChangedIgnoresUnresolvedPane(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	firstMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("first window creation failed: %v", err)
	}
	defer firstMainWindow.Destroy()
	secondMainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("second window creation failed: %v", err)
	}
	defer secondMainWindow.Destroy()
	firstMainWindow.TabBar().SetVisible(true)
	secondMainWindow.TabBar().SetVisible(true)
	first := &browserWindow{id: "window-1", mainWindow: firstMainWindow}
	second := &browserWindow{id: "window-2", mainWindow: secondMainWindow}
	app := &App{
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id,
	}

	app.handlePaneFullscreenChanged(entity.PaneID("missing-pane"), true)

	if got := windowTabBarVisible(t, firstMainWindow); !got {
		t.Fatalf("first window tab bar visible = %v, want true", got)
	}
	if got := windowTabBarVisible(t, secondMainWindow); !got {
		t.Fatalf("second window tab bar visible = %v, want true", got)
	}
}

func TestApp_HandleAccentKeyPressDelegatesToOwningWindow(t *testing.T) {
	first := newTestAccentUseCase(t, true)
	second := newTestAccentUseCase(t, false)
	firstWindow := &browserWindow{id: "window-1", insertAccentUC: first}
	secondWindow := &browserWindow{id: "window-2", insertAccentUC: second}
	app := &App{
		browserWindows:      map[string]*browserWindow{firstWindow.id: firstWindow, secondWindow.id: secondWindow},
		lastFocusedWindowID: secondWindow.id,
	}

	if got := app.handleAccentKeyPress(context.Background(), uint('e'), 0); got {
		t.Fatalf("handleAccentKeyPress returned %v, want false from active shell handler", got)
	}
	if got := first.IsPickerVisible(); !got {
		t.Fatalf("first shell accent handler should remain visible")
	}
	if got := second.IsPickerVisible(); got {
		t.Fatalf("second shell accent handler should remain hidden")
	}
}

func TestApp_MoveActivePaneToTabFromBrowserWindowAnchorsOwningWindow(t *testing.T) {
	tabs := entity.NewTabList()
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabs.Add(tab1)
	tabs.Add(tab2)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}
	app := &App{
		tabs:                tabs,
		movePaneToTabUC:     usecase.NewMovePaneToTabUseCase(func() string { return "new-tab" }),
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(&config.Config{}),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: second.id,
	}

	if err := app.moveActivePaneToTabFromBrowserWindow(context.Background(), first, ""); err != nil {
		t.Fatalf("moveActivePaneToTabFromBrowserWindow returned error: %v", err)
	}

	if tabs.Find(tab1.ID) != nil {
		t.Fatalf("tab-1 should have been moved from owning window")
	}
	if tabs.Find(tab2.ID) == nil {
		t.Fatalf("tab-2 should remain when owning window is preserved")
	}
}

func TestApp_MoveActivePaneToTabFromBrowserWindowActivatesTargetOwnerForCrossWindowTab(t *testing.T) {
	tabs := entity.NewTabList()
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tab3 := entity.NewTab(entity.TabID("tab-3"), entity.WorkspaceID("workspace-3"), entity.NewPane(entity.PaneID("pane-3")))
	tabs.Add(tab1)
	tabs.Add(tab2)
	tabs.Add(tab3)
	tabs.SetActive(tab1.ID)
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}
	app := &App{
		tabs:                tabs,
		movePaneToTabUC:     usecase.NewMovePaneToTabUseCase(func() string { return "generated" }),
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(&config.Config{}),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second, tab3.ID: second},
		lastFocusedWindowID: first.id,
	}

	if err := app.moveActivePaneToTabFromBrowserWindow(context.Background(), first, tab3.ID); err != nil {
		t.Fatalf("moveActivePaneToTabFromBrowserWindow returned error: %v", err)
	}

	if tabs.Find(tab1.ID) != nil {
		t.Fatalf("tab-1 should be removed after its active pane moves away")
	}
	if tabs.Find(tab2.ID) == nil {
		t.Fatalf("tab-2 should remain in the target owner window")
	}
	if tabs.Find(tab3.ID) == nil {
		t.Fatalf("tab-3 should remain as the move target")
	}
	if app.tabs.ActiveTabID != tab3.ID {
		t.Fatalf("active tab = %q, want %q", app.tabs.ActiveTabID, tab3.ID)
	}
	if app.lastFocusedWindowID != second.id {
		t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, second.id)
	}
}

func TestApp_MoveActivePaneToTabFromBrowserWindowCrossWindowDerivedMirror(t *testing.T) {
	// app.tabs initially contains ONLY source tab (not target tab) to simulate
	// a target created only in browserWindow.tabs (the real-world scenario that
	// CodeRabbit flagged). The fix ensures buildMovePaneToTabInput syncs the
	// derived global mirror before passing a.tabs to the move usecase.
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab3 := entity.NewTab(entity.TabID("tab-3"), entity.WorkspaceID("workspace-3"), entity.NewPane(entity.PaneID("pane-3")))

	// app.tabs contains only tab1 — tab3 is absent from the mirror.
	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.SetActive(tab1.ID)

	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}

	secondTabs := entity.NewTabList()
	secondTabs.Add(tab3)
	secondTabs.SetActive(tab3.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	initialPaneCount := tab3.Workspace.PaneCount()

	app := &App{
		tabs:                tabs,
		movePaneToTabUC:     usecase.NewMovePaneToTabUseCase(func() string { return "generated" }),
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(&config.Config{}),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab3.ID: second},
		lastFocusedWindowID: first.id,
	}

	if err := app.moveActivePaneToTabFromBrowserWindow(context.Background(), first, tab3.ID); err != nil {
		t.Fatalf("moveActivePaneToTabFromBrowserWindow returned error: %v", err)
	}

	// The derived mirror must now contain tab3 (synced by the fix).
	if app.tabs.Find(tab3.ID) == nil {
		t.Fatalf("tab-3 should have been synced into the derived global mirror")
	}

	// No new tab should have been generated — the move should target tab3.
	if app.tabs.Find(entity.TabID("generated")) != nil {
		t.Fatalf("a new tab was incorrectly generated; the move should target tab-3")
	}

	// The moved pane should be in tab3's workspace.
	if got := tab3.Workspace.PaneCount(); got != initialPaneCount+1 {
		t.Fatalf("tab-3 pane count = %d, want %d (original + moved pane)", got, initialPaneCount+1)
	}

	// windowForTab for tab3 must remain second.
	if app.windowForTab[tab3.ID] != second {
		t.Fatalf("windowForTab[tab-3] = %p, want second window %p", app.windowForTab[tab3.ID], second)
	}
}

func TestApp_InitKeyboardHandlerDoesNotReattachExistingWindowInput(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	sentinelKeyboardHandler := &input.KeyboardHandler{}
	sentinelShortcutHandler := &input.GlobalShortcutHandler{}
	bw := &browserWindow{
		id:                    "window-1",
		mainWindow:            mainWindow,
		keyboardHandler:       sentinelKeyboardHandler,
		globalShortcutHandler: sentinelShortcutHandler,
	}
	app := &App{
		deps:           &Dependencies{},
		runtimeConfig:  runtimeConfigStateForTest(&config.Config{}),
		mainWindow:     mainWindow,
		browserWindows: map[string]*browserWindow{bw.id: bw},
		kbDispatcher:   dispatcher.NewKeyboardDispatcher(context.Background(), nil, nil, nil, nil, dispatcher.KeyboardActions{}, func(context.Context) entity.PaneID { return "" }),
	}

	app.initKeyboardHandler(context.Background())

	if bw.keyboardHandler != sentinelKeyboardHandler {
		t.Fatalf("keyboardHandler was reattached")
	}
	if bw.globalShortcutHandler != sentinelShortcutHandler {
		t.Fatalf("globalShortcutHandler was reattached")
	}
}

func newTestAccentUseCase(t *testing.T, pickerVisible bool) *usecase.InsertAccentUseCase {
	t.Helper()
	uc := usecase.NewInsertAccentUseCase(&testFocusedInputProvider{}, nil, nil)
	if uc == nil {
		t.Fatal("failed to create accent use case")
	}
	setAccentUseCaseBoolField(t, uc, "pickerVisible", pickerVisible)
	return uc
}

func setAccentUseCaseBoolField(t *testing.T, uc *usecase.InsertAccentUseCase, field string, value bool) {
	t.Helper()
	rv := reflect.ValueOf(uc).Elem()
	fv := rv.FieldByName(field)
	if !fv.IsValid() {
		t.Fatalf("InsertAccentUseCase missing %s field", field)
	}
	reflect.NewAt(fv.Type(), unsafe.Pointer(fv.UnsafeAddr())).Elem().SetBool(value)
}

type testFocusedInputProvider struct{}

func (*testFocusedInputProvider) GetFocusedInput() port.TextInputTarget { return nil }
func (*testFocusedInputProvider) SetFocusedInput(port.TextInputTarget)  {}

func TestApp_UpdateBrowserWindowTabBarVisibilityAutoHidesSingleTabButPreservesAllocation(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow := &window.MainWindow{}
	tabBar := component.NewTabBar()
	if tabBar == nil {
		t.Fatal("tab bar creation failed")
	}
	setWindowTabBar(t, mainWindow, tabBar)
	bw := &browserWindow{id: "window-1", mainWindow: mainWindow}
	cfg := &config.Config{}
	cfg.Workspace.HideTabBarWhenSingleTab = true
	app := &App{
		deps:          &Dependencies{},
		runtimeConfig: runtimeConfigStateForTest(cfg),
	}

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	tabBar.AddTab(tab1)

	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, true)

	tabBar.AddTab(tab2)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, false)

	tabBar.SetVisible(false)
	app.updateBrowserWindowTabBarVisibility(bw)
	if got := tabBar.Box().GetVisible(); got {
		t.Fatalf("tab bar visible = %v, want false while explicitly hidden", got)
	}

	tabBar.SetVisible(true)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, false)

	tabBar.RemoveTab(tab2.ID)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, true)
}

func TestTabBarSetVisibleDoesNotClearAutoHiddenInteractionState(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	tabBar := component.NewTabBar()
	if tabBar == nil {
		t.Fatal("tab bar creation failed")
	}

	tabBar.SetAutoHidden(true)
	tabBar.SetVisible(false)
	tabBar.SetVisible(true)

	box := tabBar.Box()
	if !box.GetVisible() {
		t.Fatal("tab bar should stay mounted when explicitly visible")
	}
	if got := box.GetOpacity(); got != 0.0 {
		t.Fatalf("tab bar opacity = %v, want 0 while auto-hidden", got)
	}
	if got := box.GetCanTarget(); got {
		t.Fatalf("tab bar can target = %v, want false while auto-hidden", got)
	}
	if got := box.GetFocusable(); got {
		t.Fatalf("tab bar focusable = %v, want false while auto-hidden", got)
	}
}

func TestApp_UpdateBrowserWindowTabBarVisibilityHonorsHideWhenSingleTabDisabled(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow := &window.MainWindow{}
	tabBar := component.NewTabBar()
	if tabBar == nil {
		t.Fatal("tab bar creation failed")
	}
	setWindowTabBar(t, mainWindow, tabBar)
	bw := &browserWindow{id: "window-1", mainWindow: mainWindow}
	cfg := &config.Config{}
	cfg.Workspace.HideTabBarWhenSingleTab = false
	app := &App{
		deps:          &Dependencies{},
		runtimeConfig: runtimeConfigStateForTest(cfg),
	}

	app.updateBrowserWindowTabBarVisibility(bw)

	assertWindowTabBarAutoHidden(t, mainWindow, false)
}

func TestApp_ActivePaneIDForNilBrowserWindowIgnoresStaleOverride(t *testing.T) {
	ctx := context.Background()
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.SetActivePaneOverride(entity.PaneID("stale-pane"))
	app := &App{contentCoord: contentCoord}

	if got := app.activePaneIDForBrowserWindow(nil); got != "" {
		t.Fatalf("active pane for nil browser window = %q, want empty", got)
	}
}

func TestApp_BuildRestoredWindowUIUpdatesEmptyWindowTabBarVisibility(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mw, err := window.New(context.Background(), gtkApp, "bottom")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mw.Destroy()

	mw.SetTabBarContentInsetVisible(true)
	bw := &browserWindow{id: "window-1", mainWindow: mw, tabs: entity.NewTabList()}
	cfg := &config.Config{}
	cfg.Workspace.HideTabBarWhenSingleTab = true
	app := appWithRuntimeConfigForTest(cfg)

	app.buildRestoredWindowUI(context.Background(), []*browserWindow{bw})

	assertWindowTabBarAutoHidden(t, mw, true)
	if mw.HasTabBarContentInset() {
		t.Fatal("empty restored bottom window should not keep tab bar content inset")
	}
}

func TestApp_UpdateBrowserWindowTabBarVisibility_BottomInset(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	// Use a real bottom MainWindow so SetTabBarContentInsetVisible is effective.
	mainWindow, err := window.New(context.Background(), gtkApp, "bottom")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	bw := &browserWindow{id: "window-1", mainWindow: mainWindow}
	cfg := &config.Config{}
	cfg.Workspace.HideTabBarWhenSingleTab = true
	app := &App{
		deps:          &Dependencies{},
		runtimeConfig: runtimeConfigStateForTest(cfg),
	}

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))

	// One tab + hide enabled: auto-hidden and no inset
	mainWindow.TabBar().AddTab(tab1)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, true)
	if mainWindow.HasTabBarContentInset() {
		t.Fatal("one tab: expected no content area inset")
	}

	// Two tabs: visible and inset present
	mainWindow.TabBar().AddTab(tab2)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, false)
	if !mainWindow.HasTabBarContentInset() {
		t.Fatal("two tabs: expected content area inset")
	}

	// Remove back to one tab: auto-hidden and no inset
	mainWindow.TabBar().RemoveTab(tab2.ID)
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, true)
	if mainWindow.HasTabBarContentInset() {
		t.Fatal("back to one tab: expected no content area inset")
	}
}

func TestApp_HandlePaneFullscreenChanged_ClearsBottomInset(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	// Use a bottom MainWindow so SetTabBarContentInsetVisible is effective.
	mainWindow, err := window.New(context.Background(), gtkApp, "bottom")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("workspace-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("workspace-2"), entity.NewPane(entity.PaneID("pane-2")))
	mainWindow.TabBar().AddTab(tab1)
	mainWindow.TabBar().AddTab(tab2)

	tabs := entity.NewTabList()
	tabs.Add(tab1)
	tabs.Add(tab2)
	tabs.SetActive(tab2.ID)

	bw := &browserWindow{id: "window-1", mainWindow: mainWindow, tabs: tabs}
	cfg := &config.Config{}
	cfg.Workspace.HideTabBarWhenSingleTab = true
	app := &App{
		tabs:                tabs,
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(cfg),
		browserWindows:      map[string]*browserWindow{bw.id: bw},
		lastFocusedWindowID: bw.id,
	}

	// Start with two tabs: inset should be present
	app.updateBrowserWindowTabBarVisibility(bw)
	assertWindowTabBarAutoHidden(t, mainWindow, false)
	if !mainWindow.HasTabBarContentInset() {
		t.Fatal("two tabs: expected content area inset before fullscreen")
	}

	// Enter fullscreen: tab bar hidden, inset cleared
	app.handlePaneFullscreenChanged(entity.PaneID("pane-2"), true)
	if windowTabBarVisible(t, mainWindow) {
		t.Fatal("fullscreen: tab bar should be not visible")
	}
	if mainWindow.HasTabBarContentInset() {
		t.Fatal("fullscreen: expected no content area inset")
	}

	// Exit fullscreen: tab bar restored, inset restored
	app.handlePaneFullscreenChanged(entity.PaneID("pane-2"), false)
	if !windowTabBarVisible(t, mainWindow) {
		t.Fatal("exited fullscreen: tab bar should be visible")
	}
	if !mainWindow.HasTabBarContentInset() {
		t.Fatal("exited fullscreen: expected content area inset to be restored")
	}
}

func TestApp_RemoveBrowserWindowPromotesDeterministicFallbackWithMainWindow(t *testing.T) {
	for i := 0; i < 20; i++ {
		removed := &browserWindow{id: "window-z", mainWindow: &window.MainWindow{}}
		nilWindow := &browserWindow{id: "window-a"}
		firstValid := &browserWindow{id: "window-b", mainWindow: &window.MainWindow{}}
		secondValid := &browserWindow{id: "window-c", mainWindow: &window.MainWindow{}}
		app := &App{
			mainWindow: removed.mainWindow,
			browserWindows: map[string]*browserWindow{
				removed.id:     removed,
				nilWindow.id:   nilWindow,
				firstValid.id:  firstValid,
				secondValid.id: secondValid,
			},
			browserWindowOrder:  []string{removed.id, nilWindow.id, secondValid.id, firstValid.id},
			lastFocusedWindowID: removed.id,
		}

		app.removeBrowserWindow(removed.id)

		if app.mainWindow != secondValid.mainWindow {
			t.Fatalf("mainWindow = %p, want %p", app.mainWindow, secondValid.mainWindow)
		}
		if app.lastFocusedWindowID != secondValid.id {
			t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, secondValid.id)
		}
	}
}

func TestApp_BrowserWindowActivationHookUpdatesLastFocusedWindowID(t *testing.T) {
	first := &browserWindow{id: "window-1", mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", mainWindow: &window.MainWindow{}}
	app := &App{
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id,
	}

	app.handleBrowserWindowActivationChanged(second, true)

	if app.lastFocusedWindowID != second.id {
		t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, second.id)
	}
}

func TestApp_ActivateBrowserWindowTracksLastFocusedWindow(t *testing.T) {
	first := &browserWindow{id: "window-1", mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", mainWindow: &window.MainWindow{}}
	app := &App{
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
	}

	app.activateBrowserWindow(second)

	if app.lastFocusedWindowID != second.id {
		t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, second.id)
	}
}

func TestApp_LastFocusedBrowserWindowFallsBackWhenOwnerMissing(t *testing.T) {
	first := &browserWindow{id: "window-1"}
	second := &browserWindow{id: "window-2"}
	app := &App{
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: second.id,
	}

	got := app.lastFocusedBrowserWindow()

	if got != second {
		t.Fatalf("lastFocusedBrowserWindow = %p, want %p", got, second)
	}
}

func TestApp_CreatePopupTabUsesParentPaneOwnerWhenFocusIsStale(t *testing.T) {
	ctx := context.Background()
	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	secondTab := entity.NewTab(entity.TabID("second-tab"), entity.WorkspaceID("ws-second"), entity.NewPane(entity.PaneID("pane-second")))
	globalTabs := entity.NewTabList()
	globalTabs.Add(firstTab)
	globalTabs.Add(secondTab)
	firstTabs := entity.NewTabList()
	firstTabs.Add(firstTab)
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab)
	first := &browserWindow{id: "window-1", tabs: firstTabs, mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", tabs: secondTabs, mainWindow: &window.MainWindow{}}
	app := &App{
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(&config.Config{}),
		tabs:                globalTabs,
		tabsUC:              usecase.NewManageTabsUseCase(func() string { return "popup-tab" }),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{firstTab.ID: first, secondTab.ID: second},
		workspaceViews:      make(map[entity.TabID]*component.WorkspaceView),
		lastFocusedWindowID: first.id,
		mainWindow:          first.mainWindow,
	}
	app.initTabCoordinator(ctx)

	err := app.createPopupTab(ctx, contentcoord.InsertPopupInput{
		ParentPaneID: secondTab.Workspace.ActivePaneID,
		PopupPane:    entity.NewPane(entity.PaneID("popup-pane")),
		TargetURI:    "https://example.com/popup",
	})
	if err != nil {
		t.Fatalf("createPopupTab returned error: %v", err)
	}

	if first.tabs.Find(entity.TabID("popup-tab")) != nil {
		t.Fatalf("popup tab was added to stale focused window")
	}
	created := second.tabs.Find(entity.TabID("popup-tab"))
	if created == nil {
		t.Fatalf("popup tab was not added to parent pane owner window")
	}
	if got := app.windowForTab[created.ID]; got != second {
		t.Fatalf("popup tab owner = %p, want %p", got, second)
	}
}

func TestApp_RemoveBrowserWindowClearsLastFocusedFallback(t *testing.T) {
	first := &browserWindow{id: "window-1", mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", mainWindow: &window.MainWindow{}}
	app := &App{
		mainWindow:          second.mainWindow,
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: second.id,
	}

	app.removeBrowserWindow(second.id)

	if app.lastFocusedWindowID != first.id {
		t.Fatalf("lastFocusedWindowID = %q, want %q", app.lastFocusedWindowID, first.id)
	}
}

func TestApp_TabCoordinatorCallbacksUseTargetWindowWhenFocusIsStale(t *testing.T) {
	ctx := context.Background()
	firstTabs := entity.NewTabList()
	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	firstTabs.Add(firstTab)
	secondTabs := entity.NewTabList()
	secondTab := entity.NewTab(entity.TabID("second-tab"), entity.WorkspaceID("ws-second"), entity.NewPane(entity.PaneID("pane-second")))
	secondTabs.Add(secondTab)

	first := &browserWindow{id: "window-1", tabs: firstTabs, mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", tabs: secondTabs, mainWindow: &window.MainWindow{}}
	app := &App{
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(&config.Config{}),
		tabs:                entity.NewTabList(),
		tabsUC:              usecase.NewManageTabsUseCase(func() string { return "created-tab" }),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id,
		mainWindow:          first.mainWindow,
		windowForTab:        map[entity.TabID]*browserWindow{firstTab.ID: first, secondTab.ID: second},
		workspaceViews:      make(map[entity.TabID]*component.WorkspaceView),
	}
	app.tabs.Add(firstTab)
	app.tabs.Add(secondTab)
	app.initTabCoordinator(ctx)

	created, err := app.tabCoord.Create(ctx, app.tabTargetForBrowserWindow(second), "https://example.com")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if got := app.windowForTab[created.ID]; got != second {
		t.Fatalf("created tab owner = %p, want second window %p", got, second)
	}
	if app.mainWindow != second.mainWindow {
		t.Fatalf("mainWindow after create switch = %p, want %p", app.mainWindow, second.mainWindow)
	}

	app.mainWindow = first.mainWindow
	app.lastFocusedWindowID = first.id
	if err := app.tabCoord.Switch(ctx, app.tabTargetForBrowserWindow(second), secondTab.ID); err != nil {
		t.Fatalf("Switch returned error: %v", err)
	}
	if app.mainWindow != second.mainWindow {
		t.Fatalf("mainWindow after target switch = %p, want %p", app.mainWindow, second.mainWindow)
	}
}

func TestApp_ActivateBrowserWindowAddsGlobalSnapshotWithoutChangingWindowTabPosition(t *testing.T) {
	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	secondTab := entity.NewTab(entity.TabID("second-tab"), entity.WorkspaceID("ws-second"), entity.NewPane(entity.PaneID("pane-second")))
	firstTabs := entity.NewTabList()
	firstTabs.Add(firstTab)
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab)
	secondTabs.SetActive(secondTab.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs, mainWindow: &window.MainWindow{}}
	second := &browserWindow{id: "window-2", tabs: secondTabs, mainWindow: &window.MainWindow{}}
	globalTabs := entity.NewTabList()
	globalTabs.Add(firstTab)
	app := &App{
		tabs:           globalTabs,
		browserWindows: map[string]*browserWindow{first.id: first, second.id: second},
	}

	app.activateBrowserWindow(second)

	if secondTab.Position != 0 {
		t.Fatalf("window tab position = %d, want 0", secondTab.Position)
	}
	globalSecond := app.tabs.Find(secondTab.ID)
	if globalSecond == nil {
		t.Fatalf("global tab snapshot was not added")
	}
	if globalSecond == secondTab {
		t.Fatalf("global tab should be a snapshot, not the live per-window tab")
	}
	if globalSecond.Position != 1 {
		t.Fatalf("global tab position = %d, want 1", globalSecond.Position)
	}
}

func TestApp_ActiveWorkspaceRequiresFocusedBrowserWindow(t *testing.T) {
	globalTab := entity.NewTab(entity.TabID("global-tab"), entity.WorkspaceID("ws-global"), entity.NewPane(entity.PaneID("pane-global")))
	globalTabs := entity.NewTabList()
	globalTabs.Add(globalTab)
	globalTabs.SetActive(globalTab.ID)
	app := &App{
		tabs:           globalTabs,
		browserWindows: map[string]*browserWindow{},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{globalTab.ID: {}},
	}

	if got := app.activeWorkspace(); got != nil {
		t.Fatalf("activeWorkspace() = %p, want nil without focused browser window", got)
	}
	if got := app.activeWorkspaceView(); got != nil {
		t.Fatalf("activeWorkspaceView() = %p, want nil without focused browser window", got)
	}
}

func TestApp_ActiveWorkspaceUsesLastFocusedWindowTabsNotGlobalActiveTab(t *testing.T) {
	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	secondTab := entity.NewTab(entity.TabID("second-tab"), entity.WorkspaceID("ws-second"), entity.NewPane(entity.PaneID("pane-second")))
	firstTabs := entity.NewTabList()
	firstTabs.Add(firstTab)
	firstTabs.SetActive(firstTab.ID)
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab)
	secondTabs.SetActive(secondTab.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	second := &browserWindow{id: "window-2", tabs: secondTabs}
	globalTabs := entity.NewTabList()
	globalTabs.Add(firstTab)
	globalTabs.Add(secondTab)
	globalTabs.SetActive(firstTab.ID)
	secondView := &component.WorkspaceView{}
	app := &App{
		tabs:                globalTabs,
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: second.id,
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{secondTab.ID: secondView},
	}

	if got := app.activeWorkspace(); got != secondTab.Workspace {
		t.Fatalf("activeWorkspace() = %p, want second workspace %p", got, secondTab.Workspace)
	}
	if got := app.activeWorkspaceView(); got != secondView {
		t.Fatalf("activeWorkspaceView() = %p, want second view %p", got, secondView)
	}
}

// recordingWebView is a complete port.WebView that records which navigation
// methods were called and with what arguments. It does not use reflect or unsafe.
type recordingWebView struct {
	id port.WebViewID

	loadURICalled          bool
	loadURILastURI         string
	reloadCalls            int
	reloadBypassCacheCalls int
	stopCalls              int
	goBackCalls            int
	goForwardCalls         int
	loadHTMLCalls          int
	setZoomLevelCalls      int
	setZoomLevelLastLevel  float64
	openDevToolsCalls      int
	printPageCalls         int
	destroyCalls           int
}

func (f *recordingWebView) ID() port.WebViewID { return f.id }

func (f *recordingWebView) LoadURI(_ context.Context, uri string) error {
	f.loadURICalled = true
	f.loadURILastURI = uri
	return nil
}

func (f *recordingWebView) LoadHTML(_ context.Context, _, _ string) error {
	f.loadHTMLCalls++
	return nil
}

func (f *recordingWebView) Reload(_ context.Context) error {
	f.reloadCalls++
	return nil
}

func (f *recordingWebView) ReloadBypassCache(_ context.Context) error {
	f.reloadBypassCacheCalls++
	return nil
}

func (f *recordingWebView) Stop(_ context.Context) error {
	f.stopCalls++
	return nil
}

func (f *recordingWebView) GoBack(_ context.Context) error {
	f.goBackCalls++
	return nil
}

func (f *recordingWebView) GoForward(_ context.Context) error {
	f.goForwardCalls++
	return nil
}

func (f *recordingWebView) State() port.WebViewState { return port.WebViewState{} }

func (f *recordingWebView) URI() string   { return f.loadURILastURI }
func (f *recordingWebView) Title() string { return "" }

func (f *recordingWebView) IsLoading() bool            { return false }
func (f *recordingWebView) EstimatedProgress() float64 { return 0 }
func (f *recordingWebView) CanGoBack() bool            { return false }
func (f *recordingWebView) CanGoForward() bool         { return false }

func (f *recordingWebView) SetZoomLevel(_ context.Context, level float64) error {
	f.setZoomLevelCalls++
	f.setZoomLevelLastLevel = level
	return nil
}

func (f *recordingWebView) OpenDevTools() { f.openDevToolsCalls++ }
func (f *recordingWebView) PrintPage()    { f.printPageCalls++ }

func (f *recordingWebView) GetZoomLevel() float64                     { return 1.0 }
func (f *recordingWebView) GetFindController() port.FindController    { return nil }
func (f *recordingWebView) SetCallbacks(_ *port.WebViewCallbacks)     {}
func (f *recordingWebView) RunJavaScript(_ context.Context, _ string) {}
func (f *recordingWebView) SetBackgroundColor(_, _, _, _ float64)     {}
func (f *recordingWebView) ResetBackgroundToDefault()                 {}
func (f *recordingWebView) Favicon() port.Texture                     { return nil }
func (f *recordingWebView) Generation() uint64                        { return 0 }
func (f *recordingWebView) IsFullscreen() bool                        { return false }
func (f *recordingWebView) IsPlayingAudio() bool                      { return false }
func (f *recordingWebView) IsDestroyed() bool                         { return f.destroyCalls > 0 }
func (f *recordingWebView) Destroy()                                  { f.destroyCalls++ }

func TestApp_AttachPopupToTabDestroysPopupWhenWorkspaceViewMissing(t *testing.T) {
	ctx := context.Background()
	popupWV := &recordingWebView{id: 1}
	app := &App{workspaceViews: map[entity.TabID]*component.WorkspaceView{}}

	app.attachPopupToTab(ctx, entity.TabID("missing-tab"), entity.NewPane(entity.PaneID("popup-pane")), popupWV)

	if popupWV.destroyCalls != 1 {
		t.Fatalf("popup webview destroy calls = %d, want 1", popupWV.destroyCalls)
	}
}

func TestApp_AttachPopupToTabDestroysPopupWhenPaneNil(t *testing.T) {
	ctx := context.Background()
	popupWV := &recordingWebView{id: 1}
	tabID := entity.TabID("tab-1")
	app := &App{workspaceViews: map[entity.TabID]*component.WorkspaceView{tabID: &component.WorkspaceView{}}}

	app.attachPopupToTab(ctx, tabID, nil, popupWV)

	if popupWV.destroyCalls != 1 {
		t.Fatalf("popup webview destroy calls = %d, want 1", popupWV.destroyCalls)
	}
}

func TestApp_AttachPopupToTabSkipsRegistrationWhenPaneViewMissing(t *testing.T) {
	ctx := context.Background()
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	tabID := entity.TabID("tab-1")
	pane := entity.NewPane(entity.PaneID("missing-pane"))
	app := &App{
		contentCoord: contentCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tabID: &component.WorkspaceView{},
		},
	}

	popupWV := &recordingWebView{id: 1}
	app.attachPopupToTab(ctx, tabID, pane, popupWV)

	if got := contentCoord.GetWebView(pane.ID); got != nil {
		t.Fatalf("popup webview was registered for missing pane view: %v", got)
	}
	if popupWV.destroyCalls != 1 {
		t.Fatalf("popup webview destroy calls = %d, want 1", popupWV.destroyCalls)
	}
}

func TestApp_AttachPopupToTabReleasesRegistrationWhenWrapFails(t *testing.T) {
	ctx := context.Background()
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	tabID := entity.TabID("tab-1")
	pane := entity.NewPane(entity.PaneID("popup-pane"))

	factory := layoutmocks.NewMockWidgetFactory(t)
	container := layoutmocks.NewMockBoxWidget(t)
	overlay := layoutmocks.NewMockOverlayWidget(t)
	factory.EXPECT().NewBox(layout.OrientationVertical, 0).Return(container).Once()
	container.EXPECT().SetHexpand(true).Once()
	container.EXPECT().SetVexpand(true).Once()
	container.EXPECT().SetVisible(true).Once()
	factory.EXPECT().NewOverlay().Return(overlay).Once()
	overlay.EXPECT().SetHexpand(true).Once()
	overlay.EXPECT().SetVexpand(true).Once()
	overlay.EXPECT().SetChild(container).Once()
	overlay.EXPECT().SetVisible(true).Once()

	wsView := component.NewWorkspaceView(ctx, factory)
	container.EXPECT().AddCssClass("single-pane").Once()
	wsView.RegisterPaneView(pane.ID, &component.PaneView{})

	app := &App{
		contentCoord: contentCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tabID: wsView,
		},
	}
	popupWV := &recordingWebView{id: 1}

	app.attachPopupToTab(ctx, tabID, pane, popupWV)

	if got := contentCoord.GetWebView(pane.ID); got != nil {
		t.Fatalf("popup webview registration remained after wrap failure: %v", got)
	}
	if popupWV.destroyCalls != 1 {
		t.Fatalf("popup webview destroy calls = %d, want 1", popupWV.destroyCalls)
	}
}

func TestApp_BrowserWindowWebViewActionsIgnoreStaleFocusedWindow(t *testing.T) {
	ctx := context.Background()

	// Build two browser windows with independent bw.tabs and active tabs/panes.
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("ws-2"), entity.NewPane(entity.PaneID("pane-2")))

	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}

	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	// Create recording webviews.
	recordingWv1 := &recordingWebView{id: 1}
	recordingWv2 := &recordingWebView{id: 2}

	// Create content coordinator with initialized internal maps to avoid
	// nil-map panics in SetNavigationOrigin during NavigateWebView calls.
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), recordingWv1)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-2"), recordingWv2)

	// Create NavigationCoordinator (no navigateUC needed for direct webview calls).
	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:                entity.NewTabList(),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id, // stale! should NOT be used
		contentCoord:        contentCoord,
		navCoord:            navCoord,
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab1.ID: &component.WorkspaceView{},
			tab2.ID: &component.WorkspaceView{},
		},
	}
	app.tabs.Add(tab1)
	app.tabs.Add(tab2)

	// Call window-scoped app helpers for the second window.
	if err := app.navigateFromBrowserWindow(ctx, second, "https://google.com"); err != nil {
		t.Fatalf("navigateFromBrowserWindow returned error: %v", err)
	}
	if err := app.reloadBrowserWindow(ctx, second, false); err != nil {
		t.Fatalf("reloadBrowserWindow (soft) returned error: %v", err)
	}
	if err := app.reloadBrowserWindow(ctx, second, true); err != nil {
		t.Fatalf("reloadBrowserWindow (hard) returned error: %v", err)
	}
	if err := app.stopBrowserWindow(ctx, second); err != nil {
		t.Fatalf("stopBrowserWindow returned error: %v", err)
	}
	if err := app.goBackBrowserWindow(ctx, second); err != nil {
		t.Fatalf("goBackBrowserWindow returned error: %v", err)
	}
	if err := app.goForwardBrowserWindow(ctx, second); err != nil {
		t.Fatalf("goForwardBrowserWindow returned error: %v", err)
	}

	assertRecordingWebViewIdle(t, "first", recordingWv1)
	assertRecordingWebViewActions(t, "second", recordingWv2, "https://google.com")
}

func assertRecordingWebViewIdle(t *testing.T, name string, wv *recordingWebView) {
	t.Helper()
	if wv.loadURICalled || wv.loadURILastURI != "" {
		t.Errorf("%s webview LoadURI was called, expected zero calls", name)
	}
	if wv.reloadCalls > 0 {
		t.Errorf("%s webview Reload calls = %d, want 0", name, wv.reloadCalls)
	}
	if wv.reloadBypassCacheCalls > 0 {
		t.Errorf("%s webview ReloadBypassCache calls = %d, want 0", name, wv.reloadBypassCacheCalls)
	}
	if wv.stopCalls > 0 {
		t.Errorf("%s webview Stop calls = %d, want 0", name, wv.stopCalls)
	}
	if wv.goBackCalls > 0 {
		t.Errorf("%s webview GoBack calls = %d, want 0", name, wv.goBackCalls)
	}
	if wv.goForwardCalls > 0 {
		t.Errorf("%s webview GoForward calls = %d, want 0", name, wv.goForwardCalls)
	}
}

func assertRecordingWebViewActions(t *testing.T, name string, wv *recordingWebView, wantURI string) {
	t.Helper()
	if !wv.loadURICalled {
		t.Errorf("%s webview LoadURI was not called", name)
	}
	if wv.loadURILastURI != wantURI {
		t.Errorf("%s webview LoadURI URI = %q, want %q", name, wv.loadURILastURI, wantURI)
	}
	if wv.reloadCalls != 1 {
		t.Errorf("%s webview Reload calls = %d, want 1", name, wv.reloadCalls)
	}
	if wv.reloadBypassCacheCalls != 1 {
		t.Errorf("%s webview ReloadBypassCache calls = %d, want 1", name, wv.reloadBypassCacheCalls)
	}
	if wv.stopCalls != 1 {
		t.Errorf("%s webview Stop calls = %d, want 1", name, wv.stopCalls)
	}
	if wv.goBackCalls != 1 {
		t.Errorf("%s webview GoBack calls = %d, want 1", name, wv.goBackCalls)
	}
	if wv.goForwardCalls != 1 {
		t.Errorf("%s webview GoForward calls = %d, want 1", name, wv.goForwardCalls)
	}
}

func assertRecordingWebViewBrowserActionsIdle(t *testing.T, name string, wv *recordingWebView) {
	t.Helper()
	if wv.reloadCalls > 0 {
		t.Errorf("%s webview Reload calls = %d, want 0", name, wv.reloadCalls)
	}
	if wv.reloadBypassCacheCalls > 0 {
		t.Errorf("%s webview ReloadBypassCache calls = %d, want 0", name, wv.reloadBypassCacheCalls)
	}
	if wv.stopCalls > 0 {
		t.Errorf("%s webview Stop calls = %d, want 0", name, wv.stopCalls)
	}
	if wv.goBackCalls > 0 {
		t.Errorf("%s webview GoBack calls = %d, want 0", name, wv.goBackCalls)
	}
	if wv.goForwardCalls > 0 {
		t.Errorf("%s webview GoForward calls = %d, want 0", name, wv.goForwardCalls)
	}
	if wv.printPageCalls > 0 {
		t.Errorf("%s webview PrintPage calls = %d, want 0", name, wv.printPageCalls)
	}
	if wv.openDevToolsCalls > 0 {
		t.Errorf("%s webview OpenDevTools calls = %d, want 0", name, wv.openDevToolsCalls)
	}
	if wv.setZoomLevelCalls > 0 {
		t.Errorf("%s webview SetZoomLevel calls = %d, want 0", name, wv.setZoomLevelCalls)
	}
}

func assertRecordingWebViewBrowserActionsCalledOnce(t *testing.T, name string, wv *recordingWebView) {
	t.Helper()
	if wv.reloadCalls != 1 {
		t.Errorf("%s webview Reload calls = %d, want 1", name, wv.reloadCalls)
	}
	if wv.reloadBypassCacheCalls != 1 {
		t.Errorf("%s webview ReloadBypassCache calls = %d, want 1", name, wv.reloadBypassCacheCalls)
	}
	if wv.stopCalls != 1 {
		t.Errorf("%s webview Stop calls = %d, want 1", name, wv.stopCalls)
	}
	if wv.goBackCalls != 1 {
		t.Errorf("%s webview GoBack calls = %d, want 1", name, wv.goBackCalls)
	}
	if wv.goForwardCalls != 1 {
		t.Errorf("%s webview GoForward calls = %d, want 1", name, wv.goForwardCalls)
	}
	if wv.printPageCalls != 1 {
		t.Errorf("%s webview PrintPage calls = %d, want 1", name, wv.printPageCalls)
	}
	if wv.openDevToolsCalls != 1 {
		t.Errorf("%s webview OpenDevTools calls = %d, want 1", name, wv.openDevToolsCalls)
	}
	if wv.setZoomLevelCalls != 1 {
		t.Errorf("%s webview SetZoomLevel calls = %d, want 1", name, wv.setZoomLevelCalls)
	}
}

func TestApp_OmniboxNavigateCallbackCapturesBrowserWindow(t *testing.T) {
	second := &browserWindow{id: "window-2"}

	var capturedBW *browserWindow
	var capturedURL string
	testNavigate := func(_ context.Context, bw *browserWindow, url string) error {
		capturedBW = bw
		capturedURL = url
		return nil
	}

	ctx := context.Background()
	cb := omniboxNavigateForBrowserWindow(ctx, second, testNavigate)
	if err := cb(ctx, "https://google.com"); err != nil {
		t.Fatalf("omnibox navigate callback returned error: %v", err)
	}

	if capturedBW != second {
		capturedID := "<nil>"
		if capturedBW != nil {
			capturedID = capturedBW.id
		}
		t.Errorf("captured browser window = %p (id=%s), want %p (id=%s)", capturedBW, capturedID, second, second.id)
	}
	if capturedURL != "https://google.com" {
		t.Errorf("captured URL = %q, want %q", capturedURL, "https://google.com")
	}
}

func TestApp_DispatchBrowserWindowActionUsesSourceWindow(t *testing.T) {
	ctx := context.Background()

	// Build two browser windows with independent bw.tabs and active panes.
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("ws-2"), entity.NewPane(entity.PaneID("pane-2")))

	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}

	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	// Create recording webviews.
	recordingWv1 := &recordingWebView{id: 1, loadURILastURI: "https://first.example"}
	recordingWv2 := &recordingWebView{id: 2, loadURILastURI: "https://second.example"}

	// Register recording webviews in content coordinator.
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), recordingWv1)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-2"), recordingWv2)

	// Create navCoord (no navigateUC needed for direct webview calls).
	navCoord := coordinator.NewNavigationCoordinator(ctx, nil, contentCoord)

	app := &App{
		tabs:                entity.NewTabList(),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		lastFocusedWindowID: first.id, // stale! should NOT be used
		contentCoord:        contentCoord,
		navCoord:            navCoord,
		deps:                &Dependencies{ZoomUC: usecase.NewManageZoomUseCase(mockZoomRepo(t), 1.0, nil)},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab1.ID: &component.WorkspaceView{},
			tab2.ID: &component.WorkspaceView{},
		},
	}
	app.tabs.Add(tab1)
	app.tabs.Add(tab2)

	// Dispatch browser window actions targeting the second window.
	for _, a := range []input.Action{
		input.ActionReload,
		input.ActionHardReload,
		input.ActionStop,
		input.ActionGoBack,
		input.ActionGoForward,
		input.ActionPrintPage,
		input.ActionOpenDevTools,
		input.ActionZoomIn,
	} {
		if err := app.dispatchBrowserWindowAction(ctx, second, a); err != nil {
			t.Fatalf("dispatchBrowserWindowAction (%s) returned error: %v", a, err)
		}
	}

	// Assert first recording webview receives zero calls (stale focus is ignored).
	assertRecordingWebViewBrowserActionsIdle(t, "first", recordingWv1)

	// Assert second recording webview receives exactly one call per action.
	assertRecordingWebViewBrowserActionsCalledOnce(t, "second", recordingWv2)
}

func TestApp_DispatchBrowserWindowActionZoomInSupportsFileURLs(t *testing.T) {
	ctx := context.Background()

	tab := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	tabs := entity.NewTabList()
	tabs.Add(tab)
	tabs.SetActive(tab.ID)
	bw := &browserWindow{id: "window-1", tabs: tabs}

	recordingWv := &recordingWebView{id: 1, loadURILastURI: "file:///tmp/demo.html"}
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), recordingWv)

	app := &App{
		browserWindows: map[string]*browserWindow{bw.id: bw},
		tabs:           entity.NewTabList(),
		windowForTab:   map[entity.TabID]*browserWindow{tab.ID: bw},
		contentCoord:   contentCoord,
		deps:           &Dependencies{ZoomUC: usecase.NewManageZoomUseCase(mockZoomRepo(t), 1.0, nil)},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{
			tab.ID: &component.WorkspaceView{},
		},
	}
	app.tabs.Add(tab)
	app.tabs.SetActive(tab.ID)

	if err := app.dispatchBrowserWindowAction(ctx, bw, input.ActionZoomIn); err != nil {
		t.Fatalf("dispatchBrowserWindowAction returned error: %v", err)
	}
	if recordingWv.setZoomLevelCalls != 1 {
		t.Fatalf("webview SetZoomLevel calls = %d, want 1", recordingWv.setZoomLevelCalls)
	}
	if diff := math.Abs(recordingWv.setZoomLevelLastLevel - 1.1); diff > 0.0001 {
		t.Fatalf("webview SetZoomLevel level = %f, want 1.1", recordingWv.setZoomLevelLastLevel)
	}
}

func counterIDGen() func() string {
	var counter int
	return func() string {
		counter++
		return strconv.Itoa(counter)
	}
}

func TestApp_DispatchBrowserWindowActionSwitchTabIndexUsesSourceWindow(t *testing.T) {
	ctx := context.Background()

	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	firstTabs := entity.NewTabList()
	firstTabs.Add(firstTab)
	firstTabs.SetActive(firstTab.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}

	secondTab1 := entity.NewTab(entity.TabID("second-tab-1"), entity.WorkspaceID("ws-second-1"), entity.NewPane(entity.PaneID("pane-second-1")))
	secondTab2 := entity.NewTab(entity.TabID("second-tab-2"), entity.WorkspaceID("ws-second-2"), entity.NewPane(entity.PaneID("pane-second-2")))
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab1)
	secondTabs.Add(secondTab2)
	secondTabs.SetActive(secondTab1.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			NewPaneURL: "about:blank",
		},
	}
	app := &App{
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(cfg),
		tabsUC:              usecase.NewManageTabsUseCase(counterIDGen()),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{firstTab.ID: first, secondTab1.ID: second, secondTab2.ID: second},
		lastFocusedWindowID: first.id, // stale — should NOT be used
	}
	app.initTabCoordinator(ctx)

	err := app.dispatchBrowserWindowAction(ctx, second, input.ActionSwitchTabIndex2)
	if err != nil {
		t.Fatalf("dispatchBrowserWindowAction returned error: %v", err)
	}

	if got := secondTabs.ActiveTabID; got != secondTab2.ID {
		t.Fatalf("second active tab = %q, want %q", got, secondTab2.ID)
	}
	if got := firstTabs.ActiveTabID; got != firstTab.ID {
		t.Fatalf("first active tab = %q, want %q", got, firstTab.ID)
	}
}

func TestApp_DispatchBrowserWindowActionSwitchTabIndexCreatesOnlyOneMissingTab(t *testing.T) {
	ctx := context.Background()

	firstTab := entity.NewTab(entity.TabID("first-tab"), entity.WorkspaceID("ws-first"), entity.NewPane(entity.PaneID("pane-first")))
	firstTabs := entity.NewTabList()
	firstTabs.Add(firstTab)
	firstTabs.SetActive(firstTab.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}

	secondTab := entity.NewTab(entity.TabID("second-tab-1"), entity.WorkspaceID("ws-second-1"), entity.NewPane(entity.PaneID("pane-second-1")))
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab)
	secondTabs.SetActive(secondTab.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	cfg := &config.Config{
		Workspace: config.WorkspaceConfig{
			NewPaneURL: "about:blank",
		},
	}
	app := &App{
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(cfg),
		tabsUC:              usecase.NewManageTabsUseCase(counterIDGen()),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{firstTab.ID: first, secondTab.ID: second},
		lastFocusedWindowID: first.id, // stale — should NOT be used
	}
	app.initTabCoordinator(ctx)

	// Dispatch ActionSwitchTabIndex4 targeting the second window.
	err := app.dispatchBrowserWindowAction(ctx, second, input.ActionSwitchTabIndex4)
	if err != nil {
		t.Fatalf("dispatchBrowserWindowAction returned error: %v", err)
	}

	// Out-of-range switches should create a single new tab in the source window.
	if got := secondTabs.Count(); got != 2 {
		t.Fatalf("second target tab count = %d, want 2", got)
	}
	if got := secondTabs.ActiveTabID; got != secondTabs.Tabs[1].ID {
		t.Fatalf("active tab = %q, want %q", got, secondTabs.Tabs[1].ID)
	}

	if got := firstTabs.Count(); got != 1 {
		t.Fatalf("first target tab count = %d, want 1", got)
	}
	if got := firstTabs.ActiveTabID; got != firstTab.ID {
		t.Fatalf("first active tab = %q, want %q", got, firstTab.ID)
	}

	if got := app.windowForTab[secondTabs.Tabs[1].ID]; got != second {
		t.Fatalf("windowForTab[%s] = %p, want second window %p", secondTabs.Tabs[1].ID, got, second)
	}
}

func TestApp_SwitchBrowserWindowTabIndexNegativeIndexIsIgnored(t *testing.T) {
	ctx := context.Background()

	secondTab := entity.NewTab(entity.TabID("second-tab-1"), entity.WorkspaceID("ws-second-1"), entity.NewPane(entity.PaneID("pane-second-1")))
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab)
	secondTabs.SetActive(secondTab.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	cfg := &config.Config{Workspace: config.WorkspaceConfig{NewPaneURL: "about:blank"}}
	app := &App{
		deps:           &Dependencies{},
		runtimeConfig:  runtimeConfigStateForTest(cfg),
		tabsUC:         usecase.NewManageTabsUseCase(counterIDGen()),
		browserWindows: map[string]*browserWindow{second.id: second},
		windowForTab:   map[entity.TabID]*browserWindow{secondTab.ID: second},
	}
	app.initTabCoordinator(ctx)

	err := app.switchBrowserWindowTabIndex(ctx, second, -1)
	if err != nil {
		t.Fatalf("switchBrowserWindowTabIndex returned error: %v", err)
	}
	if got := secondTabs.Count(); got != 1 {
		t.Fatalf("second target tab count = %d, want 1", got)
	}
	if got := secondTabs.ActiveTabID; got != secondTab.ID {
		t.Fatalf("active tab = %q, want %q", got, secondTab.ID)
	}
}

func TestApp_DispatchBrowserWindowActionSwitchTabIndexNegativeDoesNotMutateWindowState(t *testing.T) {
	ctx := context.Background()

	second := &browserWindow{id: "window-2"}

	cfg := &config.Config{Workspace: config.WorkspaceConfig{NewPaneURL: "about:blank"}}
	app := &App{
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(cfg),
		tabsUC:              usecase.NewManageTabsUseCase(counterIDGen()),
		browserWindows:      map[string]*browserWindow{second.id: second},
		lastFocusedWindowID: "window-1",
	}
	app.initTabCoordinator(ctx)

	err := app.switchBrowserWindowTabIndex(ctx, second, -1)
	if err != nil {
		t.Fatalf("switchBrowserWindowTabIndex returned error: %v", err)
	}
	if got := app.lastFocusedWindowID; got != "window-1" {
		t.Fatalf("lastFocusedWindowID = %q, want %q", got, "window-1")
	}
	if second.tabs != nil {
		t.Fatalf("second tabs should stay nil for invalid index")
	}
}

func TestApp_DispatchBrowserWindowActionSwitchTabIndexMissingURLDoesNotCreateTab(t *testing.T) {
	ctx := context.Background()

	secondTab := entity.NewTab(entity.TabID("second-tab-1"), entity.WorkspaceID("ws-second-1"), entity.NewPane(entity.PaneID("pane-second-1")))
	secondTabs := entity.NewTabList()
	secondTabs.Add(secondTab)
	secondTabs.SetActive(secondTab.ID)
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	app := &App{
		deps:           &Dependencies{},
		runtimeConfig:  runtimeConfigStateForTest(&config.Config{}),
		tabsUC:         usecase.NewManageTabsUseCase(counterIDGen()),
		browserWindows: map[string]*browserWindow{second.id: second},
		windowForTab:   map[entity.TabID]*browserWindow{secondTab.ID: second},
	}
	app.initTabCoordinator(ctx)

	err := app.dispatchBrowserWindowAction(ctx, second, input.ActionSwitchTabIndex4)
	if err == nil {
		t.Fatal("dispatchBrowserWindowAction returned nil error, want config error")
	}
	if got := secondTabs.Count(); got != 1 {
		t.Fatalf("second target tab count = %d, want 1", got)
	}
	if got := secondTabs.ActiveTabID; got != secondTab.ID {
		t.Fatalf("active tab = %q, want %q", got, secondTab.ID)
	}
}

func TestApp_SwitchBrowserWindowTabIndexMissingURLDoesNotMutateWindowState(t *testing.T) {
	ctx := context.Background()

	second := &browserWindow{id: "window-2"}

	app := &App{
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(&config.Config{}),
		tabsUC:              usecase.NewManageTabsUseCase(counterIDGen()),
		browserWindows:      map[string]*browserWindow{second.id: second},
		lastFocusedWindowID: "window-1",
	}
	app.initTabCoordinator(ctx)

	err := app.switchBrowserWindowTabIndex(ctx, second, 3)
	if err == nil {
		t.Fatal("switchBrowserWindowTabIndex returned nil error, want config error")
	}
	if got := app.lastFocusedWindowID; got != "window-1" {
		t.Fatalf("lastFocusedWindowID = %q, want %q", got, "window-1")
	}
	if second.tabs != nil {
		t.Fatalf("second tabs should stay nil when new pane URL is missing")
	}
}

func TestApp_WorkspaceOmniboxNavigateUsesOwnerWindow(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	ctx := context.Background()
	tab1 := entity.NewTab(entity.TabID("tab-1"), entity.WorkspaceID("ws-1"), entity.NewPane(entity.PaneID("pane-1")))
	tab2 := entity.NewTab(entity.TabID("tab-2"), entity.WorkspaceID("ws-2"), entity.NewPane(entity.PaneID("pane-2")))
	firstTabs := entity.NewTabList()
	firstTabs.Add(tab1)
	firstTabs.SetActive(tab1.ID)
	secondTabs := entity.NewTabList()
	secondTabs.Add(tab2)
	secondTabs.SetActive(tab2.ID)
	first := &browserWindow{id: "window-1", tabs: firstTabs}
	second := &browserWindow{id: "window-2", tabs: secondTabs}

	recordingWv1 := &recordingWebView{id: 1}
	recordingWv2 := &recordingWebView{id: 2}
	contentCoord := contentcoord.NewCoordinator(ctx, nil, nil, nil, nil, nil, nil, nil)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-1"), recordingWv1)
	contentCoord.RegisterPopupWebView(entity.PaneID("pane-2"), recordingWv2)

	app := &App{
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigStateForTest(&config.Config{}),
		widgetFactory:       layout.NewGtkWidgetFactory(),
		browserWindows:      map[string]*browserWindow{first.id: first, second.id: second},
		windowForTab:        map[entity.TabID]*browserWindow{tab1.ID: first, tab2.ID: second},
		lastFocusedWindowID: first.id,
		contentCoord:        contentCoord,
		navCoord:            coordinator.NewNavigationCoordinator(ctx, nil, contentCoord),
		workspaceViews:      map[entity.TabID]*component.WorkspaceView{},
	}

	app.createWorkspaceViewWithoutAttach(ctx, tab2)
	wsView := app.workspaceViews[tab2.ID]
	if wsView == nil {
		t.Fatal("workspace view was not created for second tab")
	}
	wsView.ShowOmnibox(ctx, "")
	cb := omniboxNavigateCallbackForTest(t, wsView.GetOmnibox())
	if err := cb(ctx, "https://example.com"); err != nil {
		t.Fatalf("omnibox navigate callback returned error: %v", err)
	}

	if recordingWv1.loadURICalled {
		t.Errorf("first window webview navigated to %q, want no navigation", recordingWv1.loadURILastURI)
	}
	if !recordingWv2.loadURICalled || recordingWv2.loadURILastURI != "https://example.com" {
		t.Errorf("second window webview navigation = called %v uri %q, want https://example.com", recordingWv2.loadURICalled, recordingWv2.loadURILastURI)
	}
}

func TestApp_FloatingOmniboxNavigateUsesSessionPane(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	ctx := context.Background()
	factory := layout.NewGtkWidgetFactory()
	overlay := factory.NewOverlay()

	loadedURL := ""
	pane := component.NewFloatingPane(overlay, component.FloatingPaneOptions{
		OnNavigate: func(_ context.Context, url string) error {
			loadedURL = url
			return nil
		},
	})
	session := &floatingWorkspaceSession{
		paneID:  entity.PaneID("floating-pane"),
		pane:    pane,
		overlay: overlay,
	}
	app := &App{widgetFactory: factory}

	app.showFloatingOmnibox(ctx, session)
	cb := omniboxNavigateCallbackForTest(t, session.omnibox)
	if err := cb(ctx, "https://example.com"); err != nil {
		t.Fatalf("floating omnibox navigate callback returned error: %v", err)
	}

	if loadedURL != "https://example.com" {
		t.Fatalf("floating pane loaded URL = %q, want https://example.com", loadedURL)
	}
}

// TestApp_RestoreSessionDoesNotLeakStaleWindowsIntoTabMerge verifies that existing
// browserWindows not participating in the restore do not leak their tabs into the
// restored global tab list or UI aggregation. Only runtimeWindows (the windows
// actually restored) contribute.
func TestApp_RestoreSessionDoesNotLeakStaleWindowsIntoTabMerge(t *testing.T) {
	// Two stale browserWindows exist in the map; only one window is restored.
	staleBW1 := &browserWindow{id: "stale-w1"}
	staleBW2 := &browserWindow{id: "stale-w2"}

	staleTab1 := entity.NewTab(entity.TabID("stale-tab"), entity.WorkspaceID("stale-ws"), entity.NewPane(entity.PaneID("stale-pane")))
	staleTab1.Name = "StaleTab"
	staleBW1.tabs = entity.NewTabList()
	staleBW1.tabs.Add(staleTab1)

	staleBW2.tabs = entity.NewTabList()

	// One restored window with its own tab. Tab names survive the
	// snapshot/restore cycle even though IDs are regenerated.
	restoredTabs := entity.NewTabList()
	restoredTab := entity.NewTab(entity.TabID("restored-tab"), entity.WorkspaceID("restored-ws"), entity.NewPane(entity.PaneID("restored-pane")))
	restoredTab.Name = "RestoredTab"
	restoredTabs.Add(restoredTab)

	sessionID := entity.SessionID("test-session")
	state := entity.SnapshotFromWindowTabLists(sessionID, []entity.WindowTabListState{
		{WindowID: "saved-w1", Tabs: restoredTabs},
	}, 0, time.Unix(123, 0))

	mainWindow := &window.MainWindow{}
	runtimeBW := &browserWindow{id: "runtime-w1", mainWindow: mainWindow}

	app := &App{
		deps: &Dependencies{
			SessionStateRepo: mockSessionStateRepoWithSnapshot(t, sessionID, state),
		},
		runtimeConfig: runtimeConfigStateForTest(&config.Config{}),
		mainWindow:    mainWindow,
		browserWindows: map[string]*browserWindow{
			staleBW1.id:  staleBW1,
			staleBW2.id:  staleBW2,
			runtimeBW.id: runtimeBW,
		},
		tabs: entity.NewTabList(),
	}
	// Seed global tabs with a stale tab to verify it is replaced.
	app.tabs.Add(staleTab1)

	if err := app.restoreSession(context.Background(), sessionID); err != nil {
		t.Fatalf("restoreSession returned error: %v", err)
	}

	// Global tabs must only contain the restored tab, not stale tabs.
	if got := app.tabs.Count(); got != 1 {
		t.Fatalf("tabs.Count() = %d, want 1 (only restored tab)", got)
	}
	// Verify by name since IDs are regenerated during restore.
	restored := app.tabs.Tabs[0]
	if restored == nil || restored.Name != "RestoredTab" {
		t.Fatalf("restored tab name = %q, want RestoredTab", firstTabName(app.tabs))
	}
	if app.tabs.Find(staleTab1.ID) != nil {
		t.Fatal("stale tab leaked into global tabs")
	}
}

func firstTabName(tl *entity.TabList) string {
	if len(tl.Tabs) == 0 || tl.Tabs[0] == nil {
		return ""
	}
	return tl.Tabs[0].Name
}

// TestApp_RestoreSessionHonorsActiveWindowIndex verifies that when restoring a
// v2 multi-window session, the ActiveWindowIndex from the snapshot is used to
// determine the focused window and global active tab.
func TestApp_RestoreSessionHonorsActiveWindowIndex(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	// Window 0 has a tab named "Tab-W0", window 1 has "Tab-W1" (active).
	// Tab names survive snapshot/restore; IDs are regenerated.
	pane0 := entity.NewPane("pane-w0")
	tabW0 := entity.NewTab("tab-w0", "ws-w0", pane0)
	tabW0.Name = "Tab-W0"
	tabs0 := entity.NewTabList()
	tabs0.Add(tabW0)

	pane1 := entity.NewPane("pane-w1")
	tabW1 := entity.NewTab("tab-w1", "ws-w1", pane1)
	tabW1.Name = "Tab-W1"
	tabs1 := entity.NewTabList()
	tabs1.Add(tabW1)
	tabs1.SetActive("tab-w1")

	sessionID := entity.SessionID("test-sess")
	// ActiveWindowIndex = 1: second window should be focused.
	state := entity.SnapshotFromWindowTabLists(sessionID, []entity.WindowTabListState{
		{WindowID: "saved-w0", Tabs: tabs0},
		{WindowID: "saved-w1", Tabs: tabs1},
	}, 1, time.Unix(123, 0))

	firstBW := &browserWindow{id: "runtime-w0", mainWindow: mainWindow}
	// Factory creates the second runtime window; share the mainWindow so the UI
	// restore loop can walk its tab bar without needing a second GTK window.
	var runtimeW1 *browserWindow
	app := &App{
		deps: &Dependencies{
			SessionStateRepo: mockSessionStateRepoWithSnapshot(t, sessionID, state),
		},
		runtimeConfig:  runtimeConfigStateForTest(&config.Config{}),
		mainWindow:     mainWindow,
		widgetFactory:  layout.NewGtkWidgetFactory(),
		browserWindows: map[string]*browserWindow{firstBW.id: firstBW},
		tabs:           entity.NewTabList(),
		windowForTab:   map[entity.TabID]*browserWindow{},
		workspaceViews: map[entity.TabID]*component.WorkspaceView{},
		browserWindowFactory: func(ctx context.Context, url string) (*browserWindow, error) {
			runtimeW1 = &browserWindow{id: "runtime-w1", mainWindow: mainWindow}
			return runtimeW1, nil
		},
	}

	if err := app.restoreSession(context.Background(), sessionID); err != nil {
		t.Fatalf("restoreSession returned error: %v", err)
	}

	// lastFocusedWindowID must point to the active window (index 1).
	if app.lastFocusedWindowID != "runtime-w1" {
		t.Errorf("lastFocusedWindowID = %s, want runtime-w1 (active window index 1)", app.lastFocusedWindowID)
	}

	// Global active tab must come from the active window (Tab-W1), not the first (Tab-W0).
	activeTab := app.tabs.ActiveTab()
	if activeTab == nil {
		t.Fatal("active tab is nil")
	}
	if activeTab.Name != "Tab-W1" {
		t.Errorf("active tab name = %q, want Tab-W1 (from active window)", activeTab.Name)
	}
}

// TestApp_RestoreSessionFailsOnAdditionalWindowCreationError verifies that
// restoreSession returns an error (not silent partial restore) when creating
// a browser window for an additional restored window fails.
func TestApp_RestoreSessionFailsOnAdditionalWindowCreationError(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mainWindow, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mainWindow.Destroy()

	pane0 := entity.NewPane("pane-w0")
	tab0 := entity.NewTab("tab-w0", "ws-w0", pane0)
	tabs0 := entity.NewTabList()
	tabs0.Add(tab0)

	pane1 := entity.NewPane("pane-w1")
	tab1 := entity.NewTab("tab-w1", "ws-w1", pane1)
	tabs1 := entity.NewTabList()
	tabs1.Add(tab1)

	sessionID := entity.SessionID("test-sess")
	state := entity.SnapshotFromWindowTabLists(sessionID, []entity.WindowTabListState{
		{WindowID: "saved-w0", Tabs: tabs0},
		{WindowID: "saved-w1", Tabs: tabs1},
	}, 0, time.Unix(123, 0))

	firstBW := &browserWindow{id: "runtime-w0", mainWindow: mainWindow}
	factoryErr := errors.New("window creation failed")
	app := &App{
		deps: &Dependencies{
			SessionStateRepo: mockSessionStateRepoWithSnapshot(t, sessionID, state),
		},
		runtimeConfig:  runtimeConfigStateForTest(&config.Config{}),
		mainWindow:     mainWindow,
		browserWindows: map[string]*browserWindow{firstBW.id: firstBW},
		tabs:           entity.NewTabList(),
		windowForTab:   map[entity.TabID]*browserWindow{},
		browserWindowFactory: func(ctx context.Context, url string) (*browserWindow, error) {
			return nil, factoryErr
		},
	}

	gotErr := app.restoreSession(context.Background(), sessionID)
	if gotErr == nil {
		t.Fatal("restoreSession returned nil error, want failure when additional window creation fails")
	}
	if !strings.Contains(gotErr.Error(), "create browser window 1 for restore") {
		t.Errorf("error message should mention window creation for restore, got: %v", gotErr)
	}
	if !errors.Is(gotErr, factoryErr) {
		t.Errorf("error should wrap factoryErr, got: %v", gotErr)
	}
}

func TestMainWindow_BottomTabBar_ContentAreaHasInset(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mw, err := window.New(context.Background(), gtkApp, "bottom")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mw.Destroy()

	// Bottom window should start WITHOUT the inset
	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-bottom") {
		t.Fatal("bottom-tab-bar content area: should NOT have inset at window creation")
	}

	// HasTabBarContentInset must return false initially
	if mw.HasTabBarContentInset() {
		t.Fatal("HasTabBarContentInset should return false initially")
	}

	// SetTabBarContentInsetVisible(true) must add the class
	mw.SetTabBarContentInsetVisible(true)
	if !mw.ContentArea().HasCssClass("content-area-tabbar-inset-bottom") {
		t.Fatal("bottom-tab-bar content area: expected content-area-tabbar-inset-bottom CSS class after SetTabBarContentInsetVisible(true)")
	}
	if !mw.HasTabBarContentInset() {
		t.Fatal("HasTabBarContentInset should return true after SetTabBarContentInsetVisible(true)")
	}

	// SetTabBarContentInsetVisible(false) must remove the class
	mw.SetTabBarContentInsetVisible(false)
	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-bottom") {
		t.Fatal("bottom-tab-bar content area: should NOT have inset after SetTabBarContentInsetVisible(false)")
	}
	if mw.HasTabBarContentInset() {
		t.Fatal("HasTabBarContentInset should return false after SetTabBarContentInsetVisible(false)")
	}

	// Verify tab bar remains a non-measured overlay regardless of inset state
	if mw.ContentOverlay().GetMeasureOverlay(mw.TabBar().Widget()) {
		t.Fatal("bottom-tab-bar: expected tab bar to remain a non-measured overlay")
	}
}

func TestMainWindow_TopTabBar_ContentAreaHasTopInset(t *testing.T) {
	gtkApp := requireGTKDisplayApp(t)
	defer gtkApp.Unref()

	mw, err := window.New(context.Background(), gtkApp, "top")
	if err != nil {
		t.Fatalf("window creation failed: %v", err)
	}
	defer mw.Destroy()

	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-top") {
		t.Fatal("top-tab-bar content area: should NOT have inset at window creation")
	}
	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-bottom") {
		t.Fatal("top-tab-bar content area: should NOT have content-area-tabbar-inset-bottom CSS class")
	}

	mw.SetTabBarContentInsetVisible(true)
	if !mw.ContentArea().HasCssClass("content-area-tabbar-inset-top") {
		t.Fatal("top-tab-bar content area: expected content-area-tabbar-inset-top CSS class after SetTabBarContentInsetVisible(true)")
	}
	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-bottom") {
		t.Fatal("top-tab-bar content area: should NOT have bottom inset after SetTabBarContentInsetVisible(true)")
	}
	if !mw.HasTabBarContentInset() {
		t.Fatal("HasTabBarContentInset should return true for top inset")
	}

	mw.SetTabBarContentInsetVisible(false)
	if mw.ContentArea().HasCssClass("content-area-tabbar-inset-top") {
		t.Fatal("top-tab-bar content area: should NOT have inset after SetTabBarContentInsetVisible(false)")
	}
	if mw.HasTabBarContentInset() {
		t.Fatal("HasTabBarContentInset should return false after removing top inset")
	}

	if mw.ContentOverlay().GetMeasureOverlay(mw.TabBar().Widget()) {
		t.Fatal("top-tab-bar: expected tab bar to remain a non-measured overlay")
	}
}
