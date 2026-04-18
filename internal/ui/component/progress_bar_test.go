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
