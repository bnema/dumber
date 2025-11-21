package webext

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/bnema/dumber/internal/webext/api"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// APIRequest represents an API call from the web process
type APIRequest struct {
	ExtensionID string          `json:"extensionId"`
	Function    string          `json:"function"`
	Args        json.RawMessage `json:"args"`
}

// APIResponse represents the response to an API call
type APIResponse struct {
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

// Dispatcher routes WebExtension API calls to appropriate handlers
type Dispatcher struct {
	manager       *Manager
	storageAPI    *api.StorageAPIDispatcher
	runtimeAPI    *api.RuntimeAPIDispatcher
	tabsAPI       *api.TabsAPIDispatcher
	webRequestAPI *api.WebRequestAPI
}

// NewDispatcher creates a new API dispatcher
func NewDispatcher(manager *Manager) *Dispatcher {
	return &Dispatcher{
		manager:       manager,
		storageAPI:    api.NewStorageAPIDispatcher(manager.dataDir),
		runtimeAPI:    api.NewRuntimeAPIDispatcher(manager),
		tabsAPI:       api.NewTabsAPIDispatcher(manager), // Pass manager as PaneInfoProvider
		webRequestAPI: manager.webRequest,
	}
}

// HandleUserMessage handles UserMessage from web process extensions
// This is the main entry point for all webext:api calls
// Returns true to indicate the message was handled
func (d *Dispatcher) HandleUserMessage(message *webkit.UserMessage) bool {
	msgName := message.Name()

	// Only handle webext:api messages
	if !strings.HasPrefix(msgName, "webext:api") {
		return false
	}

	log.Printf("[dispatcher] Received message: %s", msgName)

	// Extract message parameters
	params := message.Parameters()
	if params == nil {
		d.sendErrorReply(message, "missing message parameters")
		return true
	}

	// Parse the JSON payload
	jsonStr := params.String()
	// Remove quotes if present (GVariant string format)
	if len(jsonStr) >= 2 && jsonStr[0] == '"' && jsonStr[len(jsonStr)-1] == '"' {
		jsonStr = jsonStr[1 : len(jsonStr)-1]
	}

	var req APIRequest
	if err := json.Unmarshal([]byte(jsonStr), &req); err != nil {
		log.Printf("[dispatcher] Failed to parse API request: %v", err)
		d.sendErrorReply(message, fmt.Sprintf("invalid request format: %v", err))
		return true
	}

	log.Printf("[dispatcher] API call: %s.%s from extension %s",
		strings.Split(req.Function, ".")[0], req.Function, req.ExtensionID)

	// Dispatch to appropriate handler
	ctx := context.Background()
	result, err := d.dispatch(ctx, &req)
	if err != nil {
		d.sendErrorReply(message, err.Error())
		return true
	}

	// Send success response
	d.sendSuccessReply(message, result)
	return true
}

// dispatch routes the API call to the appropriate handler
func (d *Dispatcher) dispatch(ctx context.Context, req *APIRequest) (interface{}, error) {
	// Split function name into namespace and method (e.g., "storage.local.get" -> ["storage", "local.get"])
	parts := strings.SplitN(req.Function, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid function name format: %s", req.Function)
	}

	namespace := parts[0]
	method := parts[1]

	switch namespace {
	case "storage":
		return d.handleStorageAPI(ctx, req.ExtensionID, method, req.Args)

	case "runtime":
		return d.handleRuntimeAPI(ctx, req.ExtensionID, method, req.Args)

	case "tabs":
		return d.handleTabsAPI(ctx, req.ExtensionID, method, req.Args)

	default:
		return nil, fmt.Errorf("unsupported API namespace: %s", namespace)
	}
}

// handleStorageAPI handles storage.* API calls
func (d *Dispatcher) handleStorageAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	// Parse storage area and method (e.g., "local.get" -> area="local", method="get")
	parts := strings.SplitN(method, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid storage method format: %s", method)
	}

	area := parts[0]      // "local", "sync", "managed"
	operation := parts[1] // "get", "set", "remove", "clear"

	// Currently only support "local" area
	if area != "local" {
		return nil, fmt.Errorf("unsupported storage area: %s (only 'local' is supported)", area)
	}

	switch operation {
	case "get":
		var keys interface{}
		if len(args) > 0 && string(args) != "null" {
			if err := json.Unmarshal(args, &keys); err != nil {
				return nil, fmt.Errorf("invalid arguments for storage.local.get: %w", err)
			}
		}
		return d.storageAPI.Get(ctx, extID, keys)

	case "set":
		var items map[string]interface{}
		if err := json.Unmarshal(args, &items); err != nil {
			return nil, fmt.Errorf("invalid arguments for storage.local.set: %w", err)
		}
		return nil, d.storageAPI.Set(ctx, extID, items)

	case "remove":
		var keys interface{}
		if err := json.Unmarshal(args, &keys); err != nil {
			return nil, fmt.Errorf("invalid arguments for storage.local.remove: %w", err)
		}
		return nil, d.storageAPI.Remove(ctx, extID, keys)

	case "clear":
		return nil, d.storageAPI.Clear(ctx, extID)

	default:
		return nil, fmt.Errorf("unsupported storage operation: %s", operation)
	}
}

