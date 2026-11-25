package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/domain/entity"
)

// RegisterHomeHandlers registers the dumb://home messages.
func RegisterHomeHandlers(_ context.Context, router port.WebUIHandlerRouter, home port.HomePage) error {
	if home == nil {
		return errors.New("RegisterHomeHandlers: home must not be nil")
	}

	handlers := map[string]port.WebUIMessageHandlerFunc{
		port.MsgHomeLoad: func(ctx context.Context, _ port.WebViewID, _ json.RawMessage) (any, error) {
			return home.LoadHome(ctx)
		},
		port.MsgHomeSites: func(ctx context.Context, _ port.WebViewID, payload json.RawMessage) (any, error) {
			req, err := decodeHomeSitesRequest(payload)
			if err != nil {
				return nil, err
			}
			return home.HomeSites(ctx, req)
		},
		// The payload is ignored: the opened path is always the native one.
		port.MsgHomeOpenConfig: func(ctx context.Context, _ port.WebViewID, _ json.RawMessage) (any, error) {
			return home.OpenHomeConfig(ctx)
		},
	}
	for _, msgType := range []string{port.MsgHomeLoad, port.MsgHomeSites, port.MsgHomeOpenConfig} {
		if err := port.RegisterWebUIMessage(router, msgType, handlers[msgType]); err != nil {
			return err
		}
	}
	return nil
}

func decodeHomeSitesRequest(payload json.RawMessage) (dto.HomeSitesRequest, error) {
	var req dto.HomeSitesRequest
	if len(payload) == 0 || string(payload) == "null" {
		return req, errors.New("home_sites: missing payload")
	}
	if err := json.Unmarshal(payload, &req); err != nil {
		return req, fmt.Errorf("home_sites: invalid payload: %w", err)
	}
	switch req.Kind {
	case dto.HomeSitesTop, dto.HomeSitesRecent:
	default:
		return req, fmt.Errorf("home_sites: unknown kind %q", req.Kind)
	}
	if req.Limit < 0 || req.Limit > entity.HomeMaxLimit {
		return req, fmt.Errorf("home_sites: limit must be between %d and %d", entity.HomeMinLimit, entity.HomeMaxLimit)
	}
	return req, nil
}
