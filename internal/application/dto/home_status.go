package dto

// Home service status states.
const (
	// HomeServiceStateUp means the service answered below 500, or its TLS
	// certificate could not be verified.
	HomeServiceStateUp = "up"
	// HomeServiceStateError means the service answered with a 5xx status.
	HomeServiceStateError = "error"
	// HomeServiceStateDown means the probe could not reach the service.
	HomeServiceStateDown = "down"
)

// HomeStatusRequest is the payload of the home_status message.
type HomeStatusRequest struct {
	ServiceID string `json:"service_id"`
}

// HomeStatusResponse is returned by home_status.
type HomeStatusResponse struct {
	ServiceID     string `json:"service_id"`
	State         string `json:"state"`
	LatencyMS     int    `json:"latency_ms"`
	Code          int    `json:"code,omitempty"`
	TLSUnverified bool   `json:"tls_unverified,omitempty"`
}
