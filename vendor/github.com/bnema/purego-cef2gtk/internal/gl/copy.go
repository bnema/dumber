package gl

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"github.com/bnema/purego-cef2gtk/internal/dmabuf"
)

const (
	NoError          uint32 = 0
	InvalidEnum      uint32 = 0x0500
	InvalidValue     uint32 = 0x0501
	InvalidOperation uint32 = 0x0502
	OutOfMemory      uint32 = 0x0505
)

var (
	ErrInvalidTexture                      = errors.New("invalid GL texture")
	ErrInvalidSize                         = errors.New("invalid GL copy size")
	ErrMissingGLEGLImageTargetTexture2DOES = errors.New("glEGLImageTargetTexture2DOES unavailable")
)

type Texture uint32

type Program uint32

type Shader uint32

type Buffer uint32

type VertexArray uint32

type GLFramebuffer uint32

// Driver is the small GL surface used by the textured-quad copy path. Loader
// implements it; tests can provide fakes without creating a live GL context.
type Driver interface {
	GenTextures(n int32, textures *uint32)
	DeleteTextures(n int32, textures *uint32)
	BindTexture(target uint32, texture uint32)
	TexImage2D(target uint32, level, internalformat, width, height, border int32, format, xtype uint32, pixels unsafe.Pointer)
	TexParameteri(target uint32, pname uint32, param int32)
	ActiveTexture(texture uint32)

	GenBuffers(n int32, buffers *uint32)
	DeleteBuffers(n int32, buffers *uint32)
	BindBuffer(target uint32, buffer uint32)
	BufferData(target uint32, size int64, data unsafe.Pointer, usage uint32)

	CreateShader(shaderType uint32) uint32
	ShaderSource(shader uint32, count int32, source **byte, length *int32)
	CompileShader(shader uint32)
	GetShaderiv(shader uint32, pname uint32, params *int32)
	GetShaderInfoLog(shader uint32, bufSize int32, length *int32, infoLog *byte)
	DeleteShader(shader uint32)

	CreateProgram() uint32
	AttachShader(program uint32, shader uint32)
	LinkProgram(program uint32)
	GetProgramiv(program uint32, pname uint32, params *int32)
	GetProgramInfoLog(program uint32, bufSize int32, length *int32, infoLog *byte)
	UseProgram(program uint32)
	DeleteProgram(program uint32)
	GetUniformLocation(program uint32, name *byte) int32
	GetAttribLocation(program uint32, name *byte) int32
	Uniform1i(location int32, v0 int32)

	GenVertexArrays(n int32, arrays *uint32)
	BindVertexArray(array uint32)
	DeleteVertexArrays(n int32, arrays *uint32)
	VertexAttribPointer(index uint32, size int32, xtype uint32, normalized bool, stride int32, pointer uintptr)
	EnableVertexAttribArray(index uint32)
	DisableVertexAttribArray(index uint32)

	GenFramebuffers(n int32, framebuffers *uint32)
	DeleteFramebuffers(n int32, framebuffers *uint32)
	BindFramebuffer(target uint32, framebuffer uint32)
	FramebufferTexture2D(target uint32, attachment uint32, textarget uint32, texture uint32, level int32)
	CheckFramebufferStatus(target uint32) uint32

	Viewport(x, y, width, height int32)
	DrawArrays(mode uint32, first, count int32)
	GetError() uint32
}

