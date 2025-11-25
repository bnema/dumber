package browser

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/dumber/internal/config"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
)

// SetupDownloadHandler connects the download-started signal to handle browser downloads.
// This should be called after the NetworkSession is initialized.
func SetupDownloadHandler(session *webkit.NetworkSession) {
	if session == nil {
		return
	}

	session.ConnectDownloadStarted(func(download *webkit.Download) {
		handleDownloadStarted(download)
	})
	log.Printf("[browser] Download handler connected")
}

// handleDownloadStarted handles the download-started signal from NetworkSession.
// This provides basic browser download functionality: auto-download to configured directory.
func handleDownloadStarted(download *webkit.Download) {
	if download == nil {
		return
	}

	// Get the download request
	request := download.Request()
	if request == nil {
		log.Printf("[browser] Download started but no request available")
		return
	}

	uri := request.URI()
	log.Printf("[browser] Download started: %s", uri)

	// Get suggested filename from response
	response := download.Response()
	var suggestedFilename string
	if response != nil {
		suggestedFilename = response.SuggestedFilename()
	}

	// Fallback to URL basename if no suggested filename
	if suggestedFilename == "" {
		suggestedFilename = filepath.Base(uri)
		if suggestedFilename == "" || suggestedFilename == "." || suggestedFilename == "/" {
			suggestedFilename = "download"
		}
	}

	// Get download directory from config
	downloadDir := filepath.Join(os.Getenv("HOME"), "Downloads") // Default fallback
	if cfg := config.Get(); cfg != nil && cfg.Downloads.DefaultLocation != "" {
		downloadDir = cfg.Downloads.DefaultLocation
		// Expand ~ if present
		if strings.HasPrefix(downloadDir, "~/") {
			downloadDir = filepath.Join(os.Getenv("HOME"), downloadDir[2:])
		}
	}

	// Ensure download directory exists
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		log.Printf("[browser] Failed to create download directory %s: %v", downloadDir, err)
		download.Cancel()
		return
	}

	// Build destination path
	destPath := filepath.Join(downloadDir, suggestedFilename)

	// Handle filename conflicts (simple uniquify)
	destPath = uniquifyDownloadPath(destPath)

	// Set the destination
	download.SetDestination(destPath)

	log.Printf("[browser] Downloading to: %s", destPath)

	// Connect to finished signal for logging
	download.ConnectFinished(func() {
		log.Printf("[browser] Download completed: %s", destPath)
	})

	// Connect to failed signal for error handling
	download.ConnectFailed(func(err error) {
		log.Printf("[browser] Download failed: %s - error: %v", uri, err)
	})
}

// uniquifyDownloadPath generates a unique file path by adding (1), (2), etc. if file exists
func uniquifyDownloadPath(path string) string {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return path
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	nameWithoutExt := base[:len(base)-len(ext)]

	for i := 1; ; i++ {
		newPath := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", nameWithoutExt, i, ext))
		if _, err := os.Stat(newPath); os.IsNotExist(err) {
			return newPath
		}
	}
}
