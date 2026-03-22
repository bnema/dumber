package bootstrap

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/infrastructure/config"
)

// BuildHandlerDeps constructs handler dependencies from the config manager.
// It combines keybinding use cases and config save into a single struct.
func BuildHandlerDeps(_ context.Context) (*port.HandlerDeps, error) {
	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	gateway := config.NewKeybindingsGateway(mgr)
	saveUC := usecase.NewSaveWebUIConfigUseCase(config.NewWebUIConfigGateway(mgr))

	return &port.HandlerDeps{
		SaveConfig:             saveUC.Execute,
		KeybindingsGetter:      usecase.NewGetKeybindingsUseCase(gateway),
		KeybindingSetter:       usecase.NewSetKeybindingUseCase(gateway, gateway),
		KeybindingResetter:     usecase.NewResetKeybindingUseCase(gateway),
		AllKeybindingsResetter: usecase.NewResetAllKeybindingsUseCase(gateway),
	}, nil
}
