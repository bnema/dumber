package webkit

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/bnema/dumber/internal/config"
	"github.com/bnema/dumber/internal/db"
	"github.com/bnema/dumber/internal/logging"
	gio "github.com/diamondburned/gotk4/pkg/gio/v2"
	gtk "github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// TLSCertificateInfo holds information about a TLS certificate for display
type TLSCertificateInfo struct {
	Subject   string
	Issuer    string
	NotBefore string
	NotAfter  string
	IsExpired bool
	Errors    []string
}

// setupTLSErrorHandler sets up the TLS error handler for the WebView
func (wv *WebView) setupTLSErrorHandler() {
	wv.view.ConnectLoadFailedWithTLSErrors(func(failingUri string, certificate gio.TLSCertificater, errors gio.TLSCertificateFlags) bool {
		logging.Warn(fmt.Sprintf("[tls] Load failed with TLS errors for: %s, errors: %v", failingUri, errors))

		// Extract hostname from URI
		hostname := extractHostname(failingUri)
		if hostname == "" {
			logging.Error(fmt.Sprintf("[tls] Failed to extract hostname from URI: %s", failingUri))
			return false // Let WebKit handle it
		}

		// Check if user has previously made a decision for this hostname
		// Note: We use hostname-only matching because certificate properties
		// from GIO are unstable (GDateTime objects change on each access)
		decision, exists := checkStoredCertificateDecision(hostname)
		if exists {
			if decision == "accepted" {
				logging.Debug(fmt.Sprintf("[tls] Certificate previously accepted for %s, allowing", hostname))
				wv.allowCertificateAndReload(certificate, hostname, failingUri)
				return true // Signal handled
			} else {
				logging.Debug(fmt.Sprintf("[tls] Certificate previously rejected for %s, blocking", hostname))
				return false // Let WebKit show error
			}
		}

		// No stored decision - show dialog to user
		logging.Debug(fmt.Sprintf("[tls] No stored decision for %s, showing dialog", hostname))
		wv.showTLSErrorDialog(failingUri, hostname, certificate, errors)

		return true // Signal handled - we're managing the error
	})
}

// extractHostname extracts the hostname from a URI
func extractHostname(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil {
		// Try to extract manually
		if strings.Contains(uri, "://") {
			parts := strings.SplitN(uri, "://", 2)
			if len(parts) == 2 {
				host := strings.Split(parts[1], "/")[0]
				// Remove port if present
				host = strings.Split(host, ":")[0]
				return host
			}
		}
		return ""
	}

	// Remove port if present
	host := parsed.Hostname()
	return host
}

// checkStoredCertificateDecision checks if user has previously decided on this hostname
func checkStoredCertificateDecision(hostname string) (string, bool) {
	cfg := config.Get()
	if cfg == nil {
		logging.Error(fmt.Sprintf("[tls] Config not available, cannot check stored decisions"))
		return "", false
	}

	dbPath := cfg.Database.Path
	if dbPath == "" {
		logging.Error(fmt.Sprintf("[tls] Database path not configured"))
		return "", false
	}

	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		logging.Error(fmt.Sprintf("[tls] Failed to open database: %v", err))
		return "", false
	}
	defer func() {
		if err := database.Close(); err != nil {
			logging.Warn(fmt.Sprintf("[tls] warning: failed to close database: %v", err))
		}
	}()

	queries := db.New(database)
	ctx := context.Background()

	// Get any validation for this hostname (we don't verify cert hash anymore)
	validation, err := queries.GetCertificateValidationByHostname(ctx, hostname)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false
		}
		logging.Error(fmt.Sprintf("[tls] Error checking certificate validation: %v", err))
		return "", false
	}

	return validation.UserDecision, true
}

// storeCertificateDecision stores the user's decision about a hostname
func storeCertificateDecision(hostname, decision string, expiresAt sql.NullTime) error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("config not available")
	}

	dbPath := cfg.Database.Path
	if dbPath == "" {
		return fmt.Errorf("database path not configured")
	}

	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			logging.Warn(fmt.Sprintf("[tls] warning: failed to close database: %v", err))
		}
	}()

	queries := db.New(database)
	ctx := context.Background()

	logging.Debug(fmt.Sprintf("[tls] Saving certificate decision to database: hostname=%s, decision=%s, expiresAt=%v",
		hostname, decision, expiresAt))

	// Use empty string for cert hash since we're doing hostname-only matching
	if err := queries.StoreCertificateValidation(ctx, hostname, "", decision, expiresAt); err != nil {
		logging.Error(fmt.Sprintf("[tls] Failed to save certificate decision to database: %v", err))
		return err
	}

	logging.Debug(fmt.Sprintf("[tls] Certificate decision saved successfully in database"))
	return nil
}

// TLS Certificate Flag constants from GIO
// These match the C enum values from gioenums.h
const (
	tlsCertificateUnknownCA    gio.TLSCertificateFlags = 1 << 0 // G_TLS_CERTIFICATE_UNKNOWN_CA
	tlsCertificateBadIdentity  gio.TLSCertificateFlags = 1 << 1 // G_TLS_CERTIFICATE_BAD_IDENTITY
	tlsCertificateNotActivated gio.TLSCertificateFlags = 1 << 2 // G_TLS_CERTIFICATE_NOT_ACTIVATED
	tlsCertificateExpired      gio.TLSCertificateFlags = 1 << 3 // G_TLS_CERTIFICATE_EXPIRED
	tlsCertificateRevoked      gio.TLSCertificateFlags = 1 << 4 // G_TLS_CERTIFICATE_REVOKED
	tlsCertificateInsecure     gio.TLSCertificateFlags = 1 << 5 // G_TLS_CERTIFICATE_INSECURE
	tlsCertificateGenericError gio.TLSCertificateFlags = 1 << 6 // G_TLS_CERTIFICATE_GENERIC_ERROR
)

