package port

import (
	"context"

	"bnema.dev/dumber/internal/domain/entity"
)

// OmniboxPreferencesSaver persists omnibox preference changes.
type OmniboxPreferencesSaver interface {
	SaveOmniboxInitialBehavior(ctx context.Context, behavior entity.OmniboxInitialBehavior) error
}
