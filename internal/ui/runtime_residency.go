package ui

import "time"

// ResidencyTimeoutFromMillis resolves the effective bounded-residency idle
// timeout for CEF from the startup configuration value in milliseconds.
// Values outside 0..300000 are rejected by config validation; this resolver
// additionally clamps defensively so an unvalidated caller can never arm an
// unbounded or negative deadline.
func ResidencyTimeoutFromMillis(timeoutMs int) time.Duration {
	if timeoutMs <= 0 {
		return 0
	}
	if timeoutMs > 300000 {
		timeoutMs = 300000
	}
	return time.Duration(timeoutMs) * time.Millisecond
}

// SetResidencyTimeout latches the effective idle timeout once at process
// startup. Call it before App.Run; later config reloads must use
// ResidencyTimeoutRestartRequired instead of calling this again.
func (a *App) SetResidencyTimeout(timeout time.Duration) {
	if a == nil {
		return
	}
	a.residencyTimeout = timeout
}

// LatchedResidencyTimeout returns the startup-latched idle timeout.
func (a *App) LatchedResidencyTimeout() time.Duration {
	if a == nil {
		return 0
	}
	return a.residencyTimeout
}

// ResidencyTimeoutRestartRequired reports whether a reloaded configuration
// value differs from the startup latch. A true result means the process must
// restart to apply it; residency is never partially hot-applied.
func (a *App) ResidencyTimeoutRestartRequired(timeoutMs int) bool {
	if a == nil {
		return false
	}
	return ResidencyTimeoutFromMillis(timeoutMs) != a.residencyTimeout
}
