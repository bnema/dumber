package cef

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/puregotk/v4/gtk"
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
	ctx    context.Context
	gl     *glLoader
	glArea *gtk.GLArea

	// GL resources (created on first render).
	texture      uint32
	popupTexture uint32
	pbo          [2]uint32
	pboIndex     int
	program      uint32
	vao          uint32
	vbo          uint32

	// Surface dimensions (in device pixels, i.e. scaled).
	width  int32
	height int32
	scale  int32

	// geometry mirrors the current surface size for lock-free CEF callbacks.
	widthAtomic  atomic.Int32
	heightAtomic atomic.Int32

	// Staging: handlePaint copies dirty rects here, render signal uploads.
	mu          sync.Mutex
	staging     []byte
	dirtyRects  []rect
	needsUpload bool
	sizeChanged bool

	// Popup staging for native CEF widgets like <select> menus.
	popupVisible     bool
	popupRect        rect
	popupStaging     []byte
	popupWidth       int32
	popupHeight      int32
	popupNeedsUpload bool
	popupSizeChanged bool

	// GL initialized flag.
	glReady bool

	// Diagnostics (main thread only).
	viewRectSeq           atomic.Uint64
	screenInfoSeq         atomic.Uint64
	paintSeq              atomic.Uint64
	lastQueuedPaintSeq    atomic.Uint64
	glRenderSeq           atomic.Uint64
	paintCount            uint64
	acceleratedPaintCount uint64
	queueRenderCount      uint64
	glRenderCount         uint64
	uploadCount           uint64
	fullUploadCount       uint64
	paintBytes            uint64
	uploadBytes           uint64
	dirtyRectCount        uint64
	paintCopyTotalNs      uint64
	uploadTotalNs         uint64
	renderTotalNs         uint64
	maxPaintCopyNs        int64
	maxUploadNs           int64
	maxRenderNs           int64
	diagLastLogAt         time.Time
	diagLastPaintCount    uint64
	diagLastAccelCount    uint64
	diagLastQueueCount    uint64
	diagLastRenderCount   uint64
	diagLastUploadCount   uint64
	diagLastFullUploads   uint64
	diagLastPaintBytes    uint64
	diagLastUploadBytes   uint64
	diagLastPaintCopyNs   uint64
	diagLastUploadNs      uint64
	diagLastRenderNs      uint64

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
func newRenderPipeline(ctx context.Context, gl *glLoader, scale int32) *renderPipeline {
	if scale < 1 {
		scale = 1
	}

	rp := &renderPipeline{
		ctx:   ctx,
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
func (rp *renderPipeline) handlePaint(buffer []byte, width, height int32, rects []rect, paintSeq uint64) {
	if len(buffer) == 0 || width <= 0 || height <= 0 {
		return
	}

	startedAt := time.Now()
	rp.lastQueuedPaintSeq.Store(paintSeq)
	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Debug().
			Uint64("paint_seq", paintSeq).
			Int32("width", width).
			Int32("height", height).
			Int("rect_count", len(rects)).
			Int("buffer_len", len(buffer)).
			Msg("cef: handlePaint begin")
	}
	rp.mu.Lock()

	bufSize, ok := bgraBufferSize(width, height)
	if !ok {
		rp.mu.Unlock()
		return
	}
	copiedBytes := uint64(0)
	sizeChanged := false

	// Detect size change, or first paint (staging not yet allocated).
	if stagingNeedsReset(rp.staging, rp.width, rp.height, width, height, bufSize) {
		rp.width = width
		rp.height = height
		rp.widthAtomic.Store(width)
		rp.heightAtomic.Store(height)
		rp.staging = make([]byte, bufSize)
		rp.sizeChanged = true
		sizeChanged = true
		// On size change, copy the entire buffer.
		copiedBytes = uint64(copy(rp.staging, buffer))
	} else {
		// Copy only dirty rect rows.
		var truncated bool
		copiedBytes, rects, truncated = copyDirtyRectsIntoStaging(rp.staging, buffer, width, height, rects)
		if truncated && rp.ctx != nil {
			logging.FromContext(rp.ctx).Warn().
				Uint64("paint_seq", paintSeq).
				Int32("width", width).
				Int32("height", height).
				Int("buffer_len", len(buffer)).
				Int("expected_len", bufSize).
				Msg("cef: truncated paint buffer while copying dirty rects")
		}
	}

	// Accumulate dirty rects across multiple OnPaint calls between renders.
	// If we replaced instead of appending, paints that arrive before the
	// next GL render would lose their dirty rect metadata — the pixels are
	// in staging but never uploaded to the texture, causing trails/ghosts.
	rp.dirtyRects = append(rp.dirtyRects, rects...)
	rp.needsUpload = true

	rp.mu.Unlock()

	copyDuration := time.Since(startedAt)
	rp.paintCount++
	rp.queueRenderCount++
	rp.paintBytes += copiedBytes
	rp.dirtyRectCount += uint64(len(rects))
	rp.paintCopyTotalNs += uint64(copyDuration)
	if copyDuration.Nanoseconds() > rp.maxPaintCopyNs {
		rp.maxPaintCopyNs = copyDuration.Nanoseconds()
	}

	rp.glArea.QueueRender()
	if rp.ctx != nil {
		if sizeChanged && copiedBytes < uint64(bufSize) {
			logging.FromContext(rp.ctx).Warn().
				Uint64("paint_seq", paintSeq).
				Int32("width", width).
				Int32("height", height).
				Int("buffer_len", len(buffer)).
				Int("expected_len", bufSize).
				Msg("cef: truncated full paint buffer on resize")
		}
		logging.FromContext(rp.ctx).Debug().
			Uint64("paint_seq", paintSeq).
			Uint64("copied_bytes", copiedBytes).
			Bool("size_changed", sizeChanged).
			Dur("copy_duration", copyDuration).
			Msg("cef: handlePaint end")
	}
	rp.maybeLogDiagnostics()
}

// onGLRender is the GTK "render" signal handler. GL context is current.
func (rp *renderPipeline) onGLRender() bool {
	renderSeq := rp.glRenderSeq.Add(1)
	paintSeq := rp.lastQueuedPaintSeq.Load()
	renderStartedAt := time.Now()
	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Debug().
			Uint64("render_seq", renderSeq).
			Uint64("paint_seq", paintSeq).
			Msg("cef: onGLRender begin")
	}
	rp.mu.Lock()

	needsUpload := rp.needsUpload
	sizeChanged := rp.sizeChanged
	rp.needsUpload = false

	// Snapshot dirty rects for upload and clear the accumulator.
	var dirtyRects []rect
	if needsUpload {
		dirtyRects = make([]rect, len(rp.dirtyRects))
		copy(dirtyRects, rp.dirtyRects)
		rp.dirtyRects = rp.dirtyRects[:0]
	}

	rp.mu.Unlock()

	gl := rp.gl

	if !rp.glReady || sizeChanged {
		rp.initGL()
	}

	if !rp.glReady {
		rp.glRenderCount++
		renderDuration := time.Since(renderStartedAt)
		rp.renderTotalNs += uint64(renderDuration)
		if renderDuration.Nanoseconds() > rp.maxRenderNs {
			rp.maxRenderNs = renderDuration.Nanoseconds()
		}
		rp.maybeLogDiagnostics()
		return true
	}

	if needsUpload {
		uploadStartedAt := time.Now()
		uploadedBytes := rp.uploadToPBO(dirtyRects, sizeChanged, renderSeq, paintSeq)
		uploadDuration := time.Since(uploadStartedAt)
		rp.uploadCount++
		if sizeChanged {
			rp.fullUploadCount++
		}
		rp.uploadBytes += uploadedBytes
		rp.uploadTotalNs += uint64(uploadDuration)
		if uploadDuration.Nanoseconds() > rp.maxUploadNs {
			rp.maxUploadNs = uploadDuration.Nanoseconds()
		}
	}

	// Draw the main view texture.
	w, h, _ := rp.viewRectSize()
	gl.viewport(0, 0, w, h)
	gl.clearColor(0, 0, 0, 1)
	gl.clear(glColorBufferBit)
	rp.drawTexture(rp.texture)

	if rp.uploadPopupTexture(renderSeq, paintSeq) {
		rp.drawPopupOverlay(w, h)
	}

	rp.glRenderCount++
	renderDuration := time.Since(renderStartedAt)
	rp.renderTotalNs += uint64(renderDuration)
	if renderDuration.Nanoseconds() > rp.maxRenderNs {
		rp.maxRenderNs = renderDuration.Nanoseconds()
	}
	rp.maybeLogDiagnostics()
	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Debug().
			Uint64("render_seq", renderSeq).
			Uint64("paint_seq", paintSeq).
			Dur("render_duration", renderDuration).
			Bool("needs_upload", needsUpload).
			Bool("size_changed", sizeChanged).
			Msg("cef: onGLRender end")
	}

	return true
}

