// Package webcrypto implements the W3C Web Cryptography API.
// https://www.w3.org/TR/WebCryptoAPI/
package webcrypto

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"errors"
)

// KeyType represents the type of a CryptoKey.
type KeyType string

const (
	KeyTypeSecret  KeyType = "secret"
	KeyTypePublic  KeyType = "public"
	KeyTypePrivate KeyType = "private"
)

// KeyUsage represents permitted operations for a key.
type KeyUsage string

const (
	UsageEncrypt    KeyUsage = "encrypt"
	UsageDecrypt    KeyUsage = "decrypt"
	UsageSign       KeyUsage = "sign"
	UsageVerify     KeyUsage = "verify"
	UsageDeriveKey  KeyUsage = "deriveKey"
	UsageDeriveBits KeyUsage = "deriveBits"
	UsageWrapKey    KeyUsage = "wrapKey"
	UsageUnwrapKey  KeyUsage = "unwrapKey"
)

// Algorithm identifies a cryptographic algorithm.
type Algorithm struct {
	Name string
}

// CryptoKey represents a cryptographic key.
type CryptoKey struct {
	Type        KeyType
	Extractable bool
	Algorithm   Algorithm
	Usages      []KeyUsage
	handle      any // Internal key material
}

// Handle returns the underlying key material.
// For secret keys: []byte
// For RSA keys: *rsa.PrivateKey or *rsa.PublicKey
// For ECDSA keys: *ecdsa.PrivateKey or *ecdsa.PublicKey
func (k *CryptoKey) Handle() any {
	return k.handle
}

// SecretKey returns the key as raw bytes (for symmetric keys).
func (k *CryptoKey) SecretKey() ([]byte, error) {
	if k.Type != KeyTypeSecret {
		return nil, errors.New("not a secret key")
	}
	if b, ok := k.handle.([]byte); ok {
		return b, nil
	}
	return nil, errors.New("invalid key handle")
}

// RSAPrivateKey returns the key as an RSA private key.
func (k *CryptoKey) RSAPrivateKey() (*rsa.PrivateKey, error) {
	if k.Type != KeyTypePrivate {
		return nil, errors.New("not a private key")
	}
	if key, ok := k.handle.(*rsa.PrivateKey); ok {
		return key, nil
	}
	return nil, errors.New("not an RSA key")
}

// RSAPublicKey returns the key as an RSA public key.
func (k *CryptoKey) RSAPublicKey() (*rsa.PublicKey, error) {
	if k.Type == KeyTypePrivate {
		if key, ok := k.handle.(*rsa.PrivateKey); ok {
			return &key.PublicKey, nil
		}
	}
	if key, ok := k.handle.(*rsa.PublicKey); ok {
		return key, nil
	}
	return nil, errors.New("not an RSA key")
}

// ECDSAPrivateKey returns the key as an ECDSA private key.
func (k *CryptoKey) ECDSAPrivateKey() (*ecdsa.PrivateKey, error) {
	if k.Type != KeyTypePrivate {
		return nil, errors.New("not a private key")
	}
	if key, ok := k.handle.(*ecdsa.PrivateKey); ok {
		return key, nil
	}
	return nil, errors.New("not an ECDSA key")
}

// ECDSAPublicKey returns the key as an ECDSA public key.
func (k *CryptoKey) ECDSAPublicKey() (*ecdsa.PublicKey, error) {
	if k.Type == KeyTypePrivate {
		if key, ok := k.handle.(*ecdsa.PrivateKey); ok {
			return &key.PublicKey, nil
		}
	}
	if key, ok := k.handle.(*ecdsa.PublicKey); ok {
		return key, nil
	}
	return nil, errors.New("not an ECDSA key")
}

// HasUsage checks if the key has a specific usage.
func (k *CryptoKey) HasUsage(usage KeyUsage) bool {
	for _, u := range k.Usages {
		if u == usage {
			return true
		}
	}
	return false
}

// NewSecretKey creates a new secret (symmetric) key.
func NewSecretKey(algorithm Algorithm, extractable bool, usages []KeyUsage, keyData []byte) *CryptoKey {
	return &CryptoKey{
		Type:        KeyTypeSecret,
		Extractable: extractable,
		Algorithm:   algorithm,
		Usages:      usages,
		handle:      keyData,
	}
}

// NewPublicKey creates a new public key.
func NewPublicKey(algorithm Algorithm, extractable bool, usages []KeyUsage, key any) *CryptoKey {
	return &CryptoKey{
		Type:        KeyTypePublic,
		Extractable: extractable,
		Algorithm:   algorithm,
		Usages:      usages,
		handle:      key,
	}
}

// NewPrivateKey creates a new private key.
func NewPrivateKey(algorithm Algorithm, extractable bool, usages []KeyUsage, key any) *CryptoKey {
	return &CryptoKey{
		Type:        KeyTypePrivate,
		Extractable: extractable,
		Algorithm:   algorithm,
		Usages:      usages,
		handle:      key,
	}
}

// CryptoKeyPair represents an asymmetric key pair.
type CryptoKeyPair struct {
	PublicKey  *CryptoKey
	PrivateKey *CryptoKey
}
