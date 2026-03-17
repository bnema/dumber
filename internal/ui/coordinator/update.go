package coordinator

import (
	"context"
	"fmt"
	"sync"

	"github.com/jwijenbergh/puregotk/v4/glib"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
)

// UpdateCoordinator handles update checking and notification.
type UpdateCoordinator struct {
	checkUC         *usecase.CheckUpdateUseCase
	applyUC         *usecase.ApplyUpdateUseCase
	toaster         *component.Toaster
	enableOnStartup bool
	autoDownload    bool
	mu              sync.RWMutex // Protects status and lastInfo
	status          entity.UpdateStatus
	lastInfo        *usecase.CheckUpdateOutput
}

// NewUpdateCoordinator creates a new update coordinator.
// enableOnStartup controls whether an update check runs at startup.
// autoDownload controls whether updates are downloaded automatically in the background.
func NewUpdateCoordinator(
	checkUC *usecase.CheckUpdateUseCase,
	applyUC *usecase.ApplyUpdateUseCase,
	toaster *component.Toaster,
	enableOnStartup bool,
	autoDownload bool,
) *UpdateCoordinator {
	return &UpdateCoordinator{
		checkUC:         checkUC,
		applyUC:         applyUC,
		toaster:         toaster,
		enableOnStartup: enableOnStartup,
		autoDownload:    autoDownload,
		status:          entity.UpdateStatusUnknown,
	}
}

// CheckOnStartup triggers an update check if enabled by config.
func (c *UpdateCoordinator) CheckOnStartup(ctx context.Context) {
	log := logging.FromContext(ctx)

	if !c.enableOnStartup {
		log.Debug().Msg("update check on startup disabled")
		return
	}

	go c.checkAsync(ctx)
}

func (c *UpdateCoordinator) checkAsync(ctx context.Context) {
	log := logging.FromContext(ctx)

	result, err := c.checkUC.Execute(ctx, usecase.CheckUpdateInput{})
	if err != nil {
		log.Warn().Err(err).Msg("background update check failed")
		c.mu.Lock()
		c.status = entity.UpdateStatusFailed
		c.mu.Unlock()
		return
	}

	c.mu.Lock()
	c.lastInfo = result
	if !result.UpdateAvailable {
		c.status = entity.UpdateStatusUpToDate
		c.mu.Unlock()
		log.Debug().
			Str("version", result.CurrentVersion).
			Msg("already on latest version")
		return
	}
	c.status = entity.UpdateStatusAvailable
	c.mu.Unlock()
	log.Info().
		Str("current", result.CurrentVersion).
		Str("latest", result.LatestVersion).
		Bool("can_auto_update", result.CanAutoUpdate).
		Msg("update available")

	c.showUpdateNotification(ctx, result)

	if c.autoDownload && result.CanAutoUpdate {
		go c.downloadAsync(ctx, result.DownloadURL)
	}
}

func (c *UpdateCoordinator) showUpdateNotification(ctx context.Context, result *usecase.CheckUpdateOutput) {
	var msg string
	switch {
	case result.CanAutoUpdate && c.autoDownload:
		msg = fmt.Sprintf("Downloading update %s...", result.LatestVersion)
	default:
		msg = fmt.Sprintf("Update %s available", result.LatestVersion)
	}

	cb := glib.SourceFunc(func(_ uintptr) bool {
		c.toaster.Show(ctx, msg, component.ToastInfo)
		return false
	})
	glib.IdleAdd(&cb, 0)
}

func (c *UpdateCoordinator) downloadAsync(ctx context.Context, downloadURL string) {
	log := logging.FromContext(ctx)

	c.mu.Lock()
	c.status = entity.UpdateStatusDownloading
	c.mu.Unlock()

	result, err := c.applyUC.Execute(ctx, usecase.ApplyUpdateInput{
		DownloadURL: downloadURL,
	})
	if err != nil {
		log.Error().Err(err).Msg("update download failed")
		c.mu.Lock()
		c.status = entity.UpdateStatusFailed
		c.mu.Unlock()
		c.showToast(ctx, "Update download failed", component.ToastError)
		return
	}

	c.mu.Lock()
	if result.Status == entity.UpdateStatusReady {
		c.status = entity.UpdateStatusReady
		c.mu.Unlock()
		log.Info().Msg("update ready, will apply on exit")
		c.showToast(ctx, "Update ready - applies on exit", component.ToastSuccess)
	} else {
		c.status = entity.UpdateStatusFailed
		c.mu.Unlock()
		log.Warn().Str("message", result.Message).Msg("update staging failed")
		c.showToast(ctx, result.Message, component.ToastWarning)
	}
}

func (c *UpdateCoordinator) showToast(ctx context.Context, msg string, level component.ToastLevel) {
	cb := glib.SourceFunc(func(_ uintptr) bool {
		c.toaster.Show(ctx, msg, level)
		return false
	})
	glib.IdleAdd(&cb, 0)
}

// FinalizeOnExit applies a staged update on process exit.
func (c *UpdateCoordinator) FinalizeOnExit(ctx context.Context) error {
	if c.applyUC == nil {
		return nil
	}
	return c.applyUC.FinalizeOnExit(ctx)
}

// Status returns the current update status.
func (c *UpdateCoordinator) Status() entity.UpdateStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.status
}

// LastCheckResult returns the most recent update check result.
func (c *UpdateCoordinator) LastCheckResult() *usecase.CheckUpdateOutput {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastInfo
}

// HasPendingUpdate returns true if a downloaded update is waiting to be applied.
func (c *UpdateCoordinator) HasPendingUpdate(ctx context.Context) bool {
	if c.applyUC == nil {
		return false
	}
	return c.applyUC.HasPendingUpdate(ctx)
}