// handleRuntimeAPI handles runtime.* API calls
func (d *Dispatcher) handleRuntimeAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	switch method {
	case "sendMessage":
		var message interface{}
		if err := json.Unmarshal(args, &message); err != nil {
			return nil, fmt.Errorf("invalid arguments for runtime.sendMessage: %w", err)
		}
		return d.runtimeAPI.SendMessage(ctx, extID, message)

	case "connect":
		var connectInfo api.ConnectInfo
		if len(args) > 0 && string(args) != "null" {
			if err := json.Unmarshal(args, &connectInfo); err != nil {
				return nil, fmt.Errorf("invalid arguments for runtime.connect: %w", err)
			}
		}
		return d.runtimeAPI.Connect(ctx, extID, &connectInfo)

	case "port.postMessage":
		var portPayload struct {
			PortID  string      `json:"portId"`
			Message interface{} `json:"message"`
		}
		if err := json.Unmarshal(args, &portPayload); err != nil {
			return nil, fmt.Errorf("invalid arguments for runtime.port.postMessage: %w", err)
		}
		return d.runtimeAPI.PortPostMessage(ctx, extID, portPayload.PortID, portPayload.Message)

	case "port.disconnect":
		var portPayload struct {
			PortID string `json:"portId"`
		}
		if err := json.Unmarshal(args, &portPayload); err != nil {
			return nil, fmt.Errorf("invalid arguments for runtime.port.disconnect: %w", err)
		}
		return d.runtimeAPI.PortDisconnect(ctx, extID, portPayload.PortID)

	default:
		return nil, fmt.Errorf("unsupported runtime method: %s", method)
	}
}

// handleTabsAPI handles tabs.* API calls
func (d *Dispatcher) handleTabsAPI(ctx context.Context, extID, method string, args json.RawMessage) (interface{}, error) {
	switch method {
	case "query":
		var queryInfo map[string]interface{}
		if len(args) > 0 && string(args) != "null" {
			if err := json.Unmarshal(args, &queryInfo); err != nil {
				return nil, fmt.Errorf("invalid arguments for tabs.query: %w", err)
			}
		}
		return d.tabsAPI.Query(ctx, queryInfo)

	case "get":
		var tabID int64
		if err := json.Unmarshal(args, &tabID); err != nil {
			return nil, fmt.Errorf("invalid arguments for tabs.get: %w", err)
		}
		return d.tabsAPI.Get(ctx, tabID)

	default:
		return nil, fmt.Errorf("unsupported tabs method: %s", method)
	}
}

// sendSuccessReply sends a successful response to the web process
func (d *Dispatcher) sendSuccessReply(message *webkit.UserMessage, data interface{}) {
	response := APIResponse{Data: data}
	jsonData, err := json.Marshal(response)
	if err != nil {
		log.Printf("[dispatcher] Failed to marshal response: %v", err)
		d.sendErrorReply(message, fmt.Sprintf("failed to marshal response: %v", err))
		return
	}

	variant := glib.NewVariantString(string(jsonData))
	reply := webkit.NewUserMessage("", variant)
	message.SendReply(reply)
}

// sendErrorReply sends an error response to the web process
func (d *Dispatcher) sendErrorReply(message *webkit.UserMessage, errorMsg string) {
	response := APIResponse{Error: errorMsg}
	jsonData, err := json.Marshal(response)
	if err != nil {
		log.Printf("[dispatcher] Failed to marshal error response: %v", err)
		// Fall back to simple error message
		variant := glib.NewVariantString(fmt.Sprintf(`{"error":"%s"}`, errorMsg))
		reply := webkit.NewUserMessage("error", variant)
		message.SendReply(reply)
		return
	}

	variant := glib.NewVariantString(string(jsonData))
	reply := webkit.NewUserMessage("", variant)
	message.SendReply(reply)
}
