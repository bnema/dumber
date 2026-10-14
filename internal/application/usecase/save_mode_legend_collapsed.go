package usecase

import (
	"context"
	"fmt"

	"github.com/bnema/dumber/internal/application/port"
)

// SaveModeLegendCollapsedUseCase persists whether the mode legend is collapsed
// to its toaster header. The preference is shared by every mode and window.
type SaveModeLegendCollapsedUseCase struct {
	saver port.ModeLegendPreferencesSaver
}

func NewSaveModeLegendCollapsedUseCase(saver port.ModeLegendPreferencesSaver) *SaveModeLegendCollapsedUseCase {
	return &SaveModeLegendCollapsedUseCase{saver: saver}
}

func (uc *SaveModeLegendCollapsedUseCase) Execute(ctx context.Context, collapsed bool) error {
	if uc == nil || uc.saver == nil {
		return fmt.Errorf("mode legend preferences saver is nil")
	}
	return uc.saver.SaveModeLegendCollapsed(ctx, collapsed)
}
