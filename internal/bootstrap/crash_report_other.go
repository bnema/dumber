//go:build !linux && !darwin

package bootstrap

func collectCoreDumpDiagnostics() crashCoreDump {
	return crashCoreDump{
		RLimitCoreSoft: "unsupported",
		RLimitCoreHard: "unsupported",
		Hint:           "Core dump diagnostics are only available on Linux and macOS.",
	}
}
