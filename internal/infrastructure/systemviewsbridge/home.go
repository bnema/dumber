package systemviewsbridge

import (
	"context"
	"fmt"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
)

// HomeLoad returns the dumb://home layout and its load diagnostics.
func (c *Client) HomeLoad(ctx context.Context) (dto.HomeLoadResponse, error) {
	return request[dto.HomeLoadResponse](c, ctx, port.MsgHomeLoad, struct {
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
	return request[dto.HomeSitesResponse](c, ctx, port.MsgHomeSites, struct {
		RequestID string `json:"requestId"`
		dto.HomeSitesRequest
	}{RequestID: nextRequestID(), HomeSitesRequest: req})
}

// HomeOpenConfig asks the browser to open home.toml, creating it if missing.
func (c *Client) HomeOpenConfig(ctx context.Context) (dto.HomeOpenConfigResponse, error) {
	return request[dto.HomeOpenConfigResponse](c, ctx, port.MsgHomeOpenConfig, struct {
		RequestID string `json:"requestId"`
	}{RequestID: nextRequestID()})
}

// HomeWidgetData returns the data of the api widget of serviceID.
func (c *Client) HomeWidgetData(ctx context.Context, serviceID string) (dto.HomeWidgetDataResponse, error) {
	if serviceID == "" {
		return dto.HomeWidgetDataResponse{}, fmt.Errorf("home widget data: missing service id")
	}
	return request[dto.HomeWidgetDataResponse](c, ctx, port.MsgHomeWidgetData, struct {
		RequestID string `json:"requestId"`
		dto.HomeWidgetDataRequest
	}{RequestID: nextRequestID(), HomeWidgetDataRequest: dto.HomeWidgetDataRequest{ServiceID: serviceID}})
}

// HomeStatus probes the url of serviceID and reports whether it is up.
func (c *Client) HomeStatus(ctx context.Context, serviceID string) (dto.HomeStatusResponse, error) {
	if serviceID == "" {
		return dto.HomeStatusResponse{}, fmt.Errorf("home status: missing service id")
	}
	return request[dto.HomeStatusResponse](c, ctx, port.MsgHomeStatus, struct {
		RequestID string `json:"requestId"`
		dto.HomeStatusRequest
	}{RequestID: nextRequestID(), HomeStatusRequest: dto.HomeStatusRequest{ServiceID: serviceID}})
}

// HomeAction runs a declared action of a home service.
func (c *Client) HomeAction(ctx context.Context, req dto.HomeActionRequest) (dto.HomeActionResponse, error) {
	if req.ServiceID == "" || req.ActionID == "" {
		return dto.HomeActionResponse{}, fmt.Errorf("home action: missing service id or action id")
	}
	return request[dto.HomeActionResponse](c, ctx, port.MsgHomeAction, struct {
		RequestID string `json:"requestId"`
		dto.HomeActionRequest
	}{RequestID: nextRequestID(), HomeActionRequest: req})
}