// Loader-backed Driver methods.
func (l *Loader) GenTextures(n int32, textures *uint32)     { l.genTextures(n, textures) }
func (l *Loader) DeleteTextures(n int32, textures *uint32)  { l.deleteTextures(n, textures) }
func (l *Loader) BindTexture(target uint32, texture uint32) { l.bindTexture(target, texture) }
func (l *Loader) TexImage2D(target uint32, level, internalformat, width, height, border int32, format, xtype uint32, pixels unsafe.Pointer) {
	l.texImage2D(target, level, internalformat, width, height, border, format, xtype, pixels)
}
func (l *Loader) TexParameteri(target uint32, pname uint32, param int32) {
	l.texParameteri(target, pname, param)
}
func (l *Loader) ActiveTexture(texture uint32)            { l.activeTexture(texture) }
func (l *Loader) GenBuffers(n int32, buffers *uint32)     { l.genBuffers(n, buffers) }
func (l *Loader) DeleteBuffers(n int32, buffers *uint32)  { l.deleteBuffers(n, buffers) }
func (l *Loader) BindBuffer(target uint32, buffer uint32) { l.bindBuffer(target, buffer) }
func (l *Loader) BufferData(target uint32, size int64, data unsafe.Pointer, usage uint32) {
	l.bufferData(target, size, data, usage)
}
func (l *Loader) CreateShader(shaderType uint32) uint32 { return l.createShader(shaderType) }
func (l *Loader) ShaderSource(shader uint32, count int32, source **byte, length *int32) {
	l.shaderSource(shader, count, source, length)
}
func (l *Loader) CompileShader(shader uint32) { l.compileShader(shader) }
func (l *Loader) GetShaderiv(shader uint32, pname uint32, params *int32) {
	l.getShaderiv(shader, pname, params)
}
func (l *Loader) GetShaderInfoLog(shader uint32, bufSize int32, length *int32, infoLog *byte) {
	l.getShaderInfoLog(shader, bufSize, length, infoLog)
}
func (l *Loader) DeleteShader(shader uint32)                 { l.deleteShader(shader) }
func (l *Loader) CreateProgram() uint32                      { return l.createProgram() }
func (l *Loader) AttachShader(program uint32, shader uint32) { l.attachShader(program, shader) }
func (l *Loader) LinkProgram(program uint32)                 { l.linkProgram(program) }
func (l *Loader) GetProgramiv(program uint32, pname uint32, params *int32) {
	l.getProgramiv(program, pname, params)
}
func (l *Loader) GetProgramInfoLog(program uint32, bufSize int32, length *int32, infoLog *byte) {
	l.getProgramInfoLog(program, bufSize, length, infoLog)
}
func (l *Loader) UseProgram(program uint32)    { l.useProgram(program) }
func (l *Loader) DeleteProgram(program uint32) { l.deleteProgram(program) }
func (l *Loader) GetUniformLocation(program uint32, name *byte) int32 {
	return l.getUniformLocation(program, name)
}
func (l *Loader) GetAttribLocation(program uint32, name *byte) int32 {
	return l.getAttribLocation(program, name)
}
func (l *Loader) Uniform1i(location int32, v0 int32)         { l.uniform1i(location, v0) }
func (l *Loader) GenVertexArrays(n int32, arrays *uint32)    { l.genVertexArrays(n, arrays) }
func (l *Loader) BindVertexArray(array uint32)               { l.bindVertexArray(array) }
func (l *Loader) DeleteVertexArrays(n int32, arrays *uint32) { l.deleteVertexArrays(n, arrays) }
func (l *Loader) VertexAttribPointer(index uint32, size int32, xtype uint32, normalized bool, stride int32, pointer uintptr) {
	l.vertexAttribPointer(index, size, xtype, normalized, stride, pointer)
}
func (l *Loader) EnableVertexAttribArray(index uint32)          { l.enableVertexAttribArray(index) }
func (l *Loader) DisableVertexAttribArray(index uint32)         { l.disableVertexAttribArray(index) }
func (l *Loader) GenFramebuffers(n int32, framebuffers *uint32) { l.genFramebuffers(n, framebuffers) }
func (l *Loader) DeleteFramebuffers(n int32, framebuffers *uint32) {
	l.deleteFramebuffers(n, framebuffers)
}
func (l *Loader) BindFramebuffer(target uint32, framebuffer uint32) {
	l.bindFramebuffer(target, framebuffer)
}
func (l *Loader) FramebufferTexture2D(target uint32, attachment uint32, textarget uint32, texture uint32, level int32) {
	l.framebufferTexture2D(target, attachment, textarget, texture, level)
}
func (l *Loader) CheckFramebufferStatus(target uint32) uint32 {
	return l.checkFramebufferStatus(target)
}
func (l *Loader) Viewport(x, y, width, height int32)         { l.viewport(x, y, width, height) }
func (l *Loader) DrawArrays(mode uint32, first, count int32) { l.drawArrays(mode, first, count) }
func (l *Loader) GetError() uint32                           { return l.getError() }
func (l *Loader) ReadPixels(x, y, width, height int32, format, xtype uint32, pixels unsafe.Pointer) {
	l.readPixels(x, y, width, height, format, xtype, pixels)
}
func (l *Loader) TimerQuerySupported() bool {
	return l != nil && l.genQueries != nil && l.deleteQueries != nil && l.beginQuery != nil && l.endQuery != nil && l.getQueryObjectuiv != nil && l.getQueryObjectui64v != nil
}
func (l *Loader) GenQueries(n int32, ids *uint32)     { l.genQueries(n, ids) }
func (l *Loader) DeleteQueries(n int32, ids *uint32)  { l.deleteQueries(n, ids) }
func (l *Loader) BeginQuery(target uint32, id uint32) { l.beginQuery(target, id) }
func (l *Loader) EndQuery(target uint32)              { l.endQuery(target) }
func (l *Loader) GetQueryObjectuiv(id uint32, pname uint32, params *uint32) {
	l.getQueryObjectuiv(id, pname, params)
}
func (l *Loader) GetQueryObjectui64v(id uint32, pname uint32, params *uint64) {
	l.getQueryObjectui64v(id, pname, params)
}

