package cache

import (
	"crypto/md5"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"  // Register GIF decoder
	_ "image/jpeg" // Register JPEG decoder
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/sergeymakinen/go-ico"
)

// FaviconCache manages favicon caching to local files
type FaviconCache struct {
	cacheDir string
}

// NewFaviconCache creates a new favicon cache
func NewFaviconCache() (*FaviconCache, error) {
	dataDir, err := config.GetDataDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get data directory: %w", err)
	}

	cacheDir := filepath.Join(dataDir, "favicons")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create favicon cache directory: %w", err)
	}

	return &FaviconCache{
		cacheDir: cacheDir,
	}, nil
}

// GetCachedPath returns the local file path for a favicon URL if cached, empty string otherwise
func (fc *FaviconCache) GetCachedPath(faviconURL string) string {
	if faviconURL == "" {
		return ""
	}

	filename := fc.getFilename(faviconURL)
	cachedPath := filepath.Join(fc.cacheDir, filename)

	// Check if file exists and is not too old (7 days)
	if info, err := os.Stat(cachedPath); err == nil {
		if time.Since(info.ModTime()) < 7*24*time.Hour {
			return cachedPath
		}
	}

	return ""
}

// CacheAsync downloads and caches a favicon asynchronously
func (fc *FaviconCache) CacheAsync(faviconURL string) {
	if faviconURL == "" {
		return
	}

	go func() {
		if err := fc.downloadFavicon(faviconURL); err != nil {
			// Silently ignore errors in background download
		}
	}()
}

// getFilename generates a safe filename for a favicon URL - always PNG for fuzzel compatibility
func (fc *FaviconCache) getFilename(faviconURL string) string {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(faviconURL)))
	return hash + ".png"
}

// downloadFavicon downloads a favicon and saves it to cache
func (fc *FaviconCache) downloadFavicon(faviconURL string) error {
	filename := fc.getFilename(faviconURL)
	cachedPath := filepath.Join(fc.cacheDir, filename)

	// Don't download if already exists and is recent
	if info, err := os.Stat(cachedPath); err == nil {
		if time.Since(info.ModTime()) < 7*24*time.Hour {
			return nil
		}
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(faviconURL)
	if err != nil {
		return fmt.Errorf("failed to download favicon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download favicon: status %d", resp.StatusCode)
	}

	// Limit response body size
	limitedBody := io.LimitReader(resp.Body, 1024*1024) // 1MB max

	// Download to temporary file first
	tempDownloadFile := cachedPath + ".download"
	downloadFile, err := os.Create(tempDownloadFile)
	if err != nil {
		return fmt.Errorf("failed to create download temp file: %w", err)
	}
	defer downloadFile.Close()
	defer os.Remove(tempDownloadFile)

	// Save raw favicon data
	if _, err := io.Copy(downloadFile, limitedBody); err != nil {
		return fmt.Errorf("failed to save favicon data: %w", err)
	}
	downloadFile.Close()

	// Detect if it's an ICO file and convert with ffmpeg if needed
	if err := fc.convertToPNG(tempDownloadFile, cachedPath, faviconURL); err != nil {
		return fmt.Errorf("failed to convert favicon: %w", err)
	}

	return nil
}

// convertToPNG converts a favicon file to PNG format
func (fc *FaviconCache) convertToPNG(inputPath, outputPath, faviconURL string) error {
	// Check if it's an ICO file based on URL or content type
	if strings.Contains(strings.ToLower(faviconURL), ".ico") {
		log.Printf("[favicon] ICO file detected for %s, using go-ico decoder", faviconURL)
		return fc.convertICOWithGoICO(inputPath, outputPath, faviconURL)
	}

	// Try standard Go image decoder for other formats (PNG, JPEG, GIF)
	if img, err := fc.tryStandardImageDecode(inputPath); err == nil {
		log.Printf("[favicon] Successfully decoded %s with standard library", faviconURL)

		// Apply dark theme improvements
		img = invertIfTooDark(img)

		// Save as PNG
		return fc.savePNG(img, outputPath)
	}

	// Standard decode failed, fallback to go-ico
	log.Printf("[favicon] Standard decode failed for %s, trying go-ico decoder", faviconURL)
	return fc.convertICOWithGoICO(inputPath, outputPath, faviconURL)
}

// tryStandardImageDecode attempts to decode image with Go standard library
func (fc *FaviconCache) tryStandardImageDecode(filePath string) (image.Image, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	return img, err
}

// savePNG saves an image as PNG
func (fc *FaviconCache) savePNG(img image.Image, outputPath string) error {
	tempFile := outputPath + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()

	if err := png.Encode(file, img); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to encode PNG: %w", err)
	}

	file.Close()

	// Atomic move
	if err := os.Rename(tempFile, outputPath); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to move PNG to cache: %w", err)
	}

	return nil
}

