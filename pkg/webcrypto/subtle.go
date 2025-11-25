package webcrypto

import (
	"errors"
	"fmt"
)

// Common errors
var (
	ErrUnsupportedAlgorithm = errors.New("unsupported algorithm")
	ErrInvalidKey           = errors.New("invalid key for this operation")
	ErrInvalidKeyUsage      = errors.New("key does not have required usage")
	ErrInvalidData          = errors.New("invalid data")
	ErrNotExtractable       = errors.New("key is not extractable")
)

// SubtleCrypto provides cryptographic operations.
type SubtleCrypto struct{}

// NewSubtleCrypto creates a new SubtleCrypto instance.
func NewSubtleCrypto() *SubtleCrypto {
	return &SubtleCrypto{}
}

// AlgorithmParams holds algorithm-specific parameters.
type AlgorithmParams struct {
	Name string
	// AES parameters
	Iv      []byte
	Counter []byte
	Length  int
	// RSA-OAEP parameters
	Label []byte
	// ECDSA parameters
	Hash string
	// Key derivation parameters
	Salt       []byte
	Info       []byte
	Iterations int
}

// ParseAlgorithm extracts algorithm parameters from various input formats.
func ParseAlgorithm(input any) (*AlgorithmParams, error) {
	switch v := input.(type) {
	case string:
		return &AlgorithmParams{Name: v}, nil
	case map[string]any:
		params := &AlgorithmParams{}
		if name, ok := v["name"].(string); ok {
			params.Name = name
		} else {
			return nil, errors.New("algorithm name is required")
		}
		if iv, ok := v["iv"].([]byte); ok {
			params.Iv = iv
		}
		if counter, ok := v["counter"].([]byte); ok {
			params.Counter = counter
		}
		if length, ok := v["length"].(int); ok {
			params.Length = length
		} else if length, ok := v["length"].(float64); ok {
			params.Length = int(length)
		}
		if label, ok := v["label"].([]byte); ok {
			params.Label = label
		}
		if hash, ok := v["hash"].(string); ok {
			params.Hash = hash
		} else if hashMap, ok := v["hash"].(map[string]any); ok {
			if name, ok := hashMap["name"].(string); ok {
				params.Hash = name
			}
		}
		if salt, ok := v["salt"].([]byte); ok {
			params.Salt = salt
		}
		if info, ok := v["info"].([]byte); ok {
			params.Info = info
		}
		if iterations, ok := v["iterations"].(int); ok {
			params.Iterations = iterations
		} else if iterations, ok := v["iterations"].(float64); ok {
			params.Iterations = int(iterations)
		}
		return params, nil
	default:
		return nil, fmt.Errorf("invalid algorithm format: %T", input)
	}
}

// Encrypt encrypts data using the specified algorithm and key.
func (s *SubtleCrypto) Encrypt(algorithm any, key *CryptoKey, data []byte) ([]byte, error) {
	params, err := ParseAlgorithm(algorithm)
	if err != nil {
		return nil, err
	}

	if !key.HasUsage(UsageEncrypt) {
		return nil, ErrInvalidKeyUsage
	}

	switch params.Name {
	case "AES-CBC":
		return aesCBCEncrypt(key, params.Iv, data)
	case "AES-GCM":
		return aesGCMEncrypt(key, params.Iv, data, nil)
	case "AES-CTR":
		return aesCTREncrypt(key, params.Counter, params.Length, data)
	case "RSA-OAEP":
		return rsaOAEPEncrypt(key, params.Hash, params.Label, data)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, params.Name)
	}
}

// Decrypt decrypts data using the specified algorithm and key.
func (s *SubtleCrypto) Decrypt(algorithm any, key *CryptoKey, data []byte) ([]byte, error) {
	params, err := ParseAlgorithm(algorithm)
	if err != nil {
		return nil, err
	}

	if !key.HasUsage(UsageDecrypt) {
		return nil, ErrInvalidKeyUsage
	}

	switch params.Name {
	case "AES-CBC":
		return aesCBCDecrypt(key, params.Iv, data)
	case "AES-GCM":
		return aesGCMDecrypt(key, params.Iv, data, nil)
	case "AES-CTR":
		return aesCTRDecrypt(key, params.Counter, params.Length, data)
	case "RSA-OAEP":
		return rsaOAEPDecrypt(key, params.Hash, params.Label, data)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, params.Name)
	}
}

// Sign generates a signature using the specified algorithm and key.
func (s *SubtleCrypto) Sign(algorithm any, key *CryptoKey, data []byte) ([]byte, error) {
	params, err := ParseAlgorithm(algorithm)
	if err != nil {
		return nil, err
	}

	if !key.HasUsage(UsageSign) {
		return nil, ErrInvalidKeyUsage
	}

	switch params.Name {
	case "HMAC":
		return hmacSign(key, data)
	case "ECDSA":
		return ecdsaSign(key, params.Hash, data)
	case "RSASSA-PKCS1-v1_5":
		return rsassaPKCS1Sign(key, params.Hash, data)
	case "RSA-PSS":
		return rsaPSSSign(key, params.Hash, data)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, params.Name)
	}
}

