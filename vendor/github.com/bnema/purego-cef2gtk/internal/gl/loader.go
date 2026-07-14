// Package gl contains purego OpenGL helpers used by the accelerated renderer.
package gl

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/bnema/purego"
)

const (
	Texture2D    uint32 = 0x0DE1
	RGBA         uint32 = 0x1908
	UnsignedByte uint32 = 0x1401

	ColorBufferBit uint32 = 0x00004000
	TriangleStrip  uint32 = 0x0005
	Triangles      uint32 = 0x0004

	FragmentShader uint32 = 0x8B30
	VertexShader   uint32 = 0x8B31
	Float          uint32 = 0x1406

	ArrayBuffer uint32 = 0x8892
	StaticDraw  uint32 = 0x88E4

	Linear           uint32 = 0x2601
	TextureMinFilter uint32 = 0x2801
	TextureMagFilter uint32 = 0x2800
	ClampToEdge      uint32 = 0x812F
	TextureWrapS     uint32 = 0x2802
	TextureWrapT     uint32 = 0x2803

	Texture0 uint32 = 0x84C0

	Framebuffer                     uint32 = 0x8D40
	DrawFramebuffer                 uint32 = 0x8CA9
	ColorAttachment0                uint32 = 0x8CE0
	FramebufferComplete             uint32 = 0x8CD5
	FramebufferIncompleteAttachment uint32 = 0x8CD6

	CompileStatus uint32 = 0x8B81
	LinkStatus    uint32 = 0x8B82
	InfoLogLength uint32 = 0x8B84
)

// Loader holds dynamically loaded OpenGL function pointers.
type Loader struct {
	handle uintptr

	genTextures    func(n int32, textures *uint32)
	deleteTextures func(n int32, textures *uint32)
	bindTexture    func(target uint32, texture uint32)
	texImage2D     func(target uint32, level, internalformat, width, height, border int32, format, xtype uint32, pixels unsafe.Pointer)
	texParameteri  func(target uint32, pname uint32, param int32)
	activeTexture  func(texture uint32)

	genBuffers    func(n int32, buffers *uint32)
	deleteBuffers func(n int32, buffers *uint32)
	bindBuffer    func(target uint32, buffer uint32)
	bufferData    func(target uint32, size int64, data unsafe.Pointer, usage uint32)

	createShader     func(shaderType uint32) uint32
	shaderSource     func(shader uint32, count int32, source **byte, length *int32)
	compileShader    func(shader uint32)
	getShaderiv      func(shader uint32, pname uint32, params *int32)
	getShaderInfoLog func(shader uint32, bufSize int32, length *int32, infoLog *byte)
	deleteShader     func(shader uint32)

	createProgram      func() uint32
	attachShader       func(program uint32, shader uint32)
	linkProgram        func(program uint32)
	getProgramiv       func(program uint32, pname uint32, params *int32)
	getProgramInfoLog  func(program uint32, bufSize int32, length *int32, infoLog *byte)
	useProgram         func(program uint32)
	deleteProgram      func(program uint32)
	getUniformLocation func(program uint32, name *byte) int32
	getAttribLocation  func(program uint32, name *byte) int32
	uniform1i          func(location int32, v0 int32)

	genVertexArrays          func(n int32, arrays *uint32)
	bindVertexArray          func(array uint32)
	deleteVertexArrays       func(n int32, arrays *uint32)
	vertexAttribPointer      func(index uint32, size int32, xtype uint32, normalized bool, stride int32, pointer uintptr)
	enableVertexAttribArray  func(index uint32)
	disableVertexAttribArray func(index uint32)

	genFramebuffers        func(n int32, framebuffers *uint32)
	deleteFramebuffers     func(n int32, framebuffers *uint32)
	bindFramebuffer        func(target uint32, framebuffer uint32)
	framebufferTexture2D   func(target uint32, attachment uint32, textarget uint32, texture uint32, level int32)
	checkFramebufferStatus func(target uint32) uint32

	clear      func(mask uint32)
	clearColor func(red, green, blue, alpha float32)
	viewport   func(x, y, width, height int32)
	drawArrays func(mode uint32, first, count int32)
	getError   func() uint32
	readPixels func(x, y, width, height int32, format, xtype uint32, pixels unsafe.Pointer)

	genQueries          func(n int32, ids *uint32)
	deleteQueries       func(n int32, ids *uint32)
	beginQuery          func(target uint32, id uint32)
	endQuery            func(target uint32)
	getQueryObjectuiv   func(id uint32, pname uint32, params *uint32)
	getQueryObjectui64v func(id uint32, pname uint32, params *uint64)

	eglImageTargetTexture2DOES func(target uint32, image uintptr)
}

// NewLoader opens libGL and loads required OpenGL function pointers.
func NewLoader() (*Loader, error) {
	var handle uintptr
	var err error
	for _, name := range []string{"libGL.so.1", "libOpenGL.so.0", "libGL.so"} {
		handle, err = purego.Dlopen(name, purego.RTLD_LAZY)
		if err == nil {
			break
		}
	}
	if handle == 0 {
		return nil, fmt.Errorf("open OpenGL library: %w", err)
	}

	loader := &Loader{handle: handle}
	if err := loader.registerAll(); err != nil {
		if closeErr := loader.Close(); closeErr != nil {
			return nil, errors.Join(err, fmt.Errorf("close OpenGL library after registration failure: %w", closeErr))
		}
		return nil, err
	}
	return loader, nil
}

