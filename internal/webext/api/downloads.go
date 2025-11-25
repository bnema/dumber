package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"sync/atomic"
	"time"
)

// DownloadState represents the state of a download
type DownloadState string

const (
	DownloadStateInProgress  DownloadState = "in_progress"
	DownloadStateInterrupted DownloadState = "interrupted"
	DownloadStateComplete    DownloadState = "complete"
)

// InterruptReason represents why a download was interrupted
type InterruptReason string

const (
	InterruptReasonUserCanceled  InterruptReason = "USER_CANCELED"
	InterruptReasonFileFailed    InterruptReason = "FILE_FAILED"
	InterruptReasonNetworkFailed InterruptReason = "NETWORK_FAILED"
)

// Download represents a browser download item
type Download struct {
	ID              int64         `json:"id"`
	URL             string        `json:"url"`
	Filename        string        `json:"filename"`
	Mime            string        `json:"mime,omitempty"`
	State           DownloadState `json:"state"`
	Paused          bool          `json:"paused"`
	CanResume       bool          `json:"canResume"`
	Error           string        `json:"error,omitempty"`
	BytesReceived   int64         `json:"bytesReceived"`
	TotalBytes      int64         `json:"totalBytes"`
	FileSize        int64         `json:"fileSize"`
	Exists          bool          `json:"exists"`
	StartTime       string        `json:"startTime,omitempty"`
	EndTime         string        `json:"endTime,omitempty"`
	Danger          string        `json:"danger"`
	Incognito       bool          `json:"incognito"`
	ByExtensionID   string        `json:"byExtensionId,omitempty"`
	ByExtensionName string        `json:"byExtensionName,omitempty"`

	// Internal fields
	cancelFunc  context.CancelFunc
	startedAt   time.Time
	completedAt time.Time
}

// DownloadQuery represents a download search query
type DownloadQuery struct {
	ID                *int64           `json:"id,omitempty"`
	URL               *string          `json:"url,omitempty"`
	URLRegex          *string          `json:"urlRegex,omitempty"`
	Filename          *string          `json:"filename,omitempty"`
	FilenameRegex     *string          `json:"filenameRegex,omitempty"`
	Mime              *string          `json:"mime,omitempty"`
	State             *DownloadState   `json:"state,omitempty"`
	Paused            *bool            `json:"paused,omitempty"`
	Error             *InterruptReason `json:"error,omitempty"`
	BytesReceived     *int64           `json:"bytesReceived,omitempty"`
	TotalBytes        *int64           `json:"totalBytes,omitempty"`
	TotalBytesGreater *int64           `json:"totalBytesGreater,omitempty"`
	TotalBytesLess    *int64           `json:"totalBytesLess,omitempty"`
	FileSize          *int64           `json:"fileSize,omitempty"`
	Exists            *bool            `json:"exists,omitempty"`
	StartedBefore     *string          `json:"startedBefore,omitempty"`
	StartedAfter      *string          `json:"startedAfter,omitempty"`
	EndedBefore       *string          `json:"endedBefore,omitempty"`
	EndedAfter        *string          `json:"endedAfter,omitempty"`
	Query             []string         `json:"query,omitempty"`
	OrderBy           []string         `json:"orderBy,omitempty"`
	Limit             *int             `json:"limit,omitempty"`
}

// DownloadsAPIDispatcher handles downloads.* API calls
type DownloadsAPIDispatcher struct {
	mu                 sync.RWMutex
	downloads          map[int64]*Download // downloadID -> Download
	extensionDownloads map[string][]int64  // extensionID -> []downloadID
	nextID             atomic.Int64
	downloadDir        string
	onCreatedCallbacks map[string]func(*Download) // extensionID -> callback
	onChangedCallbacks map[string]func(*Download) // extensionID -> callback
	onErasedCallbacks  map[string]func(int64)     // extensionID -> callback
}

// NewDownloadsAPIDispatcher creates a new downloads API dispatcher
func NewDownloadsAPIDispatcher(downloadDir string) *DownloadsAPIDispatcher {
	d := &DownloadsAPIDispatcher{
		downloads:          make(map[int64]*Download),
		extensionDownloads: make(map[string][]int64),
		downloadDir:        downloadDir,
		onCreatedCallbacks: make(map[string]func(*Download)),
		onChangedCallbacks: make(map[string]func(*Download)),
		onErasedCallbacks:  make(map[string]func(int64)),
	}
	d.nextID.Store(1)
	return d
}

