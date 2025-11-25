package webcrypto

import (
	"encoding/base64"
	"strings"
)

// base64URLEncode encodes bytes to base64url without padding.
func base64URLEncode(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

// base64URLDecode decodes base64url with or without padding.
func base64URLDecode(s string) ([]byte, error) {
	// Add padding if necessary
	switch len(s) % 4 {
	case 2:
		s += "=="
	case 3:
		s += "="
	}
	return base64.URLEncoding.DecodeString(s)
}

// usagesToStrings converts KeyUsage slice to string slice.
func usagesToStrings(usages []KeyUsage) []string {
	result := make([]string, len(usages))
	for i, u := range usages {
		result[i] = string(u)
	}
	return result
}

// stringsToUsages converts string slice to KeyUsage slice.
func stringsToUsages(strs []string) []KeyUsage {
	result := make([]KeyUsage, len(strs))
	for i, s := range strs {
		result[i] = KeyUsage(s)
	}
	return result
}
