// Package usecase contains application business logic.
package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/logging"
)

// CopyURLUseCase handles copying URLs to the system clipboard.
type CopyURLUseCase struct {
	clipboard port.Clipboard
}

// NewCopyURLUseCase creates a new CopyURLUseCase.
func NewCopyURLUseCase(clipboard port.Clipboard) *CopyURLUseCase {
	return &CopyURLUseCase{
		clipboard: clipboard,
	}
}

// Copy copies the given URL to the clipboard.
// Returns nil on success, error on failure.
// The caller is responsible for showing toast notifications on the UI thread.
func (uc *CopyURLUseCase) Copy(ctx context.Context, url string) error {
	log := logging.FromContext(ctx)

	if url == "" {
		log.Debug().Msg("copy URL: empty URL")
		return fmt.Errorf("empty URL")
	}

	if uc.clipboard == nil {
		log.Warn().Msg("copy URL: clipboard is nil")
		return fmt.Errorf("clipboard not available")
	}

	if err := uc.clipboard.WriteText(ctx, url); err != nil {
		log.Error().Err(err).Str("url", url).Msg("copy URL: clipboard write failed")
		return fmt.Errorf("clipboard write failed: %w", err)
	}

	log.Debug().Str("url", url).Msg("URL copied to clipboard")
	return nil
}
