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
// copies dirty regions into a CPU-side staging buffer on the CEF UI thread,
// and GTK later uploads from staging → PBO → texture before drawing a
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
	// Atomics allow lock-free reads from CEF callbacks (viewRectSize).
	widthAtomic  atomic.Int32
	heightAtomic atomic.Int32
	scale        int32

	// Staging: handlePaint copies dirty rects here, render signal uploads.
	mu              sync.Mutex
	staging         []byte
	dirtyRects      []rect
	needsUpload     bool
	sizeChanged     bool
	forceFullUpload bool

	// Popup staging for native CEF widgets like <select> menus.
	popupVisible     bool
	popupRect        rect
	popupStaging     []byte
	popupWidth       int32
	popupHeight      int32
	popupNeedsUpload bool
	popupSizeChanged bool

	// GL initialized flag.
	glReady  bool
	glWidth  int32
	glHeight int32

	// Diagnostics can be updated from both the CEF UI thread and GTK thread.
	diagMu                sync.Mutex
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

// handlePaint is called from CEF's OnPaint callback. It copies dirty rect
// regions from the transient CEF buffer into the persistent staging buffer and
// marks the next GTK render for upload.
func (rp *renderPipeline) handlePaint(buffer []byte, width, height int32, rects []rect, paintSeq uint64) {
	if len(buffer) == 0 || width <= 0 || height <= 0 {
		return
	}

	startedAt := time.Now()
	rp.lastQueuedPaintSeq.Store(paintSeq)
	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Trace().
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
	if stagingNeedsReset(rp.staging, rp.widthAtomic.Load(), rp.heightAtomic.Load(), width, height, bufSize) {
		rp.widthAtomic.Store(width)
		rp.heightAtomic.Store(height)
		rp.staging = make([]byte, bufSize)
		rp.sizeChanged = true
		rp.forceFullUpload = true
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
	rp.dirtyRects = coalesceDirtyRects(rp.dirtyRects, rp.widthAtomic.Load(), rp.heightAtomic.Load())
	rp.needsUpload = true

	rp.mu.Unlock()

	copyDuration := time.Since(startedAt)
	rp.diagMu.Lock()
	rp.paintCount++
	rp.paintBytes += copiedBytes
	rp.dirtyRectCount += uint64(len(rects))
	rp.paintCopyTotalNs += uint64(copyDuration)
	if copyDuration.Nanoseconds() > rp.maxPaintCopyNs {
		rp.maxPaintCopyNs = copyDuration.Nanoseconds()
	}
	rp.diagMu.Unlock()

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
		logging.FromContext(rp.ctx).Trace().
			Uint64("paint_seq", paintSeq).
			Uint64("copied_bytes", copiedBytes).
			Bool("size_changed", sizeChanged).
			Dur("copy_duration", copyDuration).
			Msg("cef: handlePaint end")
	}
	rp.maybeLogDiagnostics()
}

func (rp *renderPipeline) queuePaintRender() {
	rp.diagMu.Lock()
	rp.queueRenderCount++
	rp.diagMu.Unlock()
	rp.glArea.QueueRender()
}

