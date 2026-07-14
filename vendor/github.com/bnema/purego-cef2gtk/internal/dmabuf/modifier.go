package dmabuf

const (
	// ModifierLinear is DRM_FORMAT_MOD_LINEAR.
	ModifierLinear uint64 = 0
	// ModifierInvalid is DRM_FORMAT_MOD_INVALID and means no explicit modifier.
	ModifierInvalid uint64 = ^uint64(0)
)

// ModifierHiLo splits a DRM format modifier into the high/low uint32 values used
// by EGL_DMA_BUF_PLANE*_MODIFIER_HI/LO_EXT attributes.
func ModifierHiLo(modifier uint64) (hi, lo uint32) {
	return uint32(modifier >> 32), uint32(modifier)
}

// ModifierHasExplicitAttrs reports whether EGL modifier attributes should be emitted.
func ModifierHasExplicitAttrs(modifier uint64) bool {
	return modifier != ModifierInvalid
}
