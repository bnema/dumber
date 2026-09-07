package ui

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResidencyTimeoutFromMillis(t *testing.T) {
	require.Equal(t, time.Duration(0), ResidencyTimeoutFromMillis(0))
	require.Equal(t, time.Duration(0), ResidencyTimeoutFromMillis(-5))
	require.Equal(t, 60*time.Second, ResidencyTimeoutFromMillis(60000))
	require.Equal(t, 300000*time.Millisecond, ResidencyTimeoutFromMillis(300000))
	require.Equal(t, 300000*time.Millisecond, ResidencyTimeoutFromMillis(999999))
}

func TestResidencyTimeoutLatchAndRestartRequired(t *testing.T) {
	app := &App{}
	require.Equal(t, time.Duration(0), app.LatchedResidencyTimeout())

	app.SetResidencyTimeout(60 * time.Second)
	require.Equal(t, 60*time.Second, app.LatchedResidencyTimeout())
	require.False(t, app.ResidencyTimeoutRestartRequired(60000))
	require.True(t, app.ResidencyTimeoutRestartRequired(0))
	require.True(t, app.ResidencyTimeoutRestartRequired(120000))

	// A nil app never panics and reports no change.
	var nilApp *App
	nilApp.SetResidencyTimeout(time.Minute)
	require.Equal(t, time.Duration(0), nilApp.LatchedResidencyTimeout())
	require.False(t, nilApp.ResidencyTimeoutRestartRequired(60000))
}