func (rp *renderPipeline) drawTexture(texture uint32) {
	if texture == 0 {
		return
	}
	gl := rp.gl
	gl.useProgram(rp.program)
	gl.bindTexture(glTexture2D, texture)
	gl.bindVertexArray(rp.vao)
	gl.drawArrays(glTriangleStrip, 0, 4)
	gl.bindVertexArray(0)
	gl.bindTexture(glTexture2D, 0)
	gl.useProgram(0)
}

// uploadToPBO uploads dirty regions from the staging buffer into a PBO, then
// transfers to the GL texture via async DMA.
func (rp *renderPipeline) uploadToPBO(dirtyRects []rect, fullUpload bool, renderSeq, paintSeq uint64) uint64 {
	gl := rp.gl

	w, h, _ := rp.viewRectSize()
	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Debug().
			Uint64("render_seq", renderSeq).
			Uint64("paint_seq", paintSeq).
			Int32("width", w).
			Int32("height", h).
			Int("dirty_rect_count", len(dirtyRects)).
			Bool("full_upload", fullUpload).
			Msg("cef: uploadToPBO begin")
	}

	bufSize := int64(w) * int64(h) * 4
	stride := int(w) * 4

	// Bind the write PBO.
	gl.bindBuffer(glPixelUnpackBuffer, rp.pbo[rp.pboIndex])

	// Orphan the buffer to avoid GPU stall, then map.
	gl.bufferData(glPixelUnpackBuffer, bufSize, nil, glStreamDraw)
	mapped := gl.mapBuffer(glPixelUnpackBuffer, glWriteOnly)
	if mapped == nil {
		gl.bindBuffer(glPixelUnpackBuffer, 0)
		return 0
	}

	mappedSlice := unsafe.Slice((*byte)(mapped), int(bufSize))
	uploadedBytes := uint64(0)

	rp.mu.Lock()
	if fullUpload {
		copied := copy(mappedSlice, rp.staging)
		if copied < len(mappedSlice) {
			clear(mappedSlice[copied:])
		}
		uploadedBytes = uint64(copied)
	} else {
		for _, rawRect := range dirtyRects {
			r, ok := clampDirtyRect(rawRect, w, h)
			if !ok {
				continue
			}
			for row := r.Y; row < r.Y+r.Height; row++ {
				off := int(row)*stride + int(r.X)*4
				rowBytes := int(r.Width) * 4
				copy(mappedSlice[off:off+rowBytes], rp.staging[off:off+rowBytes])
				uploadedBytes += uint64(rowBytes)
			}
		}
	}
	rp.mu.Unlock()

	if !gl.unmapBuffer(glPixelUnpackBuffer) {
		if rp.ctx != nil {
			logging.FromContext(rp.ctx).Error().
				Int32("width", w).
				Int32("height", h).
				Bool("full_upload", fullUpload).
				Msg("cef: glUnmapBuffer reported corrupted PBO contents; dropping frame")
		}
		gl.bindBuffer(glPixelUnpackBuffer, 0)
		return 0
	}

	// Transfer PBO → texture. With PBO bound, the last arg is a byte offset.
	gl.bindTexture(glTexture2D, rp.texture)
	if fullUpload {
		gl.texSubImage2D(glTexture2D, 0, 0, 0, w, h, glBGRA, glUnsignedByte, nil)
	} else {
		// Use GL_UNPACK_ROW_LENGTH so GL understands the PBO's full-width
		// stride, allowing one glTexSubImage2D per dirty rect instead of
		// one per row.
		gl.pixelStorei(glUnpackRowLength, w)
		for _, rawRect := range dirtyRects {
			r, ok := clampDirtyRect(rawRect, w, h)
			if !ok {
				continue
			}
			offset := uintptr(int(r.Y)*stride + int(r.X)*4)
			gl.texSubImage2D(glTexture2D, 0, r.X, r.Y, r.Width, r.Height,
				glBGRA, glUnsignedByte, pboOffset(offset))
		}
		gl.pixelStorei(glUnpackRowLength, 0)
	}
	gl.bindTexture(glTexture2D, 0)

	// Swap PBO index for next frame.
	rp.pboIndex = 1 - rp.pboIndex

	gl.bindBuffer(glPixelUnpackBuffer, 0)
	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Debug().
			Uint64("render_seq", renderSeq).
			Uint64("paint_seq", paintSeq).
			Uint64("uploaded_bytes", uploadedBytes).
			Int("next_pbo_index", rp.pboIndex).
			Msg("cef: uploadToPBO end")
	}
	return uploadedBytes
}

