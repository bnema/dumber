package main

import (
	"context"

	"bnema.dev/dumber/internal/application/dto"
	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/ui/systemviews"
)

type bridgeServices interface {
	port.SystemviewHistoryService
	port.SystemviewFavoritesService
	port.SystemviewConfigService
	systemviews.HomeService
}

func newBridgeApp(dom systemviews.DOM, locationURI string, bridge bridgeServices) *systemviews.App {
	route := systemviews.ParseRoute(locationURI)
	return systemviews.NewApp(systemviews.Dependencies{
		DOM:         dom,
		History:     bridge,
		Favorites:   bridge,
		Config:      bridgeConfigProxy{bridge: bridge, route: route},
		Home:        bridge,
		LocationURI: locationURI,
	})
}

type bridgeConfigProxy struct {
	bridge bridgeServices
	route  systemviews.Route
}

func (p bridgeConfigProxy) Current(ctx context.Context) (dto.SystemviewConfigPayload, error) {
	return p.bridge.Current(ctx)
}

func (p bridgeConfigProxy) Default(ctx context.Context) (dto.SystemviewConfigPayload, error) {
	return p.bridge.Default(ctx)
}

func (p bridgeConfigProxy) Save(ctx context.Context, cfg dto.WebUIConfig) error {
	return p.bridge.Save(ctx, cfg)
}

func (p bridgeConfigProxy) GetKeybindings(ctx context.Context) (port.KeybindingsConfig, error) {
	// Keybindings are only rendered by dumb://config; skip the bridge load for
	// other systemview routes to keep initial history/favorites loads cheap.
	if p.route != systemviews.RouteConfig {
		return port.KeybindingsConfig{}, nil
	}
	return p.bridge.GetKeybindings(ctx)
}

func (p bridgeConfigProxy) SetKeybinding(ctx context.Context, req port.SetKeybindingRequest) (port.SetKeybindingResponse, error) {
	return p.bridge.SetKeybinding(ctx, req)
}

func (p bridgeConfigProxy) ResetKeybinding(ctx context.Context, req port.ResetKeybindingRequest) error {
	return p.bridge.ResetKeybinding(ctx, req)
}

func (p bridgeConfigProxy) ResetAllKeybindings(ctx context.Context) error {
	return p.bridge.ResetAllKeybindings(ctx)
}
