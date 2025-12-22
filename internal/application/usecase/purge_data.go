package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
)

// PurgeDataUseCase handles discovering and purging application data.
type PurgeDataUseCase struct {
	fs      port.FileSystem
	xdg     port.XDGPaths
	desktop port.DesktopIntegration
}

// NewPurgeDataUseCase creates a new PurgeDataUseCase.
func NewPurgeDataUseCase(fs port.FileSystem, xdg port.XDGPaths, desktop port.DesktopIntegration) *PurgeDataUseCase {
	return &PurgeDataUseCase{fs: fs, xdg: xdg, desktop: desktop}
}

// GetPurgeTargets returns all available purge targets with their current state.
func (uc *PurgeDataUseCase) GetPurgeTargets(ctx context.Context) ([]entity.PurgeTarget, error) {
	configDir, err := uc.xdg.ConfigDir()
	if err != nil {
		return nil, err
	}
	dataDir, err := uc.xdg.DataDir()
	if err != nil {
		return nil, err
	}
	stateDir, err := uc.xdg.StateDir()
	if err != nil {
		return nil, err
	}
	cacheDir, err := uc.xdg.CacheDir()
	if err != nil {
		return nil, err
	}
	filterJSONDir, err := uc.xdg.FilterJSONDir()
	if err != nil {
		return nil, err
	}
	filterStoreDir, err := uc.xdg.FilterStoreDir()
	if err != nil {
		return nil, err
	}
	filterCacheDir, err := uc.xdg.FilterCacheDir()
	if err != nil {
		return nil, err
	}

	baseTargets := []entity.PurgeTarget{
		{Type: entity.PurgeTargetConfig, Path: configDir, Description: "config"},
		{Type: entity.PurgeTargetData, Path: dataDir, Description: "data"},
		{Type: entity.PurgeTargetState, Path: stateDir, Description: "state"},
		{Type: entity.PurgeTargetCache, Path: cacheDir, Description: "cache"},
		{Type: entity.PurgeTargetFilterJSON, Path: filterJSONDir, Description: "filter JSON cache"},
		{Type: entity.PurgeTargetFilterStore, Path: filterStoreDir, Description: "compiled filters"},
		{Type: entity.PurgeTargetFilterCache, Path: filterCacheDir, Description: "filter cache"},
	}

	status, statusErr := uc.desktop.GetStatus(ctx)
	if statusErr != nil {
		// Desktop status should not block purge targets discovery.
		logging.FromContext(ctx).Warn().Err(statusErr).Msg("failed to get desktop integration status")
	}

	if status != nil {
		baseTargets = append(baseTargets,
			entity.PurgeTarget{Type: entity.PurgeTargetDesktopFile, Path: status.DesktopFilePath, Description: "desktop file"},
			entity.PurgeTarget{Type: entity.PurgeTargetIcon, Path: status.IconFilePath, Description: "icon"},
		)
	}

	var targets []entity.PurgeTarget
	for _, t := range baseTargets {
		if t.Path == "" {
			t.Exists = false
			t.Size = 0
			targets = append(targets, t)
			continue
		}
		exists, err := uc.fs.Exists(ctx, t.Path)
		if err != nil {
			return nil, err
		}
		t.Exists = exists
		if !exists {
			t.Size = 0
			targets = append(targets, t)
			continue
		}
		size, err := uc.fs.GetSize(ctx, t.Path)
		if err != nil {
			return nil, err
		}
		t.Size = size
		targets = append(targets, t)
	}

	return targets, nil
}

// PurgeInput specifies which target types to purge.
type PurgeInput struct {
	TargetTypes []entity.PurgeTargetType
}

// PurgeOutput contains the results of the purge operation.
type PurgeOutput struct {
	Results      []entity.PurgeResult
	TotalSize    int64
	SuccessCount int
	FailureCount int
}

// Execute purges the selected target types.
// Continues on errors, collecting all results.
func (uc *PurgeDataUseCase) Execute(ctx context.Context, input PurgeInput) (*PurgeOutput, error) {
	log := logging.FromContext(ctx)

	targets, err := uc.GetPurgeTargets(ctx)
	if err != nil {
		return nil, err
	}

	selected := make(map[entity.PurgeTargetType]struct{}, len(input.TargetTypes))
	for _, tt := range input.TargetTypes {
		selected[tt] = struct{}{}
	}

	out := &PurgeOutput{}
	for _, t := range targets {
		if _, ok := selected[t.Type]; !ok {
			continue
		}
		if !t.Exists {
			continue
		}

		res := entity.PurgeResult{Target: t}
		out.TotalSize += t.Size

		switch t.Type {
		case entity.PurgeTargetDesktopFile:
			err = uc.desktop.RemoveDesktopFile(ctx)
		case entity.PurgeTargetIcon:
			err = uc.desktop.RemoveIcon(ctx)
		default:
			err = uc.fs.RemoveAll(ctx, t.Path)
		}

		if err != nil {
			res.Success = false
			res.Error = err
			out.FailureCount++
			log.Warn().Err(err).Str("path", t.Path).Int("type", int(t.Type)).Msg("purge target failed")
		} else {
			res.Success = true
			out.SuccessCount++
			log.Info().Str("path", t.Path).Int("type", int(t.Type)).Msg("purge target removed")
		}

		out.Results = append(out.Results, res)
	}

	if out.FailureCount > 0 {
		return out, fmt.Errorf("failed to remove %d items", out.FailureCount)
	}
	return out, nil
}

// PurgeAll purges all existing targets (for --force mode).
func (uc *PurgeDataUseCase) PurgeAll(ctx context.Context) (*PurgeOutput, error) {
	targets, err := uc.GetPurgeTargets(ctx)
	if err != nil {
		return nil, err
	}

	var types []entity.PurgeTargetType
	for _, t := range targets {
		types = append(types, t.Type)
	}

	return uc.Execute(ctx, PurgeInput{TargetTypes: types})
}
