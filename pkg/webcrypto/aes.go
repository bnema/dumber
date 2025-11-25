package webcrypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
)

// AES-CBC encrypt
func aesCBCEncrypt(key *CryptoKey, iv, plaintext []byte) ([]byte, error) {
	keyBytes, err := key.SecretKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("iv must be %d bytes", aes.BlockSize)
	}

	// PKCS7 padding
	padding := aes.BlockSize - len(plaintext)%aes.BlockSize
	padded := make([]byte, len(plaintext)+padding)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padding)
	}

	ciphertext := make([]byte, len(padded))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, padded)

	return ciphertext, nil
}

// AES-CBC decrypt
func aesCBCDecrypt(key *CryptoKey, iv, ciphertext []byte) ([]byte, error) {
	keyBytes, err := key.SecretKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	if len(iv) != aes.BlockSize {
		return nil, fmt.Errorf("iv must be %d bytes", aes.BlockSize)
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, errors.New("ciphertext is not a multiple of the block size")
	}

	plaintext := make([]byte, len(ciphertext))
	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(plaintext, ciphertext)

	// Remove PKCS7 padding
	if len(plaintext) == 0 {
		return nil, errors.New("invalid padding")
	}
	padding := int(plaintext[len(plaintext)-1])
	if padding > aes.BlockSize || padding == 0 {
		return nil, errors.New("invalid padding")
	}
	for i := len(plaintext) - padding; i < len(plaintext); i++ {
		if plaintext[i] != byte(padding) {
			return nil, errors.New("invalid padding")
		}
	}

	return plaintext[:len(plaintext)-padding], nil
}

// AES-GCM encrypt
func aesGCMEncrypt(key *CryptoKey, iv, plaintext, additionalData []byte) ([]byte, error) {
	keyBytes, err := key.SecretKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// GCM appends the auth tag to the ciphertext
	return gcm.Seal(nil, iv, plaintext, additionalData), nil
}

// AES-GCM decrypt
func aesGCMDecrypt(key *CryptoKey, iv, ciphertext, additionalData []byte) ([]byte, error) {
	keyBytes, err := key.SecretKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	return gcm.Open(nil, iv, ciphertext, additionalData)
}

// AES-CTR encrypt (same as decrypt for CTR mode)
func aesCTREncrypt(key *CryptoKey, counter []byte, length int, plaintext []byte) ([]byte, error) {
	keyBytes, err := key.SecretKey()
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return nil, err
	}

	if len(counter) != aes.BlockSize {
		return nil, fmt.Errorf("counter must be %d bytes", aes.BlockSize)
	}

	ciphertext := make([]byte, len(plaintext))
	stream := cipher.NewCTR(block, counter)
	stream.XORKeyStream(ciphertext, plaintext)

	return ciphertext, nil
}

// AES-CTR decrypt (same as encrypt for CTR mode)
func aesCTRDecrypt(key *CryptoKey, counter []byte, length int, ciphertext []byte) ([]byte, error) {
	return aesCTREncrypt(key, counter, length, ciphertext)
}

// generateAESKey generates a new AES key.
func generateAESKey(algorithm string, length int, extractable bool, usages []KeyUsage) (*CryptoKey, error) {
	if length == 0 {
		length = 256 // Default to AES-256
	}

	switch length {
	case 128, 192, 256:
		// Valid key lengths
	default:
		return nil, fmt.Errorf("invalid AES key length: %d", length)
	}

	keyBytes := make([]byte, length/8)
	if _, err := rand.Read(keyBytes); err != nil {
		return nil, err
	}

	return NewSecretKey(Algorithm{Name: algorithm}, extractable, usages, keyBytes), nil
}

// importAESKey imports an AES key from raw or JWK format.
func importAESKey(format string, keyData any, algorithm string, extractable bool, usages []KeyUsage) (*CryptoKey, error) {
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
		// Extract 'k' parameter (base64url-encoded key)
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

	// Validate key length
	switch len(keyBytes) {
	case 16, 24, 32: // 128, 192, 256 bits
		// Valid
	default:
		return nil, fmt.Errorf("invalid AES key length: %d bytes", len(keyBytes))
	}

	return NewSecretKey(Algorithm{Name: algorithm}, extractable, usages, keyBytes), nil
}

// exportAESKey exports an AES key to raw or JWK format.
func exportAESKey(format string, key *CryptoKey) (any, error) {
	keyBytes, err := key.SecretKey()
	if err != nil {
		return nil, err
	}

	switch format {
	case "raw":
		return keyBytes, nil
	case "jwk":
		algName := ""
		switch len(keyBytes) {
		case 16:
			algName = "A128CBC" // Simplified, real JWK would need algorithm-specific alg
		case 24:
			algName = "A192CBC"
		case 32:
			algName = "A256CBC"
		}
		return map[string]any{
			"kty":     "oct",
			"k":       base64URLEncode(keyBytes),
			"alg":     algName,
			"key_ops": usagesToStrings(key.Usages),
			"ext":     key.Extractable,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}