// getTLSErrorMessages converts TLS certificate flags to human-readable messages
func getTLSErrorMessages(errors gio.TLSCertificateFlags) []string {
	var messages []string

	if errors&tlsCertificateUnknownCA != 0 {
		messages = append(messages, "• The certificate authority is not trusted")
	}
	if errors&tlsCertificateBadIdentity != 0 {
		messages = append(messages, "• The certificate does not match the site identity")
	}
	if errors&tlsCertificateNotActivated != 0 {
		messages = append(messages, "• The certificate is not yet valid")
	}
	if errors&tlsCertificateExpired != 0 {
		messages = append(messages, "• The certificate has expired")
	}
	if errors&tlsCertificateRevoked != 0 {
		messages = append(messages, "• The certificate has been revoked")
	}
	if errors&tlsCertificateInsecure != 0 {
		messages = append(messages, "• The certificate uses an insecure algorithm")
	}
	if errors&tlsCertificateGenericError != 0 {
		messages = append(messages, "• A generic error occurred validating the certificate")
	}

	if len(messages) == 0 {
		messages = append(messages, "• Unknown certificate error")
	}

	return messages
}

// showTLSErrorDialog shows a dialog asking the user how to handle the TLS error
func (wv *WebView) showTLSErrorDialog(failingUri, hostname string, certificate gio.TLSCertificater, errors gio.TLSCertificateFlags) {
	// This must run on the main thread
	wv.RunOnMainThread(func() {
		// Get error messages
		errorMessages := getTLSErrorMessages(errors)

		// Build detailed message
		detail := fmt.Sprintf("The site %s has a certificate error:\n\n%s\n\nProceeding is unsafe and may expose your data to attackers.",
			hostname,
			strings.Join(errorMessages, "\n"))

		// Create AlertDialog using CGO wrapper
		dialog := newAlertDialog(fmt.Sprintf("Certificate Error for %s", hostname))
		if dialog == nil {
			logging.Error(fmt.Sprintf("[tls] Failed to create alert dialog"))
			return
		}

		// Set dialog properties
		dialog.SetDetail(detail)
		dialog.SetModal(true)

		// Set buttons: Go Back (0), Proceed Once (1), Always Accept (2)
		dialog.SetButtons([]string{"Go Back", "Proceed Once (Unsafe)", "Always Accept This Site"})
		dialog.SetDefaultButton(0) // Default to "Go Back"
		dialog.SetCancelButton(0)  // Cancel = "Go Back"

		// Get parent window
		var parentWindow *gtk.Window
		if wv.window != nil {
			parentWindow = wv.window.AsWindow()
		}

		// Show dialog and handle response
		dialog.Choose(context.Background(), parentWindow, func(result gio.AsyncResulter) {
			buttonIndex, err := dialog.ChooseFinish(result)
			if err != nil {
				logging.Error(fmt.Sprintf("[tls] Dialog error: %v", err))
				return
			}

			logging.Debug(fmt.Sprintf("[tls] User chose button %d for %s", buttonIndex, hostname))

			switch buttonIndex {
			case 0: // Go Back
				logging.Debug(fmt.Sprintf("[tls] User chose to go back"))
				// Do nothing - let the load fail

			case 1: // Proceed Once
				logging.Debug(fmt.Sprintf("[tls] User chose to proceed once"))
				wv.allowCertificateAndReload(certificate, hostname, failingUri)

			case 2: // Always Accept This Site
				logging.Debug(fmt.Sprintf("[tls] User chose to always accept for %s", hostname))
				// Store decision in database (expires in 30 days)
				expiresAt := sql.NullTime{
					Time:  time.Now().Add(30 * 24 * time.Hour),
					Valid: true,
				}
				if err := storeCertificateDecision(hostname, "accepted", expiresAt); err != nil {
					logging.Error(fmt.Sprintf("[tls] Failed to store certificate decision: %v", err))
				} else {
					logging.Debug(fmt.Sprintf("[tls] Stored certificate acceptance for %s (expires in 30 days)", hostname))
				}
				wv.allowCertificateAndReload(certificate, hostname, failingUri)
			}
		})
	})
}

// allowCertificateAndReload allows the certificate for the host and reloads the page
func (wv *WebView) allowCertificateAndReload(certificate gio.TLSCertificater, hostname, uri string) {
	wv.RunOnMainThread(func() {
		// Get the network session
		session := wv.view.NetworkSession()
		if session == nil {
			logging.Error(fmt.Sprintf("[tls] Failed to get network session"))
			return
		}

		// Allow the certificate for this host
		session.AllowTLSCertificateForHost(certificate, hostname)
		logging.Debug(fmt.Sprintf("[tls] Certificate exception added for host: %s", hostname))

		// Reload the page
		wv.view.LoadURI(uri)
		logging.Debug(fmt.Sprintf("[tls] Reloading %s with certificate exception", uri))
	})
}

// CleanupExpiredCertificateValidations removes expired certificate validations from the database
// This should be called on application startup
func CleanupExpiredCertificateValidations() error {
	cfg := config.Get()
	if cfg == nil {
		return fmt.Errorf("config not available")
	}

	dbPath := cfg.Database.Path
	if dbPath == "" {
		return fmt.Errorf("database path not configured")
	}

	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			logging.Warn(fmt.Sprintf("[tls] warning: failed to close database: %v", err))
		}
	}()

	queries := db.New(database)
	ctx := context.Background()

	if err := queries.DeleteExpiredCertificateValidations(ctx); err != nil {
		return fmt.Errorf("failed to delete expired validations: %w", err)
	}

	logging.Debug(fmt.Sprintf("[tls] Cleaned up expired certificate validations"))
	return nil
}
