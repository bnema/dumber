package ui

import (
	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/bootstrap"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

func runtimeConfigSnapshotForTest(cfg *config.Config) port.RuntimeConfigSnapshot {
	return bootstrap.RuntimeConfigSnapshotFromConfig(cfg)
}

func appWithRuntimeConfigForTest(cfg *config.Config) *App {
	return &App{
		deps:                &Dependencies{},
		runtimeConfig:       runtimeConfigSnapshotForTest(cfg),
		runtimeConfigLoaded: true,
	}
}
