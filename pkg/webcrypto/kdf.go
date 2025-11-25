package webcrypto

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	"hash"
	"io"

	"golang.org/x/crypto/hkdf"
	"golang.org/x/crypto/pbkdf2"
)

// getHashFunc returns the hash function for the given algorithm name.
func getHashFunc(name string) (func() hash.Hash, error) {
	switch name {
	case "SHA-1":
		return sha1.New, nil
	case "SHA-256":
		return sha256.New, nil
	case "SHA-384":
		return sha512.New384, nil
	case "SHA-512":
		return sha512.New, nil
	default:
		return nil, fmt.Errorf("unsupported hash algorithm: %s", name)
	}
}

// pbkdf2DeriveBits derives bits using PBKDF2.
func pbkdf2DeriveBits(baseKey *CryptoKey, salt []byte, iterations int, hashAlg string, length int) ([]byte, error) {
	password, err := baseKey.SecretKey()
	if err != nil {
		return nil, err
	}

	if iterations <= 0 {
		return nil, errors.New("iterations must be positive")
	}

	if length <= 0 || length%8 != 0 {
		return nil, errors.New("length must be a positive multiple of 8")
	}

	h, err := getHashFunc(hashAlg)
	if err != nil {
		return nil, err
	}

	return pbkdf2.Key(password, salt, iterations, length/8, h), nil
}

// hkdfDeriveBits derives bits using HKDF.
func hkdfDeriveBits(baseKey *CryptoKey, salt, info []byte, hashAlg string, length int) ([]byte, error) {
	secret, err := baseKey.SecretKey()
	if err != nil {
		return nil, err
	}

	if length <= 0 || length%8 != 0 {
		return nil, errors.New("length must be a positive multiple of 8")
	}

	h, err := getHashFunc(hashAlg)
	if err != nil {
		return nil, err
	}

	reader := hkdf.New(h, secret, salt, info)
	derived := make([]byte, length/8)
	if _, err := io.ReadFull(reader, derived); err != nil {
		return nil, err
	}

	return derived, nil
}

// importPBKDF2Key imports a key for use with PBKDF2.
func importPBKDF2Key(format string, keyData any, extractable bool, usages []KeyUsage) (*CryptoKey, error) {
	if format != "raw" {
		return nil, fmt.Errorf("PBKDF2 only supports 'raw' format, got: %s", format)
	}

	keyBytes, ok := keyData.([]byte)
	if !ok {
		return nil, errors.New("raw key data must be []byte")
	}

	return NewSecretKey(Algorithm{Name: "PBKDF2"}, extractable, usages, keyBytes), nil
}

// importHKDFKey imports a key for use with HKDF.
func importHKDFKey(format string, keyData any, extractable bool, usages []KeyUsage) (*CryptoKey, error) {
	if format != "raw" {
		return nil, fmt.Errorf("HKDF only supports 'raw' format, got: %s", format)
	}

	keyBytes, ok := keyData.([]byte)
	if !ok {
		return nil, errors.New("raw key data must be []byte")
	}

	return NewSecretKey(Algorithm{Name: "HKDF"}, extractable, usages, keyBytes), nil
}