// NewBackendFromCurrentContext returns a Loader-backed GL driver. Callers must
// invoke it only while the intended GL context is current on this OS thread.
func NewBackendFromCurrentContext() (*Loader, error) { return NewLoader() }

// ImportEGLImageToTexture creates a temporary GL texture backed by an EGLImage.
func (l *Loader) ImportEGLImageToTexture(image uintptr) (Texture, error) {
	if l.eglImageTargetTexture2DOES == nil {
		return 0, ErrMissingGLEGLImageTargetTexture2DOES
	}
	var tex uint32
	l.GenTextures(1, &tex)
	l.BindTexture(Texture2D, tex)
	setTextureParameters(l, Texture2D)
	l.eglImageTargetTexture2DOES(Texture2D, image)
	if err := CheckError(l, "import EGLImage to texture"); err != nil {
		l.DeleteTextures(1, &tex)
		return 0, err
	}
	return Texture(tex), nil
}

type TexturedQuadCopier struct {
	gl       Driver
	program  uint32
	vbo      uint32
	vao      uint32
	samplLoc int32
	posLoc   int32
	uvLoc    int32
}

func NewTexturedQuadCopier(driver Driver) (*TexturedQuadCopier, error) {
	return NewTexturedQuadCopierForAPI(driver, GLAPIOpenGL)
}

const (
	GLAPIOpenGL   = "opengl"
	GLAPIOpenGLES = "opengles"
)

func NewTexturedQuadCopierForAPI(driver Driver, glAPI string) (*TexturedQuadCopier, error) {
	vertexShader, fragmentShader := texturedQuadShadersForAPI(glAPI)
	program, err := CompileProgram(driver, vertexShader, fragmentShader)
	if err != nil {
		return nil, err
	}
	c := &TexturedQuadCopier{gl: driver, program: uint32(program)}
	samplerName := cStringBytes("u_texture")
	c.samplLoc = driver.GetUniformLocation(c.program, &samplerName[0])
	runtime.KeepAlive(samplerName)
	posName := cStringBytes("a_pos")
	c.posLoc = driver.GetAttribLocation(c.program, &posName[0])
	runtime.KeepAlive(posName)
	uvName := cStringBytes("a_uv")
	c.uvLoc = driver.GetAttribLocation(c.program, &uvName[0])
	runtime.KeepAlive(uvName)
	if c.posLoc < 0 || c.uvLoc < 0 || c.samplLoc < 0 {
		driver.DeleteProgram(c.program)
		return nil, fmt.Errorf("initialize textured quad copier: missing shader locations")
	}
	driver.GenVertexArrays(1, &c.vao)
	driver.BindVertexArray(c.vao)
	driver.GenBuffers(1, &c.vbo)
	driver.BindBuffer(ArrayBuffer, c.vbo)
	uploadQuadVertices(driver, quadVerticesIdentity)
	driver.EnableVertexAttribArray(uint32(c.posLoc))
	driver.VertexAttribPointer(uint32(c.posLoc), 2, Float, false, 16, 0)
	driver.EnableVertexAttribArray(uint32(c.uvLoc))
	driver.VertexAttribPointer(uint32(c.uvLoc), 2, Float, false, 16, 8)
	driver.BindVertexArray(0)
	if err := CheckError(driver, "initialize textured quad copier"); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *TexturedQuadCopier) Close() {
	if c == nil || c.gl == nil {
		return
	}
	if c.vbo != 0 {
		c.gl.DeleteBuffers(1, &c.vbo)
		c.vbo = 0
	}
	if c.vao != 0 {
		c.gl.DeleteVertexArrays(1, &c.vao)
		c.vao = 0
	}
	if c.program != 0 {
		c.gl.DeleteProgram(c.program)
		c.program = 0
	}
}

