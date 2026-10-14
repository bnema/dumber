package port

import "context"

// ModeLegendPreferencesSaver persists mode legend preference changes.
type ModeLegendPreferencesSaver interface {
	SaveModeLegendCollapsed(ctx context.Context, collapsed bool) error
}
