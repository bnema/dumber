// Package dialog provides UI dialog implementations for the application layer.
package dialog

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/logging"
	"github.com/bnema/dumber/internal/ui/component"
)

const maxPermissionLogURLLen = 96

type permissionPopup interface {
	Show(ctx context.Context, heading, body string, callback func(allowed, persistent bool))
}

type permissionDialogRequest struct {
	ctx       context.Context
	origin    string
	permTypes []entity.PermissionType
	metadata  entity.PermissionMetadata
	callback  func(result port.PermissionDialogResult)
}

// PermissionDialog implements the port.PermissionDialogPresenter interface.
// It uses a custom PermissionPopup overlay to sidestep the purego ConnectResponse bug
// and match the app's custom UI style.
type PermissionDialog struct {
	popup permissionPopup

	mu     sync.Mutex
	active bool
	queue  []permissionDialogRequest
}

// NewPermissionDialog creates a new permission dialog presenter.
// The popup is created once and reused for each permission request.
func NewPermissionDialog(popup *component.PermissionPopup) *PermissionDialog {
	return &PermissionDialog{
		popup: popup,
	}
}

// ShowPermissionDialog displays a permission request dialog to the user.
func (d *PermissionDialog) ShowPermissionDialog(
	ctx context.Context,
	origin string,
	permTypes []entity.PermissionType,
	metadata entity.PermissionMetadata,
	callback func(result port.PermissionDialogResult),
) {
	req := permissionDialogRequest{
		ctx:       ctx,
		origin:    origin,
		permTypes: permTypes,
		metadata:  metadata,
		callback:  callback,
	}

	if !d.enqueueOrStart(req) {
		return
	}

	d.showRequest(req)
}

func (d *PermissionDialog) enqueueOrStart(req permissionDialogRequest) bool {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.active {
		d.queue = append(d.queue, req)
		return false
	}

	d.active = true
	return true
}

func (d *PermissionDialog) showRequest(req permissionDialogRequest) {
	ctx := req.ctx
	origin := req.origin
	permTypes := req.permTypes
	metadata := req.metadata
	callback := req.callback

	log := logging.FromContext(ctx)

	if d.popup == nil {
		log.Error().Msg("permission popup not available")
		callback(port.PermissionDialogResult{Allowed: false, Persistent: false})
		d.showNextQueuedRequest()
		return
	}

	// Build dialog text
	heading := d.buildHeading(permTypes)
	body := d.buildBody(origin, permTypes, metadata)

	if parsePermFlags(permTypes).dataAccess {
		log.Info().
			Str("origin", origin).
			Str("requesting_domain", logging.TruncateURL(metadata[entity.PermissionMetadataKeyRequestingDomain], maxPermissionLogURLLen)).
			Str("current_domain", logging.TruncateURL(metadata[entity.PermissionMetadataKeyCurrentDomain], maxPermissionLogURLLen)).
			Msg("showing website data access permission dialog")
	}

	d.popup.Show(ctx, heading, body, func(allowed, persistent bool) {
		if parsePermFlags(permTypes).dataAccess {
			log.Info().
				Str("origin", origin).
				Str("requesting_domain", logging.TruncateURL(metadata[entity.PermissionMetadataKeyRequestingDomain], maxPermissionLogURLLen)).
				Str("current_domain", logging.TruncateURL(metadata[entity.PermissionMetadataKeyCurrentDomain], maxPermissionLogURLLen)).
				Bool("allowed", allowed).
				Bool("persistent", persistent).
				Msg("website data access permission dialog response")
		}
		log.Debug().
			Bool("allowed", allowed).
			Bool("persistent", persistent).
			Msg("permission popup response")
		callback(port.PermissionDialogResult{Allowed: allowed, Persistent: persistent})
		d.showNextQueuedRequest()
	})

	log.Debug().
		Str("origin", origin).
		Strs("types", entity.PermissionTypesToStrings(permTypes)).
		Msg("showing permission popup")
}

