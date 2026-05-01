package logging

import (
	"context"

	"github.com/rs/zerolog"

	"github.com/bnema/dumber/internal/application/port"
)

// NewPortLogger adapts the zerolog logger stored in ctx to the application
// logging boundary.
func NewPortLogger(ctx context.Context) port.Logger {
	if ctx == nil {
		ctx = context.Background()
	}
	return portLogger{ctx: ctx}
}

type portLogger struct {
	ctx context.Context
}

func (l portLogger) Debug(msg string, fields ...port.LogField) {
	addLogFields(FromContext(l.ctx).Debug(), fields).Msg(msg)
}

func (l portLogger) Info(msg string, fields ...port.LogField) {
	addLogFields(FromContext(l.ctx).Info(), fields).Msg(msg)
}

func (l portLogger) Warn(msg string, fields ...port.LogField) {
	addLogFields(FromContext(l.ctx).Warn(), fields).Msg(msg)
}

func addLogFields(event *zerolog.Event, fields []port.LogField) *zerolog.Event {
	for _, field := range fields {
		event = event.Interface(field.Key, field.Value)
	}
	return event
}
