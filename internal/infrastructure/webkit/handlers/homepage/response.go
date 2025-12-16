package homepage

import (
	"encoding/json"
)

// Response represents the standard response format sent back to the frontend.
// The frontend expects: { requestId, success, data?, error? }
type Response struct {
	RequestID string `json:"requestId"`
	Success   bool   `json:"success"`
	Data      any    `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
}

// NewSuccessResponse creates a success response with data.
func NewSuccessResponse(requestID string, data any) Response {
	return Response{
		RequestID: requestID,
		Success:   true,
		Data:      data,
	}
}

// NewErrorResponse creates an error response.
func NewErrorResponse(requestID string, err error) Response {
	errMsg := "unknown error"
	if err != nil {
		errMsg = err.Error()
	}
	return Response{
		RequestID: requestID,
		Success:   false,
		Error:     errMsg,
	}
}

// baseRequest is used to extract the requestId from any payload.
type baseRequest struct {
	RequestID string `json:"requestId"`
}

// ParseRequestID extracts the requestId from a raw JSON payload.
func ParseRequestID(payload json.RawMessage) string {
	var base baseRequest
	if err := json.Unmarshal(payload, &base); err != nil {
		return ""
	}
	return base.RequestID
}

// ParsePayload extracts the requestId and unmarshals the payload into the target type.
// Returns the requestId, the parsed payload, and any error.
func ParsePayload[T any](payload json.RawMessage) (string, T, error) {
	var base baseRequest
	var target T

	if err := json.Unmarshal(payload, &base); err != nil {
		return "", target, err
	}
	if err := json.Unmarshal(payload, &target); err != nil {
		return base.RequestID, target, err
	}
	return base.RequestID, target, nil
}
