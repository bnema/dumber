package port

import (
	"context"

	"github.com/bnema/dumber/internal/domain/entity"
)

// OmniboxPreferencesSaver persists omnibox preference changes.
type OmniboxPreferencesSaver interface {
	SaveOmniboxInitialBehavior(ctx context.Context, behavior entity.OmniboxInitialBehavior) error
}
