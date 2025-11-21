package webext

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeLocale(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"en_US.UTF-8", "en-US"},
		{"fr_FR.UTF-8", "fr-FR"},
		{"en_US", "en-US"},
		{"en", "en"},
		{"fr", "fr"},
		{"zh_CN.UTF-8", "zh-CN"},
		{"pt_BR", "pt-BR"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := normalizeLocale(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeLocale(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetLanguageOnly(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"en-US", "en"},
		{"en", "en"},
		{"fr-FR", "fr"},
		{"zh-CN", "zh"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := getLanguageOnly(tt.input)
			if result != tt.expected {
				t.Errorf("getLanguageOnly(%q) = %q; want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestResolveLocalePriority(t *testing.T) {
	// Create a mock extension
	ext := &Extension{
		Manifest: &Manifest{
			DefaultLocale: "fr",
		},
	}

	// Test with system locale set
	os.Setenv("LC_MESSAGES", "en_US.UTF-8")
	defer os.Unsetenv("LC_MESSAGES")

	locales := resolveLocalePriority(ext)

	// Should prioritize: en-US, en, fr, en (fallback)
	// But avoid duplicates
	expected := []string{"en-US", "en", "fr"}
	if len(locales) != len(expected) {
		t.Errorf("resolveLocalePriority() returned %d locales; want %d", len(locales), len(expected))
	}

	for i, locale := range expected {
		if i >= len(locales) || locales[i] != locale {
			t.Errorf("resolveLocalePriority()[%d] = %q; want %q", i, locales[i], locale)
		}
	}
}

func TestGetSystemUILanguage(t *testing.T) {
	tests := []struct {
		name         string
		lcMessages   string
		lcAll        string
		lang         string
		expectedLang string // Just the language part (e.g., "en" from "en-US")
	}{
		{
			name:         "LC_MESSAGES set",
			lcMessages:   "fr_FR.UTF-8",
			expectedLang: "fr",
		},
		{
			name:         "LC_ALL set (overrides)",
			lcMessages:   "en_US.UTF-8",
			lcAll:        "de_DE.UTF-8",
			expectedLang: "de",
		},
		{
			name:         "LANG set (lowest priority)",
			lang:         "es_ES.UTF-8",
			expectedLang: "es",
		},
		{
			name:         "C locale falls back to en",
			lcMessages:   "C",
			expectedLang: "en",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear env vars
			os.Unsetenv("LC_MESSAGES")
			os.Unsetenv("LC_ALL")
			os.Unsetenv("LANG")

			// Set test values
			if tt.lcMessages != "" {
				os.Setenv("LC_MESSAGES", tt.lcMessages)
			}
			if tt.lcAll != "" {
				os.Setenv("LC_ALL", tt.lcAll)
			}
			if tt.lang != "" {
				os.Setenv("LANG", tt.lang)
			}

			result := getSystemUILanguage()
			lang := getLanguageOnly(result)
			if lang != tt.expectedLang {
				t.Errorf("getSystemUILanguage() language = %q; want %q", lang, tt.expectedLang)
			}

			// Cleanup
			os.Unsetenv("LC_MESSAGES")
			os.Unsetenv("LC_ALL")
			os.Unsetenv("LANG")
		})
	}
}

func TestLoadTranslationsForLocale(t *testing.T) {
	// Create temporary extension directory structure
	tmpDir := t.TempDir()
	localesDir := filepath.Join(tmpDir, "_locales", "en")
	if err := os.MkdirAll(localesDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create messages.json
	messagesJSON := `{
		"extensionName": {
			"message": "Test Extension",
			"description": "Name of the extension"
		},
		"greeting": {
			"message": "Hello, $1!",
			"description": "Greeting message"
		}
	}`
	messagesPath := filepath.Join(localesDir, "messages.json")
	if err := os.WriteFile(messagesPath, []byte(messagesJSON), 0644); err != nil {
		t.Fatal(err)
	}

	// Test loading
	messages, err := loadTranslationsForLocale(tmpDir, "en")
	if err != nil {
		t.Fatalf("loadTranslationsForLocale() error = %v", err)
	}

	if len(messages) != 2 {
		t.Errorf("loadTranslationsForLocale() loaded %d messages; want 2", len(messages))
	}

	if msg, ok := messages["extensionName"]; !ok || msg.Message != "Test Extension" {
		t.Errorf("extensionName message = %q; want 'Test Extension'", msg.Message)
	}

	if msg, ok := messages["greeting"]; !ok || msg.Message != "Hello, $1!" {
		t.Errorf("greeting message = %q; want 'Hello, $1!'", msg.Message)
	}
}

func TestSerializeTranslations(t *testing.T) {
	tests := []struct {
		name     string
		input    *I18nTranslations
		expected string
	}{
		{
			name:     "nil translations",
			input:    nil,
			expected: "{}",
		},
		{
			name: "empty messages",
			input: &I18nTranslations{
				Locale:   "en",
				Messages: make(map[string]I18nMessage),
			},
			expected: "{}",
		},
		{
			name: "single message",
			input: &I18nTranslations{
				Locale: "en",
				Messages: map[string]I18nMessage{
					"extensionName": {
						Message:     "Test",
						Description: "Test description",
					},
				},
			},
			expected: `{"extensionName":{"message":"Test","description":"Test description"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := SerializeTranslations(tt.input)
			if err != nil {
				t.Errorf("SerializeTranslations() error = %v", err)
			}
			if result != tt.expected {
				t.Errorf("SerializeTranslations() = %q; want %q", result, tt.expected)
			}
		})
	}
}