func (d *PermissionDialog) showNextQueuedRequest() {
	d.mu.Lock()
	if len(d.queue) == 0 {
		d.active = false
		d.mu.Unlock()
		return
	}

	next := d.queue[0]
	d.queue = d.queue[1:]
	d.mu.Unlock()

	d.showRequest(next)
}

// permFlags holds parsed permission type flags.
type permFlags struct {
	mic, cam, display, dataAccess bool
}

// parsePermFlags extracts boolean flags from permission types.
func parsePermFlags(permTypes []entity.PermissionType) permFlags {
	var f permFlags
	for _, pt := range permTypes {
		switch pt {
		case entity.PermissionTypeMicrophone:
			f.mic = true
		case entity.PermissionTypeCamera:
			f.cam = true
		case entity.PermissionTypeDisplay:
			f.display = true
		case entity.PermissionTypeWebsiteDataAccess:
			f.dataAccess = true
		}
	}
	return f
}

// joinPermissionLabels joins labels with commas and "and".
func joinPermissionLabels(labels []string) string {
	switch len(labels) {
	case 0:
		return ""
	case 1:
		return labels[0]
	case 2:
		return labels[0] + " and " + labels[1]
	default:
		return strings.Join(labels[:len(labels)-1], ", ") +
			", and " + labels[len(labels)-1]
	}
}

// buildHeading creates the dialog heading based on permission types.
func (d *PermissionDialog) buildHeading(
	permTypes []entity.PermissionType,
) string {
	f := parsePermFlags(permTypes)
	var labels []string
	if f.mic {
		labels = append(labels, "Microphone")
	}
	if f.cam {
		labels = append(labels, "Camera")
	}
	if f.display {
		labels = append(labels, "Screen Sharing")
	}
	if f.dataAccess {
		labels = append(labels, "Data Access")
	}
	switch {
	case len(labels) == 0:
		return "Allow Permission?"
	case len(labels) == 1 && f.dataAccess:
		return "Allow Third-Party Data Access?"
	case len(labels) == 1 && f.display:
		return "Allow Screen Sharing?"
	case len(labels) == 1:
		return "Allow " + labels[0] + " Access?"
	default:
		return "Allow " + joinPermissionLabels(labels) + "?"
	}
}

// buildBody creates the dialog body text.
func (d *PermissionDialog) buildBody(
	origin string, permTypes []entity.PermissionType, metadata entity.PermissionMetadata,
) string {
	f := parsePermFlags(permTypes)
	var parts []string
	if f.mic && f.cam {
		parts = append(parts, "access your microphone and camera")
	} else if f.mic {
		parts = append(parts, "access your microphone")
	} else if f.cam {
		parts = append(parts, "access your camera")
	}
	if f.display {
		parts = append(parts, "share your screen")
	}
	if f.dataAccess {
		reqDomain := metadata[entity.PermissionMetadataKeyRequestingDomain]
		curDomain := metadata[entity.PermissionMetadataKeyCurrentDomain]
		if reqDomain != "" && curDomain != "" {
			parts = append(parts, fmt.Sprintf(
				"allow %s to access its data (including cookies) while you browse %s",
				reqDomain, curDomain))
		} else if len(parts) == 0 {
			parts = append(parts, "access its stored data while you browse this site")
		} else {
			parts = append(parts, "access its stored data")
		}
	}

	var action string
	switch {
	case len(parts) == 0:
		action = "access your device"
	case len(parts) == 1:
		action = parts[0]
	case len(parts) == 2 && (!f.mic || !f.cam):
		// Simple two-part join when neither part contains "and".
		action = parts[0] + " and " + parts[1]
	default:
		// Oxford-comma join for 3+ parts, or when a part already
		// contains "and" (e.g. "access your microphone and camera").
		action = strings.Join(parts[:len(parts)-1], ", ") +
			", and " + parts[len(parts)-1]
	}

	return fmt.Sprintf("%s wants to %s.", origin, action)
}

// Ensure PermissionDialog implements the interface.
var _ port.PermissionDialogPresenter = (*PermissionDialog)(nil)
