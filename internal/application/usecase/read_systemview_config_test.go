package usecase

import (
	"context"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestReadSystemviewConfigUseCase_Current(t *testing.T) {
	reader := portmocks.NewMockSystemviewConfigReader(t)
	want := port.SystemviewConfigPayload{EngineType: "webkit"}
	reader.EXPECT().Current(mock.Anything).Return(want, nil).Once()

	uc := NewReadSystemviewConfigUseCase(reader)

	got, err := uc.Current(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestReadSystemviewConfigUseCase_Default(t *testing.T) {
	reader := portmocks.NewMockSystemviewConfigReader(t)
	want := port.SystemviewConfigPayload{EngineType: "webkit"}
	reader.EXPECT().Default(mock.Anything).Return(want, nil).Once()

	uc := NewReadSystemviewConfigUseCase(reader)

	got, err := uc.Default(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestNewReadSystemviewConfigUseCase_NilReaderPanics(t *testing.T) {
	require.PanicsWithValue(t, "NewReadSystemviewConfigUseCase: reader is nil", func() {
		NewReadSystemviewConfigUseCase(nil)
	})
}

func TestReadSystemviewConfigUseCase_NilUseCaseReturnsSentinel(t *testing.T) {
	var uc *ReadSystemviewConfigUseCase

	_, err := uc.Current(context.Background())
	require.ErrorIs(t, err, ErrNilSystemviewConfigReader)

	_, err = uc.Default(context.Background())
	require.ErrorIs(t, err, ErrNilSystemviewConfigReader)
}
