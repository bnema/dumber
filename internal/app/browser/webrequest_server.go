package browser

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/bnema/dumber/internal/webext"
	"github.com/bnema/dumber/internal/webext/api"
)

// WebRequestServer handles UNIX socket IPC for webRequest blocking decisions.
// This replaces GLib message-based IPC to avoid main loop re-entrancy when
// blocking in send-request signal handlers.
type WebRequestServer struct {
	listener   net.Listener
	manager    *webext.Manager
	socketPath string
	mu         sync.RWMutex
	closed     bool
}

// webRequestIPCRequest is the JSON structure sent from WebProcess
type webRequestIPCRequest struct {
	Details api.RequestDetails `json:"details"`
}

// webRequestIPCResponse is the JSON structure sent back to WebProcess
type webRequestIPCResponse struct {
	Cancel         bool              `json:"cancel"`
	RedirectURL    string            `json:"redirectUrl,omitempty"`
	RequestHeaders map[string]string `json:"requestHeaders,omitempty"`
}

// NewWebRequestServer creates a new WebRequest IPC server.
func NewWebRequestServer(manager *webext.Manager) *WebRequestServer {
	return &WebRequestServer{
		manager: manager,
	}
}

// Start initializes and starts the UNIX socket server.
func (s *WebRequestServer) Start(socketPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.socketPath = socketPath

	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	// Remove stale socket file if it exists
	os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to create UNIX socket: %w", err)
	}
	s.listener = ln

	// Set socket permissions (user only for security)
	if err := os.Chmod(socketPath, 0600); err != nil {
		ln.Close()
		os.Remove(socketPath)
		return fmt.Errorf("failed to set socket permissions: %w", err)
	}

	// Start accepting connections in background
	go s.acceptLoop()

	log.Printf("[webRequest] Socket server started at: %s", socketPath)
	return nil
}

// SocketPath returns the path to the UNIX socket.
func (s *WebRequestServer) SocketPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.socketPath
}

// Close stops the server and cleans up resources.
func (s *WebRequestServer) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}
	s.closed = true

	if s.listener != nil {
		s.listener.Close()
	}
	if s.socketPath != "" {
		os.Remove(s.socketPath)
	}

	log.Printf("[webRequest] Socket server stopped")
}

// acceptLoop handles incoming connections from WebProcess extensions.
func (s *WebRequestServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.RLock()
			closed := s.closed
			s.mu.RUnlock()
			if closed {
				return // Normal shutdown
			}
			log.Printf("[webRequest] Accept error: %v", err)
			continue
		}

		// Handle each connection in its own goroutine
		go s.handleConnection(conn)
	}
}

// handleConnection processes requests from a single WebProcess connection.
// Each WebProcess extension connects once and sends multiple requests.
func (s *WebRequestServer) handleConnection(conn net.Conn) {
	defer conn.Close()

	log.Printf("[webRequest] New WebProcess connection from %v", conn.RemoteAddr())

	reader := bufio.NewReader(conn)
	encoder := json.NewEncoder(conn)

	for {
		// Read one line (one JSON request)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			// Connection closed or error - this is normal when WebProcess exits
			return
		}

		// Parse request
		var req webRequestIPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			log.Printf("[webRequest] Failed to parse request: %v", err)
			// Send empty response to unblock WebProcess
			encoder.Encode(webRequestIPCResponse{})
			continue
		}

		// Process the webRequest decision
		resp := s.processRequest(req.Details)

		// Send response back
		if err := encoder.Encode(resp); err != nil {
			log.Printf("[webRequest] Failed to send response: %v", err)
			return
		}
	}
}

// processRequest evaluates webRequest blocking rules for a request.
func (s *WebRequestServer) processRequest(details api.RequestDetails) webRequestIPCResponse {
	s.mu.RLock()
	manager := s.manager
	s.mu.RUnlock()

	if manager == nil {
		log.Printf("[webRequest] processRequest: manager is nil")
		return webRequestIPCResponse{}
	}

	// Get extensions with webRequest capability
	enabledExts := manager.GetEnabledExtensionsWithWebRequest()
	if len(enabledExts) == 0 {
		log.Printf("[webRequest] processRequest: no extensions with webRequest")
		return webRequestIPCResponse{}
	}

	resp := webRequestIPCResponse{}

	// Dispatch to each extension's webRequest listeners
	log.Printf("[webRequest] Dispatching to %d extensions: %v", len(enabledExts), enabledExts)
	for _, extID := range enabledExts {
		log.Printf("[webRequest] Calling DispatchWebRequestEvent for %s", extID)
		bgResp, err := manager.DispatchWebRequestEvent(extID, "onBeforeRequest", details)
		if err != nil {
			log.Printf("[webRequest] onBeforeRequest error for %s: %v", extID, err)
			continue
		}

		if bgResp == nil {
			log.Printf("[webRequest] onBeforeRequest returned nil for %s", extID)
		} else {
			log.Printf("[webRequest] onBeforeRequest returned cancel=%v for %s", bgResp.Cancel, extID)
		}

		if bgResp != nil {
			// Merge decisions (cancel is OR'd, first redirect wins)
			resp.Cancel = resp.Cancel || bgResp.Cancel
			if resp.RedirectURL == "" && bgResp.RedirectURL != "" {
				resp.RedirectURL = bgResp.RedirectURL
			}
			if resp.RequestHeaders == nil && bgResp.RequestHeaders != nil {
				resp.RequestHeaders = bgResp.RequestHeaders
			}

			// If cancelled, no need to check other extensions
			if resp.Cancel {
				break
			}
		}
	}

	return resp
}