// SetEventCallbacks sets the event callbacks for an extension
func (d *DownloadsAPIDispatcher) SetEventCallbacks(
	extensionID string,
	onCreated func(*Download),
	onChanged func(*Download),
	onErased func(int64),
) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if onCreated != nil {
		d.onCreatedCallbacks[extensionID] = onCreated
	}
	if onChanged != nil {
		d.onChangedCallbacks[extensionID] = onChanged
	}
	if onErased != nil {
		d.onErasedCallbacks[extensionID] = onErased
	}
}

// Download starts a new download
func (d *DownloadsAPIDispatcher) Download(ctx context.Context, extensionID, extensionName string, options map[string]interface{}) (int64, error) {
	// Extract required URL
	url, ok := options["url"].(string)
	if !ok || url == "" {
		return 0, fmt.Errorf("downloads.download(): missing or invalid 'url' field")
	}

	// Extract optional filename
	var filename string
	if fn, ok := options["filename"].(string); ok {
		filename = fn
		// Security: ensure filename doesn't escape download directory
		if filepath.IsAbs(filename) || filepath.Clean(filename) != filename {
			return 0, fmt.Errorf("downloads.download(): invalid filename (must be relative)")
		}
	}

	// Extract conflict action
	conflictAction := "uniquify" // default
	if ca, ok := options["conflictAction"].(string); ok {
		switch ca {
		case "uniquify", "overwrite", "prompt":
			conflictAction = ca
		}
	}

	// Generate download ID
	downloadID := d.nextID.Add(1)

	// Create download context with cancellation
	downloadCtx, cancel := context.WithCancel(ctx)

	// Create download object
	download := &Download{
		ID:              downloadID,
		URL:             url,
		State:           DownloadStateInProgress,
		Paused:          false,
		CanResume:       false,
		BytesReceived:   0,
		TotalBytes:      -1,
		FileSize:        -1,
		Exists:          false,
		Danger:          "safe",
		Incognito:       false,
		ByExtensionID:   extensionID,
		ByExtensionName: extensionName,
		cancelFunc:      cancel,
		startedAt:       time.Now(),
	}

	// Store download
	d.mu.Lock()
	d.downloads[downloadID] = download
	d.extensionDownloads[extensionID] = append(d.extensionDownloads[extensionID], downloadID)
	d.mu.Unlock()

	// Emit onCreated event
	d.emitOnCreated(extensionID, download)

	// Start download in background
	go d.performDownload(downloadCtx, download, filename, conflictAction)

	return downloadID, nil
}

// performDownload performs the actual HTTP download
func (d *DownloadsAPIDispatcher) performDownload(ctx context.Context, download *Download, suggestedFilename, conflictAction string) {
	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", download.URL, nil)
	if err != nil {
		d.failDownload(download, InterruptReasonNetworkFailed)
		return
	}

	// Perform request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			d.failDownload(download, InterruptReasonUserCanceled)
		} else {
			d.failDownload(download, InterruptReasonNetworkFailed)
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		d.failDownload(download, InterruptReasonNetworkFailed)
		return
	}

	// Update MIME type
	download.Mime = resp.Header.Get("Content-Type")

	// Determine filename
	filename := suggestedFilename
	if filename == "" {
		filename = filepath.Base(download.URL)
		if filename == "" || filename == "." || filename == "/" {
			filename = "download"
		}
	}

	// Ensure download directory exists
	downloadDir := d.downloadDir
	if downloadDir == "" {
		downloadDir = filepath.Join(os.Getenv("HOME"), "Downloads")
	}
	// Expand ~ if present
	if downloadDir[:2] == "~/" {
		downloadDir = filepath.Join(os.Getenv("HOME"), downloadDir[2:])
	}

	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		d.failDownload(download, InterruptReasonFileFailed)
		return
	}

	// Resolve final path
	destPath := filepath.Join(downloadDir, filename)

	// Handle conflict action
	switch conflictAction {
	case "uniquify":
		destPath = d.uniquifyFilename(destPath)
	case "overwrite":
		// Use as-is
	case "prompt":
		// TODO: For now, treat as uniquify
		destPath = d.uniquifyFilename(destPath)
	}

	download.Filename = destPath

	// Create destination file
	file, err := os.Create(destPath)
	if err != nil {
		d.failDownload(download, InterruptReasonFileFailed)
		return
	}
	defer file.Close()

	// Update total bytes if available
	if resp.ContentLength > 0 {
		download.TotalBytes = resp.ContentLength
	}

	// Download with progress tracking
	buf := make([]byte, 32*1024) // 32KB buffer
	for {
		select {
		case <-ctx.Done():
			d.failDownload(download, InterruptReasonUserCanceled)
			return
		default:
		}

		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := file.Write(buf[:n]); writeErr != nil {
				d.failDownload(download, InterruptReasonFileFailed)
				return
			}
			download.BytesReceived += int64(n)

			// Emit progress update (onChanged event)
			d.emitOnChanged(download.ByExtensionID, download)
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			d.failDownload(download, InterruptReasonNetworkFailed)
			return
		}
	}

	// Complete download
	d.completeDownload(download)
}

