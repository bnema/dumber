package systemviewsbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bnema/dumber/internal/application/dto"
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
	native         Transport
	fetch          Transport
	requestTimeout time.Duration
}

var _ port.SystemviewConfigService = (*Client)(nil)
var _ port.SystemviewHistoryService = (*Client)(nil)
var _ port.SystemviewFavoritesService = (*Client)(nil)

var requestSeq atomic.Uint64

const defaultBridgeRequestTimeout = 15 * time.Second

// ClientOption customizes a bridge client.
type ClientOption func(*Client)

// WithRequestTimeout sets the timeout used when callers provide no deadline.
func WithRequestTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.requestTimeout = timeout
	}
}

// NewClient creates a bridge client with native WebKit and fetch transports.
func NewClient(native, fetch Transport, opts ...ClientOption) *Client {
	client := &Client{native: native, fetch: fetch, requestTimeout: defaultBridgeRequestTimeout}
	for _, opt := range opts {
		if opt != nil {
			opt(client)
		}
	}
	return client
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
	if limit < 0 {
		return nil, fmt.Errorf("timeline limit must be non-negative, got %d", limit)
	}
	if offset < 0 {
		return nil, fmt.Errorf("timeline offset must be non-negative, got %d", offset)
	}
	return request[[]*entity.HistoryEntry](c, ctx, "history_timeline", struct {
		RequestID string `json:"requestId"`
		Limit     int    `json:"limit"`
		Offset    int    `json:"offset"`
	}{RequestID: nextRequestID(), Limit: limit, Offset: offset})
}

func (c *Client) TimelineByDomain(ctx context.Context, domain string, limit, offset int) ([]*entity.HistoryEntry, error) {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return nil, fmt.Errorf("timeline-by-domain domain must be non-empty")
	}
	if limit < 0 {
		return nil, fmt.Errorf("timeline-by-domain limit must be non-negative, got %d", limit)
	}
	if offset < 0 {
		return nil, fmt.Errorf("timeline-by-domain offset must be non-negative, got %d", offset)
	}
	return request[[]*entity.HistoryEntry](c, ctx, "history_timeline_by_domain", struct {
		RequestID string `json:"requestId"`
		Domain    string `json:"domain"`
		Limit     int    `json:"limit"`
		Offset    int    `json:"offset"`
	}{RequestID: nextRequestID(), Domain: domain, Limit: limit, Offset: offset})
}

func (c *Client) TimelineWindow(ctx context.Context, before time.Time, beforeID int64, domain string) (*entity.HistoryWindow, error) {
	if beforeID < 0 {
		return nil, fmt.Errorf("history window cursor beforeID must be non-negative")
	}
	if before.IsZero() != (beforeID == 0) {
		return nil, fmt.Errorf("history window cursor requires before and beforeID")
	}
	beforeCursor := ""
	if !before.IsZero() {
		beforeCursor = before.Format(time.RFC3339Nano)
	}
	return request[*entity.HistoryWindow](c, ctx, "history_timeline_window", struct {
		RequestID string `json:"requestId"`
		Before    string `json:"before,omitempty"`
		BeforeID  int64  `json:"beforeId,omitempty"`
		Domain    string `json:"domain,omitempty"`
	}{RequestID: nextRequestID(), Before: beforeCursor, BeforeID: beforeID, Domain: strings.TrimSpace(domain)})
}

func (c *Client) Search(ctx context.Context, query string, limit int) ([]*entity.HistoryEntry, error) {
	if limit < 0 {
		return nil, fmt.Errorf("search limit must be non-negative, got %d", limit)
	}
	return request[[]*entity.HistoryEntry](c, ctx, "history_search_fts", struct {
		RequestID string `json:"requestId"`
		Query     string `json:"query"`
		Limit     int    `json:"limit"`
	}{RequestID: nextRequestID(), Query: query, Limit: limit})
}

func (c *Client) DeleteEntry(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("delete entry id must be positive, got %d", id)
	}
	_, err := request[struct{}](c, ctx, "history_delete_entry", struct {
		RequestID string `json:"requestId"`
		ID        int64  `json:"id"`
	}{RequestID: nextRequestID(), ID: id})
	return err
}

func (c *Client) DeleteRange(ctx context.Context, rangeID string) error {
	rangeID = strings.TrimSpace(rangeID)
	if rangeID == "" {
		return errors.New("history range ID cannot be empty")
	}
	_, err := request[struct{}](c, ctx, "history_delete_range", struct {
		RequestID string `json:"requestId"`
		Range     string `json:"range"`
	}{RequestID: nextRequestID(), Range: rangeID})
	return err
}

