// Package cefadapter converts purego-cef callback data into internal models.
package cefadapter

import (
	"errors"
	"fmt"

	"github.com/bnema/purego-cef/cef"

	"github.com/bnema/purego-cef2gtk/internal/dmabuf"
)

var (
	ErrNilAcceleratedPaintInfo = errors.New("nil accelerated paint info")
	ErrUnsupportedColorType    = errors.New("unsupported CEF color type")
)

// BorrowedFrameFromAcceleratedPaint converts CEF accelerated paint metadata to a
// callback-scoped borrowed DMABUF frame description. New single-plane consumers
// should use SinglePlaneFrameFromAcceleratedPaint to avoid allocating this slice.
func BorrowedFrameFromAcceleratedPaint(info *cef.AcceleratedPaintInfo) (dmabuf.BorrowedFrame, error) {
	frame, err := SinglePlaneFrameFromAcceleratedPaint(info)
	if err != nil {
		return dmabuf.BorrowedFrame{}, err
	}
	return dmabuf.BorrowedFrame{
		CodedSize:   frame.CodedSize,
		VisibleRect: frame.VisibleRect,
		ContentRect: frame.ContentRect,
		SourceSize:  frame.SourceSize,
		Format:      frame.Format,
		Modifier:    frame.Modifier,
		Planes:      []dmabuf.Plane{frame.Plane},
	}, nil
}

// SinglePlaneFrameFromAcceleratedPaint converts the supported one-plane CEF
// metadata without allocating a plane slice. The returned FD is callback-scoped
// and remains owned by CEF.
func SinglePlaneFrameFromAcceleratedPaint(info *cef.AcceleratedPaintInfo) (dmabuf.SinglePlaneFrame, error) {
	if info == nil {
		return dmabuf.SinglePlaneFrame{}, ErrNilAcceleratedPaintInfo
	}
	if info.PlaneCount != 1 {
		return dmabuf.SinglePlaneFrame{}, fmt.Errorf("%w: got %d, want 1", dmabuf.ErrUnsupportedPlanes, info.PlaneCount)
	}
	plane := info.Planes[0]
	frame := dmabuf.SinglePlaneFrame{
		CodedSize:   dmabuf.Size{Width: info.Extra.CodedSize.Width, Height: info.Extra.CodedSize.Height},
		VisibleRect: dmabuf.Rect{X: info.Extra.VisibleRect.X, Y: info.Extra.VisibleRect.Y, Width: info.Extra.VisibleRect.Width, Height: info.Extra.VisibleRect.Height},
		ContentRect: dmabuf.Rect{X: info.Extra.ContentRect.X, Y: info.Extra.ContentRect.Y, Width: info.Extra.ContentRect.Width, Height: info.Extra.ContentRect.Height},
		SourceSize:  dmabuf.Size{Width: info.Extra.SourceSize.Width, Height: info.Extra.SourceSize.Height},
		Format:      dmabufFormatFromCEFColorType(cef.ColorType(info.Format)),
		Modifier:    info.Modifier,
		Plane: dmabuf.Plane{
			FD:     int(plane.Fd),
			Stride: plane.Stride,
			Offset: plane.Offset,
			Size:   plane.Size,
		},
	}
	if frame.Format == 0 {
		return dmabuf.SinglePlaneFrame{}, fmt.Errorf("%w: %d", ErrUnsupportedColorType, info.Format)
	}
	if err := frame.Validate(); err != nil {
		return dmabuf.SinglePlaneFrame{}, err
	}
	return frame, nil
}

func dmabufFormatFromCEFColorType(colorType cef.ColorType) dmabuf.FourCC {
	switch colorType {
	case cef.ColorTypeBgra8888:
		// CEF BGRA byte order corresponds to DRM_FORMAT_ARGB8888 on little-endian Linux.
		return dmabuf.FormatARGB8888
	case cef.ColorTypeRgba8888:
		// CEF RGBA byte order corresponds to DRM_FORMAT_ABGR8888 on little-endian Linux.
		return dmabuf.FormatABGR8888
	default:
		return 0
	}
}
