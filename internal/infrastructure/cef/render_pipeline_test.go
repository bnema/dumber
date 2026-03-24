package cef

import (
	"bytes"
	"testing"

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
