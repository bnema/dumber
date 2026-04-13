//go:build linux || darwin

package bootstrap

import (
	"strconv"

	"golang.org/x/sys/unix"
)

const unknownDiag = "unknown"

func collectCoreDumpDiagnostics() crashCoreDump {
	d := crashCoreDump{
		Hint: "Run `coredumpctl list | rg -i \"dumber\"` and include matching entries in the issue.",
	}
	var limit unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_CORE, &limit); err != nil {
		d.RLimitCoreSoft = unknownDiag
		d.RLimitCoreHard = unknownDiag
		return d
	}
	d.RLimitCoreSoft = formatRlimitCore(limit.Cur)
	d.RLimitCoreHard = formatRlimitCore(limit.Max)
	return d
}

func formatRlimitCore(value uint64) string {
	if value == unix.RLIM_INFINITY {
		return "infinity"
	}
	return strconv.FormatUint(value, 10)
}