// Close releases the OpenGL library handle.
func (l *Loader) Close() error {
	if l == nil || l.handle == 0 {
		return nil
	}
	err := purego.Dlclose(l.handle)
	l.handle = 0
	return err
}

func (l *Loader) registerAll() (retErr error) {
	currentSymbol := ""
	defer func() {
		if r := recover(); r != nil {
			retErr = fmt.Errorf("register OpenGL function %s: %v", currentSymbol, r)
		}
	}()

	register := func(dst any, name string) {
		currentSymbol = name
		purego.RegisterLibFunc(dst, l.handle, name)
	}

	register(&l.genTextures, "glGenTextures")
	register(&l.deleteTextures, "glDeleteTextures")
	register(&l.bindTexture, "glBindTexture")
	register(&l.texImage2D, "glTexImage2D")
	register(&l.texParameteri, "glTexParameteri")
	register(&l.activeTexture, "glActiveTexture")

	register(&l.genBuffers, "glGenBuffers")
	register(&l.deleteBuffers, "glDeleteBuffers")
	register(&l.bindBuffer, "glBindBuffer")
	register(&l.bufferData, "glBufferData")

	register(&l.createShader, "glCreateShader")
	register(&l.shaderSource, "glShaderSource")
	register(&l.compileShader, "glCompileShader")
	register(&l.getShaderiv, "glGetShaderiv")
	register(&l.getShaderInfoLog, "glGetShaderInfoLog")
	register(&l.deleteShader, "glDeleteShader")

	register(&l.createProgram, "glCreateProgram")
	register(&l.attachShader, "glAttachShader")
	register(&l.linkProgram, "glLinkProgram")
	register(&l.getProgramiv, "glGetProgramiv")
	register(&l.getProgramInfoLog, "glGetProgramInfoLog")
	register(&l.useProgram, "glUseProgram")
	register(&l.deleteProgram, "glDeleteProgram")
	register(&l.getUniformLocation, "glGetUniformLocation")
	register(&l.getAttribLocation, "glGetAttribLocation")
	register(&l.uniform1i, "glUniform1i")

	register(&l.genVertexArrays, "glGenVertexArrays")
	register(&l.bindVertexArray, "glBindVertexArray")
	register(&l.deleteVertexArrays, "glDeleteVertexArrays")
	register(&l.vertexAttribPointer, "glVertexAttribPointer")
	register(&l.enableVertexAttribArray, "glEnableVertexAttribArray")
	register(&l.disableVertexAttribArray, "glDisableVertexAttribArray")

	register(&l.genFramebuffers, "glGenFramebuffers")
	register(&l.deleteFramebuffers, "glDeleteFramebuffers")
	register(&l.bindFramebuffer, "glBindFramebuffer")
	register(&l.framebufferTexture2D, "glFramebufferTexture2D")
	register(&l.checkFramebufferStatus, "glCheckFramebufferStatus")

	register(&l.clear, "glClear")
	register(&l.clearColor, "glClearColor")
	register(&l.viewport, "glViewport")
	register(&l.drawArrays, "glDrawArrays")
	register(&l.getError, "glGetError")
	register(&l.readPixels, "glReadPixels")

	l.registerOptional("glGenQueries", &l.genQueries)
	l.registerOptional("glDeleteQueries", &l.deleteQueries)
	l.registerOptional("glBeginQuery", &l.beginQuery)
	l.registerOptional("glEndQuery", &l.endQuery)
	l.registerOptional("glGetQueryObjectuiv", &l.getQueryObjectuiv)
	l.registerOptional("glGetQueryObjectui64v", &l.getQueryObjectui64v)

	// glEGLImageTargetTexture2DOES is extension-provided on some stacks. Keep the
	// core loader usable when absent; import callers return a clear error instead.
	if sym := l.extensionProc("glEGLImageTargetTexture2DOES"); sym != 0 {
		purego.RegisterFunc(&l.eglImageTargetTexture2DOES, sym)
	}

	return nil
}

func (l *Loader) registerOptional(name string, dst any) {
	if sym := l.extensionProc(name); sym != 0 {
		purego.RegisterFunc(dst, sym)
	}
}

func (l *Loader) extensionProc(name string) uintptr {
	if sym, err := purego.Dlsym(l.handle, name); err == nil {
		return sym
	}

	var handle uintptr
	for _, lib := range []string{"libEGL.so.1", "libEGL.so"} {
		var err error
		handle, err = purego.Dlopen(lib, purego.RTLD_LAZY)
		if err == nil {
			break
		}
	}
	if handle == 0 {
		return 0
	}
	defer func() {
		_ = purego.Dlclose(handle) // best-effort cleanup for temporary EGL proc lookup
	}()

	var eglGetProcAddress func(*byte) uintptr
	proc, err := purego.Dlsym(handle, "eglGetProcAddress")
	if err != nil {
		return 0
	}
	purego.RegisterFunc(&eglGetProcAddress, proc)
	nameBytes := cStringBytes(name)
	addr := eglGetProcAddress(&nameBytes[0])
	runtime.KeepAlive(nameBytes)
	return addr
}
