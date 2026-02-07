package usecase

import (
	"context"
	"errors"
	"testing"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/build"
	"github.com/bnema/dumber/internal/domain/entity"
)

func TestCheckUpdateUseCase_TransientErrorReturnsNoUpdate(t *testing.T) {
	uc := NewCheckUpdateUseCase(
		stubUpdateChecker{err: port.ErrUpdateCheckTransient},
		stubUpdateApplier{},
		build.Info{Version: "1.2.3"},
	)

	out, err := uc.Execute(context.Background(), CheckUpdateInput{})
	if err != nil {
		t.Fatalf("Execute() unexpected error: %v", err)
	}
	if out.UpdateAvailable {
		t.Fatal("UpdateAvailable = true, want false")
	}
	if out.CurrentVersion != "1.2.3" || out.LatestVersion != "1.2.3" {
		t.Fatalf("unexpected versions: current=%q latest=%q", out.CurrentVersion, out.LatestVersion)
	}
}

func TestCheckUpdateUseCase_NonTransientErrorPropagates(t *testing.T) {
	uc := NewCheckUpdateUseCase(
		stubUpdateChecker{err: errors.New("boom")},
		stubUpdateApplier{},
		build.Info{Version: "1.2.3"},
	)

	_, err := uc.Execute(context.Background(), CheckUpdateInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

type stubUpdateChecker struct {
	info *entity.UpdateInfo
	err  error
}

func (s stubUpdateChecker) CheckForUpdate(context.Context, string) (*entity.UpdateInfo, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.info, nil
}

type stubUpdateApplier struct{}

func (stubUpdateApplier) CanSelfUpdate(context.Context) bool {
	return false
}

func (stubUpdateApplier) SelfUpdateBlockedReason(context.Context) port.SelfUpdateBlockedReason {
	return port.SelfUpdateBlockedNotWritable
}

func (stubUpdateApplier) GetBinaryPath() (string, error) {
	return "", nil
}

func (stubUpdateApplier) StageUpdate(context.Context, string) error {
	return nil
}

func (stubUpdateApplier) HasStagedUpdate(context.Context) bool {
	return false
}

func (stubUpdateApplier) ApplyOnExit(context.Context) (string, error) {
	return "", nil
}

func (stubUpdateApplier) ClearStagedUpdate(context.Context) error {
	return nil
}