// Current intentionally targets the direct HTTP config endpoint rather than a
// snake_case bridge message so CEF/WebKit fetch shims can reuse /api/config.
func (c *Client) Current(ctx context.Context) (dto.SystemviewConfigPayload, error) {
	return request[dto.SystemviewConfigPayload](c, ctx, "/api/config", struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

// Default intentionally targets the direct HTTP config endpoint rather than a
// snake_case bridge message so CEF/WebKit fetch shims can reuse /api/config/default.
func (c *Client) Default(ctx context.Context) (dto.SystemviewConfigPayload, error) {
	return request[dto.SystemviewConfigPayload](c, ctx, "/api/config/default", struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

func (c *Client) Save(ctx context.Context, cfg dto.WebUIConfig) error {
	_, err := request[struct{}](c, ctx, "save_config", cfg)
	return err
}

func (c *Client) GetKeybindings(ctx context.Context) (port.KeybindingsConfig, error) {
	return request[port.KeybindingsConfig](c, ctx, "get_keybindings", struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

func (c *Client) SetKeybinding(ctx context.Context, req port.SetKeybindingRequest) (port.SetKeybindingResponse, error) {
	return request[port.SetKeybindingResponse](c, ctx, "set_keybinding", req)
}

func (c *Client) ResetKeybinding(ctx context.Context, req port.ResetKeybindingRequest) error {
	_, err := request[struct{}](c, ctx, "reset_keybinding", req)
	return err
}

func (c *Client) ResetAllKeybindings(ctx context.Context) error {
	_, err := request[struct{}](c, ctx, "reset_all_keybindings", struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
	return err
}

func (c *Client) Stats(ctx context.Context) (*entity.HistoryStats, error) {
	return request[*entity.HistoryStats](c, ctx, "history_stats", struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

func (c *Client) Analytics(ctx context.Context) (*entity.HistoryAnalytics, error) {
	return request[*entity.HistoryAnalytics](c, ctx, "history_analytics", struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

func (c *Client) DomainStats(ctx context.Context, limit int) ([]*entity.DomainStat, error) {
	if limit < 0 {
		return nil, fmt.Errorf("domain stats limit must be non-negative, got %d", limit)
	}
	return request[[]*entity.DomainStat](c, ctx, "history_domain_stats", struct {
		RequestID string `json:"requestId"`
		Limit     int    `json:"limit"`
	}{RequestID: nextRequestID(), Limit: limit})
}

func (c *Client) DeleteDomain(ctx context.Context, domain string) error {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return errors.New("history domain cannot be empty")
	}
	_, err := request[struct{}](c, ctx, "history_delete_domain", struct {
		RequestID string `json:"requestId"`
		Domain    string `json:"domain"`
	}{RequestID: nextRequestID(), Domain: domain})
	return err
}

func (c *Client) List(ctx context.Context) ([]*entity.Favorite, error) {
	return request[[]*entity.Favorite](c, ctx, "favorite_list", struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

func (c *Client) CreateFavorite(ctx context.Context, input dto.FavoriteCreateInput) (*entity.Favorite, error) {
	return request[*entity.Favorite](c, ctx, "favorite_create", favoriteCreatePayload(input))
}

func (c *Client) UpdateFavorite(ctx context.Context, input dto.FavoriteUpdateInput) (*entity.Favorite, error) {
	if input.ID <= 0 {
		return nil, fmt.Errorf("update favorite id must be positive, got %d", input.ID)
	}
	return request[*entity.Favorite](c, ctx, "favorite_update", favoriteUpdatePayload(input))
}

func (c *Client) DeleteFavorite(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("delete favorite id must be positive, got %d", id)
	}
	_, err := request[struct{}](c, ctx, "favorite_delete", struct {
		RequestID string `json:"requestId"`
		ID        int64  `json:"id"`
	}{RequestID: nextRequestID(), ID: id})
	return err
}

func favoriteCreatePayload(input dto.FavoriteCreateInput) any {
	return struct {
		RequestID  string  `json:"requestId"`
		URL        string  `json:"url"`
		Title      string  `json:"title"`
		FaviconURL string  `json:"favicon_url"`
		FolderID   *int64  `json:"folder_id"`
		Tags       []int64 `json:"tags"`
	}{
		RequestID:  nextRequestID(),
		URL:        input.URL,
		Title:      input.Title,
		FaviconURL: input.FaviconURL,
		FolderID:   folderIDPayload(input.FolderID),
		Tags:       tagIDPayloads(input.Tags),
	}
}

func favoriteUpdatePayload(input dto.FavoriteUpdateInput) any {
	return struct {
		RequestID   string `json:"requestId"`
		ID          int64  `json:"id"`
		Title       string `json:"title"`
		FaviconURL  string `json:"favicon_url"`
		FolderID    *int64 `json:"folder_id"`
		ShortcutKey *int   `json:"shortcut_key"`
	}{
		RequestID:   nextRequestID(),
		ID:          int64(input.ID),
		Title:       input.Title,
		FaviconURL:  input.FaviconURL,
		FolderID:    folderIDPayload(input.FolderID),
		ShortcutKey: input.ShortcutKey,
	}
}

func folderIDPayload(id *entity.FolderID) *int64 {
	if id == nil {
		return nil
	}
	value := int64(*id)
	return &value
}

func tagIDPayloads(ids []entity.TagID) []int64 {
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		out = append(out, int64(id))
	}
	return out
}

func (c *Client) ListFolders(ctx context.Context) ([]*entity.Folder, error) {
	return request[[]*entity.Folder](c, ctx, "folder_list", struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

func (c *Client) ListTags(ctx context.Context) ([]*entity.Tag, error) {
	return request[[]*entity.Tag](c, ctx, "tag_list", struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

func (c *Client) SetShortcut(ctx context.Context, favoriteID int64, shortcutKey *int) error {
	if favoriteID <= 0 {
		return fmt.Errorf("set shortcut favorite id must be positive, got %d", favoriteID)
	}
	_, err := request[struct{}](c, ctx, "favorite_set_shortcut", struct {
		RequestID   string `json:"requestId"`
		FavoriteID  int64  `json:"favorite_id"`
		ShortcutKey *int   `json:"shortcut_key"`
	}{RequestID: nextRequestID(), FavoriteID: favoriteID, ShortcutKey: shortcutKey})
	return err
}

func (c *Client) SetFolder(ctx context.Context, favoriteID int64, folderID *int64) error {
	if favoriteID <= 0 {
		return fmt.Errorf("set folder favorite id must be positive, got %d", favoriteID)
	}
	_, err := request[struct{}](c, ctx, "favorite_set_folder", struct {
		RequestID  string `json:"requestId"`
		FavoriteID int64  `json:"favorite_id"`
		FolderID   *int64 `json:"folder_id"`
	}{RequestID: nextRequestID(), FavoriteID: favoriteID, FolderID: folderID})
	return err
}

func (c *Client) CreateFolder(ctx context.Context, name, icon string, parentID *int64) (*entity.Folder, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("folder name is required")
	}
	var iconPtr *string
	if trimmedIcon := strings.TrimSpace(icon); trimmedIcon != "" {
		iconPtr = &trimmedIcon
	}
	return request[*entity.Folder](c, ctx, "folder_create", struct {
		RequestID string  `json:"requestId"`
		Name      string  `json:"name"`
		Icon      *string `json:"icon"`
		ParentID  *int64  `json:"parent_id,omitempty"`
	}{RequestID: nextRequestID(), Name: name, Icon: iconPtr, ParentID: parentID})
}

func (c *Client) UpdateFolder(ctx context.Context, id int64, name, icon string) error {
	if id <= 0 {
		return fmt.Errorf("update folder id must be positive, got %d", id)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("folder name is required")
	}
	var iconPtr *string
	if trimmed := strings.TrimSpace(icon); trimmed != "" {
		iconPtr = &trimmed
	}
	_, err := request[struct{}](c, ctx, "folder_update", struct {
		RequestID string  `json:"requestId"`
		ID        int64   `json:"id"`
		Name      string  `json:"name"`
		Icon      *string `json:"icon"`
	}{RequestID: nextRequestID(), ID: id, Name: name, Icon: iconPtr})
	return err
}

func (c *Client) DeleteFolder(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("delete folder id must be positive, got %d", id)
	}
	_, err := request[struct{}](c, ctx, "folder_delete", struct {
		RequestID string `json:"requestId"`
		ID        int64  `json:"id"`
	}{RequestID: nextRequestID(), ID: id})
	return err
}

func (c *Client) CreateTag(ctx context.Context, name, color string) (*entity.Tag, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("tag name is required")
	}
	var colorPtr *string
	if trimmedColor := strings.TrimSpace(color); trimmedColor != "" {
		colorPtr = &trimmedColor
	}
	return request[*entity.Tag](c, ctx, "tag_create", struct {
		RequestID string  `json:"requestId"`
		Name      string  `json:"name"`
		Color     *string `json:"color"`
	}{RequestID: nextRequestID(), Name: name, Color: colorPtr})
}

func (c *Client) UpdateTag(ctx context.Context, id int64, name, color string) error {
	if id <= 0 {
		return fmt.Errorf("update tag id must be positive, got %d", id)
	}
	trimmedName := strings.TrimSpace(name)
	if trimmedName == "" {
		return fmt.Errorf("tag name is required")
	}
	namePtr := &trimmedName
	var colorPtr *string
	if trimmedColor := strings.TrimSpace(color); trimmedColor != "" {
		colorPtr = &trimmedColor
	}
	_, err := request[struct{}](c, ctx, "tag_update", struct {
		RequestID string  `json:"requestId"`
		ID        int64   `json:"id"`
		Name      *string `json:"name"`
		Color     *string `json:"color"`
	}{RequestID: nextRequestID(), ID: id, Name: namePtr, Color: colorPtr})
	return err
}

func (c *Client) DeleteTag(ctx context.Context, id int64) error {
	if id <= 0 {
		return fmt.Errorf("delete tag id must be positive, got %d", id)
	}
	_, err := request[struct{}](c, ctx, "tag_delete", struct {
		RequestID string `json:"requestId"`
		ID        int64  `json:"id"`
	}{RequestID: nextRequestID(), ID: id})
	return err
}

func (c *Client) AssignTag(ctx context.Context, favoriteID, tagID int64) error {
	if favoriteID <= 0 {
		return fmt.Errorf("assign tag favorite id must be positive, got %d", favoriteID)
	}
	if tagID <= 0 {
		return fmt.Errorf("assign tag id must be positive, got %d", tagID)
	}
	_, err := request[struct{}](c, ctx, "tag_assign", struct {
		RequestID  string `json:"requestId"`
		FavoriteID int64  `json:"favorite_id"`
		TagID      int64  `json:"tag_id"`
	}{RequestID: nextRequestID(), FavoriteID: favoriteID, TagID: tagID})
	return err
}

func (c *Client) RemoveTag(ctx context.Context, favoriteID, tagID int64) error {
	if favoriteID <= 0 {
		return fmt.Errorf("remove tag favorite id must be positive, got %d", favoriteID)
	}
	if tagID <= 0 {
		return fmt.Errorf("remove tag id must be positive, got %d", tagID)
	}
	_, err := request[struct{}](c, ctx, "tag_remove", struct {
		RequestID  string `json:"requestId"`
		FavoriteID int64  `json:"favorite_id"`
		TagID      int64  `json:"tag_id"`
	}{RequestID: nextRequestID(), FavoriteID: favoriteID, TagID: tagID})
	return err
}

func (c *Client) transport() Transport {
	if c == nil {
		return nil
	}
	if c.native != nil && c.native.Available() {
		return c.native
	}
	if c.fetch != nil && c.fetch.Available() {
		return c.fetch
	}
	return nil
}

func request[T any](c *Client, ctx context.Context, msgType string, payload any) (T, error) {
	var zero T

	ctx, cancel := c.withBridgeRequestTimeout(ctx)
	defer cancel()

	raw, err := c.Send(ctx, msgType, payload)
	if err != nil {
		return zero, err
	}

	return decodeBridgeResponse[T](raw)
}

// withBridgeRequestTimeout applies the bridge request timeout; ctx must be non-nil.
func (c *Client) withBridgeRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := defaultBridgeRequestTimeout
	if c != nil {
		timeout = c.requestTimeout
	}
	if _, ok := ctx.Deadline(); ok || timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

type bridgeResponse struct {
	RequestID string          `json:"requestId"`
	Success   bool            `json:"success"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     string          `json:"error,omitempty"`
}

func decodeBridgeResponse[T any](body []byte) (T, error) {
	var result T

	var resp bridgeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return result, fmt.Errorf("unmarshal bridge response: %w", err)
	}
	if !resp.Success {
		if resp.Error == "" {
			return result, errors.New("bridge request failed")
		}
		return result, errors.New(resp.Error)
	}
	if len(resp.Data) == 0 {
		return result, nil
	}

	if err := json.Unmarshal(resp.Data, &result); err != nil {
		return result, fmt.Errorf("unmarshal bridge data: %w", err)
	}

	return result, nil
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
