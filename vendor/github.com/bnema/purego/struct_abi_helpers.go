// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The Ebitengine Authors

package purego

import (
	"math"
	"reflect"
	"unsafe"
)

func packFloat32Pair(low, high uintptr) uintptr {
	return uintptr((uint64(high) << 32) | (uint64(low) & math.MaxUint32))
}

func highFloat32Bits(word uintptr) uint32 {
	return uint32(uint64(word) >> 32)
}

func flushPackedWord(val *uint64, shift *byte, flushed *bool, isFloat bool, addFloat, addInt func(uintptr)) {
	*flushed = true
	if isFloat {
		addFloat(uintptr(*val))
	} else {
		addInt(uintptr(*val))
	}
	*val = 0
	*shift = 0
}

func stableValuePointer(v reflect.Value) (unsafe.Pointer, any) {
	if v.CanAddr() {
		return v.Addr().UnsafePointer(), nil
	}
	tmp := reflect.New(v.Type())
	tmp.Elem().Set(v)
	return tmp.UnsafePointer(), tmp.Interface()
}
