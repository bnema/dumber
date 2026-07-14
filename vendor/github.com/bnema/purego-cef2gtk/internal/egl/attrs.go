package egl

import (
	"errors"
	"fmt"
	"math"

	"github.com/bnema/purego-cef2gtk/internal/dmabuf"
)

const (
	// ExtensionDMABUFImportModifiers gates EGL_DMA_BUF_PLANE*_MODIFIER_*_EXT attrs.
	ExtensionDMABUFImportModifiers = "EGL_EXT_image_dma_buf_import_modifiers"

	attributeNone                 Attribute = 0x3038
	attributeHeight               Attribute = 0x3056
	attributeWidth                Attribute = 0x3057
	attributeLinuxDRMFourCC       Attribute = 0x3271
	attributeDMABUFPlane0FD       Attribute = 0x3272
	attributeDMABUFPlane0Offset   Attribute = 0x3273
	attributeDMABUFPlane0Pitch    Attribute = 0x3274
	attributeDMABUFPlane0ModLoEXT Attribute = 0x3443
	attributeDMABUFPlane0ModHiEXT Attribute = 0x3444
)

var (
	ErrInvalidOffset          = errors.New("invalid EGL DMABUF plane offset")
	ErrInvalidStrideAttribute = errors.New("invalid EGL DMABUF plane stride attribute")
	ErrInvalidFDAttribute     = errors.New("invalid EGL DMABUF plane fd attribute")
)

// Attribute is an EGLint image attribute key or value.
type Attribute int32

// DMABUFImageAttributes builds the EGL image attribute list for importing the
// supported one-plane RGB DMABUF frame shape. The returned list is terminated by
// EGL_NONE.
func DMABUFImageAttributes(frame dmabuf.BorrowedFrame, extensions Extensions) ([]Attribute, error) {
	if err := frame.Validate(); err != nil {
		return nil, fmt.Errorf("build EGL DMABUF image attributes: %w", err)
	}
	if !extensions.Has(ExtensionDMABUFImport) {
		return nil, fmt.Errorf("build EGL DMABUF image attributes: %w: %s", ErrMissingDisplaySupport, ExtensionDMABUFImport)
	}
	if frame.Planes[0].FD > math.MaxInt32 {
		return nil, fmt.Errorf("build EGL DMABUF image attributes: %w: %d", ErrInvalidFDAttribute, frame.Planes[0].FD)
	}
	if frame.Planes[0].Offset > math.MaxInt32 {
		return nil, fmt.Errorf("build EGL DMABUF image attributes: %w: %d", ErrInvalidOffset, frame.Planes[0].Offset)
	}
	if frame.Planes[0].Stride > math.MaxInt32 {
		return nil, fmt.Errorf("build EGL DMABUF image attributes: %w: %d", ErrInvalidStrideAttribute, frame.Planes[0].Stride)
	}

	// EGL expects these values as EGLint attributes. File descriptors are kernel
	// ints and already validated positive by dmabuf.BorrowedFrame.Validate.
	// FourCC and modifier hi/lo values are raw bit patterns, so casts to Attribute
	// intentionally preserve those bits even when their unsigned representation
	// exceeds MaxInt32.
	attrs := []Attribute{
		attributeWidth, Attribute(frame.CodedSize.Width),
		attributeHeight, Attribute(frame.CodedSize.Height),
		attributeLinuxDRMFourCC, Attribute(frame.Format),
		attributeDMABUFPlane0FD, Attribute(frame.Planes[0].FD),
		attributeDMABUFPlane0Offset, Attribute(frame.Planes[0].Offset),
		attributeDMABUFPlane0Pitch, Attribute(frame.Planes[0].Stride),
	}

	if dmabuf.ModifierHasExplicitAttrs(frame.Modifier) {
		if !extensions.Has(ExtensionDMABUFImportModifiers) {
			return nil, fmt.Errorf("build EGL DMABUF image attributes: %w: %s", ErrMissingDisplaySupport, ExtensionDMABUFImportModifiers)
		}
		hi, lo := dmabuf.ModifierHiLo(frame.Modifier)
		attrs = append(attrs,
			attributeDMABUFPlane0ModLoEXT, Attribute(lo),
			attributeDMABUFPlane0ModHiEXT, Attribute(hi),
		)
	}

	attrs = append(attrs, attributeNone)
	return attrs, nil
}
