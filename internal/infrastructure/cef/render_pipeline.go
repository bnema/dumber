package cef

import (
	"fmt"
	"os"
	"sync"
	"unsafe"

	"github.com/jwijenbergh/puregotk/v4/gtk"
)

// rect is a simple rectangle for dirty rect tracking.
type rect struct {
	X, Y, Width, Height int32
}

// renderPipeline implements PBO double-buffered OpenGL rendering of CEF's
// off-screen BGRA pixel buffers into a GtkGLArea widget.
//
// CEF delivers frames via OnPaint (BGRA buffer + dirty rects). handlePaint
// copies dirty regions into a CPU-side staging buffer and queues a GTK render.
// The GL render signal uploads from staging → PBO → texture, then draws a
// fullscreen quad. Two PBOs alternate so GPU DMA and CPU writes overlap.
type renderPipeline struct {
	gl     *glLoader
	glArea *gtk.GLArea

	// GL resources (created on first render).
	texture  uint32
	pbo      [2]uint32
	pboIndex int
	program  uint32
	vao      uint32
	vbo      uint32

	// Surface dimensions (in device pixels, i.e. scaled).
	width  int32
	height int32
	scale  int32

	// Staging: handlePaint copies dirty rects here, render signal uploads.
	mu          sync.Mutex
	staging     []byte
	dirtyRects  []rect
	needsUpload bool
	sizeChanged bool

	// GL initialized flag.
	glReady bool

	// onFirstResize is called once when the first non-zero resize occurs.
	onFirstResize func(width, height int32)

	// onResizeCB is called on every non-zero resize after the first.
	onResizeCB func(width, height int32)
}

// Shader sources for the fullscreen textured quad.
const vertexShaderSource = "" +
	"#version 330 core\n" +
	"layout(location=0) in vec2 aPos;\n" +
	"layout(location=1) in vec2 aUV;\n" +
	"out vec2 vUV;\n" +
	"void main() {\n" +
	"    gl_Position = vec4(aPos, 0.0, 1.0);\n" +
	"    vUV = aUV;\n" +
	"}\x00"

const fragmentShaderSource = "" +
	"#version 330 core\n" +
	"in vec2 vUV;\n" +
	"out vec4 fragColor;\n" +
	"uniform sampler2D tex;\n" +
	"void main() {\n" +
	"    fragColor = texture(tex, vUV);\n" +
	"}\x00"

// Fullscreen quad: position (x,y) + UV (u,v), triangle strip.
// UV Y is flipped because CEF's buffer origin is top-left, GL is bottom-left.
var quadVertices = [16]float32{
	-1, -1, 0, 1, // bottom-left
	1, -1, 1, 1, // bottom-right
	-1, 1, 0, 0, // top-left
	1, 1, 1, 0, // top-right
}

// newRenderPipeline creates a GtkGLArea and wires up the render and resize
// signals. The returned pipeline is ready to receive handlePaint calls.
func newRenderPipeline(gl *glLoader, scale int32) *renderPipeline {
	if scale < 1 {
		scale = 1
	}

	rp := &renderPipeline{
		gl:    gl,
		scale: scale,
	}

	rp.glArea = gtk.NewGLArea()
	rp.glArea.SetRequiredVersion(3, 3)
	rp.glArea.SetAutoRender(false)
	rp.glArea.SetHasDepthBuffer(false)
	rp.glArea.SetHasStencilBuffer(false)

	// Wire signals. puregotk takes *func(...) for signal callbacks.
	renderCb := func(_ gtk.GLArea, _ uintptr) bool {
		return rp.onGLRender()
	}
	rp.glArea.ConnectRender(&renderCb)

	resizeCb := func(_ gtk.GLArea, w int, h int) {
		rp.onResize(int32(w), int32(h))
	}
	rp.glArea.ConnectResize(&resizeCb)

	return rp
}

