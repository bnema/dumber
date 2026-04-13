package cef

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
)

// OpenGL constants for PBO-based texture rendering.
const (
	glTexture2D    = 0x0DE1
	glBGRA         = 0x80E1
	glRGBA         = 0x1908
	glUnsignedByte = 0x1401

	glPixelUnpackBuffer = 0x88EC
	glStreamDraw        = 0x88E0
	glWriteOnly         = 0x88B9

	glColorBufferBit = 0x00004000
	glTriangleStrip  = 0x0005

	glFragmentShader = 0x8B30
	glVertexShader   = 0x8B31
	glFloat          = 0x1406

	glArrayBuffer = 0x8892
	glStaticDraw  = 0x88E4

	glLinear           = 0x2601
	glTextureMinFilter = 0x2801
	glTextureMagFilter = 0x2800
	glClampToEdge      = 0x812F
	glTextureWrapS     = 0x2802
	glTextureWrapT     = 0x2803

	glUnpackRowLength = 0x0CF2
)

// glLoader holds dynamically loaded OpenGL function pointers.
// All functions are loaded from libGL.so.1 via purego at runtime (no CGo).
type glLoader struct {
	handle uintptr

	// Texture functions
	genTextures    func(n int32, textures *uint32)
	deleteTextures func(n int32, textures *uint32)
	bindTexture    func(target uint32, texture uint32)
	texImage2D     func(
		target uint32, level, internalformat, width, height, border int32,
		format, xtype uint32, pixels unsafe.Pointer,
	)
	texSubImage2D func(
		target uint32, level, xoffset, yoffset, width, height int32,
		format, xtype uint32, pixels unsafe.Pointer,
	)
	texParameteri func(target uint32, pname uint32, param int32)

	// PBO / buffer functions
	genBuffers    func(n int32, buffers *uint32)
	deleteBuffers func(n int32, buffers *uint32)
	bindBuffer    func(target uint32, buffer uint32)
	bufferData    func(target uint32, size int64, data unsafe.Pointer, usage uint32)
	mapBuffer     func(target uint32, access uint32) unsafe.Pointer
	unmapBuffer   func(target uint32) bool

	// Shader functions
	createShader       func(shaderType uint32) uint32
	shaderSource       func(shader uint32, count int32, source **byte, length *int32)
	compileShader      func(shader uint32)
	deleteShader       func(shader uint32)
	createProgram      func() uint32
	attachShader       func(program uint32, shader uint32)
	linkProgram        func(program uint32)
	useProgram         func(program uint32)
	getUniformLocation func(program uint32, name *byte) int32
	uniform1i          func(location int32, v0 int32)

	// VAO / VBO functions
	genVertexArrays         func(n int32, arrays *uint32)
	bindVertexArray         func(array uint32)
	deleteVertexArrays      func(n int32, arrays *uint32)
	vertexAttribPointer     func(index uint32, size int32, xtype uint32, normalized bool, stride int32, pointer uintptr)
	enableVertexAttribArray func(index uint32)

	// Pixel store
	pixelStorei func(pname uint32, param int32)

	// Draw / state functions
	clear      func(mask uint32)
	clearColor func(red float32, green float32, blue float32, alpha float32)
	viewport   func(x int32, y int32, width int32, height int32)
	drawArrays func(mode uint32, first int32, count int32)
	enable     func(cap uint32)
	disable    func(cap uint32)
}

// Close releases the OpenGL library handle.
func (gl *glLoader) Close() error {
	if gl == nil {
		return nil
	}
	if gl.handle != 0 {
		err := purego.Dlclose(gl.handle)
		gl.handle = 0
		return err
	}
	return nil
}

// newGLLoader opens libGL.so.1 and loads all required OpenGL function pointers.
func newGLLoader() (*glLoader, error) {
	handle, err := purego.Dlopen("libGL.so.1", purego.RTLD_LAZY)
	if err != nil {
		return nil, fmt.Errorf("failed to open libGL.so.1: %w", err)
	}

	gl := &glLoader{handle: handle}

	if err := gl.registerAll(); err != nil {
		return nil, fmt.Errorf("failed to register GL functions: %w", err)
	}

	return gl, nil
}

