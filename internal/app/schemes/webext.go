package schemes

import (
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
)

// WebExtHandler handles the dumb-extension:// scheme.
type WebExtHandler struct {
	getExtensionByID   func(id string) (Extension, bool)
	isExtensionEnabled func(id string) bool
}

// Extension represents the minimal interface needed from webext.Extension.
type Extension interface {
	GetID() string
	GetInstallDir() string
}

// NewWebExtHandler creates a new WebExtension scheme handler.
func NewWebExtHandler(
	getExtensionByID func(id string) (Extension, bool),
	isExtensionEnabled func(id string) bool,
) *WebExtHandler {
	return &WebExtHandler{
		getExtensionByID:   getExtensionByID,
		isExtensionEnabled: isExtensionEnabled,
	}
}

// Handle processes dumb-extension:// scheme requests.
func (h *WebExtHandler) Handle(req *webkit.URISchemeRequest) {
	if req == nil {
		return
	}

	uri := req.URI()
	log.Printf("[schemes] dumb-extension:// request received: %s", uri)

	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != SchemeDumbExtension {
		req.FinishError(fmt.Errorf("invalid extension URI: %s", uri))
		return
	}

	extID := parsed.Host
	if extID == "" {
		req.FinishError(fmt.Errorf("missing extension id in %s", uri))
		return
	}

	relPath := strings.TrimPrefix(parsed.Path, "/")
	if relPath == "" {
		relPath = "index.html"
	}

	ext, ok := h.getExtensionByID(extID)
	if !ok || ext == nil {
		req.FinishError(fmt.Errorf("extension not found: %s", extID))
		return
	}

	if !h.isExtensionEnabled(extID) {
		req.FinishError(fmt.Errorf("extension disabled: %s", extID))
		return
	}

	basePath := filepath.Clean(ext.GetInstallDir())
	target := filepath.Clean(filepath.Join(basePath, relPath))
	if !strings.HasPrefix(target, basePath+string(os.PathSeparator)) && target != basePath {
		req.FinishError(fmt.Errorf("invalid extension path"))
		return
	}

	data, readErr := os.ReadFile(target)
	if readErr != nil {
		req.FinishError(fmt.Errorf("resource not found: %s", relPath))
		return
	}

	mimeType := GuessMimeType(target)
	if err := FinishRequestWithData(req, mimeType, data); err != nil {
		req.FinishError(fmt.Errorf("failed to stream resource"))
	}
}
