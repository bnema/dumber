package webext

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// ContentScriptInjector handles injection of content scripts into web pages
type ContentScriptInjector struct {
	manager *Manager
}

// NewContentScriptInjector creates a new content script injector
func NewContentScriptInjector(manager *Manager) *ContentScriptInjector {
	return &ContentScriptInjector{
		manager: manager,
	}
}

// GetScriptsForURL returns all content scripts that should be injected for a given URL
func (inj *ContentScriptInjector) GetScriptsForURL(url string) []InjectionScript {
	matches := inj.manager.GetContentScriptsForURL(url)

	var scripts []InjectionScript
	for _, match := range matches {
		script := InjectionScript{
			ExtensionID: match.Extension.ID,
			ExtensionName: match.Extension.Manifest.Name,
			RunAt: ParseRunAt(match.ContentScript.RunAt),
			AllFrames: match.ContentScript.AllFrames,
			JSFiles: match.ContentScript.GetJSFiles(match.Extension.Path),
			CSSFiles: match.ContentScript.GetCSSFiles(match.Extension.Path),
		}
		scripts = append(scripts, script)
	}

	return scripts
}

// InjectionScript represents a script to be injected
type InjectionScript struct {
	ExtensionID   string
	ExtensionName string
	RunAt         RunAtTiming
	AllFrames     bool
	JSFiles       []string
	CSSFiles      []string
}

// LoadScriptContent reads the content of a script file
func (inj *ContentScriptInjector) LoadScriptContent(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read script %s: %w", path, err)
	}
	return string(data), nil
}

// LoadAllScripts loads all JS content for an injection script
func (inj *ContentScriptInjector) LoadAllScripts(script InjectionScript) ([]string, error) {
	var contents []string

	for _, jsFile := range script.JSFiles {
		content, err := inj.LoadScriptContent(jsFile)
		if err != nil {
			log.Printf("[injector] Warning: failed to load script %s: %v", jsFile, err)
			continue
		}
		contents = append(contents, content)
	}

	return contents, nil
}

// LoadAllCSS loads all CSS content for an injection script
func (inj *ContentScriptInjector) LoadAllCSS(script InjectionScript) ([]string, error) {
	var contents []string

	for _, cssFile := range script.CSSFiles {
		content, err := inj.LoadScriptContent(cssFile)
		if err != nil {
			log.Printf("[injector] Warning: failed to load CSS %s: %v", cssFile, err)
			continue
		}
		contents = append(contents, content)
	}

	return contents, nil
}

// GetWebExtensionShimPath returns the path to the webextensions.js shim
// This shim provides the chrome.* API in content scripts
func (inj *ContentScriptInjector) GetWebExtensionShimPath() string {
	// TODO: Embed this in the binary or install to a known location
	return filepath.Join("/usr/local/share/dumber/js", "webextensions.js")
}

// LoadWebExtensionShim loads the webextensions.js shim content
func (inj *ContentScriptInjector) LoadWebExtensionShim() (string, error) {
	shimPath := inj.GetWebExtensionShimPath()

	// Check if shim exists
	if _, err := os.Stat(shimPath); os.IsNotExist(err) {
		// Return a minimal shim for now
		return inj.GetMinimalShim(), nil
	}

	return inj.LoadScriptContent(shimPath)
}

// GetMinimalShim returns a minimal chrome.* API shim for content scripts
func (inj *ContentScriptInjector) GetMinimalShim() string {
	return `
// Minimal WebExtension API shim for content scripts
(function() {
	'use strict';

	// Create chrome namespace if it doesn't exist
	if (typeof chrome === 'undefined') {
		window.chrome = {};
	}

	// chrome.runtime API
	chrome.runtime = {
		// Send message to background script
		sendMessage: function(message, callback) {
			// TODO: Bridge to native handler via user message
			console.log('[webext] chrome.runtime.sendMessage:', message);
			if (callback) {
				callback({success: false, error: 'Not implemented'});
			}
		},

		// Listen for messages
		onMessage: {
			addListener: function(callback) {
				// TODO: Register listener for messages from background
				console.log('[webext] chrome.runtime.onMessage.addListener');
			}
		},

		// Get extension URL
		getURL: function(path) {
			// TODO: Return proper extension:// URL
			return 'extension://' + path;
		}
	};

	// chrome.storage API (stub)
	chrome.storage = {
		local: {
			get: function(keys, callback) {
				console.log('[webext] chrome.storage.local.get:', keys);
				callback({});
			},
			set: function(items, callback) {
				console.log('[webext] chrome.storage.local.set:', items);
				if (callback) callback();
			}
		}
	};

	console.log('[webext] WebExtension API shim loaded');
})();
`
}
