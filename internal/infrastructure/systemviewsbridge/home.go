package systemviewsbridge

import (
	"context"
	"fmt"

	"bnema.dev/dumber/internal/application/dto"
)

// Home message types. TODO: switch to the port constants once they land.
const (
	msgHomeLoad       = "home_load"
	msgHomeSites      = "home_sites"
	msgHomeOpenConfig = "home_open_config"
)

// HomeLoad returns the dumb://home layout and its load diagnostics.
func (c *Client) HomeLoad(ctx context.Context) (dto.HomeLoadResponse, error) {
	return request[dto.HomeLoadResponse](c, ctx, msgHomeLoad, struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

// HomeSites returns the rows of the top_sites or recent widget.
func (c *Client) HomeSites(ctx context.Context, req dto.HomeSitesRequest) (dto.HomeSitesResponse, error) {
	switch req.Kind {
	case dto.HomeSitesTop, dto.HomeSitesRecent:
	default:
		return dto.HomeSitesResponse{}, fmt.Errorf("unknown home sites kind %q", req.Kind)
	}
	if req.Limit < 0 {
		return dto.HomeSitesResponse{}, fmt.Errorf("home sites limit must be non-negative, got %d", req.Limit)
	}
	return request[dto.HomeSitesResponse](c, ctx, msgHomeSites, struct {
		RequestID string `json:"requestId"`
		dto.HomeSitesRequest
	}{RequestID: nextRequestID(), HomeSitesRequest: req})
}

// HomeOpenConfig asks the browser to open home.toml, creating it if missing.
func (c *Client) HomeOpenConfig(ctx context.Context) (dto.HomeOpenConfigResponse, error) {
	return request[dto.HomeOpenConfigResponse](c, ctx, msgHomeOpenConfig, struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}