// convertICOWithGoICO converts ICO files to PNG using go-ico library
func (fc *FaviconCache) convertICOWithGoICO(inputPath, outputPath, faviconURL string) error {
	// Open the ICO file
	file, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open ICO file: %w", err)
	}
	defer file.Close()

	// Decode ICO file
	img, err := ico.Decode(file)
	if err != nil {
		return fmt.Errorf("failed to decode ICO file %s: %w", faviconURL, err)
	}

	log.Printf("[favicon] Successfully decoded ICO file %s with go-ico", faviconURL)

	// Apply dark theme improvements
	img = invertIfTooDark(img)

	// Save as PNG
	return fc.savePNG(img, outputPath)
}

// CleanOld removes cached favicons older than 30 days
func (fc *FaviconCache) CleanOld() error {
	entries, err := os.ReadDir(fc.cacheDir)
	if err != nil {
		return err
	}

	cutoff := time.Now().Add(-30 * 24 * time.Hour)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			os.Remove(filepath.Join(fc.cacheDir, entry.Name()))
		}
	}

	return nil
}

// invertIfTooDark inverts icons that are too dark for dark theme visibility
// Only inverts truly monochrome dark icons, preserves colorful icons
func invertIfTooDark(img image.Image) image.Image {
	bounds := img.Bounds()

	// Sample pixels to calculate statistics
	var totalR, totalG, totalB uint32
	var pixelCount uint32
	var colorVariance uint32
	var transparentPixels uint32

	// Sample every 4th pixel for performance
	for y := bounds.Min.Y; y < bounds.Max.Y; y += 4 {
		for x := bounds.Min.X; x < bounds.Max.X; x += 4 {
			r, g, b, a := img.At(x, y).RGBA()

			if a == 0 {
				transparentPixels++
				continue
			}

			if a > 0 { // Only count non-transparent pixels
				totalR += r
				totalG += g
				totalB += b
				pixelCount++

				// Calculate color variance to detect colorful vs monochrome icons
				// High variance = colorful (don't invert), low variance = monochrome
				rNorm := r >> 8 // Convert to 0-255 range
				gNorm := g >> 8
				bNorm := b >> 8

				// Simple color variance: difference between max and min color channels
				maxColor := rNorm
				if gNorm > maxColor {
					maxColor = gNorm
				}
				if bNorm > maxColor {
					maxColor = bNorm
				}

				minColor := rNorm
				if gNorm < minColor {
					minColor = gNorm
				}
				if bNorm < minColor {
					minColor = bNorm
				}

				colorVariance += maxColor - minColor
			}
		}
	}

	if pixelCount == 0 {
		return img // All transparent
	}

	// Calculate average brightness (0-65535 range)
	avgR := totalR / pixelCount
	avgG := totalG / pixelCount
	avgB := totalB / pixelCount

	// Calculate perceived brightness using standard luminance formula
	brightness := (299*avgR + 587*avgG + 114*avgB) / 1000

	// Calculate average color variance (0-255 range)
	avgColorVariance := colorVariance / pixelCount

	// Calculate transparency ratio
	totalPixels := pixelCount + transparentPixels
	transparencyRatio := float64(transparentPixels) / float64(totalPixels)

	// Only invert if:
	// 1. Icon is very dark (brightness < 20000, low brightness)
	// 2. AND icon is mostly monochrome (low color variance < 20)
	// 3. AND icon is not mostly transparent (< 80% transparent)
	// This makes dark monochrome icons visible on dark themes while preserving colorful icons

	shouldInvert := brightness < 20000 && avgColorVariance < 20 && transparencyRatio < 0.8

	log.Printf("[favicon] Brightness analysis: brightness=%d, colorVariance=%d, transparency=%.2f, shouldInvert=%t",
		brightness, avgColorVariance, transparencyRatio, shouldInvert)

	if shouldInvert {
		log.Printf("[favicon] Inverting dark monochrome icon to light for better dark theme visibility")
		return invertImage(img)
	}

	return img
}

// invertImage inverts the colors of an image with edge smoothing
func invertImage(img image.Image) image.Image {
	bounds := img.Bounds()
	inverted := image.NewRGBA(bounds)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()

			// Skip fully transparent pixels to avoid noise
			if a == 0 {
				inverted.Set(x, y, color.RGBA{A: 0})
				continue
			}

			// Convert to 8-bit for easier processing
			r8 := uint8(r >> 8)
			g8 := uint8(g >> 8)
			b8 := uint8(b >> 8)
			a8 := uint8(a >> 8)

			// Invert RGB but preserve alpha
			newR := 255 - r8
			newG := 255 - g8
			newB := 255 - b8

			// For semi-transparent pixels, blend the inversion more gently
			// This reduces edge noise by making transitions smoother
			if a8 < 255 {
				// Reduce inversion intensity for semi-transparent pixels
				blendFactor := float64(a8) / 255.0
				newR = uint8(float64(r8) + (float64(newR-r8) * blendFactor))
				newG = uint8(float64(g8) + (float64(newG-g8) * blendFactor))
				newB = uint8(float64(b8) + (float64(newB-b8) * blendFactor))
			}

			inverted.Set(x, y, color.RGBA{
				R: newR,
				G: newG,
				B: newB,
				A: a8,
			})
		}
	}

	return inverted
}
