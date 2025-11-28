package api

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	soup "github.com/diamondburned/gotk4-webkitgtk/pkg/soup/v3"
	webkit "github.com/diamondburned/gotk4-webkitgtk/pkg/webkit/v6"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// Cookie represents a browser cookie
type Cookie struct {
	Name           string `json:"name"`
	Value          string `json:"value"`
	Domain         string `json:"domain"`
	Path           string `json:"path"`
	Secure         bool   `json:"secure"`
	HTTPOnly       bool   `json:"httpOnly"`
	SameSite       string `json:"sameSite"`                 // "no_restriction", "lax", "strict"
	ExpirationDate *int64 `json:"expirationDate,omitempty"` // Unix timestamp, nil for session cookies
	StoreID        string `json:"storeId,omitempty"`        // Cookie store identifier (always "0" for now)
}

// CookieStore represents a cookie storage container
type CookieStore struct {
	ID     string  `json:"id"`
	TabIDs []int64 `json:"tabIds"`
}

// HostPermissionChecker validates if an extension has permission for a URL
type HostPermissionChecker interface {
	HasHostPermission(url string) bool
	HasPermission(permission string) bool
	GetExtensionID() string
}

// CookiesAPIDispatcher handles cookies.* API calls
type CookiesAPIDispatcher struct {
	cookieManager *webkit.CookieManager
}

// NewCookiesAPIDispatcher creates a new cookies API dispatcher
// cookieManager can be nil initially and set later via SetCookieManager
func NewCookiesAPIDispatcher(cookieManager *webkit.CookieManager) *CookiesAPIDispatcher {
	return &CookiesAPIDispatcher{
		cookieManager: cookieManager,
	}
}

// SetCookieManager sets the cookie manager (used for late initialization)
func (d *CookiesAPIDispatcher) SetCookieManager(cookieManager *webkit.CookieManager) {
	d.cookieManager = cookieManager
}

// checkInitialized returns an error if the cookie manager is not initialized
func (d *CookiesAPIDispatcher) checkInitialized() error {
	if d.cookieManager == nil {
		return fmt.Errorf("cookie manager not initialized (network session not created yet)")
	}
	return nil
}

