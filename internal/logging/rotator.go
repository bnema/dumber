package logging

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type LogRotator struct {
	mu          sync.Mutex
	baseDir     string
	baseName    string
	maxSize     int64 // bytes
	maxAge      time.Duration
	maxBackups  int
	compress    bool
	currentFile *os.File
	currentSize int64
}

func NewLogRotator(baseDir string, maxSizeMB, maxBackups, maxAgeDays int, compress bool) (*LogRotator, error) {
	r := &LogRotator{
		baseDir:    baseDir,
		baseName:   "dumber.log",
		maxSize:    int64(maxSizeMB) * 1024 * 1024, // Convert MB to bytes
		maxAge:     time.Duration(maxAgeDays) * 24 * time.Hour,
		maxBackups: maxBackups,
		compress:   compress,
	}

	if err := r.openCurrentFile(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *LogRotator) openCurrentFile() error {
	logPath := filepath.Join(r.baseDir, r.baseName)

	// Get current file size if it exists
	if info, err := os.Stat(logPath); err == nil {
		r.currentSize = info.Size()
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	r.currentFile = file
	return nil
}

func (r *LogRotator) Write(p []byte) (n int, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentFile == nil {
		if err := r.openCurrentFile(); err != nil {
			return 0, err
		}
	}

	// Check if rotation is needed
	if r.currentSize+int64(len(p)) > r.maxSize {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = r.currentFile.Write(p)
	if err != nil {
		return n, err
	}

	r.currentSize += int64(n)
	return n, nil
}

func (r *LogRotator) rotate() error {
	if r.currentFile != nil {
		if err := r.currentFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close current log file: %v\n", err)
		}
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("2006-01-02-15-04-05")
	backupName := fmt.Sprintf("%s.%s", r.baseName, timestamp)

	currentPath := filepath.Join(r.baseDir, r.baseName)
	backupPath := filepath.Join(r.baseDir, backupName)

	// Move current file to backup
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("failed to rotate log file: %w", err)
	}

	// Compress if enabled
	if r.compress {
		if err := r.compressFile(backupPath); err != nil {
			// Log warning but don't fail rotation
			fmt.Fprintf(os.Stderr, "Warning: failed to compress log file %s: %v\n", backupPath, err)
		} else {
			// Remove uncompressed file after successful compression
			if err := os.Remove(backupPath); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to remove uncompressed log file %s: %v\n", backupPath, err)
			}
		}
	}

	// Clean up old files
	r.cleanup()

	// Reset size counter and open new file
	r.currentSize = 0
	return r.openCurrentFile()
}

func (r *LogRotator) compressFile(filePath string) error {
	inputFile, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer func() {
		if err := inputFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close input file during compression: %v\n", err)
		}
	}()

	outputPath := filePath + ".gz"
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := outputFile.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close output file during compression: %v\n", err)
		}
	}()

	gzipWriter := gzip.NewWriter(outputFile)
	defer func() {
		if err := gzipWriter.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close gzip writer: %v\n", err)
		}
	}()

	_, err = io.Copy(gzipWriter, inputFile)
	return err
}

func (r *LogRotator) cleanup() {
	files, err := os.ReadDir(r.baseDir)
	if err != nil {
		return
	}

	var backupFiles []os.FileInfo
	now := time.Now()

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		name := file.Name()
		if !strings.HasPrefix(name, r.baseName+".") {
			continue
		}

		info, err := file.Info()
		if err != nil {
			continue
		}

		// Remove files older than maxAge
		if r.maxAge > 0 && now.Sub(info.ModTime()) > r.maxAge {
			if err := os.Remove(filepath.Join(r.baseDir, name)); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to remove old log file: %v\n", err)
			}
			continue
		}

		backupFiles = append(backupFiles, info)
	}

	// Remove excess backup files (keep only maxBackups)
	if r.maxBackups > 0 && len(backupFiles) > r.maxBackups {
		// Sort by modification time (oldest first)
		sort.Slice(backupFiles, func(i, j int) bool {
			return backupFiles[i].ModTime().Before(backupFiles[j].ModTime())
		})

		// Remove oldest files
		for i := 0; i < len(backupFiles)-r.maxBackups; i++ {
			if err := os.Remove(filepath.Join(r.baseDir, backupFiles[i].Name())); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to remove excess backup file: %v\n", err)
			}
		}
	}
}

func (r *LogRotator) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.currentFile != nil {
		return r.currentFile.Close()
	}
	return nil
}
