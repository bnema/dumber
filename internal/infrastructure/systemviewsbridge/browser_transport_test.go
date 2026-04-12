package systemviewsbridge

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDecodeBrowserRequest_DirectConfigEndpoint(t *testing.T) {
	t.Parallel()

	body, err := buildMessageEnvelope("/api/config", struct {
		RequestID string `json:"requestId"`
	}{RequestID: "req-1"})
	if err != nil {
		t.Fatalf("buildMessageEnvelope() error = %v", err)
	}

	req, err := decodeBrowserRequest(body)
	if err != nil {
		t.Fatalf("decodeBrowserRequest() error = %v", err)
	}
	if req.messageType != "/api/config" {
		t.Fatalf("messageType = %q, want %q", req.messageType, "/api/config")
	}
	if req.requestID != "req-1" {
		t.Fatalf("requestID = %q, want %q", req.requestID, "req-1")
	}
	if !req.directAPI {
		t.Fatal("directAPI = false, want true")
	}
}

func TestDecodeBrowserRequest_MessageEnvelopeUsesBridgeShim(t *testing.T) {
	t.Parallel()

	body, err := buildMessageEnvelope("history_timeline", struct {
		RequestID string `json:"requestId"`
		Limit     int    `json:"limit"`
	}{RequestID: "req-2", Limit: 25})
	if err != nil {
		t.Fatalf("buildMessageEnvelope() error = %v", err)
	}

	req, err := decodeBrowserRequest(body)
	if err != nil {
		t.Fatalf("decodeBrowserRequest() error = %v", err)
	}
	if req.messageType != "history_timeline" {
		t.Fatalf("messageType = %q, want %q", req.messageType, "history_timeline")
	}
	if req.requestID != "req-2" {
		t.Fatalf("requestID = %q, want %q", req.requestID, "req-2")
	}
	if req.directAPI {
		t.Fatal("directAPI = true, want false")
	}
	if !bytes.Equal(req.body, body) {
		t.Fatalf("body = %s, want %s", req.body, body)
	}
}

func TestNormalizeBridgeShimResponse_UnwrapsNestedBridgeEnvelope(t *testing.T) {
	t.Parallel()

	got, err := normalizeBridgeShimResponse("req-3", []byte(`{"data":{"requestId":"req-3","success":true,"data":[{"title":"Example"}]},"_callback":"__dumber_homepage_response"}`))
	if err != nil {
		t.Fatalf("normalizeBridgeShimResponse() error = %v", err)
	}

	var resp bridgeResponse
	if err := json.Unmarshal(got, &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Success {
		t.Fatal("success = false, want true")
	}
	if resp.RequestID != "req-3" {
		t.Fatalf("requestId = %q, want %q", resp.RequestID, "req-3")
	}
	if string(resp.Data) != `[{"title":"Example"}]` {
		t.Fatalf("data = %s, want %s", resp.Data, `[{"title":"Example"}]`)
	}
}

func TestNormalizeBridgeShimResponse_WrapsRawCallbackPayload(t *testing.T) {
	t.Parallel()

	got, err := normalizeBridgeShimResponse("req-4", []byte(`{"data":{"status":"success"},"_callback":"__dumber_config_saved"}`))
	if err != nil {
		t.Fatalf("normalizeBridgeShimResponse() error = %v", err)
	}

	var resp bridgeResponse
	if err := json.Unmarshal(got, &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Success {
		t.Fatal("success = false, want true")
	}
	if resp.RequestID != "req-4" {
		t.Fatalf("requestId = %q, want %q", resp.RequestID, "req-4")
	}
	if string(resp.Data) != `{"status":"success"}` {
		t.Fatalf("data = %s, want %s", resp.Data, `{"status":"success"}`)
	}
}

func TestWrapDirectAPIResponse_PreservesPayload(t *testing.T) {
	t.Parallel()

	got, err := wrapDirectAPIResponse("req-5", []byte(`{"engine_type":"webkit"}`))
	if err != nil {
		t.Fatalf("wrapDirectAPIResponse() error = %v", err)
	}

	var resp bridgeResponse
	if err := json.Unmarshal(got, &resp); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !resp.Success {
		t.Fatal("success = false, want true")
	}
	if resp.RequestID != "req-5" {
		t.Fatalf("requestId = %q, want %q", resp.RequestID, "req-5")
	}
	if string(resp.Data) != `{"engine_type":"webkit"}` {
		t.Fatalf("data = %s, want %s", resp.Data, `{"engine_type":"webkit"}`)
	}
}
