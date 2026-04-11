package systemviewsbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
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

var requestSeq atomic.Uint64

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

func (c *Client) Timeline(ctx context.Context, limit, offset int) ([]*entity.HistoryEntry, error) {
	return request[[]*entity.HistoryEntry](c, ctx, "history_timeline", struct {
		RequestID string `json:"requestId"`
		Limit     int    `json:"limit"`
		Offset    int    `json:"offset"`
	}{RequestID: nextRequestID(), Limit: limit, Offset: offset})
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]*entity.HistoryEntry, error) {
	return request[[]*entity.HistoryEntry](c, ctx, "history_search_fts", struct {
		RequestID string `json:"requestId"`
		Query     string `json:"query"`
		Limit     int    `json:"limit"`
	}{RequestID: nextRequestID(), Query: query, Limit: limit})
}

func (c *Client) DeleteEntry(ctx context.Context, id int64) error {
	_, err := request[struct{}](c, ctx, "history_delete_entry", struct {
		RequestID string `json:"requestId"`
		ID        int64  `json:"id"`
	}{RequestID: nextRequestID(), ID: id})
	return err
}

func (c *Client) DeleteRange(ctx context.Context, rangeID string) error {
	_, err := request[struct{}](c, ctx, "history_delete_range", struct {
		RequestID string `json:"requestId"`
		Range     string `json:"range"`
	}{RequestID: nextRequestID(), Range: rangeID})
	return err
}

func (c *Client) Analytics(ctx context.Context) (*entity.HistoryAnalytics, error) {
	return request[*entity.HistoryAnalytics](c, ctx, "history_analytics", struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

func (c *Client) DomainStats(ctx context.Context, limit int) ([]*entity.DomainStat, error) {
	return request[[]*entity.DomainStat](c, ctx, "history_domain_stats", struct {
		RequestID string `json:"requestId"`
		Limit     int    `json:"limit"`
	}{RequestID: nextRequestID(), Limit: limit})
}

func (c *Client) DeleteDomain(ctx context.Context, domain string) error {
	_, err := request[struct{}](c, ctx, "history_delete_domain", struct {
		RequestID string `json:"requestId"`
		Domain    string `json:"domain"`
	}{RequestID: nextRequestID(), Domain: domain})
	return err
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

func request[T any](c *Client, ctx context.Context, msgType string, payload any) (T, error) {
	var zero T

	raw, err := c.Send(ctx, msgType, payload)
	if err != nil {
		return zero, err
	}

	return decodeBridgeResponse[T](raw)
}

type bridgeResponse struct {
	RequestID string          `json:"requestId"`
	Success   bool            `json:"success"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     string          `json:"error,omitempty"`
}

func decodeBridgeResponse[T any](body []byte) (T, error) {
	var zero T

	var resp bridgeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return zero, fmt.Errorf("unmarshal bridge response: %w", err)
	}
	if !resp.Success {
		if resp.Error == "" {
			return zero, errors.New("bridge request failed")
		}
		return zero, errors.New(resp.Error)
	}
	if len(resp.Data) == 0 {
		return zero, nil
	}

	if err := json.Unmarshal(resp.Data, &zero); err != nil {
		return zero, fmt.Errorf("unmarshal bridge data: %w", err)
	}

	return zero, nil
}

func nextRequestID() string {
	return fmt.Sprintf("systemviews-%d", requestSeq.Add(1))
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
