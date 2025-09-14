// Package converter provides types and functions for converting EasyList filters to WebKit content blocking format.
package converter

// WebKitRule represents a WebKit content blocking rule
type WebKitRule struct {
	Trigger Trigger `json:"trigger"`
	Action  Action  `json:"action"`
}

// Trigger defines when a rule should be applied
type Trigger struct {
	URLFilter                string   `json:"url-filter"`
	URLFilterIsCaseSensitive *bool    `json:"url-filter-is-case-sensitive,omitempty"`
	IfDomain                 []string `json:"if-domain,omitempty"`
	UnlessDomain             []string `json:"unless-domain,omitempty"`
	IfTopURL                 []string `json:"if-top-url,omitempty"`
	UnlessTopURL             []string `json:"unless-top-url,omitempty"`
	ResourceType             []string `json:"resource-type,omitempty"`
	LoadType                 []string `json:"load-type,omitempty"`
}

// Action defines what to do when a rule matches
type Action struct {
	Type     string `json:"type"`
	Selector string `json:"selector,omitempty"`
}

// ActionType constants for WebKit actions
const (
	ActionTypeBlock               = "block"
	ActionTypeIgnorePreviousRules = "ignore-previous-rules"
	ActionTypeBlockCookies        = "block-cookies"
	ActionTypeCSSDisplayNone      = "css-display-none"
)

// ResourceType constants for WebKit resource types
const (
	ResourceTypeScript      = "script"
	ResourceTypeImage       = "image"
	ResourceTypeStyleSheet  = "style-sheet"
	ResourceTypeFont        = "font"
	ResourceTypeMedia       = "media"
	ResourceTypeSVGDocument = "svg-document"
	ResourceTypeDocument    = "document"
	ResourceTypePopup       = "popup"
)

// LoadType constants for WebKit load types
const (
	LoadTypeFirstParty = "first-party"
	LoadTypeThirdParty = "third-party"
)
