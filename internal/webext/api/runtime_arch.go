package api

import "runtime"

// getPlatformArchConst returns the CPU architecture
// Reference: https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/API/runtime/PlatformArch
func getPlatformArchConst() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86-64"
	case "386":
		return "x86-32"
	case "arm64", "arm":
		return "arm"
	default:
		return "unknown"
	}
}
