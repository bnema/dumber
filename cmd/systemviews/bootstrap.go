package main

import (
	"context"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/ui/systemviews"
)

type bridgeServices interface {
	port.SystemviewHistoryService
	port.SystemviewFavoritesService
	port.SystemviewConfigService
}

func newBridgeApp(dom systemviews.DOM, locationURI string, bridge bridgeServices) *systemviews.App {
	return systemviews.NewApp(systemviews.Dependencies{
		DOM:         dom,
		History:     bridge,
		Favorites:   bridge,
		Config:      bridgeConfigProxy{bridge: bridge, route: systemviews.ParseRoute(locationURI)},
		LocationURI: locationURI,
	})
}

type bridgeConfigProxy struct {
	bridge bridgeServices
	route  systemviews.Route
}

func (p bridgeConfigProxy) Current(ctx context.Context) (port.SystemviewConfigPayload, error) {
	return p.bridge.Current(ctx)
}

func (p bridgeConfigProxy) Default(ctx context.Context) (port.SystemviewConfigPayload, error) {
	return p.bridge.Default(ctx)
}

func (p bridgeConfigProxy) Save(ctx context.Context, cfg port.WebUIConfig) error {
	return p.bridge.Save(ctx, cfg)
}

func (p bridgeConfigProxy) GetKeybindings(ctx context.Context) (port.KeybindingsConfig, error) {
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
