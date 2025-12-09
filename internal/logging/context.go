package logging

import (
	"context"

	"github.com/rs/zerolog"
)

// FromContext extracts the logger from context
// If no logger is found, returns a disabled logger (no-op)
func FromContext(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}

// WithContext returns a new context with the logger attached
func WithContext(ctx context.Context, logger zerolog.Logger) context.Context {
	return logger.WithContext(ctx)
}

// With creates a child logger with additional fields and returns a new context
func With(ctx context.Context, fields map[string]any) context.Context {
	logger := FromContext(ctx)
	childCtx := logger.With()

	for k, v := range fields {
		childCtx = childCtx.Interface(k, v)
	}

	childLogger := childCtx.Logger()
	return WithContext(ctx, childLogger)
}

// WithComponent creates a child logger with a component field
func WithComponent(ctx context.Context, component string) context.Context {
	logger := FromContext(ctx)
	childLogger := logger.With().Str("component", component).Logger()
	return WithContext(ctx, childLogger)
}

// WithPaneID creates a child logger with a pane_id field
func WithPaneID(ctx context.Context, paneID string) context.Context {
	logger := FromContext(ctx)
	childLogger := logger.With().Str("pane_id", paneID).Logger()
	return WithContext(ctx, childLogger)
}

// WithTabID creates a child logger with a tab_id field
func WithTabID(ctx context.Context, tabID string) context.Context {
	logger := FromContext(ctx)
	childLogger := logger.With().Str("tab_id", tabID).Logger()
	return WithContext(ctx, childLogger)
}

// WithURL creates a child logger with a url field
func WithURL(ctx context.Context, url string) context.Context {
	logger := FromContext(ctx)
	childLogger := logger.With().Str("url", url).Logger()
	return WithContext(ctx, childLogger)
}