func (rp *renderPipeline) uploadPopupTexture(renderSeq, paintSeq uint64) bool {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if !rp.popupVisible || rp.popupWidth <= 0 || rp.popupHeight <= 0 || len(rp.popupStaging) == 0 {
		return false
	}

	if !rp.popupNeedsUpload && !rp.popupSizeChanged && rp.popupTexture != 0 {
		return true
	}

	gl := rp.gl

	if rp.popupTexture == 0 {
		gl.genTextures(1, &rp.popupTexture)
		gl.bindTexture(glTexture2D, rp.popupTexture)
		gl.texParameteri(glTexture2D, glTextureMinFilter, int32(glLinear))
		gl.texParameteri(glTexture2D, glTextureMagFilter, int32(glLinear))
		gl.texParameteri(glTexture2D, glTextureWrapS, int32(glClampToEdge))
		gl.texParameteri(glTexture2D, glTextureWrapT, int32(glClampToEdge))
	} else {
		gl.bindTexture(glTexture2D, rp.popupTexture)
	}

	pixels := unsafe.Pointer(&rp.popupStaging[0])
	if rp.popupSizeChanged {
		gl.texImage2D(glTexture2D, 0, int32(glRGBA), rp.popupWidth, rp.popupHeight, 0, glBGRA, glUnsignedByte, pixels)
	} else {
		gl.texSubImage2D(glTexture2D, 0, 0, 0, rp.popupWidth, rp.popupHeight, glBGRA, glUnsignedByte, pixels)
	}
	gl.bindTexture(glTexture2D, 0)

	rp.popupNeedsUpload = false
	rp.popupSizeChanged = false

	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Debug().
			Uint64("render_seq", renderSeq).
			Uint64("paint_seq", paintSeq).
			Int32("popup_width", rp.popupWidth).
			Int32("popup_height", rp.popupHeight).
			Msg("cef: popup texture uploaded")
	}

	return true
}

