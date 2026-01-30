// Package usecase contains application use cases that orchestrate domain logic.
package usecase

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/bnema/dumber/internal/application/port"
	"github.com/bnema/dumber/internal/domain/entity"
	"github.com/bnema/dumber/internal/domain/repository"
	"github.com/bnema/dumber/internal/logging"
)

// PermissionCallback provides allow/deny functions for the permission request.
type PermissionCallback struct {
	Allow func()
	Deny  func()
}

// HandlePermissionUseCase handles media permission requests from WebKit.
// It implements the permission strategy defined in the architecture:
// - Display capture: auto-allow (XDG portal handles UI)
// - Device enumeration: auto-allow (low risk)
// - Mic/Camera: check stored → dialog → persist if "Always"
type HandlePermissionUseCase struct {
	permRepo repository.PermissionRepository
	dialog   port.PermissionDialogPresenter
}

// NewHandlePermissionUseCase creates a new permission handling use case.
func NewHandlePermissionUseCase(
	permRepo repository.PermissionRepository,
	dialog port.PermissionDialogPresenter,
) *HandlePermissionUseCase {
	return &HandlePermissionUseCase{
		permRepo: permRepo,
		dialog:   dialog,
	}
}

// SetDialogPresenter sets the dialog presenter. This can be called after
// initialization when the UI window is available.
func (uc *HandlePermissionUseCase) SetDialogPresenter(dialog port.PermissionDialogPresenter) {
	uc.dialog = dialog
}

// HandlePermissionRequest processes a permission request from WebKit.
// This is the main entry point for the permission use case.
//
// Parameters:
//   - ctx: context for cancellation and logging
//   - origin: the website origin (URI) requesting permission
//   - permTypes: the types of permissions being requested
//   - callback: callbacks to allow or deny the request (must call one)
func (uc *HandlePermissionUseCase) HandlePermissionRequest(
	ctx context.Context,
	origin string,
	permTypes []entity.PermissionType,
	callback PermissionCallback,
) {
	log := logging.FromContext(ctx).With().
		Str("component", "permission").
		Str("origin", origin).
		Strs("types", entity.PermissionTypesToStrings(permTypes)).
		Logger()

	// Validate origin
	if origin == "" {
		log.Warn().Msg("permission request with empty origin, denying")
		callback.Deny()
		return
	}

	// Check if all types are auto-allow
	if uc.isAutoAllowSet(permTypes) {
		log.Debug().Msg("auto-allowing permission request")
		callback.Allow()
		return
	}

	// For mic/camera: check stored permissions first
	decision := uc.checkStoredPermissions(ctx, origin, permTypes)
	switch decision {
	case entity.PermissionGranted:
		log.Debug().Msg("using stored permission: granted")
		callback.Allow()
		return
	case entity.PermissionDenied:
		log.Debug().Msg("using stored permission: denied")
		callback.Deny()
		return
	case entity.PermissionPrompt:
		// Fall through to show dialog
	}

	// Show dialog for undecided permissions
	uc.showPermissionDialog(ctx, origin, permTypes, callback)
}

// QueryPermissionState returns the current permission state for the W3C Permissions API.
// This is used by websites to check if they already have permission before calling getUserMedia().
func (uc *HandlePermissionUseCase) QueryPermissionState(
	ctx context.Context,
	origin string,
	permType entity.PermissionType,
) entity.PermissionDecision {
	log := logging.FromContext(ctx).With().
		Str("component", "permission").
		Str("origin", origin).
		Str("type", string(permType)).
		Logger()

	// Auto-allow types
	if entity.IsAutoAllow(permType) {
		log.Debug().Msg("query: auto-allow type returns granted")
		return entity.PermissionGranted
	}

	// Check stored permission
	record, err := uc.permRepo.Get(ctx, origin, permType)
	if err != nil {
		log.Warn().Err(err).Msg("query: failed to get stored permission, returning prompt")
		return entity.PermissionPrompt
	}

	if record == nil {
		log.Debug().Msg("query: no stored permission, returning prompt")
		return entity.PermissionPrompt
	}

	log.Debug().Str("decision", string(record.Decision)).Msg("query: returning stored permission")
	return record.Decision
}