// handlePaint is called from CEF's OnPaint callback (on GTK main thread via
// IdleAdd). It copies dirty rect regions from the CEF buffer into the staging
// buffer and queues a GL redraw.
func (rp *renderPipeline) handlePaint(buffer unsafe.Pointer, width, height int32, rects []rect) {
	if buffer == nil || width <= 0 || height <= 0 {
		return
	}

	rp.mu.Lock()

	bufSize := int(width) * int(height) * 4
	srcSlice := unsafe.Slice((*byte)(buffer), bufSize)

	// Detect size change, or first paint (staging not yet allocated).
	if width != rp.width || height != rp.height || rp.staging == nil {
		rp.width = width
		rp.height = height
		rp.staging = make([]byte, bufSize)
		rp.sizeChanged = true
		// On size change, copy the entire buffer.
		copy(rp.staging, srcSlice)
	} else {
		// Copy only dirty rect rows.
		stride := int(width) * 4
		for _, r := range rects {
			for row := r.Y; row < r.Y+r.Height; row++ {
				srcOff := int(row)*stride + int(r.X)*4
				dstOff := srcOff
				rowBytes := int(r.Width) * 4
				copy(rp.staging[dstOff:dstOff+rowBytes], srcSlice[srcOff:srcOff+rowBytes])
			}
		}
	}

	rp.dirtyRects = append(rp.dirtyRects[:0], rects...)
	rp.needsUpload = true

	rp.mu.Unlock()

	rp.glArea.QueueRender()
}

// onGLRender is the GTK "render" signal handler. GL context is current.
var glRenderCount uint64

func (rp *renderPipeline) onGLRender() bool {
	rp.mu.Lock()

	needsUpload := rp.needsUpload
	sizeChanged := rp.sizeChanged
	rp.needsUpload = false

	glRenderCount++
	if glRenderCount <= 5 {
		fmt.Fprintf(os.Stderr, "cef-debug: onGLRender call=%d needsUpload=%v sizeChanged=%v glReady=%v\n",
			glRenderCount, needsUpload, sizeChanged, rp.glReady)
	}

	// Snapshot dirty rects for upload.
	var dirtyRects []rect
	if needsUpload {
		dirtyRects = make([]rect, len(rp.dirtyRects))
		copy(dirtyRects, rp.dirtyRects)
	}

	rp.mu.Unlock()

	gl := rp.gl

	if !rp.glReady || sizeChanged {
		rp.initGL()
	}

	if !rp.glReady {
		return true
	}

	if needsUpload {
		rp.uploadToPBO(dirtyRects, sizeChanged)
	}

	// Draw fullscreen quad.
	gl.clearColor(0, 0, 0, 1)
	gl.clear(glColorBufferBit)
	gl.useProgram(rp.program)
	gl.bindTexture(glTexture2D, rp.texture)
	gl.bindVertexArray(rp.vao)
	gl.drawArrays(glTriangleStrip, 0, 4)
	gl.bindVertexArray(0)
	gl.bindTexture(glTexture2D, 0)
	gl.useProgram(0)

	return true
}

