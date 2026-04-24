//go:build !js || !wasm

package systemviews

import (
	"context"

	"github.com/bnema/dumber/internal/logging"
)

func logActionMountError(ctx context.Context, mountErr, actionErr error) {
	if mountErr == nil && actionErr == nil {
		return
	}
	logging.FromContext(ctx).
		Error().
		Err(mountErr).
		Str("action_error", errorString(actionErr)).
		Msg("failed to mount systemview action error")
}
