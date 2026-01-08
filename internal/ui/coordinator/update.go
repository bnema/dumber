package coordinator

import (
	"context"
	"fmt"
	"sync"

	"github.com/jwijenbergh/puregotk/v4/glib"

	"github.com/bnema/dumber/internal/application/usecase"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/infrastructure/config"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
)

// UpdateCoordinator handles update checking and notification.
type UpdateCoordinator struct {
	checkUC  *usecase.CheckUpdateUseCase
	applyUC  *usecase.ApplyUpdateUseCase
	toaster  *component.Toaster
	config   *config.Config
	mu       sync.RWMutex // Protects status and lastInfo
	status   entity.UpdateStatus
	lastInfo *usecase.CheckUpdateOutput
}

// NewUpdateCoordinator creates a new update coordinator.
func NewUpdateCoordinator(
	checkUC *usecase.CheckUpdateUseCase,
	applyUC *usecase.ApplyUpdateUseCase,
	toaster *component.Toaster,
	cfg *config.Config,
) *UpdateCoordinator {
	return &UpdateCoordinator{
		checkUC: checkUC,
		applyUC: applyUC,
		toaster: toaster,
		config:  cfg,
		status:  entity.UpdateStatusUnknown,
	}
}

// CheckOnStartup performs an async update check if enabled in config.
// This should be called during app initialization.
func (c *UpdateCoordinator) CheckOnStartup(ctx context.Context) {
	log := logging.FromContext(ctx)

	if !c.config.Update.EnableOnStartup {
		log.Debug().Msg("update check on startup disabled")
		return
	}

	go c.checkAsync(ctx)
}

// checkAsync performs the update check in a goroutine.
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

	// Update both lastInfo and status atomically to avoid inconsistent reads.
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

	// Show notification on GTK main thread.
	c.showUpdateNotification(ctx, result)

	// Auto-download if enabled and possible.
	if c.config.Update.AutoDownload && result.CanAutoUpdate {
		// Spawn download in separate goroutine to not block check completion.
		go c.downloadAsync(ctx, result.DownloadURL)
	}
}

// showUpdateNotification displays a toast notification about the update.
func (c *UpdateCoordinator) showUpdateNotification(ctx context.Context, result *usecase.CheckUpdateOutput) {
	var msg string
	if result.CanAutoUpdate && c.config.Update.AutoDownload {
		msg = fmt.Sprintf("Downloading update %s...", result.LatestVersion)
	} else if result.CanAutoUpdate {
		msg = fmt.Sprintf("Update %s available", result.LatestVersion)
	} else {
		msg = fmt.Sprintf("Update %s available", result.LatestVersion)
	}

	// Dispatch to GTK main thread.
	cb := glib.SourceFunc(func(_ uintptr) bool {
		c.toaster.Show(ctx, msg, component.ToastInfo)
		return false
	})
	glib.IdleAdd(&cb, 0)
}

// downloadAsync downloads and stages the update in the background.
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

// showToast dispatches a toast notification to the GTK main thread.
func (c *UpdateCoordinator) showToast(ctx context.Context, msg string, level component.ToastLevel) {
	cb := glib.SourceFunc(func(_ uintptr) bool {
		c.toaster.Show(ctx, msg, level)
		return false
	})
	glib.IdleAdd(&cb, 0)
}

// FinalizeOnExit applies any staged update during shutdown.
// This should be called from the app's shutdown handler.
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

// LastCheckResult returns the result of the last update check.
func (c *UpdateCoordinator) LastCheckResult() *usecase.CheckUpdateOutput {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastInfo
}

// HasPendingUpdate returns true if an update is staged for exit.
func (c *UpdateCoordinator) HasPendingUpdate(ctx context.Context) bool {
	if c.applyUC == nil {
		return false
	}
	return c.applyUC.HasPendingUpdate(ctx)
}