// isAutoAllowSet returns true if all permission types in the set are auto-allow.
func (uc *HandlePermissionUseCase) isAutoAllowSet(permTypes []entity.PermissionType) bool {
	_ = uc // suppress unused receiver warning, method kept for consistency
	for _, pt := range permTypes {
		if !entity.IsAutoAllow(pt) {
			return false
		}
	}
	return true
}

// checkStoredPermissions checks if all permissions in the set have stored decisions.
// Returns granted if all are granted, denied if any are denied, prompt otherwise.
func (uc *HandlePermissionUseCase) checkStoredPermissions(
	ctx context.Context,
	origin string,
	permTypes []entity.PermissionType,
) entity.PermissionDecision {
	log := logging.FromContext(ctx)

	hasPrompt := false

	for _, permType := range permTypes {
		// Skip auto-allow types
		if entity.IsAutoAllow(permType) {
			continue
		}

		// Skip non-persistable types (shouldn't happen, but defensive)
		if !entity.CanPersist(permType) {
			continue
		}

		record, err := uc.permRepo.Get(ctx, origin, permType)
		if err != nil {
			log.Warn().
				Err(err).
				Str("perm_type", string(permType)).
				Msg("failed to get stored permission")
			// On error, treat as prompt (fall back to dialog)
			hasPrompt = true
			continue
		}

		if record == nil {
			hasPrompt = true
			continue
		}

		switch record.Decision {
		case entity.PermissionDenied:
			// If any permission is denied, deny the whole request (conservative)
			return entity.PermissionDenied
		case entity.PermissionPrompt:
			hasPrompt = true
		}
	}

	if hasPrompt {
		return entity.PermissionPrompt
	}

	// All permissions are granted
	return entity.PermissionGranted
}

// showPermissionDialog shows the permission dialog and handles the result.
func (uc *HandlePermissionUseCase) showPermissionDialog(
	ctx context.Context,
	origin string,
	permTypes []entity.PermissionType,
	callback PermissionCallback,
) {
	log := logging.FromContext(ctx)

	// If no dialog presenter is set, deny the permission
	if uc.dialog == nil {
		log.Warn().Str("origin", origin).Msg("no dialog presenter available, denying permission")
		callback.Deny()
		return
	}

	uc.dialog.ShowPermissionDialog(ctx, origin, permTypes, func(result port.PermissionDialogResult) {
		if result.Allowed {
			log.Debug().Bool("persistent", result.Persistent).Msg("user allowed permission")
			callback.Allow()

			// Persist if "Always Allow" was chosen
			if result.Persistent {
				uc.persistPermission(ctx, origin, permTypes, entity.PermissionGranted)
			}
		} else {
			log.Debug().Msg("user denied permission")
			callback.Deny()

			// Persist denial if "Always Deny" was chosen (Persistent=true + Allowed=false)
			if result.Persistent {
				uc.persistPermission(ctx, origin, permTypes, entity.PermissionDenied)
			}
		}
	})
}

// persistPermission saves the permission decision to the repository.
func (uc *HandlePermissionUseCase) persistPermission(
	ctx context.Context,
	origin string,
	permTypes []entity.PermissionType,
	decision entity.PermissionDecision,
) {
	log := logging.FromContext(ctx)

	for _, permType := range permTypes {
		// Only persist allowed types
		if !entity.CanPersist(permType) {
			continue
		}

		record := &entity.PermissionRecord{
			Origin:    origin,
			Type:      permType,
			Decision:  decision,
			UpdatedAt: time.Now(),
		}

		if err := uc.permRepo.Set(ctx, record); err != nil {
			log.Warn().
				Err(err).
				Str("perm_type", string(permType)).
				Str("decision", string(decision)).
				Msg("failed to persist permission")
		}
	}
}

// ExtractOrigin extracts the origin (scheme://host) from a URI.
// This normalizes URIs to origins for permission storage.
func ExtractOrigin(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", fmt.Errorf("invalid URI: %w", err)
	}

	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("URI missing scheme or host: %s", uri)
	}

	// Return scheme://host (without path, query, fragment)
	origin := fmt.Sprintf("%s://%s", u.Scheme, u.Host)
	return origin, nil
}