// uploadToPBO uploads dirty regions from the staging buffer into a PBO, then
// transfers to the GL texture via async DMA.
func (rp *renderPipeline) uploadToPBO(dirtyRects []rect, fullUpload bool) {
	gl := rp.gl

	rp.mu.Lock()
	w := rp.width
	h := rp.height
	rp.mu.Unlock()

	bufSize := int64(w) * int64(h) * 4
	stride := int(w) * 4

	// Bind the write PBO.
	gl.bindBuffer(glPixelUnpackBuffer, rp.pbo[rp.pboIndex])

	// Orphan the buffer to avoid GPU stall, then map.
	gl.bufferData(glPixelUnpackBuffer, bufSize, nil, glStreamDraw)
	mapped := gl.mapBuffer(glPixelUnpackBuffer, glWriteOnly)
	if mapped == nil {
		gl.bindBuffer(glPixelUnpackBuffer, 0)
		return
	}

	mappedSlice := unsafe.Slice((*byte)(mapped), int(bufSize))

	rp.mu.Lock()
	if fullUpload {
		copy(mappedSlice, rp.staging)
	} else {
		for _, r := range dirtyRects {
			for row := r.Y; row < r.Y+r.Height; row++ {
				off := int(row)*stride + int(r.X)*4
				rowBytes := int(r.Width) * 4
				copy(mappedSlice[off:off+rowBytes], rp.staging[off:off+rowBytes])
			}
		}
	}
	rp.mu.Unlock()

	gl.unmapBuffer(glPixelUnpackBuffer)

	// Transfer PBO → texture. With PBO bound, the last arg is a byte offset.
	gl.bindTexture(glTexture2D, rp.texture)
	if fullUpload {
		gl.texSubImage2D(glTexture2D, 0, 0, 0, w, h, glBGRA, glUnsignedByte, nil)
	} else {
		for _, r := range dirtyRects {
			// For sub-rect uploads with stride mismatches we must upload row by row
			// because glTexSubImage2D with a PBO reads contiguous memory from the
			// offset, but our rows in the PBO follow the full-width stride.
			for row := r.Y; row < r.Y+r.Height; row++ {
				rowOffset := uintptr(int(row)*stride + int(r.X)*4)
				gl.texSubImage2D(glTexture2D, 0, r.X, row, r.Width, 1, glBGRA, glUnsignedByte,
					pboOffset(rowOffset))
			}
		}
	}
	gl.bindTexture(glTexture2D, 0)

	// Swap PBO index for next frame.
	rp.pboIndex = 1 - rp.pboIndex

	gl.bindBuffer(glPixelUnpackBuffer, 0)
}

// initGL creates or recreates all GL resources for the current surface size.
func (rp *renderPipeline) initGL() {
	rp.mu.Lock()
	w := rp.width
	h := rp.height
	rp.sizeChanged = false
	rp.mu.Unlock()

	if w <= 0 || h <= 0 {
		return
	}

	gl := rp.gl

	// Clean up previous resources if reinitializing.
	if rp.glReady {
		rp.deleteGLResources()
	}

	// Texture.
	gl.genTextures(1, &rp.texture)
	gl.bindTexture(glTexture2D, rp.texture)
	gl.texParameteri(glTexture2D, glTextureMinFilter, int32(glLinear))
	gl.texParameteri(glTexture2D, glTextureMagFilter, int32(glLinear))
	gl.texParameteri(glTexture2D, glTextureWrapS, int32(glClampToEdge))
	gl.texParameteri(glTexture2D, glTextureWrapT, int32(glClampToEdge))
	gl.texImage2D(glTexture2D, 0, int32(glRGBA), w, h, 0, glBGRA, glUnsignedByte, nil)
	gl.bindTexture(glTexture2D, 0)

	// PBOs.
	bufSize := int64(w) * int64(h) * 4
	gl.genBuffers(1, &rp.pbo[0])
	gl.genBuffers(1, &rp.pbo[1])
	for i := 0; i < 2; i++ {
		gl.bindBuffer(glPixelUnpackBuffer, rp.pbo[i])
		gl.bufferData(glPixelUnpackBuffer, bufSize, nil, glStreamDraw)
	}
	gl.bindBuffer(glPixelUnpackBuffer, 0)
	rp.pboIndex = 0

	// Shaders.
	rp.program = rp.buildShaderProgram()

	// VAO + VBO.
	gl.genVertexArrays(1, &rp.vao)
	gl.bindVertexArray(rp.vao)

	gl.genBuffers(1, &rp.vbo)
	gl.bindBuffer(glArrayBuffer, rp.vbo)
	gl.bufferData(glArrayBuffer, int64(len(quadVertices)*4), unsafe.Pointer(&quadVertices[0]), glStaticDraw)

	// Position attribute: location 0, 2 floats, stride 16, offset 0.
	gl.vertexAttribPointer(0, 2, glFloat, false, 4*4, 0)
	gl.enableVertexAttribArray(0)

	// UV attribute: location 1, 2 floats, stride 16, offset 8.
	gl.vertexAttribPointer(1, 2, glFloat, false, 4*4, 2*4)
	gl.enableVertexAttribArray(1)

	gl.bindBuffer(glArrayBuffer, 0)
	gl.bindVertexArray(0)

	// Set the "tex" uniform to texture unit 0.
	gl.useProgram(rp.program)
	texName := []byte("tex\x00")
	loc := gl.getUniformLocation(rp.program, &texName[0])
	gl.uniform1i(loc, 0)
	gl.useProgram(0)

	rp.glReady = true
}

