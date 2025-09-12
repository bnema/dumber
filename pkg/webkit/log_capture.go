//go:build webkit_cgo

package webkit

/*
#include "log_capture.h"
*/
import "C"

import (
	"time"
	"unsafe"

	"github.com/bnema/dumber/internal/logging"
)

var webkitCaptureActive = false
var webkitCaptureChan chan struct{}

//export goWebKitLogHandler
func goWebKitLogHandler(domain *C.char, level C.int, message *C.char) {
	goDomain := C.GoString(domain)
	goMessage := C.GoString(message)
	
	var levelStr string
	switch level {
	case C.G_LOG_LEVEL_ERROR:
		levelStr = "ERROR"
	case C.G_LOG_LEVEL_CRITICAL:
		levelStr = "CRITICAL"
	case C.G_LOG_LEVEL_WARNING:
		levelStr = "WARNING"
	case C.G_LOG_LEVEL_MESSAGE:
		levelStr = "MESSAGE"
	case C.G_LOG_LEVEL_INFO:
		levelStr = "INFO"
	case C.G_LOG_LEVEL_DEBUG:
		levelStr = "DEBUG"
	default:
		levelStr = "UNKNOWN"
	}
	
	logMessage := goMessage
	if goDomain != "" {
		logMessage = "[" + goDomain + "] " + goMessage
	}
	
	// Send to logging system with appropriate level
	if logger := logging.GetLogger(); logger != nil {
		logger.WriteTagged("WEBKIT-"+levelStr, logMessage)
	}
}

// InitWebKitLogCapture initializes the WebKit log capture system
func InitWebKitLogCapture() error {
	if webkitCaptureActive {
		return nil
	}
	
	// Initialize C-level log capture
	C.webkit_init_log_capture()
	
	// Set up GLib log handlers
	C.webkit_setup_glib_log_handlers()
	
	// Create channel for stopping capture
	webkitCaptureChan = make(chan struct{})
	
	// Start goroutine to read captured C output
	go webkitLogReader()
	
	webkitCaptureActive = true
	return nil
}

// StopWebKitLogCapture stops the WebKit log capture
func StopWebKitLogCapture() {
	if !webkitCaptureActive {
		return
	}
	
	// Signal the reader goroutine to stop
	close(webkitCaptureChan)
	
	// Stop C-level capture
	C.webkit_stop_log_capture()
	
	webkitCaptureActive = false
}

// webkitLogReader reads captured C output in a goroutine
func webkitLogReader() {
	buffer := make([]byte, 4096)
	
	for {
		select {
		case <-webkitCaptureChan:
			return
		default:
			// Try to read captured output
			n := int(C.webkit_read_captured_log((*C.char)(unsafe.Pointer(&buffer[0])), C.int(len(buffer))))
			if n > 0 {
				message := string(buffer[:n])
				if logger := logging.GetLogger(); logger != nil {
					logger.WriteTagged("WEBKIT-PRINTF", message)
				}
			}
			
			// Small delay to prevent busy waiting
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// StartWebKitOutputCapture starts capturing webkit stdout/stderr
func StartWebKitOutputCapture() {
	if webkitCaptureActive {
		C.webkit_start_log_capture()
	}
}

// IsWebKitLogCaptureActive returns whether WebKit log capture is active
func IsWebKitLogCaptureActive() bool {
	return webkitCaptureActive
}