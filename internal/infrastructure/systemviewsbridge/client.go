package systemviewsbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
)

// Transport sends a WebUI message envelope and returns the raw response body.
type Transport interface {
	Available() bool
	Send(ctx context.Context, body []byte) ([]byte, error)
}

// Client sends WebUI message envelopes through the first available transport.
type Client struct {
	native Transport
	fetch  Transport
}

// NewClient creates a bridge client with native WebKit and fetch transports.
func NewClient(native, fetch Transport) *Client {
	return &Client{native: native, fetch: fetch}
}

// Send builds a WebUI message envelope and sends it through the active transport.
func (c *Client) Send(ctx context.Context, msgType string, payload any) ([]byte, error) {
	if c == nil {
		return nil, errors.New("bridge client is nil")
	}

	envelope, err := buildMessageEnvelope(msgType, payload)
	if err != nil {
		return nil, err
	}

	transport := c.transport()
	if transport == nil {
		return nil, errors.New("no bridge transport available")
	}

	return transport.Send(ctx, envelope)
}

func (c *Client) transport() Transport {
	if c == nil {
		return nil
	}
	if c.native != nil && c.native.Available() {
		return c.native
	}
	if c.fetch != nil {
		return c.fetch
	}
	return nil
}

func buildMessageEnvelope(msgType string, payload any) ([]byte, error) {
	if msgType == "" {
		return nil, errors.New("message type cannot be empty")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	envelope := port.WebUIMessage{
		Type:    msgType,
		Payload: body,
	}

	data, err := json.Marshal(envelope)
	if err != nil {
		return nil, fmt.Errorf("marshal envelope: %w", err)
	}

	return data, nil
}
