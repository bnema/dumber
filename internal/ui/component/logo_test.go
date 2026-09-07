package component

import (
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/bnema/dumber/assets"
	"github.com/bnema/puregotk/v4/gdk"
	"github.com/stretchr/testify/require"
)

func resetLogoForTest() {
	logoTexture = nil
	logoTextureOnce = sync.Once{}
}

func TestLogoPNG512AssetSignatureAndDimensions(t *testing.T) {
	data := assets.LogoPNG512
	require.NotEmpty(t, data)
	require.Equal(t, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, data[:8])
	require.GreaterOrEqual(t, len(data), 33)
	require.Equal(t, "IHDR", string(data[12:16]))
	width := binary.BigEndian.Uint32(data[16:20])
	height := binary.BigEndian.Uint32(data[20:24])
	require.Equal(t, uint32(512), width)
	require.Equal(t, uint32(512), height)
	require.NotEmpty(t, assets.LogoSVG, "SVG source for desktop icons must remain")
}

func TestGetLogoTextureRequestsRasterOnce(t *testing.T) {
	resetLogoForTest()
	origAsset, origDecode := logoAssetBytes, logoDecodeTexture
	defer func() {
		logoAssetBytes, logoDecodeTexture = origAsset, origDecode
		resetLogoForTest()
	}()
	var calls atomic.Int32
	logoAssetBytes = func() []byte {
		calls.Add(1)
		return assets.LogoPNG512
	}
	logoDecodeTexture = func(data []byte) (*gdk.Texture, error) {
		require.Equal(t, assets.LogoPNG512, data)
		return nil, errors.New("native unavailable in unit test")
	}
	require.NotPanics(t, func() {
		require.Nil(t, GetLogoTexture())
		require.Nil(t, GetLogoTexture())
	})
	require.Equal(t, int32(1), calls.Load(), "raster bytes must be requested once")
}

func TestGetLogoTextureConcurrentCallersShareResult(t *testing.T) {
	resetLogoForTest()
	origAsset, origDecode := logoAssetBytes, logoDecodeTexture
	defer func() {
		logoAssetBytes, logoDecodeTexture = origAsset, origDecode
		resetLogoForTest()
	}()
	var calls atomic.Int32
	logoAssetBytes = func() []byte {
		calls.Add(1)
		return assets.LogoPNG512
	}
	logoDecodeTexture = func([]byte) (*gdk.Texture, error) {
		return nil, errors.New("native unavailable in unit test")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = GetLogoTexture()
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), calls.Load())
}

func TestGetLogoTextureFailureDoesNotPanic(t *testing.T) {
	resetLogoForTest()
	origAsset, origDecode := logoAssetBytes, logoDecodeTexture
	defer func() {
		logoAssetBytes, logoDecodeTexture = origAsset, origDecode
		resetLogoForTest()
	}()
	logoAssetBytes = func() []byte { return nil }
	logoDecodeTexture = func([]byte) (*gdk.Texture, error) {
		return nil, errors.New("must not be called with empty asset")
	}
	require.NotPanics(t, func() {
		require.Nil(t, GetLogoTexture())
	})
}