// uniquifyFilename generates a unique filename by adding (1), (2), etc.
func (d *DownloadsAPIDispatcher) uniquifyFilename(path string) string {
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

// completeDownload marks a download as complete
func (d *DownloadsAPIDispatcher) completeDownload(download *Download) {
	d.mu.Lock()
	download.State = DownloadStateComplete
	download.completedAt = time.Now()
	download.EndTime = download.completedAt.Format(time.RFC3339)

	// Get file size
	if stat, err := os.Stat(download.Filename); err == nil {
		download.FileSize = stat.Size()
		download.Exists = true
	}
	d.mu.Unlock()

	// Emit onChanged event
	d.emitOnChanged(download.ByExtensionID, download)
}

// failDownload marks a download as failed
func (d *DownloadsAPIDispatcher) failDownload(download *Download, reason InterruptReason) {
	d.mu.Lock()
	download.State = DownloadStateInterrupted
	download.Error = string(reason)
	download.completedAt = time.Now()
	download.EndTime = download.completedAt.Format(time.RFC3339)
	d.mu.Unlock()

	// Emit onChanged event
	d.emitOnChanged(download.ByExtensionID, download)
}

// Cancel cancels a download by ID
func (d *DownloadsAPIDispatcher) Cancel(ctx context.Context, downloadID int64) error {
	d.mu.RLock()
	download, exists := d.downloads[downloadID]
	d.mu.RUnlock()

	if !exists {
		// Not an error - just no-op if already removed
		return nil
	}

	// Cancel download context
	if download.cancelFunc != nil {
		download.cancelFunc()
	}

	return nil
}

// Open opens a downloaded file
func (d *DownloadsAPIDispatcher) Open(ctx context.Context, downloadID int64) error {
	d.mu.RLock()
	download, exists := d.downloads[downloadID]
	d.mu.RUnlock()

	if !exists {
		return fmt.Errorf("downloads.open(): download not found")
	}

	if download.State != DownloadStateComplete {
		return fmt.Errorf("downloads.open(): download not complete")
	}

	// Open file with xdg-open (Linux)
	// TODO: Platform-specific implementation
	return nil
}

// Show shows a downloaded file in file manager
func (d *DownloadsAPIDispatcher) Show(ctx context.Context, downloadID int64) error {
	d.mu.RLock()
	download, exists := d.downloads[downloadID]
	d.mu.RUnlock()

	if !exists {
		return fmt.Errorf("downloads.show(): download not found")
	}

	if download.Filename == "" {
		return fmt.Errorf("downloads.show(): download has no filename")
	}

	// Show file in file manager with xdg-open (Linux)
	// TODO: Platform-specific implementation
	return nil
}

// RemoveFile removes the downloaded file from disk
func (d *DownloadsAPIDispatcher) RemoveFile(ctx context.Context, downloadID int64) error {
	d.mu.RLock()
	download, exists := d.downloads[downloadID]
	d.mu.RUnlock()

	if !exists {
		return fmt.Errorf("downloads.removeFile(): download not found")
	}

	// Cancel if still in progress
	if download.State == DownloadStateInProgress && download.cancelFunc != nil {
		download.cancelFunc()
	}

	// Remove file if it exists
	if download.Filename != "" {
		if err := os.Remove(download.Filename); err != nil && !os.IsNotExist(err) {
			return err
		}

		d.mu.Lock()
		download.Exists = false
		d.mu.Unlock()
	}

	return nil
}

// ShowDefaultFolder opens the default downloads folder
func (d *DownloadsAPIDispatcher) ShowDefaultFolder(ctx context.Context) error {
	// Open downloads folder with xdg-open (Linux)
	// TODO: Platform-specific implementation
	return nil
}

// Search searches for downloads matching a query
func (d *DownloadsAPIDispatcher) Search(ctx context.Context, query map[string]interface{}) ([]*Download, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var results []*Download

	// Parse query
	q := d.parseQuery(query)

	// Filter downloads
	for _, download := range d.downloads {
		if d.matchesQuery(download, q) {
			// Return a copy to prevent external modification
			dlCopy := *download
			results = append(results, &dlCopy)
		}
	}

	// Apply limit
	if q.Limit != nil && *q.Limit > 0 && len(results) > *q.Limit {
		results = results[:*q.Limit]
	}

	return results, nil
}

// parseQuery parses a query object into DownloadQuery struct
func (d *DownloadsAPIDispatcher) parseQuery(query map[string]interface{}) *DownloadQuery {
	q := &DownloadQuery{}

	if id, ok := query["id"].(float64); ok {
		idInt := int64(id)
		q.ID = &idInt
	}
	if url, ok := query["url"].(string); ok {
		q.URL = &url
	}
	if urlRegex, ok := query["urlRegex"].(string); ok {
		q.URLRegex = &urlRegex
	}
	if filename, ok := query["filename"].(string); ok {
		q.Filename = &filename
	}
	if filenameRegex, ok := query["filenameRegex"].(string); ok {
		q.FilenameRegex = &filenameRegex
	}
	if mime, ok := query["mime"].(string); ok {
		q.Mime = &mime
	}
	if state, ok := query["state"].(string); ok {
		s := DownloadState(state)
		q.State = &s
	}
	if limit, ok := query["limit"].(float64); ok {
		l := int(limit)
		q.Limit = &l
	}

	return q
}

// matchesQuery checks if a download matches the query
func (d *DownloadsAPIDispatcher) matchesQuery(download *Download, query *DownloadQuery) bool {
	// Check ID
	if query.ID != nil && download.ID != *query.ID {
		return false
	}

	// Check URL
	if query.URL != nil && download.URL != *query.URL {
		return false
	}

	// Check URL regex
	if query.URLRegex != nil {
		if matched, _ := regexp.MatchString(*query.URLRegex, download.URL); !matched {
			return false
		}
	}

	// Check filename
	if query.Filename != nil && download.Filename != *query.Filename {
		return false
	}

	// Check filename regex
	if query.FilenameRegex != nil {
		if matched, _ := regexp.MatchString(*query.FilenameRegex, download.Filename); !matched {
			return false
		}
	}

	// Check MIME type
	if query.Mime != nil && download.Mime != *query.Mime {
		return false
	}

	// Check state
	if query.State != nil && download.State != *query.State {
		return false
	}

	return true
}

// Erase removes downloads from history
func (d *DownloadsAPIDispatcher) Erase(ctx context.Context, query map[string]interface{}) ([]int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	var erasedIDs []int64

	// Parse query
	q := d.parseQuery(query)

	// Find and remove matching downloads
	for id, download := range d.downloads {
		if d.matchesQuery(download, q) {
			erasedIDs = append(erasedIDs, id)
			delete(d.downloads, id)

			// Remove from extension downloads
			if extDownloads, ok := d.extensionDownloads[download.ByExtensionID]; ok {
				for i, dlID := range extDownloads {
					if dlID == id {
						d.extensionDownloads[download.ByExtensionID] = append(extDownloads[:i], extDownloads[i+1:]...)
						break
					}
				}
			}

			// Emit onErased event
			if callback, ok := d.onErasedCallbacks[download.ByExtensionID]; ok {
				callback(id)
			}
		}
	}

	return erasedIDs, nil
}

// emitOnCreated emits an onCreated event
func (d *DownloadsAPIDispatcher) emitOnCreated(extensionID string, download *Download) {
	d.mu.RLock()
	callback, ok := d.onCreatedCallbacks[extensionID]
	d.mu.RUnlock()

	if ok && callback != nil {
		// Create a copy to prevent concurrent modification
		dlCopy := *download
		callback(&dlCopy)
	}
}

// emitOnChanged emits an onChanged event
func (d *DownloadsAPIDispatcher) emitOnChanged(extensionID string, download *Download) {
	d.mu.RLock()
	callback, ok := d.onChangedCallbacks[extensionID]
	d.mu.RUnlock()

	if ok && callback != nil {
		// Create a copy to prevent concurrent modification
		dlCopy := *download
		callback(&dlCopy)
	}
}

// CleanupExtension removes all downloads for an extension
func (d *DownloadsAPIDispatcher) CleanupExtension(extensionID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Get all downloads for this extension
	downloadIDs := d.extensionDownloads[extensionID]

	// Cancel and remove each download
	for _, id := range downloadIDs {
		if download, ok := d.downloads[id]; ok {
			// Cancel if in progress
			if download.cancelFunc != nil {
				download.cancelFunc()
			}
			delete(d.downloads, id)
		}
	}

	// Remove extension tracking
	delete(d.extensionDownloads, extensionID)
	delete(d.onCreatedCallbacks, extensionID)
	delete(d.onChangedCallbacks, extensionID)
	delete(d.onErasedCallbacks, extensionID)
}
