// Package dmabuf models borrowed Linux DMABUF frames received from CEF.
package dmabuf

import "fmt"

// FourCC is a DRM format code.
type FourCC uint32

const (
	FormatARGB8888 FourCC = FourCC(uint32('A') | uint32('R')<<8 | uint32('2')<<16 | uint32('4')<<24)
	FormatXRGB8888 FourCC = FourCC(uint32('X') | uint32('R')<<8 | uint32('2')<<16 | uint32('4')<<24)
	FormatABGR8888 FourCC = FourCC(uint32('A') | uint32('B')<<8 | uint32('2')<<16 | uint32('4')<<24)
	FormatXBGR8888 FourCC = FourCC(uint32('X') | uint32('B')<<8 | uint32('2')<<16 | uint32('4')<<24)
)

// Supported reports whether the format is in the initial one-plane RGB allowlist.
func (f FourCC) Supported() bool {
	switch f {
	case FormatARGB8888, FormatXRGB8888, FormatABGR8888, FormatXBGR8888:
		return true
	default:
		return false
	}
}

// BytesPerPixel returns bytes per pixel for the four supported RGB formats.
// Returns 0 for unsupported or unknown formats.
func (f FourCC) BytesPerPixel() int {
	switch f {
	case FormatARGB8888, FormatXRGB8888, FormatABGR8888, FormatXBGR8888:
		return 4
	default:
		return 0
	}
}

func (f FourCC) String() string {
	if f == 0 {
		return "<zero>"
	}
	b := [4]byte{byte(f), byte(f >> 8), byte(f >> 16), byte(f >> 24)}
	for _, c := range b {
		if c < 0x20 || c > 0x7e {
			return fmt.Sprintf("FourCC(0x%08x)", uint32(f))
		}
	}
	return string(b[:])
}