// Get retrieves a single cookie by name
func (d *CookiesAPIDispatcher) Get(ctx context.Context, checker HostPermissionChecker, details map[string]interface{}) (*Cookie, error) {
	if err := d.checkInitialized(); err != nil {
		return nil, err
	}

	// Validate required permissions
	if !checker.HasPermission("cookies") {
		return nil, fmt.Errorf("cookies.get(): extension does not have 'cookies' permission")
	}

	// Extract and validate URL
	urlStr, ok := details["url"].(string)
	if !ok || urlStr == "" {
		return nil, fmt.Errorf("cookies.get(): missing or invalid 'url' field")
	}

	// Validate host permission
	if !checker.HasHostPermission(urlStr) {
		return nil, fmt.Errorf("cookies.get(): permission denied for host '%s'", urlStr)
	}

	// Extract cookie name
	name, ok := details["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("cookies.get(): missing or invalid 'name' field")
	}

	// Parse URL to validate
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("cookies.get(): invalid URL '%s': %w", urlStr, err)
	}

	// Get cookies for this URL (async operation)
	resultChan := make(chan []*soup.Cookie, 1)
	errorChan := make(chan error, 1)

	d.cookieManager.Cookies(ctx, parsedURL.String(), func(result gio.AsyncResulter) {
		cookies, err := d.cookieManager.CookiesFinish(result)
		if err != nil {
			errorChan <- err
			return
		}
		resultChan <- cookies
	})

	// Wait for result
	select {
	case err := <-errorChan:
		return nil, fmt.Errorf("cookies.get(): failed to retrieve cookies: %w", err)
	case cookies := <-resultChan:
		// Find best matching cookie (longest path wins)
		var bestMatch *soup.Cookie
		for _, cookie := range cookies {
			if cookie.Name() != name {
				continue
			}
			if bestMatch == nil {
				bestMatch = cookie
				continue
			}
			// Prefer longer paths
			if len(cookie.Path()) > len(bestMatch.Path()) {
				bestMatch = cookie
			}
		}

		if bestMatch == nil {
			return nil, nil // No matching cookie found
		}

		return soupCookieToAPI(bestMatch), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetAll retrieves all matching cookies
func (d *CookiesAPIDispatcher) GetAll(ctx context.Context, checker HostPermissionChecker, details map[string]interface{}) ([]Cookie, error) {
	if err := d.checkInitialized(); err != nil {
		return nil, err
	}

	// Validate required permissions
	if !checker.HasPermission("cookies") {
		return nil, fmt.Errorf("cookies.getAll(): extension does not have 'cookies' permission")
	}

	// URL is optional for getAll, but if provided, must have permission
	urlStr, hasURL := details["url"].(string)
	if hasURL && urlStr != "" {
		if !checker.HasHostPermission(urlStr) {
			return nil, fmt.Errorf("cookies.getAll(): permission denied for host '%s'", urlStr)
		}
	}

	// Extract optional filter fields
	filterName, _ := details["name"].(string)
	filterDomain, _ := details["domain"].(string)
	filterPath, _ := details["path"].(string)
	filterSecure, hasSecure := details["secure"].(bool)
	filterSession, hasSession := details["session"].(bool)

	// Get cookies (all if no URL, specific URL otherwise)
	var cookies []*soup.Cookie
	resultChan := make(chan []*soup.Cookie, 1)
	errorChan := make(chan error, 1)

	if hasURL && urlStr != "" {
		// Get cookies for specific URL
		parsedURL, err := url.Parse(urlStr)
		if err != nil {
			return nil, fmt.Errorf("cookies.getAll(): invalid URL '%s': %w", urlStr, err)
		}

		d.cookieManager.Cookies(ctx, parsedURL.String(), func(result gio.AsyncResulter) {
			c, err := d.cookieManager.CookiesFinish(result)
			if err != nil {
				errorChan <- err
				return
			}
			resultChan <- c
		})
	} else {
		// Get all cookies (requires checking permissions for each cookie's domain)
		d.cookieManager.AllCookies(ctx, func(result gio.AsyncResulter) {
			c, err := d.cookieManager.AllCookiesFinish(result)
			if err != nil {
				errorChan <- err
				return
			}
			resultChan <- c
		})
	}

	// Wait for result
	select {
	case err := <-errorChan:
		return nil, fmt.Errorf("cookies.getAll(): failed to retrieve cookies: %w", err)
	case cookies = <-resultChan:
		// Filter cookies based on criteria
		var filtered []Cookie
		for _, cookie := range cookies {
			// Check host permission for each cookie when getting all
			if !hasURL {
				cookieURL := fmt.Sprintf("https://%s", cookie.Domain())
				if !checker.HasHostPermission(cookieURL) {
					continue // Skip cookies for domains we don't have permission for
				}
			}

			// Apply filters
			if filterName != "" && cookie.Name() != filterName {
				continue
			}
			if filterDomain != "" && !domainMatches(cookie, filterDomain) {
				continue
			}
			if filterPath != "" && cookie.Path() != filterPath {
				continue
			}
			if hasSecure && cookie.Secure() != filterSecure {
				continue
			}
			if hasSession {
				hasExpires := cookie.Expires() != nil
				if filterSession && hasExpires {
					continue // Want session cookies, but this has expiry
				}
				if !filterSession && !hasExpires {
					continue // Want persistent cookies, but this is session
				}
			}

			filtered = append(filtered, *soupCookieToAPI(cookie))
		}

		// Sort by path length (descending) per WebExtension spec
		sort.Slice(filtered, func(i, j int) bool {
			return len(filtered[i].Path) > len(filtered[j].Path)
		})

		return filtered, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Set creates or updates a cookie
func (d *CookiesAPIDispatcher) Set(ctx context.Context, checker HostPermissionChecker, details map[string]interface{}) (*Cookie, error) {
	if err := d.checkInitialized(); err != nil {
		return nil, err
	}

	// Validate required permissions
	if !checker.HasPermission("cookies") {
		return nil, fmt.Errorf("cookies.set(): extension does not have 'cookies' permission")
	}

	// Extract and validate URL
	urlStr, ok := details["url"].(string)
	if !ok || urlStr == "" {
		return nil, fmt.Errorf("cookies.set(): missing or invalid 'url' field")
	}

	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("cookies.set(): invalid URL '%s': %w", urlStr, err)
	}

	// Check host permission for URL
	if !checker.HasHostPermission(urlStr) {
		return nil, fmt.Errorf("cookies.set(): permission denied for host '%s'", urlStr)
	}

	// Extract cookie fields
	name, _ := details["name"].(string)
	value, _ := details["value"].(string)
	domain, _ := details["domain"].(string)
	path, _ := details["path"].(string)
	secure, _ := details["secure"].(bool)
	httpOnly, _ := details["httpOnly"].(bool)
	sameSiteStr, _ := details["sameSite"].(string)

	// Default domain and path from URL if not provided
	if domain == "" {
		domain = parsedURL.Hostname()
	}
	if path == "" {
		path = parsedURL.Path
		if path == "" {
			path = "/"
		}
	}

	// Check host permission for domain if different from URL
	if domain != parsedURL.Hostname() {
		domainURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, domain)
		if !checker.HasHostPermission(domainURL) {
			return nil, fmt.Errorf("cookies.set(): permission denied for domain '%s'", domain)
		}
	}

	// Create new cookie
	cookie := soup.NewCookie(name, value, domain, path, -1)
	cookie.SetSecure(secure)
	cookie.SetHTTPOnly(httpOnly)

	// Set SameSite policy
	sameSite := sameSiteFromString(sameSiteStr)
	cookie.SetSameSitePolicy(sameSite)

	// Set expiration if provided
	if expirationDate, ok := details["expirationDate"].(float64); ok {
		expiryTime := time.Unix(int64(expirationDate), 0)
		// Convert Go time.Time to glib.DateTime
		gdatetime := glib.NewDateTimeFromUnixLocal(expiryTime.Unix())
		cookie.SetExpires(gdatetime)
	}

	// Add cookie (async operation)
	resultChan := make(chan error, 1)

	d.cookieManager.AddCookie(ctx, cookie, func(result gio.AsyncResulter) {
		err := d.cookieManager.AddCookieFinish(result)
		resultChan <- err
	})

	// Wait for result
	select {
	case err := <-resultChan:
		if err != nil {
			return nil, fmt.Errorf("cookies.set(): failed to set cookie: %w", err)
		}
		return soupCookieToAPI(cookie), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Remove deletes a cookie
func (d *CookiesAPIDispatcher) Remove(ctx context.Context, checker HostPermissionChecker, details map[string]interface{}) (*Cookie, error) {
	if err := d.checkInitialized(); err != nil {
		return nil, err
	}

	// Validate required permissions
	if !checker.HasPermission("cookies") {
		return nil, fmt.Errorf("cookies.remove(): extension does not have 'cookies' permission")
	}

	// Extract and validate URL
	urlStr, ok := details["url"].(string)
	if !ok || urlStr == "" {
		return nil, fmt.Errorf("cookies.remove(): missing or invalid 'url' field")
	}

	// Validate host permission
	if !checker.HasHostPermission(urlStr) {
		return nil, fmt.Errorf("cookies.remove(): permission denied for host '%s'", urlStr)
	}

	// Extract cookie name
	name, ok := details["name"].(string)
	if !ok || name == "" {
		return nil, fmt.Errorf("cookies.remove(): missing or invalid 'name' field")
	}

	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("cookies.remove(): invalid URL '%s': %w", urlStr, err)
	}

	// First, get the cookie to return its details
	resultChan := make(chan []*soup.Cookie, 1)
	errorChan := make(chan error, 1)

	d.cookieManager.Cookies(ctx, parsedURL.String(), func(result gio.AsyncResulter) {
		cookies, err := d.cookieManager.CookiesFinish(result)
		if err != nil {
			errorChan <- err
			return
		}
		resultChan <- cookies
	})

	// Wait for cookie list
	var cookieToRemove *soup.Cookie
	select {
	case err := <-errorChan:
		return nil, fmt.Errorf("cookies.remove(): failed to retrieve cookies: %w", err)
	case cookies := <-resultChan:
		// Find best matching cookie
		for _, cookie := range cookies {
			if cookie.Name() != name {
				continue
			}
			if cookieToRemove == nil {
				cookieToRemove = cookie
				continue
			}
			// Prefer longer paths
			if len(cookie.Path()) > len(cookieToRemove.Path()) {
				cookieToRemove = cookie
			}
		}

		if cookieToRemove == nil {
			return nil, nil // Cookie not found
		}
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Store cookie details before deletion
	removedCookie := soupCookieToAPI(cookieToRemove)

	// Delete the cookie
	deleteResultChan := make(chan error, 1)
	d.cookieManager.DeleteCookie(ctx, cookieToRemove, func(result gio.AsyncResulter) {
		err := d.cookieManager.DeleteCookieFinish(result)
		deleteResultChan <- err
	})

	// Wait for deletion result
	select {
	case err := <-deleteResultChan:
		if err != nil {
			return nil, fmt.Errorf("cookies.remove(): failed to delete cookie: %w", err)
		}
		return removedCookie, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// GetAllCookieStores returns all cookie stores
func (d *CookiesAPIDispatcher) GetAllCookieStores(ctx context.Context, checker HostPermissionChecker) ([]CookieStore, error) {
	// Validate required permissions
	if !checker.HasPermission("cookies") {
		return nil, fmt.Errorf("cookies.getAllCookieStores(): extension does not have 'cookies' permission")
	}

	// Dumber currently only has a single cookie store (default store)
	// In the future, this could support private browsing with separate stores
	stores := []CookieStore{
		{
			ID:     "0",       // Default cookie store
			TabIDs: []int64{}, // Empty means all tabs use this store
		},
	}

	return stores, nil
}

// Helper: Convert soup.Cookie to API Cookie struct
func soupCookieToAPI(cookie *soup.Cookie) *Cookie {
	apiCookie := &Cookie{
		Name:     cookie.Name(),
		Value:    cookie.Value(),
		Domain:   cookie.Domain(),
		Path:     cookie.Path(),
		Secure:   cookie.Secure(),
		HTTPOnly: cookie.HTTPOnly(),
		SameSite: sameSiteToString(cookie.SameSitePolicy()),
		StoreID:  "0", // Default store
	}

	// Add expiration date if not a session cookie
	if expires := cookie.Expires(); expires != nil {
		timestamp := expires.ToUnixUsec() / 1000000 // Convert microseconds to seconds
		apiCookie.ExpirationDate = &timestamp
	}

	return apiCookie
}

// Helper: Convert SameSite enum to string
func sameSiteToString(policy soup.SameSitePolicy) string {
	switch policy {
	case soup.SameSitePolicyNone:
		return "no_restriction"
	case soup.SameSitePolicyLax:
		return "lax"
	case soup.SameSitePolicyStrict:
		return "strict"
	default:
		return "no_restriction"
	}
}

// Helper: Convert string to SameSite enum
func sameSiteFromString(s string) soup.SameSitePolicy {
	switch strings.ToLower(s) {
	case "strict":
		return soup.SameSitePolicyStrict
	case "lax":
		return soup.SameSitePolicyLax
	case "no_restriction", "none", "":
		return soup.SameSitePolicyNone
	default:
		return soup.SameSitePolicyNone
	}
}

// Helper: Check if cookie domain matches the filter domain
func domainMatches(cookie *soup.Cookie, filterDomain string) bool {
	cookieDomain := cookie.Domain()

	// Exact match
	if cookieDomain == filterDomain {
		return true
	}

	// Cookie domain can start with '.' to indicate subdomain matching
	if strings.HasPrefix(cookieDomain, ".") {
		// Check if filterDomain ends with cookie domain
		return strings.HasSuffix(filterDomain, cookieDomain) ||
			strings.HasSuffix(filterDomain, cookieDomain[1:])
	}

	// Check if filterDomain is a subdomain of cookie domain
	return strings.HasSuffix(filterDomain, "."+cookieDomain)
}
