//go:build !linux && !darwin

package bootstrap

func collectCoreDumpDiagnostics() unexpectedCloseCoreDump {
	return unexpectedCloseCoreDump{
		RLimitCoreSoft: "unsupported",
		RLimitCoreHard: "unsupported",
		Hint:           "Core dump diagnostics are only available on Linux and macOS.",
	}
}
