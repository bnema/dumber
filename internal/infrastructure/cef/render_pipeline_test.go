package cef

import (
	"bytes"
	"context"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStagingNeedsResetWhenBufferSizeMismatches(t *testing.T) {
	staging := make([]byte, 8)

	require.True(t, stagingNeedsReset(staging, 2, 2, 2, 2, 16))
	require.False(t, stagingNeedsReset(make([]byte, 16), 2, 2, 2, 2, 16))
}

func TestCopyDirtyRectsIntoStagingClampsTruncatedBuffer(t *testing.T) {
	const (
		width  = int32(4)
		height = int32(2)
	)

	dst := bytes.Repeat([]byte{0xaa}, 32)
	src := make([]byte, 28)
	for i := range src {
		src[i] = byte(i + 1)
	}

	copied, sanitized, truncated := copyDirtyRectsIntoStaging(dst, src, width, height, []rect{{
		X:      0,
		Y:      0,
		Width:  width,
		Height: height,
	}})

	require.True(t, truncated)
	require.Equal(t, uint64(len(src)), copied)
	require.Len(t, sanitized, 1)
	require.Equal(t, src, dst[:len(src)])
	require.Equal(t, []byte{0, 0, 0, 0}, dst[len(src):])
}

func TestHandlePaintCopiesDirtyRectsIntoPersistentStaging(t *testing.T) {
	rp := &renderPipeline{
		ctx:   context.Background(),
		scale: 1,
	}

	initial := []byte{
		1, 2, 3, 4,
		5, 6, 7, 8,
		9, 10, 11, 12,
		13, 14, 15, 16,
	}
	rp.handlePaint(initial, 2, 2, []rect{{X: 0, Y: 0, Width: 2, Height: 2}}, 1)

	require.Equal(t, initial, rp.staging)
	require.True(t, rp.needsUpload)
	require.True(t, rp.sizeChanged)
	require.True(t, rp.forceFullUpload)

	rp.mu.Lock()
	rp.dirtyRects = nil
	rp.needsUpload = false
	rp.sizeChanged = false
	rp.forceFullUpload = false
	rp.mu.Unlock()

	updated := append([]byte(nil), initial...)
	copy(updated[8:12], []byte{42, 43, 44, 45})
	rp.handlePaint(updated, 2, 2, []rect{{X: 0, Y: 1, Width: 1, Height: 1}}, 2)

	require.Equal(t, updated, rp.staging)
	require.Equal(t, []rect{{X: 0, Y: 1, Width: 1, Height: 1}}, rp.dirtyRects)
	require.True(t, rp.needsUpload)
	require.False(t, rp.sizeChanged)
	require.False(t, rp.forceFullUpload)
}

// sortRects sorts rects by (X, Y) so tests can compare without caring about order.
func sortRects(rects []rect) {
	sort.Slice(rects, func(i, j int) bool {
		if rects[i].X != rects[j].X {
			return rects[i].X < rects[j].X
		}
		return rects[i].Y < rects[j].Y
	})
}

func TestCoalesceDirtyRectsEmpty(t *testing.T) {
	result := coalesceDirtyRects(nil, 1920, 1080)
	assert.Nil(t, result)

	result = coalesceDirtyRects([]rect{}, 1920, 1080)
	assert.Empty(t, result)
}

func TestCoalesceDirtyRectsSingle(t *testing.T) {
	input := []rect{{10, 20, 30, 40}}
	result := coalesceDirtyRects(input, 1920, 1080)
	require.Len(t, result, 1)
	assert.Equal(t, rect{10, 20, 30, 40}, result[0])
}

func TestCoalesceDirtyRectsDisjoint(t *testing.T) {
	// Top-left cursor blink and bottom-right scrollbar: should NOT merge.
	input := []rect{
		{0, 0, 2, 16},        // cursor at top-left
		{1900, 1060, 20, 20}, // scrollbar at bottom-right
	}
	result := coalesceDirtyRects(input, 1920, 1080)
	require.Len(t, result, 2)
	sortRects(result)
	assert.Equal(t, rect{0, 0, 2, 16}, result[0])
	assert.Equal(t, rect{1900, 1060, 20, 20}, result[1])
}

func TestCoalesceDirtyRectsOverlapping(t *testing.T) {
	// Two overlapping rects merge to their bounding box.
	input := []rect{
		{10, 10, 30, 30},
		{20, 20, 30, 30},
	}
	result := coalesceDirtyRects(input, 1920, 1080)
	require.Len(t, result, 1)
	assert.Equal(t, rect{10, 10, 40, 40}, result[0])
}

func TestCoalesceDirtyRectsEdgeAdjacent(t *testing.T) {
	// Two rects sharing an edge (right edge of A touches left edge of B).
	input := []rect{
		{0, 0, 50, 100},
		{50, 0, 50, 100},
	}
	result := coalesceDirtyRects(input, 1920, 1080)
	require.Len(t, result, 1)
	assert.Equal(t, rect{0, 0, 100, 100}, result[0])
}

func TestCoalesceDirtyRectsEdgeAdjacentVertical(t *testing.T) {
	// Two rects sharing a vertical edge (bottom of A touches top of B).
	input := []rect{
		{0, 0, 100, 50},
		{0, 50, 100, 50},
	}
	result := coalesceDirtyRects(input, 1920, 1080)
	require.Len(t, result, 1)
	assert.Equal(t, rect{0, 0, 100, 100}, result[0])
}

func TestCoalesceDirtyRectsTwoOverlapOneDisjoint(t *testing.T) {
	// Three rects: first two overlap, third is disjoint.
	input := []rect{
		{10, 10, 20, 20},
		{20, 10, 20, 20},
		{500, 500, 10, 10},
	}
	result := coalesceDirtyRects(input, 1920, 1080)
	require.Len(t, result, 2)
	sortRects(result)
	assert.Equal(t, rect{10, 10, 30, 20}, result[0])
	assert.Equal(t, rect{500, 500, 10, 10}, result[1])
}

func TestCoalesceDirtyRectsAreaThreshold(t *testing.T) {
	// Total area > 60% of surface → collapses to single full-surface rect.
	// Surface is 100x100 = 10000 pixels. 60% = 6000.
	// Two rects: 4000 + 3000 = 7000 > 6000.
	input := []rect{
		{0, 0, 100, 40},  // 4000 pixels
		{0, 40, 100, 30}, // 3000 pixels
	}
	result := coalesceDirtyRects(input, 100, 100)
	require.Len(t, result, 1)
	assert.Equal(t, rect{0, 0, 100, 100}, result[0])
}

func TestCoalesceDirtyRectsAreaThresholdBelowDoesNotCollapse(t *testing.T) {
	// Total area <= 60% of surface → no full-surface collapse.
	// Surface is 100x100 = 10000. 59% = 5900.
	input := []rect{
		{0, 0, 100, 29},  // 2900 pixels
		{0, 70, 100, 29}, // 2900 pixels → total 5800 < 6000
	}
	result := coalesceDirtyRects(input, 100, 100)
	// These two rects are disjoint (gap from Y=29 to Y=70), so no merge.
	require.Len(t, result, 2)
}

func TestCoalesceDirtyRectsFullyContained(t *testing.T) {
	// Rect A fully inside rect B → merges to rect B.
	input := []rect{
		{10, 10, 5, 5},   // small rect inside
		{0, 0, 100, 100}, // large outer rect
	}
	result := coalesceDirtyRects(input, 1920, 1080)
	require.Len(t, result, 1)
	assert.Equal(t, rect{0, 0, 100, 100}, result[0])
}

func TestCoalesceDirtyRectsChainMerge(t *testing.T) {
	// Three rects in a chain: A overlaps B, B overlaps C, but A and C don't.
	// All three should merge into one bounding box after iterative merging.
	input := []rect{
		{0, 0, 20, 20},
		{15, 0, 20, 20},
		{30, 0, 20, 20},
	}
	result := coalesceDirtyRects(input, 1920, 1080)
	require.Len(t, result, 1)
	assert.Equal(t, rect{0, 0, 50, 20}, result[0])
}

func TestCoalesceDirtyRectsDoesNotMutateInput(t *testing.T) {
	input := []rect{
		{10, 10, 30, 30},
		{20, 20, 30, 30},
	}
	inputCopy := make([]rect, len(input))
	copy(inputCopy, input)

	coalesceDirtyRects(input, 1920, 1080)

	// Original input should be unchanged.
	assert.Equal(t, inputCopy, input)
}

func TestRectsTouchSeparatedByGap(t *testing.T) {
	// One pixel gap between rects → should NOT touch.
	a := rect{0, 0, 10, 10}
	b := rect{11, 0, 10, 10}
	assert.False(t, rectsTouch(a, b))
}

func TestRectsTouchExactlyAdjacent(t *testing.T) {
	// Right edge of A at X=10, left edge of B at X=10 → touching.
	a := rect{0, 0, 10, 10}
	b := rect{10, 0, 10, 10}
	assert.True(t, rectsTouch(a, b))
}
