package usecase

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/stretchr/testify/require"
)

func TestReadSystemviewConfigUseCase_Current(t *testing.T) {
	reader := &stubSystemviewConfigReader{
		current: port.SystemviewConfigPayload{EngineType: "webkit"},
	}

	uc := NewReadSystemviewConfigUseCase(reader)

	got, err := uc.Current(context.Background())
	require.NoError(t, err)
	require.Equal(t, reader.current, got)
	require.Equal(t, []string{"current"}, reader.calls)
}

func TestReadSystemviewConfigUseCase_Default(t *testing.T) {
	reader := &stubSystemviewConfigReader{
		defaultPayload: port.SystemviewConfigPayload{EngineType: "webkit"},
	}

	uc := NewReadSystemviewConfigUseCase(reader)

	got, err := uc.Default(context.Background())
	require.NoError(t, err)
	require.Equal(t, reader.defaultPayload, got)
	require.Equal(t, []string{"default"}, reader.calls)
}

// Handwritten fake to capture stateful config reads for assertions.
type stubSystemviewConfigReader struct {
	current        port.SystemviewConfigPayload
	defaultPayload port.SystemviewConfigPayload
	calls          []string
}

func (r *stubSystemviewConfigReader) Current(context.Context) (port.SystemviewConfigPayload, error) {
	r.calls = append(r.calls, "current")
	return r.current, nil
}

func (r *stubSystemviewConfigReader) Default(context.Context) (port.SystemviewConfigPayload, error) {
	r.calls = append(r.calls, "default")
	return r.defaultPayload, nil
}