// CopyImportedToOwned renders src through a textured quad into dst. If dst is
// zero, a new owned RGBA texture is created. No production fake-success path is
// provided: framebuffer completeness and glGetError are mandatory checks.
func (c *TexturedQuadCopier) CopyImportedToOwned(src Texture, size dmabuf.Size, dst Texture) (out Texture, err error) {
	if src == 0 {
		return 0, ErrInvalidTexture
	}
	if !size.Valid() {
		return 0, fmt.Errorf("%w: %dx%d", ErrInvalidSize, size.Width, size.Height)
	}
	created := dst == 0
	defer func() {
		if created && err != nil && dst != 0 {
			dstID := uint32(dst)
			c.gl.DeleteTextures(1, &dstID)
		}
	}()
	if created {
		var tex uint32
		c.gl.GenTextures(1, &tex)
		dst = Texture(tex)
		c.gl.BindTexture(Texture2D, uint32(dst))
		setTextureParameters(c.gl, Texture2D)
		c.gl.TexImage2D(Texture2D, 0, int32(RGBA), size.Width, size.Height, 0, RGBA, UnsignedByte, nil)
	}
	var fbo uint32
	c.gl.GenFramebuffers(1, &fbo)
	defer c.gl.DeleteFramebuffers(1, &fbo)
	c.gl.BindFramebuffer(Framebuffer, fbo)
	defer c.gl.BindFramebuffer(Framebuffer, 0)
	c.gl.FramebufferTexture2D(Framebuffer, ColorAttachment0, Texture2D, uint32(dst), 0)
	if status := c.gl.CheckFramebufferStatus(Framebuffer); status != FramebufferComplete {
		return 0, fmt.Errorf("copy imported texture to owned texture: framebuffer incomplete: 0x%x", status)
	}
	c.gl.Viewport(0, 0, size.Width, size.Height)
	// CEF's imported NativePixmap samples use the opposite image-origin
	// convention from GL texture coordinates. Normalize once at the import/copy
	// boundary; the later GtkGLArea presentation keeps identity coordinates.
	c.uploadQuadVertices(quadVerticesFlipY)
	c.gl.UseProgram(c.program)
	c.gl.ActiveTexture(Texture0)
	c.gl.BindTexture(Texture2D, uint32(src))
	c.gl.Uniform1i(c.samplLoc, 0)
	c.gl.BindVertexArray(c.vao)
	c.gl.DrawArrays(TriangleStrip, 0, 4)
	c.gl.BindVertexArray(0)
	if err := CheckError(c.gl, "copy imported texture to owned texture"); err != nil {
		return 0, err
	}
	return dst, nil
}

// DrawTextureToCurrentFramebuffer renders src into the current framebuffer.
func (c *TexturedQuadCopier) DrawTextureToCurrentFramebuffer(src Texture, size dmabuf.Size) error {
	if src == 0 {
		return ErrInvalidTexture
	}
	if !size.Valid() {
		return fmt.Errorf("%w: %dx%d", ErrInvalidSize, size.Width, size.Height)
	}
	c.gl.Viewport(0, 0, size.Width, size.Height)
	c.uploadQuadVertices(quadVerticesIdentity)
	c.gl.UseProgram(c.program)
	c.gl.ActiveTexture(Texture0)
	c.gl.BindTexture(Texture2D, uint32(src))
	c.gl.Uniform1i(c.samplLoc, 0)
	c.gl.BindVertexArray(c.vao)
	c.gl.DrawArrays(TriangleStrip, 0, 4)
	c.gl.BindVertexArray(0)
	return CheckError(c.gl, "draw queued texture to GtkGLArea")
}

var (
	quadVerticesIdentity = []float32{
		-1, -1, 0, 0,
		1, -1, 1, 0,
		-1, 1, 0, 1,
		1, 1, 1, 1,
	}
	quadVerticesFlipY = []float32{
		-1, -1, 0, 1,
		1, -1, 1, 1,
		-1, 1, 0, 0,
		1, 1, 1, 0,
	}
)

func (c *TexturedQuadCopier) uploadQuadVertices(vertices []float32) {
	if c == nil || c.gl == nil {
		return
	}
	c.gl.BindBuffer(ArrayBuffer, c.vbo)
	uploadQuadVertices(c.gl, vertices)
}

func uploadQuadVertices(driver Driver, vertices []float32) {
	driver.BufferData(ArrayBuffer, int64(len(vertices)*4), unsafe.Pointer(&vertices[0]), StaticDraw)
}

func setTextureParameters(gl Driver, target uint32) {
	gl.TexParameteri(target, TextureMinFilter, int32(Linear))
	gl.TexParameteri(target, TextureMagFilter, int32(Linear))
	gl.TexParameteri(target, TextureWrapS, int32(ClampToEdge))
	gl.TexParameteri(target, TextureWrapT, int32(ClampToEdge))
}