// onGLRender is the GTK "render" signal handler. GL context is current.
func (rp *renderPipeline) onGLRender() bool {
	renderSeq := rp.glRenderSeq.Add(1)
	paintSeq := rp.lastQueuedPaintSeq.Load()
	renderStartedAt := time.Now()
	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Trace().
			Uint64("render_seq", renderSeq).
			Uint64("paint_seq", paintSeq).
			Msg("cef: onGLRender begin")
	}
	rp.mu.Lock()

	needsUpload := rp.needsUpload
	sizeChanged := rp.sizeChanged
	uploadWidth := rp.widthAtomic.Load()
	uploadHeight := rp.heightAtomic.Load()

	rp.mu.Unlock()

	gl := rp.gl

	if !rp.glReady || sizeChanged {
		rp.initGL(uploadWidth, uploadHeight)
	}

	if !rp.glReady {
		renderDuration := time.Since(renderStartedAt)
		rp.diagMu.Lock()
		rp.glRenderCount++
		rp.renderTotalNs += uint64(renderDuration)
		if renderDuration.Nanoseconds() > rp.maxRenderNs {
			rp.maxRenderNs = renderDuration.Nanoseconds()
		}
		rp.diagMu.Unlock()
		rp.maybeLogDiagnostics()
		return true
	}

	if needsUpload {
		uploadStartedAt := time.Now()
		uploadedBytes, fullUpload, didUpload := rp.uploadToPBO(renderSeq, paintSeq)
		if didUpload {
			uploadDuration := time.Since(uploadStartedAt)
			rp.diagMu.Lock()
			rp.uploadCount++
			if fullUpload {
				rp.fullUploadCount++
			}
			rp.uploadBytes += uploadedBytes
			rp.uploadTotalNs += uint64(uploadDuration)
			if uploadDuration.Nanoseconds() > rp.maxUploadNs {
				rp.maxUploadNs = uploadDuration.Nanoseconds()
			}
			rp.diagMu.Unlock()
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

	renderDuration := time.Since(renderStartedAt)
	rp.diagMu.Lock()
	rp.glRenderCount++
	rp.renderTotalNs += uint64(renderDuration)
	if renderDuration.Nanoseconds() > rp.maxRenderNs {
		rp.maxRenderNs = renderDuration.Nanoseconds()
	}
	rp.diagMu.Unlock()
	rp.maybeLogDiagnostics()
	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Trace().
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
// transfers to the GL texture via async DMA. The mutex serializes CEF-thread
// staging writes against GTK-thread PBO copies so the upload sees a consistent
// staging buffer and dirty-rect set.
func (rp *renderPipeline) uploadToPBO(renderSeq, paintSeq uint64) (uint64, bool, bool) {
	gl := rp.gl

	rp.mu.Lock()
	w := rp.glWidth
	h := rp.glHeight
	needsUpload := rp.needsUpload
	rp.mu.Unlock()
	if !needsUpload || w <= 0 || h <= 0 {
		return 0, false, false
	}

	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Trace().
			Uint64("render_seq", renderSeq).
			Uint64("paint_seq", paintSeq).
			Int32("width", w).
			Int32("height", h).
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
		return 0, false, false
	}

	mappedSlice := unsafe.Slice((*byte)(mapped), int(bufSize))
	uploadedBytes := uint64(0)
	fullUpload := false
	dirtyRects := make([]rect, 0)

	rp.mu.Lock()
	if !rp.needsUpload {
		rp.mu.Unlock()
		if !gl.unmapBuffer(glPixelUnpackBuffer) {
			gl.bindBuffer(glPixelUnpackBuffer, 0)
			return 0, false, false
		}
		gl.bindBuffer(glPixelUnpackBuffer, 0)
		return 0, false, false
	}

	if rp.widthAtomic.Load() != rp.glWidth || rp.heightAtomic.Load() != rp.glHeight {
		rp.mu.Unlock()
		if !gl.unmapBuffer(glPixelUnpackBuffer) {
			gl.bindBuffer(glPixelUnpackBuffer, 0)
			return 0, false, false
		}
		gl.bindBuffer(glPixelUnpackBuffer, 0)
		if rp.ctx != nil {
			logging.FromContext(rp.ctx).Trace().
				Uint64("render_seq", renderSeq).
				Uint64("paint_seq", paintSeq).
				Msg("cef: skipping upload while newer surface resize is pending")
		}
		return 0, false, false
	}

	fullUpload = rp.forceFullUpload
	if fullUpload {
		copied := copy(mappedSlice, rp.staging)
		if copied < len(mappedSlice) {
			clear(mappedSlice[copied:])
		}
		uploadedBytes = uint64(copied)
	} else {
		dirtyRects = append(dirtyRects, rp.dirtyRects...)
		uploadedBytes = copyDirtyRectsRaw(mappedSlice, rp.staging, w, h, dirtyRects)
	}
	rp.needsUpload = false
	rp.forceFullUpload = false
	rp.dirtyRects = rp.dirtyRects[:0]
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
		return 0, fullUpload, false
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
		logging.FromContext(rp.ctx).Trace().
			Uint64("render_seq", renderSeq).
			Uint64("paint_seq", paintSeq).
			Uint64("uploaded_bytes", uploadedBytes).
			Int("dirty_rect_count", len(dirtyRects)).
			Bool("full_upload", fullUpload).
			Int("next_pbo_index", rp.pboIndex).
			Msg("cef: uploadToPBO end")
	}
	return uploadedBytes, fullUpload, true
}

