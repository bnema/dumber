//go:build linux || darwin

package main

import (
	"context"
	"runtime/debug"
	"strconv"

	"github.com/bnema/dumber/internal/logging"
	"golang.org/x/sys/unix"
)

func enableCrashForensics() {
	debug.SetTraceback("crash")

	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &limit); err != nil {
		return
	}
	if limit.Cur >= limit.Max {
		return
	}
	limit.Cur = limit.Max
	_ = unix.Setrlimit(unix.RLIMIT_CORE, &limit)
}

func logCoreDumpLimits(ctx context.Context) {
	log := logging.FromContext(ctx)
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &limit); err != nil {
		log.Debug().Err(err).Msg("failed to read RLIMIT_CORE")
		return
	}

	log.Debug().
		Str("soft", formatRlimit(limit.Cur)).
		Str("hard", formatRlimit(limit.Max)).
		Msg("core dump limits")
}

func formatRlimit(value uint64) string {
	if value == unix.RLIM_INFINITY {
		return "infinity"
	}
	return strconv.FormatUint(value, 10)
}
