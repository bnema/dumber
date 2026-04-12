package usecase

import (
	"context"
	"testing"

	portmocks "github.com/bnema/dumber/internal/application/port/mocks"
	"github.com/bnema/dumber/internal/domain/entity"
)

func TestSaveOmniboxInitialBehaviorUseCase_AllowsRecent(t *testing.T) {
	ctx := context.Background()
	saver := portmocks.NewMockOmniboxPreferencesSaver(t)
	saver.EXPECT().SaveOmniboxInitialBehavior(ctx, entity.OmniboxInitialBehaviorRecent).Return(nil).Once()
	uc := NewSaveOmniboxInitialBehaviorUseCase(saver)

	err := uc.Execute(ctx, entity.OmniboxInitialBehaviorRecent)
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
}

func TestSaveOmniboxInitialBehaviorUseCase_AllowsTrimmedMostVisited(t *testing.T) {
	ctx := context.Background()
	saver := portmocks.NewMockOmniboxPreferencesSaver(t)
	saver.EXPECT().SaveOmniboxInitialBehavior(ctx, entity.OmniboxInitialBehaviorMostVisited).Return(nil).Once()
	uc := NewSaveOmniboxInitialBehaviorUseCase(saver)

	err := uc.Execute(ctx, entity.OmniboxInitialBehavior("  most_visited \t"))
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
}

func TestSaveOmniboxInitialBehaviorUseCase_RejectsNone(t *testing.T) {
	saver := portmocks.NewMockOmniboxPreferencesSaver(t)
	uc := NewSaveOmniboxInitialBehaviorUseCase(saver)

	err := uc.Execute(context.Background(), entity.OmniboxInitialBehaviorNone)
	if err == nil {
		t.Fatal("Execute() error = nil, want non-nil")
	}
	if got := err.Error(); got != "omnibox.initial_behavior must be one of: recent, most_visited" &&
		got != "omnibox.initial_behavior must be one of: recent, most_visited (got: none)" {
		t.Fatalf("Execute() error = %q, want validation error", got)
	}
}

func TestSaveOmniboxInitialBehaviorUseCase_RejectsEmptyString(t *testing.T) {
	saver := portmocks.NewMockOmniboxPreferencesSaver(t)
	uc := NewSaveOmniboxInitialBehaviorUseCase(saver)

	err := uc.Execute(context.Background(), entity.OmniboxInitialBehavior(""))
	if err == nil {
		t.Fatal("Execute() error = nil, want non-nil")
	}
	if got := err.Error(); got != "omnibox.initial_behavior must be one of: recent, most_visited" &&
		got != "omnibox.initial_behavior must be one of: recent, most_visited (got: )" {
		t.Fatalf("Execute() error = %q, want validation error", got)
	}
}

func TestSaveOmniboxInitialBehaviorUseCase_NilSaverReturnsError(t *testing.T) {
	uc := NewSaveOmniboxInitialBehaviorUseCase(nil)

	err := uc.Execute(context.Background(), entity.OmniboxInitialBehaviorRecent)
	if err == nil {
		t.Fatal("Execute() error = nil, want non-nil")
	}
	if err.Error() != "omnibox preferences saver is nil" {
		t.Fatalf("Execute() error = %q, want nil saver error", err.Error())
	}
}