func (rp *renderPipeline) uploadPopupTexture(renderSeq, paintSeq uint64) bool {
	// Copy staging data under lock, then release to avoid holding the mutex
	// during GL upload calls which may block on GPU synchronization.
	rp.mu.Lock()

	if !rp.popupVisible || rp.popupWidth <= 0 || rp.popupHeight <= 0 || len(rp.popupStaging) == 0 {
		rp.mu.Unlock()
		return false
	}

	if !rp.popupNeedsUpload && !rp.popupSizeChanged && rp.popupTexture != 0 {
		rp.mu.Unlock()
		return true
	}

	// Snapshot the staging data and dimensions while holding the lock.
	pixelsCopy := make([]byte, len(rp.popupStaging))
	copy(pixelsCopy, rp.popupStaging)
	popupW := rp.popupWidth
	popupH := rp.popupHeight
	sizeChanged := rp.popupSizeChanged
	needsCreate := rp.popupTexture == 0

	rp.mu.Unlock()

	// Perform GL operations without the lock.
	gl := rp.gl

	if needsCreate {
		var tex uint32
		gl.genTextures(1, &tex)
		gl.bindTexture(glTexture2D, tex)
		gl.texParameteri(glTexture2D, glTextureMinFilter, int32(glLinear))
		gl.texParameteri(glTexture2D, glTextureMagFilter, int32(glLinear))
		gl.texParameteri(glTexture2D, glTextureWrapS, int32(glClampToEdge))
		gl.texParameteri(glTexture2D, glTextureWrapT, int32(glClampToEdge))

		// Store the texture ID under lock.
		rp.mu.Lock()
		rp.popupTexture = tex
		rp.mu.Unlock()
	} else {
		rp.mu.Lock()
		tex := rp.popupTexture
		rp.mu.Unlock()
		gl.bindTexture(glTexture2D, tex)
	}

	pixels := unsafe.Pointer(&pixelsCopy[0])
	if sizeChanged {
		gl.texImage2D(glTexture2D, 0, int32(glRGBA), popupW, popupH, 0, glBGRA, glUnsignedByte, pixels)
	} else {
		gl.texSubImage2D(glTexture2D, 0, 0, 0, popupW, popupH, glBGRA, glUnsignedByte, pixels)
	}
	gl.bindTexture(glTexture2D, 0)

	// Update state under lock.
	rp.mu.Lock()
	rp.popupNeedsUpload = false
	rp.popupSizeChanged = false
	rp.mu.Unlock()

	if rp.ctx != nil {
		logging.FromContext(rp.ctx).Debug().
			Uint64("render_seq", renderSeq).
			Uint64("paint_seq", paintSeq).
			Int32("popup_width", popupW).
			Int32("popup_height", popupH).
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

// initGL creates or recreates all GL resources for the provided surface size.
func (rp *renderPipeline) initGL(width, height int32) {
	if width <= 0 || height <= 0 {
		return
	}

	gl := rp.gl

	// Clean up previous resources if reinitializing.
	if rp.glReady {
		rp.deleteGLResources()
	}

	rp.mu.Lock()
	if rp.widthAtomic.Load() == width && rp.heightAtomic.Load() == height {
		rp.sizeChanged = false
	}
	rp.forceFullUpload = true
	rp.glWidth = width
	rp.glHeight = height
	rp.mu.Unlock()

	// Texture.
	gl.genTextures(1, &rp.texture)
	gl.bindTexture(glTexture2D, rp.texture)
	gl.texParameteri(glTexture2D, glTextureMinFilter, int32(glLinear))
	gl.texParameteri(glTexture2D, glTextureMagFilter, int32(glLinear))
	gl.texParameteri(glTexture2D, glTextureWrapS, int32(glClampToEdge))
	gl.texParameteri(glTexture2D, glTextureWrapT, int32(glClampToEdge))
	gl.texImage2D(glTexture2D, 0, int32(glRGBA), width, height, 0, glBGRA, glUnsignedByte, nil)
	gl.bindTexture(glTexture2D, 0)

	// PBOs.
	bufSize := int64(width) * int64(height) * 4
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
	rp.diagMu.Lock()
	rp.acceleratedPaintCount++
	count := rp.acceleratedPaintCount
	rp.diagMu.Unlock()
	rp.maybeLogDiagnostics()
	return count
}

func (rp *renderPipeline) maybeLogDiagnostics() {
	rp.diagMu.Lock()
	defer rp.diagMu.Unlock()

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

	w, h := scaled(width), scaled(height)
	rp.widthAtomic.Store(w)
	rp.heightAtomic.Store(h)
	rp.sizeChanged = true

	firstCB := rp.onFirstResize
	if firstCB != nil {
		rp.onFirstResize = nil
	}
	resizeCB := rp.onResizeCB
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
	rp.glWidth = 0
	rp.glHeight = 0
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

// coalesceDirtyRects merges overlapping or edge-adjacent dirty rects to reduce
// redundant row-by-row PBO copies. If the total dirty area exceeds 60% of the
// surface, it collapses to a single full-surface rect.
//
// This is called inside handlePaint's mu.Lock() section so it must be fast for
// small n. O(n^2) is fine since n is typically < 20.
func coalesceDirtyRects(rects []rect, surfaceW, surfaceH int32) []rect {
	if len(rects) <= 1 {
		return rects
	}

	// Check if total dirty area exceeds 60% of the surface — if so,
	// a single full-surface upload is cheaper than per-rect copies.
	totalArea := int64(0)
	surfaceArea := int64(surfaceW) * int64(surfaceH)
	for _, r := range rects {
		totalArea += int64(r.Width) * int64(r.Height)
	}
	if surfaceArea > 0 && totalArea*100 > surfaceArea*60 {
		return []rect{{0, 0, surfaceW, surfaceH}}
	}

	// Merge overlapping or edge-adjacent rects. Two rects touch if their
	// X ranges overlap-or-touch AND Y ranges overlap-or-touch. When merged,
	// both are replaced with their bounding box. Repeat until stable.
	merged := make([]rect, len(rects))
	copy(merged, rects)

	changed := true
	for changed {
		changed = false
		for i := 0; i < len(merged); i++ {
			for j := i + 1; j < len(merged); j++ {
				if rectsTouch(merged[i], merged[j]) {
					merged[i] = boundingBox(merged[i], merged[j])
					// Remove j by swapping with last element.
					merged[j] = merged[len(merged)-1]
					merged = merged[:len(merged)-1]
					changed = true
					j-- // re-check the element now at index j
				}
			}
		}
	}

	return merged
}

// rectsTouch returns true if two rects overlap or share an edge (adjacent).
func rectsTouch(a, b rect) bool {
	// a's right edge <= b's left edge → no horizontal contact
	if a.X+a.Width < b.X || b.X+b.Width < a.X {
		return false
	}
	// a's bottom edge <= b's top edge → no vertical contact
	if a.Y+a.Height < b.Y || b.Y+b.Height < a.Y {
		return false
	}
	return true
}

// boundingBox returns the smallest rect containing both a and b.
func boundingBox(a, b rect) rect {
	x := a.X
	if b.X < x {
		x = b.X
	}
	y := a.Y
	if b.Y < y {
		y = b.Y
	}
	right := a.X + a.Width
	if br := b.X + b.Width; br > right {
		right = br
	}
	bottom := a.Y + a.Height
	if bb := b.Y + b.Height; bb > bottom {
		bottom = bb
	}
	return rect{X: x, Y: y, Width: right - x, Height: bottom - y}
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

// copyDirtyRectsRaw copies only the rows covered by dirty rects from src into
// dst at matching offsets. Both src and dst must be BGRA buffers of dimensions
// width x height. Unlike copyDirtyRectsIntoStaging, the destination buffer's
// existing data outside dirty rect rows is left untouched — the caller must
// ensure that only the copied rows will be read downstream.
//
// On a full-surface dirty rect (e.g. resize), this degrades to copying all
// rows — equivalent to a full buffer copy.
//
// Returns the total bytes copied.
func copyDirtyRectsRaw(dst, src []byte, width, height int32, rects []rect) uint64 {
	if len(dst) == 0 || len(src) == 0 || width <= 0 || height <= 0 {
		return 0
	}

	stride := int(width) * 4
	copiedBytes := uint64(0)

	for _, rawRect := range rects {
		r, ok := clampDirtyRect(rawRect, width, height)
		if !ok {
			continue
		}

		for row := r.Y; row < r.Y+r.Height; row++ {
			dstOff := int(row)*stride + int(r.X)*4
			if dstOff < 0 || dstOff >= len(dst) {
				break
			}

			rowBytes := int(r.Width) * 4
			dstEnd := dstOff + rowBytes
			if dstEnd > len(dst) {
				dstEnd = len(dst)
			}

			srcOff := int(row)*stride + int(r.X)*4
			if srcOff < 0 || srcOff >= len(src) {
				continue
			}

			srcEnd := srcOff + rowBytes
			if srcEnd > len(src) {
				srcEnd = len(src)
			}

			copyLen := srcEnd - srcOff
			maxDstLen := dstEnd - dstOff
			if copyLen > maxDstLen {
				copyLen = maxDstLen
			}

			if copyLen > 0 {
				copy(dst[dstOff:dstOff+copyLen], src[srcOff:srcOff+copyLen])
				copiedBytes += uint64(copyLen)
			}
		}
	}

	return copiedBytes
}

// pboOffset converts a byte offset into an unsafe.Pointer for use with
// glTexSubImage2D when a PBO is bound. OpenGL interprets the pointer as a
// byte offset into the bound buffer rather than a real memory address.
func pboOffset(offset uintptr) unsafe.Pointer {
	return unsafe.Add(nil, offset)
}
