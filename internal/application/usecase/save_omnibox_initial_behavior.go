package usecase

import (
	"context"
	"fmt"
	"strings"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
)

type SaveOmniboxInitialBehaviorUseCase struct {
	saver port.OmniboxPreferencesSaver
}

func NewSaveOmniboxInitialBehaviorUseCase(saver port.OmniboxPreferencesSaver) *SaveOmniboxInitialBehaviorUseCase {
	return &SaveOmniboxInitialBehaviorUseCase{saver: saver}
}

func (uc *SaveOmniboxInitialBehaviorUseCase) Execute(ctx context.Context, behavior entity.OmniboxInitialBehavior) error {
	if uc == nil || uc.saver == nil {
		return fmt.Errorf("omnibox preferences saver is nil")
	}

	behavior = entity.OmniboxInitialBehavior(strings.TrimSpace(string(behavior)))
	switch behavior {
	case entity.OmniboxInitialBehaviorRecent, entity.OmniboxInitialBehaviorMostVisited:
		return uc.saver.SaveOmniboxInitialBehavior(ctx, behavior)
	default:
		return fmt.Errorf(
			"omnibox.initial_behavior must be one of: %s, %s",
			entity.OmniboxInitialBehaviorRecent,
			entity.OmniboxInitialBehaviorMostVisited,
		)
	}
}
