// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Ebitengine Authors

//go:build !go1.23

package purego

// hostLayout is a zero-size placeholder for Go versions before 1.23
// that lack the structs package. On Go 1.23+ this is replaced by
// structs.HostLayout which tells the compiler to use host memory layout.
type hostLayout struct{}