func CheckError(gl interface{ GetError() uint32 }, op string) error {
	var names []string
	for {
		code := gl.GetError()
		if code == NoError {
			break
		}
		names = append(names, errorName(code))
	}
	if len(names) == 0 {
		return nil
	}
	return fmt.Errorf("%s: GL error %s", op, strings.Join(names, ", "))
}

func errorName(code uint32) string {
	switch code {
	case InvalidEnum:
		return "GL_INVALID_ENUM"
	case InvalidValue:
		return "GL_INVALID_VALUE"
	case InvalidOperation:
		return "GL_INVALID_OPERATION"
	case OutOfMemory:
		return "GL_OUT_OF_MEMORY"
	default:
		return fmt.Sprintf("0x%x", code)
	}
}

func CompileProgram(gl Driver, vertexSource, fragmentSource string) (Program, error) {
	vs, err := CompileShader(gl, VertexShader, vertexSource)
	if err != nil {
		return 0, err
	}
	defer gl.DeleteShader(uint32(vs))
	fs, err := CompileShader(gl, FragmentShader, fragmentSource)
	if err != nil {
		return 0, err
	}
	defer gl.DeleteShader(uint32(fs))
	program := gl.CreateProgram()
	gl.AttachShader(program, uint32(vs))
	gl.AttachShader(program, uint32(fs))
	gl.LinkProgram(program)
	var ok int32
	gl.GetProgramiv(program, LinkStatus, &ok)
	if ok == 0 {
		log := programInfoLog(gl, program)
		gl.DeleteProgram(program)
		return 0, fmt.Errorf("link GL program: %s", log)
	}
	return Program(program), nil
}

func CompileShader(gl Driver, shaderType uint32, source string) (Shader, error) {
	shader := gl.CreateShader(shaderType)
	bytes := append([]byte(source), 0)
	ptr := &bytes[0]
	length := int32(len(source))
	gl.ShaderSource(shader, 1, &ptr, &length)
	gl.CompileShader(shader)
	var ok int32
	gl.GetShaderiv(shader, CompileStatus, &ok)
	if ok == 0 {
		log := shaderInfoLog(gl, shader)
		gl.DeleteShader(shader)
		return 0, fmt.Errorf("compile GL shader: %s", log)
	}
	return Shader(shader), nil
}

func shaderInfoLog(gl Driver, shader uint32) string {
	var n int32
	gl.GetShaderiv(shader, InfoLogLength, &n)
	if n <= 1 {
		return "unknown shader compile failure"
	}
	buf := make([]byte, n)
	gl.GetShaderInfoLog(shader, n, nil, &buf[0])
	return strings.TrimRight(string(buf), "\x00")
}

func programInfoLog(gl Driver, program uint32) string {
	var n int32
	gl.GetProgramiv(program, InfoLogLength, &n)
	if n <= 1 {
		return "unknown program link failure"
	}
	buf := make([]byte, n)
	gl.GetProgramInfoLog(program, n, nil, &buf[0])
	return strings.TrimRight(string(buf), "\x00")
}

func cStringBytes(s string) []byte { return append([]byte(s), 0) }

func texturedQuadShadersForAPI(glAPI string) (vertex, fragment string) {
	if glAPI == GLAPIOpenGLES {
		return texturedQuadGLESVertexShader, texturedQuadGLESFragmentShader
	}
	return texturedQuadVertexShader, texturedQuadFragmentShader
}

const texturedQuadVertexShader = `#version 120
attribute vec2 a_pos;
attribute vec2 a_uv;
varying vec2 v_uv;
void main() {
	v_uv = a_uv;
	gl_Position = vec4(a_pos, 0.0, 1.0);
}`

const texturedQuadFragmentShader = `#version 120
uniform sampler2D u_texture;
varying vec2 v_uv;
void main() {
	gl_FragColor = texture2D(u_texture, v_uv);
}`

const texturedQuadGLESVertexShader = `#version 100
attribute vec2 a_pos;
attribute vec2 a_uv;
varying vec2 v_uv;
void main() {
	v_uv = a_uv;
	gl_Position = vec4(a_pos, 0.0, 1.0);
}`

const texturedQuadGLESFragmentShader = `#version 100
precision mediump float;
uniform sampler2D u_texture;
varying vec2 v_uv;
void main() {
	gl_FragColor = texture2D(u_texture, v_uv);
}`
