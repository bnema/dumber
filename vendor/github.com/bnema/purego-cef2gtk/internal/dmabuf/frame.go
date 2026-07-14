package dmabuf

import (
	"errors"
	"fmt"
)

const supportedPlaneCount = 1

var (
	ErrInvalidCodedSize    = errors.New("invalid coded size")
	ErrUnsupportedFormat   = errors.New("unsupported dmabuf format")
	ErrUnsupportedPlanes   = errors.New("unsupported dmabuf plane count")
	ErrInvalidPlaneFD      = errors.New("invalid dmabuf plane fd")
	ErrInvalidStride       = errors.New("invalid dmabuf plane stride")
	ErrPlaneExtentOverflow = errors.New("dmabuf plane extent arithmetic overflow")
	ErrPlaneSizeTooSmall   = errors.New("dmabuf plane size too small for coded extent")
)

// Size describes pixel dimensions.
type Size struct {
	Width  int32
	Height int32
}

// Valid reports whether both dimensions are positive.
func (s Size) Valid() bool { return s.Width > 0 && s.Height > 0 }

// Rect describes a pixel rectangle.
type Rect struct {
	X      int32
	Y      int32
	Width  int32
	Height int32
}

// Empty reports whether the rectangle has no area. Negative dimensions are treated as empty/invalid.
func (r Rect) Empty() bool { return r.Width <= 0 || r.Height <= 0 }

// Plane describes one borrowed native pixmap plane. The FD remains owned by CEF.
type Plane struct {
	FD     int
	Stride uint32
	Offset uint64
	Size   uint64
}

// BorrowedFrame describes a callback-scoped DMABUF frame. It must be imported
// and copied before the originating CEF callback returns.
type BorrowedFrame struct {
	CodedSize Size

	VisibleRect Rect
	ContentRect Rect
	SourceSize  Size

	Format   FourCC
	Modifier uint64
	Planes   []Plane
}

// checkedMulUint64 returns a*b and a boolean indicating whether the result is valid
// (false on overflow). A zero result is always valid.
func checkedMulUint64(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	result := a * b
	return result, result/b == a
}

// checkedAddUint64 returns a+b and a boolean indicating whether the result is valid
// (false on overflow).
func checkedAddUint64(a, b uint64) (uint64, bool) {
	result := a + b
	return result, result >= a
}

// Validate enforces the intentionally small initial support matrix: exactly one
// plane, one of four RGB formats, positive coded size, non-negative fd, non-zero stride,
// and overflow-safe plane extent bounds.
func (f BorrowedFrame) Validate() error {
	if !f.CodedSize.Valid() {
		return fmt.Errorf("%w: %dx%d", ErrInvalidCodedSize, f.CodedSize.Width, f.CodedSize.Height)
	}
	if !f.Format.Supported() {
		return fmt.Errorf("%w: %s", ErrUnsupportedFormat, f.Format)
	}
	if len(f.Planes) != supportedPlaneCount {
		return fmt.Errorf("%w: got %d, want %d", ErrUnsupportedPlanes, len(f.Planes), supportedPlaneCount)
	}
	plane := f.Planes[0]
	if plane.FD < 0 {
		return fmt.Errorf("%w: %d", ErrInvalidPlaneFD, plane.FD)
	}
	if plane.Stride == 0 {
		return fmt.Errorf("%w: %d", ErrInvalidStride, plane.Stride)
	}

	// Stride must be wide enough for one coded row at the format's bpp.
	bpp := uint64(f.Format.BytesPerPixel())
	minStride, ok := checkedMulUint64(uint64(f.CodedSize.Width), bpp)
	if !ok {
		return fmt.Errorf("%w: width %d * %d bytes-per-pixel overflows uint64", ErrPlaneExtentOverflow, f.CodedSize.Width, bpp)
	}
	if uint64(plane.Stride) < minStride {
		return fmt.Errorf("%w: stride %d < minimum %d (width %d * %d bytes-per-pixel)", ErrInvalidStride, plane.Stride, minStride, f.CodedSize.Width, bpp)
	}

	// Overflow-safe minimum buffer extent validation.
	stride := uint64(plane.Stride)
	height := uint64(f.CodedSize.Height)
	rowBytes, ok := checkedMulUint64(stride, height)
	if !ok {
		return fmt.Errorf("%w: stride %d * coded height %d overflows uint64", ErrPlaneExtentOverflow, plane.Stride, f.CodedSize.Height)
	}
	requiredExtent, ok := checkedAddUint64(plane.Offset, rowBytes)
	if !ok {
		return fmt.Errorf("%w: offset %d + stride*coded height %d overflows uint64", ErrPlaneExtentOverflow, plane.Offset, rowBytes)
	}
	if plane.Size > 0 && plane.Size < requiredExtent {
		return fmt.Errorf("%w: plane size %d < required minimum extent %d (offset %d + stride %d * coded height %d)", ErrPlaneSizeTooSmall, plane.Size, requiredExtent, plane.Offset, plane.Stride, f.CodedSize.Height)
	}
	return nil
}
