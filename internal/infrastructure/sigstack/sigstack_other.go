//go:build !(linux && (amd64 || arm64))

package sigstack

// Repair is a no-op outside linux/amd64 and linux/arm64.
func Repair(_ ...int) Result { return Result{} }
