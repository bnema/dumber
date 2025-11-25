package webcrypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
)

// hmacSign computes an HMAC signature.
func hmacSign(key *CryptoKey, data []byte) ([]byte, error) {
	var keyBytes []byte
	var hashAlg string

	// Get key bytes and hash algorithm
	if hk, ok := key.handle.(hmacKey); ok {
		keyBytes = hk.key
		hashAlg = hk.hash
	} else if kb, ok := key.handle.([]byte); ok {
		keyBytes = kb
		hashAlg = "SHA-256" // Default
	} else {
		return nil, ErrInvalidKey
	}

	h, err := getHashFunc(hashAlg)
	if err != nil {
		return nil, err
	}

	mac := hmac.New(h, keyBytes)
	mac.Write(data)
	return mac.Sum(nil), nil
}

// hmacVerify verifies an HMAC signature.
func hmacVerify(key *CryptoKey, signature, data []byte) (bool, error) {
	expected, err := hmacSign(key, data)
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(signature, expected) == 1, nil
}

// hmacKey wraps HMAC key with its hash algorithm.
type hmacKey struct {
	key  []byte
	hash string
}

// generateHMACKey generates a new HMAC key.
func generateHMACKey(hashAlg string, length int, extractable bool, usages []KeyUsage) (*CryptoKey, error) {
	if hashAlg == "" {
		hashAlg = "SHA-256"
	}

	// Determine key length if not specified
	if length == 0 {
		switch hashAlg {
		case "SHA-1":
			length = 160
		case "SHA-256":
			length = 256
		case "SHA-384":
			length = 384
		case "SHA-512":
			length = 512
		default:
			length = 256
		}
	}

	keyBytes := make([]byte, length/8)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}

	return &CryptoKey{
		Type:        KeyTypeSecret,
		Extractable: extractable,
		Algorithm:   Algorithm{Name: "HMAC"},
		Usages:      usages,
		handle:      hmacKey{key: keyBytes, hash: hashAlg},
	}, nil
}

// importHMACKey imports an HMAC key.
func importHMACKey(format string, keyData any, hashAlg string, extractable bool, usages []KeyUsage) (*CryptoKey, error) {
	if hashAlg == "" {
		hashAlg = "SHA-256"
	}

	var keyBytes []byte

	switch format {
	case "raw":
		var ok bool
		keyBytes, ok = keyData.([]byte)
		if !ok {
			return nil, errors.New("raw key data must be []byte")
		}
	case "jwk":
		jwk, ok := keyData.(map[string]any)
		if !ok {
			return nil, errors.New("jwk must be an object")
		}
		kStr, ok := jwk["k"].(string)
		if !ok {
			return nil, errors.New("jwk missing 'k' parameter")
		}
		var err error
		keyBytes, err = base64URLDecode(kStr)
		if err != nil {
			return nil, fmt.Errorf("invalid base64url in jwk: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported key format: %s", format)
	}

	return &CryptoKey{
		Type:        KeyTypeSecret,
		Extractable: extractable,
		Algorithm:   Algorithm{Name: "HMAC"},
		Usages:      usages,
		handle:      hmacKey{key: keyBytes, hash: hashAlg},
	}, nil
}

// exportHMACKey exports an HMAC key.
func exportHMACKey(format string, key *CryptoKey) (any, error) {
	hk, ok := key.handle.(hmacKey)
	if !ok {
		return nil, errors.New("invalid HMAC key")
	}

	switch format {
	case "raw":
		return hk.key, nil
	case "jwk":
		return map[string]any{
			"kty":     "oct",
			"k":       base64URLEncode(hk.key),
			"alg":     "HS" + hk.hash[4:], // SHA-256 -> HS256
			"key_ops": usagesToStrings(key.Usages),
			"ext":     key.Extractable,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}