// Verify verifies a signature using the specified algorithm and key.
func (s *SubtleCrypto) Verify(algorithm any, key *CryptoKey, signature, data []byte) (bool, error) {
	params, err := ParseAlgorithm(algorithm)
	if err != nil {
		return false, err
	}

	if !key.HasUsage(UsageVerify) {
		return false, ErrInvalidKeyUsage
	}

	switch params.Name {
	case "HMAC":
		return hmacVerify(key, signature, data)
	case "ECDSA":
		return ecdsaVerify(key, params.Hash, signature, data)
	case "RSASSA-PKCS1-v1_5":
		return rsassaPKCS1Verify(key, params.Hash, signature, data)
	case "RSA-PSS":
		return rsaPSSVerify(key, params.Hash, signature, data)
	default:
		return false, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, params.Name)
	}
}

// Digest computes a digest (hash) of the data.
func (s *SubtleCrypto) Digest(algorithm any, data []byte) ([]byte, error) {
	params, err := ParseAlgorithm(algorithm)
	if err != nil {
		return nil, err
	}
	return digest(params.Name, data)
}

// DeriveBits derives bits from a base key.
func (s *SubtleCrypto) DeriveBits(algorithm any, baseKey *CryptoKey, length int) ([]byte, error) {
	params, err := ParseAlgorithm(algorithm)
	if err != nil {
		return nil, err
	}

	if !baseKey.HasUsage(UsageDeriveBits) {
		return nil, ErrInvalidKeyUsage
	}

	switch params.Name {
	case "PBKDF2":
		return pbkdf2DeriveBits(baseKey, params.Salt, params.Iterations, params.Hash, length)
	case "HKDF":
		return hkdfDeriveBits(baseKey, params.Salt, params.Info, params.Hash, length)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, params.Name)
	}
}

// DeriveKey derives a new key from a base key.
func (s *SubtleCrypto) DeriveKey(algorithm any, baseKey *CryptoKey, derivedKeyType any, extractable bool, keyUsages []KeyUsage) (*CryptoKey, error) {
	// Validate algorithm (not used directly since we pass to DeriveBits)
	if _, err := ParseAlgorithm(algorithm); err != nil {
		return nil, err
	}

	derivedParams, err := ParseAlgorithm(derivedKeyType)
	if err != nil {
		return nil, err
	}

	if !baseKey.HasUsage(UsageDeriveKey) {
		return nil, ErrInvalidKeyUsage
	}

	// Determine key length from derived key type
	keyLength := derivedParams.Length
	if keyLength == 0 {
		// Default key lengths
		switch derivedParams.Name {
		case "AES-CBC", "AES-GCM", "AES-CTR":
			keyLength = 256 // Default to AES-256
		case "HMAC":
			keyLength = 256
		default:
			return nil, errors.New("unable to determine derived key length")
		}
	}

	// Derive the bits
	bits, err := s.DeriveBits(algorithm, baseKey, keyLength)
	if err != nil {
		return nil, err
	}

	return NewSecretKey(Algorithm{Name: derivedParams.Name}, extractable, keyUsages, bits), nil
}

// GenerateKey generates a new key or key pair.
func (s *SubtleCrypto) GenerateKey(algorithm any, extractable bool, keyUsages []KeyUsage) (any, error) {
	params, err := ParseAlgorithm(algorithm)
	if err != nil {
		return nil, err
	}

	switch params.Name {
	case "AES-CBC", "AES-GCM", "AES-CTR":
		return generateAESKey(params.Name, params.Length, extractable, keyUsages)
	case "HMAC":
		return generateHMACKey(params.Hash, params.Length, extractable, keyUsages)
	case "RSA-OAEP", "RSASSA-PKCS1-v1_5", "RSA-PSS":
		return generateRSAKeyPair(params.Name, params.Length, params.Hash, extractable, keyUsages)
	case "ECDSA", "ECDH":
		return generateECKeyPair(params.Name, params.Hash, extractable, keyUsages)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, params.Name)
	}
}

// ImportKey imports a key from external data.
func (s *SubtleCrypto) ImportKey(format string, keyData any, algorithm any, extractable bool, keyUsages []KeyUsage) (*CryptoKey, error) {
	params, err := ParseAlgorithm(algorithm)
	if err != nil {
		return nil, err
	}

	switch params.Name {
	case "AES-CBC", "AES-GCM", "AES-CTR":
		return importAESKey(format, keyData, params.Name, extractable, keyUsages)
	case "HMAC":
		return importHMACKey(format, keyData, params.Hash, extractable, keyUsages)
	case "PBKDF2":
		return importPBKDF2Key(format, keyData, extractable, keyUsages)
	case "HKDF":
		return importHKDFKey(format, keyData, extractable, keyUsages)
	case "RSA-OAEP", "RSASSA-PKCS1-v1_5", "RSA-PSS":
		return importRSAKey(format, keyData, params.Name, params.Hash, extractable, keyUsages)
	case "ECDSA", "ECDH":
		return importECKey(format, keyData, params.Name, params.Hash, extractable, keyUsages)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, params.Name)
	}
}

// ExportKey exports a key to the specified format.
func (s *SubtleCrypto) ExportKey(format string, key *CryptoKey) (any, error) {
	if !key.Extractable {
		return nil, ErrNotExtractable
	}

	switch key.Algorithm.Name {
	case "AES-CBC", "AES-GCM", "AES-CTR":
		return exportAESKey(format, key)
	case "HMAC":
		return exportHMACKey(format, key)
	case "RSA-OAEP", "RSASSA-PKCS1-v1_5", "RSA-PSS":
		return exportRSAKey(format, key)
	case "ECDSA", "ECDH":
		return exportECKey(format, key)
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAlgorithm, key.Algorithm.Name)
	}
}
