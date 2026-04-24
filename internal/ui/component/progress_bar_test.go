package component

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestProgressBarTimeoutRemainingLocked_UsesLatestProgressEvent(t *testing.T) {
	showAt := time.Unix(0, 0)
	progressAt := showAt.Add(20 * time.Second)
	pb := &ProgressBar{
		lastShowAt:     showAt,
		lastProgressAt: progressAt,
	}

	remaining := pb.timeoutRemainingLocked(showAt.Add(25 * time.Second))

	require.Equal(t, 25*time.Second, remaining)
}

func TestProgressBarTimeoutRemainingLocked_UsesShowTimeBeforeFirstProgress(t *testing.T) {
	showAt := time.Unix(0, 0)
	pb := &ProgressBar{lastShowAt: showAt}

	remaining := pb.timeoutRemainingLocked(showAt.Add(31 * time.Second))

	require.Zero(t, remaining)
}

func TestProgressBarTimeoutRemainingLocked_ZeroTimestamps(t *testing.T) {
	pb := &ProgressBar{}

	remaining := pb.timeoutRemainingLocked(time.Unix(10, 0))

	require.Zero(t, remaining)
}

func TestProgressBarTimeoutRemainingLocked_EqualTimestamps(t *testing.T) {
	showAt := time.Unix(0, 0)
	pb := &ProgressBar{
		lastShowAt:     showAt,
		lastProgressAt: showAt,
	}

	remaining := pb.timeoutRemainingLocked(showAt.Add(10 * time.Second))

	require.Equal(t, 20*time.Second, remaining)
}

func TestProgressBarTimeoutRemainingLocked_NegativeClampsToZero(t *testing.T) {
	showAt := time.Unix(0, 0)
	pb := &ProgressBar{
		lastShowAt:     showAt,
		lastProgressAt: showAt.Add(5 * time.Second),
	}

	remaining := pb.timeoutRemainingLocked(showAt.Add(36 * time.Second))

	require.Zero(t, remaining)
}
