package bootstrap

import (
	"context"
	"fmt"

	"bnema.dev/dumber/internal/application/port"
	"bnema.dev/dumber/internal/application/usecase"
	"bnema.dev/dumber/internal/infrastructure/config"
)

// BuildHandlerDeps constructs handler dependencies from the config manager.
// It combines keybinding use cases and config save into a single struct.
// The context is accepted for future use (e.g., passing to constructors that may need cancellation).
func BuildHandlerDeps(_ context.Context) (*port.HandlerDeps, error) {
	mgr := config.GetManager()
	if mgr == nil {
		return nil, fmt.Errorf("config manager not initialized")
	}

	gateway := config.NewKeybindingsGateway(mgr)
	saveUC := usecase.NewSaveWebUIConfigUseCase(config.NewWebUIConfigGateway(mgr))
	omniboxSaveUC := usecase.NewSaveOmniboxInitialBehaviorUseCase(config.NewOmniboxPreferencesGateway(mgr))
	modeLegendSaveUC := usecase.NewSaveModeLegendCollapsedUseCase(config.NewModeLegendPreferencesGateway(mgr))

	return &port.HandlerDeps{
		SaveConfig:                 saveUC.Execute,
		SaveOmniboxInitialBehavior: omniboxSaveUC.Execute,
		SaveModeLegendCollapsed:    modeLegendSaveUC.Execute,
		KeybindingsGetter:          usecase.NewGetKeybindingsUseCase(gateway),
		KeybindingSetter:           usecase.NewSetKeybindingUseCase(gateway, gateway),
		KeybindingResetter:         usecase.NewResetKeybindingUseCase(gateway),
		AllKeybindingsResetter:     usecase.NewResetAllKeybindingsUseCase(gateway),
	}, nil
}