func (rp *renderPipeline) drawPopupOverlay(viewWidth, viewHeight int32) {
	rp.mu.Lock()
	popupVisible := rp.popupVisible
	popupRect := rp.popupRect
	popupWidth := rp.popupWidth
	popupHeight := rp.popupHeight
	texture := rp.popupTexture
	rp.mu.Unlock()

	if !popupVisible || texture == 0 || popupWidth <= 0 || popupHeight <= 0 || viewWidth <= 0 || viewHeight <= 0 {
		return
	}

	popupRect.Width = popupWidth
	popupRect.Height = popupHeight
	popupRect, ok := clampDirtyRect(popupRect, viewWidth, viewHeight)
	if !ok {
		return
	}

	viewportY := viewHeight - popupRect.Y - popupRect.Height
	if viewportY < 0 {
		viewportY = 0
	}

	rp.gl.viewport(popupRect.X, viewportY, popupRect.Width, popupRect.Height)
	rp.drawTexture(texture)
	rp.gl.viewport(0, 0, viewWidth, viewHeight)
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

func (rp *renderPipeline) recordAcceleratedPaint() uint64 {
	rp.acceleratedPaintCount++
	rp.maybeLogDiagnostics()
	return rp.acceleratedPaintCount
}

func (rp *renderPipeline) maybeLogDiagnostics() {
	now := time.Now()
	if !rp.diagLastLogAt.IsZero() && now.Sub(rp.diagLastLogAt) < 2*time.Second {
		return
	}

	paintDelta := rp.paintCount - rp.diagLastPaintCount
	accelDelta := rp.acceleratedPaintCount - rp.diagLastAccelCount
	queueDelta := rp.queueRenderCount - rp.diagLastQueueCount
	renderDelta := rp.glRenderCount - rp.diagLastRenderCount
	uploadDelta := rp.uploadCount - rp.diagLastUploadCount
	fullUploadDelta := rp.fullUploadCount - rp.diagLastFullUploads
	paintBytesDelta := rp.paintBytes - rp.diagLastPaintBytes
	uploadBytesDelta := rp.uploadBytes - rp.diagLastUploadBytes
	paintCopyNsDelta := rp.paintCopyTotalNs - rp.diagLastPaintCopyNs
	uploadNsDelta := rp.uploadTotalNs - rp.diagLastUploadNs
	renderNsDelta := rp.renderTotalNs - rp.diagLastRenderNs
	if paintDelta == 0 && accelDelta == 0 && queueDelta == 0 && renderDelta == 0 && uploadDelta == 0 {
		return
	}

	elapsed := now.Sub(rp.diagLastLogAt)
	if rp.diagLastLogAt.IsZero() || elapsed <= 0 {
		elapsed = time.Second
	}
	elapsedSec := elapsed.Seconds()

	paintAvgUs := float64(0)
	if paintDelta > 0 {
		paintAvgUs = float64(paintCopyNsDelta) / float64(paintDelta) / 1_000
	}
	uploadAvgUs := float64(0)
	if uploadDelta > 0 {
		uploadAvgUs = float64(uploadNsDelta) / float64(uploadDelta) / 1_000
	}
	renderAvgUs := float64(0)
	if renderDelta > 0 {
		renderAvgUs = float64(renderNsDelta) / float64(renderDelta) / 1_000
	}

	logging.FromContext(rp.ctx).Debug().
		Float64("paint_hz", float64(paintDelta)/elapsedSec).
		Float64("queue_hz", float64(queueDelta)/elapsedSec).
		Float64("render_hz", float64(renderDelta)/elapsedSec).
		Float64("upload_hz", float64(uploadDelta)/elapsedSec).
		Uint64("paint_delta", paintDelta).
		Uint64("accelerated_paint_delta", accelDelta).
		Uint64("queue_delta", queueDelta).
		Uint64("render_delta", renderDelta).
		Uint64("upload_delta", uploadDelta).
		Uint64("full_upload_delta", fullUploadDelta).
		Float64("paint_mb", float64(paintBytesDelta)/(1024*1024)).
		Float64("upload_mb", float64(uploadBytesDelta)/(1024*1024)).
		Float64("avg_paint_copy_us", paintAvgUs).
		Float64("avg_upload_us", uploadAvgUs).
		Float64("avg_render_us", renderAvgUs).
		Float64("max_paint_copy_us", float64(rp.maxPaintCopyNs)/1_000).
		Float64("max_upload_us", float64(rp.maxUploadNs)/1_000).
		Float64("max_render_us", float64(rp.maxRenderNs)/1_000).
		Uint64("paint_total", rp.paintCount).
		Uint64("accelerated_paint_total", rp.acceleratedPaintCount).
		Uint64("render_total", rp.glRenderCount).
		Uint64("upload_total", rp.uploadCount).
		Msg("cef: render pipeline activity")

	rp.diagLastLogAt = now
	rp.diagLastPaintCount = rp.paintCount
	rp.diagLastAccelCount = rp.acceleratedPaintCount
	rp.diagLastQueueCount = rp.queueRenderCount
	rp.diagLastRenderCount = rp.glRenderCount
	rp.diagLastUploadCount = rp.uploadCount
	rp.diagLastFullUploads = rp.fullUploadCount
	rp.diagLastPaintBytes = rp.paintBytes
	rp.diagLastUploadBytes = rp.uploadBytes
	rp.diagLastPaintCopyNs = rp.paintCopyTotalNs
	rp.diagLastUploadNs = rp.uploadTotalNs
	rp.diagLastRenderNs = rp.renderTotalNs
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
// TODO: add glGetShaderiv + glGetShaderInfoLog to gl_loader to check compilation status.
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
	rp.widthAtomic.Store(rp.width)
	rp.heightAtomic.Store(rp.height)
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

func (rp *renderPipeline) setPopupVisible(show bool) {
	rp.mu.Lock()
	rp.popupVisible = show
	if !show {
		rp.popupStaging = nil
		rp.popupWidth = 0
		rp.popupHeight = 0
		rp.popupNeedsUpload = false
		rp.popupSizeChanged = false
	}
	rp.mu.Unlock()
	rp.glArea.QueueRender()
}

func (rp *renderPipeline) setPopupRect(popup rect) {
	rp.mu.Lock()
	rp.popupRect = rect{
		X:      popup.X * rp.scale,
		Y:      popup.Y * rp.scale,
		Width:  popup.Width * rp.scale,
		Height: popup.Height * rp.scale,
	}
	rp.mu.Unlock()
	rp.glArea.QueueRender()
}

func (rp *renderPipeline) handlePopupPaint(buffer []byte, width, height int32, paintSeq uint64) {
	if len(buffer) == 0 || width <= 0 || height <= 0 {
		return
	}

	bufSize, ok := bgraBufferSize(width, height)
	if !ok {
		return
	}

	rp.mu.Lock()
	if len(rp.popupStaging) != bufSize {
		rp.popupStaging = make([]byte, bufSize)
		rp.popupSizeChanged = true
	} else if rp.popupWidth != width || rp.popupHeight != height {
		rp.popupSizeChanged = true
	}
	copied := copy(rp.popupStaging, buffer)
	if copied < len(rp.popupStaging) {
		clear(rp.popupStaging[copied:])
	}
	rp.popupWidth = width
	rp.popupHeight = height
	rp.popupNeedsUpload = true
	rp.popupVisible = true
	rp.mu.Unlock()

	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Debug().
			Uint64("paint_seq", paintSeq).
			Int32("popup_width", width).
			Int32("popup_height", height).
			Msg("cef: handlePopupPaint")
	}

	rp.glArea.QueueRender()
}

func (rp *renderPipeline) viewRectSize() (width, height, scale int32) {
	width = rp.widthAtomic.Load()
	height = rp.heightAtomic.Load()
	if width <= 0 || height <= 0 {
		rp.mu.Lock()
		width = rp.width
		height = rp.height
		rp.mu.Unlock()
	}
	scale = rp.scale
	if scale <= 0 {
		scale = 1
	}
	return width, height, scale
}

func (rp *renderPipeline) nextViewRectSeq() uint64 {
	return rp.viewRectSeq.Add(1)
}

func (rp *renderPipeline) nextScreenInfoSeq() uint64 {
	return rp.screenInfoSeq.Add(1)
}

func (rp *renderPipeline) nextPaintSeq() uint64 {
	return rp.paintSeq.Add(1)
}

func clampDirtyRect(r rect, width, height int32) (rect, bool) {
	if width <= 0 || height <= 0 || r.Width <= 0 || r.Height <= 0 {
		return rect{}, false
	}

	x0 := max(r.X, 0)
	y0 := max(r.Y, 0)
	x1 := min(r.X+r.Width, width)
	y1 := min(r.Y+r.Height, height)

	if x1 <= x0 || y1 <= y0 {
		return rect{}, false
	}

	return rect{
		X:      x0,
		Y:      y0,
		Width:  x1 - x0,
		Height: y1 - y0,
	}, true
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
	if rp.popupTexture != 0 {
		gl.deleteTextures(1, &rp.popupTexture)
		rp.popupTexture = 0
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

func bgraBufferSize(width, height int32) (int, bool) {
	if width <= 0 || height <= 0 {
		return 0, false
	}
	size64 := int64(width) * int64(height) * 4
	if size64 <= 0 {
		return 0, false
	}
	return int(size64), true
}

func stagingNeedsReset(staging []byte, currentWidth, currentHeight, nextWidth, nextHeight int32, expectedBytes int) bool {
	return staging == nil ||
		len(staging) != expectedBytes ||
		currentWidth != nextWidth ||
		currentHeight != nextHeight
}

func copyDirtyRectsIntoStaging(dst, src []byte, width, height int32, rects []rect) (uint64, []rect, bool) {
	if len(dst) == 0 || len(src) == 0 || width <= 0 || height <= 0 {
		return 0, nil, false
	}

	stride := int(width) * 4
	copiedBytes := uint64(0)
	truncated := false
	sanitized := make([]rect, 0, len(rects))

	for _, rawRect := range rects {
		r, ok := clampDirtyRect(rawRect, width, height)
		if !ok {
			continue
		}
		sanitized = append(sanitized, r)

		for row := r.Y; row < r.Y+r.Height; row++ {
			dstOff := int(row)*stride + int(r.X)*4
			if dstOff < 0 || dstOff >= len(dst) {
				truncated = true
				break
			}

			rowBytes := int(r.Width) * 4
			dstEnd := dstOff + rowBytes
			if dstEnd > len(dst) {
				dstEnd = len(dst)
				truncated = true
			}

			srcOff := int(row)*stride + int(r.X)*4
			if srcOff < 0 || srcOff >= len(src) {
				clear(dst[dstOff:dstEnd])
				truncated = true
				continue
			}

			srcEnd := srcOff + rowBytes
			if srcEnd > len(src) {
				srcEnd = len(src)
				truncated = true
			}

			copyLen := srcEnd - srcOff
			maxDstLen := dstEnd - dstOff
			if copyLen > maxDstLen {
				copyLen = maxDstLen
				truncated = true
			}

			if copyLen > 0 {
				copy(dst[dstOff:dstOff+copyLen], src[srcOff:srcOff+copyLen])
				copiedBytes += uint64(copyLen)
			}

			if dstOff+copyLen < dstEnd {
				clear(dst[dstOff+copyLen : dstEnd])
			}
		}
	}

	return copiedBytes, sanitized, truncated
}

// pboOffset converts a byte offset into an unsafe.Pointer for use with
// glTexSubImage2D when a PBO is bound. OpenGL interprets the pointer as a
// byte offset into the bound buffer rather than a real memory address.
func pboOffset(offset uintptr) unsafe.Pointer {
	return unsafe.Add(nil, offset)
}