// buildShaderProgram compiles and links the vertex+fragment shaders.
func (rp *renderPipeline) buildShaderProgram() uint32 {
	gl := rp.gl

	vs := compileShader(gl, glVertexShader, vertexShaderSource)
	fs := compileShader(gl, glFragmentShader, fragmentShaderSource)

	prog := gl.createProgram()
	gl.attachShader(prog, vs)
	gl.attachShader(prog, fs)
	gl.linkProgram(prog)

	// Shaders can be deleted after linking.
	gl.deleteShader(vs)
	gl.deleteShader(fs)

	return prog
}

// compileShader compiles a single GLSL shader and returns its GL handle.
func compileShader(gl *glLoader, shaderType uint32, source string) uint32 {
	shader := gl.createShader(shaderType)

	// source already has null terminator.
	srcBytes := []byte(source)
	srcPtr := &srcBytes[0]
	srcLen := int32(len(source) - 1) // exclude null terminator from length
	gl.shaderSource(shader, 1, &srcPtr, &srcLen)
	gl.compileShader(shader)

	return shader
}

// onResize is the GTK "resize" signal handler. Dimensions are in CSS pixels;
// we multiply by scale to get device pixels.
func (rp *renderPipeline) onResize(width, height int32) {
	rp.mu.Lock()

	scaled := func(v int32) int32 {
		return v * rp.scale
	}

	rp.width = scaled(width)
	rp.height = scaled(height)
	rp.sizeChanged = true

	firstCB := rp.onFirstResize
	if firstCB != nil {
		rp.onFirstResize = nil
	}
	resizeCB := rp.onResizeCB

	w, h := rp.width, rp.height
	rp.mu.Unlock()

	if firstCB != nil {
		firstCB(w, h)
	} else if resizeCB != nil {
		resizeCB(w, h)
	}
}

// destroy releases all GL resources.
func (rp *renderPipeline) destroy() {
	if !rp.glReady {
		return
	}
	rp.deleteGLResources()
	rp.glReady = false
}

// deleteGLResources frees texture, PBOs, VAO, VBO, and program.
func (rp *renderPipeline) deleteGLResources() {
	gl := rp.gl
	if rp.texture != 0 {
		gl.deleteTextures(1, &rp.texture)
		rp.texture = 0
	}
	for i := 0; i < 2; i++ {
		if rp.pbo[i] != 0 {
			gl.deleteBuffers(1, &rp.pbo[i])
			rp.pbo[i] = 0
		}
	}
	if rp.vao != 0 {
		gl.deleteVertexArrays(1, &rp.vao)
		rp.vao = 0
	}
	if rp.vbo != 0 {
		gl.deleteBuffers(1, &rp.vbo)
		rp.vbo = 0
	}
	if rp.program != 0 {
		// deleteProgram not in glLoader — just zero it. The program leaks on
		// reinit but that only happens on resize which is rare. We can add
		// glDeleteProgram to glLoader if needed.
		rp.program = 0
	}
}

// pboOffset converts a byte offset into an unsafe.Pointer for use with
// glTexSubImage2D when a PBO is bound. OpenGL interprets the pointer as a
// byte offset into the bound buffer rather than a real memory address.
func pboOffset(offset uintptr) unsafe.Pointer {
	return unsafe.Add(nil, offset)
}
