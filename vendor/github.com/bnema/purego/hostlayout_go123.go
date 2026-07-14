// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Ebitengine Authors

//go:build go1.23

package purego

import "structs"

// hostLayout is structs.HostLayout on Go 1.23+, ensuring the compiler
// uses host (C-compatible) memory layout for structs that embed it.
type hostLayout = structs.HostLayout
