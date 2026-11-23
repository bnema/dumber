//go:build !(linux && (amd64 || arm64))

package cef

// repairSignalOnStack is a no-op outside linux/amd64 and linux/arm64.
func repairSignalOnStack() signalOnStackResult { return signalOnStackResult{} }