// registerAll loads every GL function pointer from the shared library.
// It wraps RegisterLibFunc panics into errors for graceful handling.
func (gl *glLoader) registerAll() (retErr error) {
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("GL symbol lookup failed: %v", r)
		}
	}()

	// Texture
	purego.RegisterLibFunc(&gl.genTextures, gl.handle, "glGenTextures")
	purego.RegisterLibFunc(&gl.deleteTextures, gl.handle, "glDeleteTextures")
	purego.RegisterLibFunc(&gl.bindTexture, gl.handle, "glBindTexture")
	purego.RegisterLibFunc(&gl.texImage2D, gl.handle, "glTexImage2D")
	purego.RegisterLibFunc(&gl.texSubImage2D, gl.handle, "glTexSubImage2D")
	purego.RegisterLibFunc(&gl.texParameteri, gl.handle, "glTexParameteri")

	// PBO / buffers
	purego.RegisterLibFunc(&gl.genBuffers, gl.handle, "glGenBuffers")
	purego.RegisterLibFunc(&gl.deleteBuffers, gl.handle, "glDeleteBuffers")
	purego.RegisterLibFunc(&gl.bindBuffer, gl.handle, "glBindBuffer")
	purego.RegisterLibFunc(&gl.bufferData, gl.handle, "glBufferData")
	purego.RegisterLibFunc(&gl.mapBuffer, gl.handle, "glMapBuffer")
	purego.RegisterLibFunc(&gl.unmapBuffer, gl.handle, "glUnmapBuffer")

	// Shaders
	purego.RegisterLibFunc(&gl.createShader, gl.handle, "glCreateShader")
	purego.RegisterLibFunc(&gl.shaderSource, gl.handle, "glShaderSource")
	purego.RegisterLibFunc(&gl.compileShader, gl.handle, "glCompileShader")
	purego.RegisterLibFunc(&gl.deleteShader, gl.handle, "glDeleteShader")
	purego.RegisterLibFunc(&gl.createProgram, gl.handle, "glCreateProgram")
	purego.RegisterLibFunc(&gl.attachShader, gl.handle, "glAttachShader")
	purego.RegisterLibFunc(&gl.linkProgram, gl.handle, "glLinkProgram")
	purego.RegisterLibFunc(&gl.useProgram, gl.handle, "glUseProgram")
	purego.RegisterLibFunc(&gl.getUniformLocation, gl.handle, "glGetUniformLocation")
	purego.RegisterLibFunc(&gl.uniform1i, gl.handle, "glUniform1i")

	// VAO / VBO
	purego.RegisterLibFunc(&gl.genVertexArrays, gl.handle, "glGenVertexArrays")
	purego.RegisterLibFunc(&gl.bindVertexArray, gl.handle, "glBindVertexArray")
	purego.RegisterLibFunc(&gl.deleteVertexArrays, gl.handle, "glDeleteVertexArrays")
	purego.RegisterLibFunc(&gl.vertexAttribPointer, gl.handle, "glVertexAttribPointer")
	purego.RegisterLibFunc(&gl.enableVertexAttribArray, gl.handle, "glEnableVertexAttribArray")

	// Pixel store
	purego.RegisterLibFunc(&gl.pixelStorei, gl.handle, "glPixelStorei")

	// Draw / state
	purego.RegisterLibFunc(&gl.clear, gl.handle, "glClear")
	purego.RegisterLibFunc(&gl.clearColor, gl.handle, "glClearColor")
	purego.RegisterLibFunc(&gl.viewport, gl.handle, "glViewport")
	purego.RegisterLibFunc(&gl.drawArrays, gl.handle, "glDrawArrays")
	purego.RegisterLibFunc(&gl.enable, gl.handle, "glEnable")
	purego.RegisterLibFunc(&gl.disable, gl.handle, "glDisable")

	return nil
}
