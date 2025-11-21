package webext

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// I18nTranslations represents loaded translations for an extension
type I18nTranslations struct {
	Locale   string                 // Resolved locale (e.g., "en", "fr")
	Messages map[string]I18nMessage // Message key → translation
}

// LoadTranslationsForExtension loads i18n translations for an extension
// using proper locale resolution fallback chain:
// 1. System UI language (from LC_MESSAGES or LC_ALL)
// 2. Extension's default_locale from manifest
// 3. Language-only fallback (e.g., "en" from "en-US")
// 4. Fallback to "en"
func LoadTranslationsForExtension(ext *Extension) (*I18nTranslations, error) {
	// Determine locale priority order
	locales := resolveLocalePriority(ext)

	// Try each locale in order until we find translations
	for _, locale := range locales {
		translations, err := loadTranslationsForLocale(ext.Path, locale)
		if err == nil {
			return &I18nTranslations{
				Locale:   locale,
				Messages: translations,
			}, nil
		}
	}

	// No translations found - return empty translations with "en" locale
	return &I18nTranslations{
		Locale:   "en",
		Messages: make(map[string]I18nMessage),
	}, nil
}

// resolveLocalePriority determines the priority order of locales to try
// Returns a slice of locale codes in order of preference
func resolveLocalePriority(ext *Extension) []string {
	var locales []string
	seen := make(map[string]bool) // Avoid duplicates

	// 1. System UI language
	systemLocale := getSystemUILanguage()
	if systemLocale != "" {
		// Try full locale (e.g., "en-US")
		if !seen[systemLocale] {
			locales = append(locales, systemLocale)
			seen[systemLocale] = true
		}

		// Try language-only version (e.g., "en" from "en-US")
		if langOnly := getLanguageOnly(systemLocale); langOnly != systemLocale {
			if !seen[langOnly] {
				locales = append(locales, langOnly)
				seen[langOnly] = true
			}
		}
	}

	// 2. Extension's default_locale
	if ext.Manifest.DefaultLocale != "" {
		defaultLocale := ext.Manifest.DefaultLocale
		if !seen[defaultLocale] {
			locales = append(locales, defaultLocale)
			seen[defaultLocale] = true
		}

		// Try language-only version
		if langOnly := getLanguageOnly(defaultLocale); langOnly != defaultLocale {
			if !seen[langOnly] {
				locales = append(locales, langOnly)
				seen[langOnly] = true
			}
		}
	}

	// 3. Fallback to "en" if not already tried
	if !seen["en"] {
		locales = append(locales, "en")
	}

	return locales
}

// getSystemUILanguage gets the system's UI language from environment variables
// Checks LC_ALL, LC_MESSAGES, and LANG in that order (LC_ALL overrides everything)
// Returns locale in format like "en-US" or "en"
func getSystemUILanguage() string {
	// Check LC_ALL first (overrides all other locale settings)
	if locale := os.Getenv("LC_ALL"); locale != "" && locale != "C" {
		return normalizeLocale(locale)
	}

	// Check LC_MESSAGES (most specific for UI messages)
	if locale := os.Getenv("LC_MESSAGES"); locale != "" && locale != "C" {
		return normalizeLocale(locale)
	}

	// Check LANG (general locale setting)
	if locale := os.Getenv("LANG"); locale != "" && locale != "C" {
		return normalizeLocale(locale)
	}

	// Default to "en" if no locale found
	return "en"
}

// normalizeLocale converts system locale format to WebExtension format
// Input: "en_US.UTF-8" or "fr_FR" or "en"
// Output: "en-US" or "fr-FR" or "en"
func normalizeLocale(locale string) string {
	// Remove encoding suffix (e.g., .UTF-8)
	if idx := strings.Index(locale, "."); idx != -1 {
		locale = locale[:idx]
	}

	// Replace underscore with hyphen (e.g., en_US → en-US)
	locale = strings.ReplaceAll(locale, "_", "-")

	// Convert to lowercase for the language part, uppercase for region
	// e.g., "en-us" → "en-US"
	parts := strings.Split(locale, "-")
	if len(parts) == 2 {
		return strings.ToLower(parts[0]) + "-" + strings.ToUpper(parts[1])
	}

	return strings.ToLower(locale)
}

// getLanguageOnly extracts just the language code from a locale
// Input: "en-US" → Output: "en"
// Input: "en" → Output: "en"
func getLanguageOnly(locale string) string {
	if idx := strings.Index(locale, "-"); idx != -1 {
		return locale[:idx]
	}
	return locale
}

// loadTranslationsForLocale loads messages.json for a specific locale
// Returns map of message key → I18nMessage
func loadTranslationsForLocale(extPath, locale string) (map[string]I18nMessage, error) {
	messagesPath := filepath.Join(extPath, "_locales", locale, "messages.json")

	data, err := os.ReadFile(messagesPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read messages.json for locale %s: %w", locale, err)
	}

	var messages map[string]I18nMessage
	if err := json.Unmarshal(data, &messages); err != nil {
		return nil, fmt.Errorf("failed to parse messages.json for locale %s: %w", locale, err)
	}

	return messages, nil
}

// SerializeTranslations converts translations map to JSON string
// This is passed to the web process for browser.i18n.getMessage() calls
func SerializeTranslations(translations *I18nTranslations) (string, error) {
	if translations == nil || len(translations.Messages) == 0 {
		return "{}", nil
	}

	jsonData, err := json.Marshal(translations.Messages)
	if err != nil {
		return "", fmt.Errorf("failed to serialize translations: %w", err)
	}

	return string(jsonData), nil
}
