package systemviewsbridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type browserRequest struct {
	messageType string
	requestID   string
	body        []byte
	directAPI   bool
}

func decodeBrowserRequest(body []byte) (browserRequest, error) {
	var envelope struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return browserRequest{}, fmt.Errorf("unmarshal browser request: %w", err)
	}
	if envelope.Type == "" {
		return browserRequest{}, fmt.Errorf("browser request missing type")
	}

	var payload struct {
		RequestID string `json:"requestId"`
	}
	if len(envelope.Payload) != 0 {
		if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
			return browserRequest{}, fmt.Errorf("unmarshal request payload: %w", err)
		}
	}

	return browserRequest{
		messageType: envelope.Type,
		requestID:   payload.RequestID,
		body:        body,
		directAPI:   strings.HasPrefix(envelope.Type, "/api/"),
	}, nil
}

func wrapDirectAPIResponse(requestID string, payload []byte) ([]byte, error) {
	return json.Marshal(bridgeResponse{
		RequestID: requestID,
		Success:   true,
		Data:      json.RawMessage(payload),
	})
}

// normalizeBridgeShimResponse accepts the bridge shapes emitted by native and
// fallback shims: a direct bridgeResponse envelope, a wrapper with data/error,
// or a wrapper whose data field itself contains a bridgeResponse envelope.
func normalizeBridgeShimResponse(requestID string, body []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty bridge shim response")
	}

	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &topLevel); err != nil {
		return nil, fmt.Errorf("unmarshal top-level bridge response: %w", err)
	}

	_, hasSuccess := topLevel["success"]
	_, hasRequestID := topLevel["requestId"]
	_, hasCallback := topLevel["_callback"]
	_, hasData := topLevel["data"]

	// Native callbacks can already return the final bridgeResponse shape.
	if (hasSuccess || hasRequestID) && !hasCallback {
		var resp bridgeResponse
		if err := json.Unmarshal(trimmed, &resp); err != nil {
			return nil, fmt.Errorf("unmarshal direct bridge response: %w", err)
		}
		if resp.RequestID == "" {
			// Some direct shims omit requestId; preserve correlation for callers.
			resp.RequestID = requestID
		}
		return json.Marshal(resp)
	}

	var wrapped struct {
		Data  json.RawMessage `json:"data,omitempty"`
		Error string          `json:"error,omitempty"`
	}
	if err := json.Unmarshal(trimmed, &wrapped); err != nil {
		return nil, fmt.Errorf("unmarshal wrapped bridge response: %w", err)
	}
	if wrapped.Error != "" {
		// Fetch-style wrappers report transport errors outside the data payload.
		return json.Marshal(bridgeResponse{RequestID: requestID, Error: wrapped.Error})
	}
	if !hasData {
		// Wrapped shim responses must carry either data or a top-level error.
		return nil, fmt.Errorf("wrapped bridge response missing data")
	}
	if len(wrapped.Data) == 0 {
		return json.Marshal(bridgeResponse{RequestID: requestID, Success: true})
	}

	var nestedFields map[string]json.RawMessage
	if err := json.Unmarshal(wrapped.Data, &nestedFields); err == nil {
		nestedSuccess, nestedHasSuccess := nestedFields["success"]
		_, nestedHasRequestID := nestedFields["requestId"]
		_, nestedHasError := nestedFields["error"]
		successIsBool := false
		if nestedHasSuccess {
			var successValue bool
			successIsBool = json.Unmarshal(nestedSuccess, &successValue) == nil
		}
		hasNestedEnvelope := successIsBool || (nestedHasRequestID && nestedHasError)
		if hasNestedEnvelope {
			var nested bridgeResponse
			if err := json.Unmarshal(wrapped.Data, &nested); err == nil {
				if nested.RequestID == "" {
					// Nested envelopes from direct API wrappers also need caller correlation.
					nested.RequestID = requestID
				}
				return json.Marshal(nested)
			}
		}
	}

	return json.Marshal(bridgeResponse{
		RequestID: requestID,
		Success:   true,
		Data:      wrapped.Data,
	})
}
