package ui

import (
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/bootstrap"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

func runtimeConfigSnapshotForTest(cfg *config.Config) port.RuntimeConfigSnapshot {
	return bootstrap.RuntimeConfigSnapshotFromConfig(cfg)
}

func runtimeConfigStateFromSnapshotForTest(snapshot port.RuntimeConfigSnapshot) *runtimeConfigState {
	state := newRuntimeConfigState(nil)
	state.Update(snapshot)
	return state
}

func runtimeConfigStateForTest(cfg *config.Config) *runtimeConfigState {
	return runtimeConfigStateFromSnapshotForTest(runtimeConfigSnapshotForTest(cfg))
}

func setRuntimeConfigSnapshotForTest(app *App, snapshot port.RuntimeConfigSnapshot) {
	if app.runtimeConfig == nil {
		app.runtimeConfig = newRuntimeConfigState(nil)
	}
	app.runtimeConfig.Update(snapshot)
}

func appWithRuntimeConfigForTest(cfg *config.Config) *App {
	return &App{
		deps:          &Dependencies{},
		runtimeConfig: runtimeConfigStateForTest(cfg),
	}
}
