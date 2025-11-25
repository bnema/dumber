package webcrypto

import (
	"bytes"
	"testing"
)

func TestDigest(t *testing.T) {
	subtle := NewSubtleCrypto()
	data := []byte("hello world")

	tests := []struct {
		alg      string
		expected string // hex of expected hash
	}{
		{"SHA-1", "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed"},
		{"SHA-256", "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"},
	}

	for _, tc := range tests {
		t.Run(tc.alg, func(t *testing.T) {
			result, err := subtle.Digest(tc.alg, data)
			if err != nil {
				t.Fatalf("Digest failed: %v", err)
			}
			if len(result) == 0 {
				t.Fatal("Digest returned empty result")
			}
		})
	}
}

func TestAESCBC(t *testing.T) {
	subtle := NewSubtleCrypto()

	// Generate key
	keyResult, err := subtle.GenerateKey(map[string]any{
		"name":   "AES-CBC",
		"length": 256,
	}, true, []KeyUsage{UsageEncrypt, UsageDecrypt})
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	key := keyResult.(*CryptoKey)

	// Test data
	plaintext := []byte("Hello, WebCrypto!")
	iv := make([]byte, 16)
	for i := range iv {
		iv[i] = byte(i)
	}

	// Encrypt
	ciphertext, err := subtle.Encrypt(map[string]any{
		"name": "AES-CBC",
		"iv":   iv,
	}, key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	if bytes.Equal(ciphertext, plaintext) {
		t.Fatal("Ciphertext should not equal plaintext")
	}

	// Decrypt
	decrypted, err := subtle.Decrypt(map[string]any{
		"name": "AES-CBC",
		"iv":   iv,
	}, key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("Decrypted data doesn't match: got %q, want %q", decrypted, plaintext)
	}
}

func TestAESGCM(t *testing.T) {
	subtle := NewSubtleCrypto()

	// Generate key
	keyResult, err := subtle.GenerateKey(map[string]any{
		"name":   "AES-GCM",
		"length": 256,
	}, true, []KeyUsage{UsageEncrypt, UsageDecrypt})
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	key := keyResult.(*CryptoKey)

	plaintext := []byte("Hello, AES-GCM!")
	iv := make([]byte, 12) // GCM typically uses 12-byte IV
	for i := range iv {
		iv[i] = byte(i)
	}

	// Encrypt
	ciphertext, err := subtle.Encrypt(map[string]any{
		"name": "AES-GCM",
		"iv":   iv,
	}, key, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decrypt
	decrypted, err := subtle.Decrypt(map[string]any{
		"name": "AES-GCM",
		"iv":   iv,
	}, key, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("Decrypted data doesn't match: got %q, want %q", decrypted, plaintext)
	}
}

func TestPBKDF2(t *testing.T) {
	subtle := NewSubtleCrypto()

	// Import password as key
	password := []byte("password123")
	baseKey, err := subtle.ImportKey("raw", password, "PBKDF2", false, []KeyUsage{UsageDeriveBits, UsageDeriveKey})
	if err != nil {
		t.Fatalf("ImportKey failed: %v", err)
	}

	salt := []byte("salt1234salt1234")

	// Derive bits
	bits, err := subtle.DeriveBits(map[string]any{
		"name":       "PBKDF2",
		"salt":       salt,
		"iterations": 100000,
		"hash":       "SHA-256",
	}, baseKey, 256)
	if err != nil {
		t.Fatalf("DeriveBits failed: %v", err)
	}

	if len(bits) != 32 {
		t.Fatalf("Expected 32 bytes, got %d", len(bits))
	}

	// Derive same bits again - should be identical
	bits2, err := subtle.DeriveBits(map[string]any{
		"name":       "PBKDF2",
		"salt":       salt,
		"iterations": 100000,
		"hash":       "SHA-256",
	}, baseKey, 256)
	if err != nil {
		t.Fatalf("DeriveBits (2) failed: %v", err)
	}

	if !bytes.Equal(bits, bits2) {
		t.Fatal("PBKDF2 should be deterministic")
	}
}

func TestHKDF(t *testing.T) {
	subtle := NewSubtleCrypto()

	// Import key material
	keyMaterial := []byte("input key material")
	baseKey, err := subtle.ImportKey("raw", keyMaterial, "HKDF", false, []KeyUsage{UsageDeriveBits})
	if err != nil {
		t.Fatalf("ImportKey failed: %v", err)
	}

	salt := []byte("salt")
	info := []byte("info")

	bits, err := subtle.DeriveBits(map[string]any{
		"name": "HKDF",
		"salt": salt,
		"info": info,
		"hash": "SHA-256",
	}, baseKey, 256)
	if err != nil {
		t.Fatalf("DeriveBits failed: %v", err)
	}

	if len(bits) != 32 {
		t.Fatalf("Expected 32 bytes, got %d", len(bits))
	}
}

func TestHMAC(t *testing.T) {
	subtle := NewSubtleCrypto()

	// Generate HMAC key
	keyResult, err := subtle.GenerateKey(map[string]any{
		"name": "HMAC",
		"hash": "SHA-256",
	}, true, []KeyUsage{UsageSign, UsageVerify})
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	key := keyResult.(*CryptoKey)
	data := []byte("message to sign")

	// Sign
	signature, err := subtle.Sign("HMAC", key, data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	if len(signature) == 0 {
		t.Fatal("Signature is empty")
	}

	// Verify
	valid, err := subtle.Verify("HMAC", key, signature, data)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !valid {
		t.Fatal("Signature should be valid")
	}

	// Verify with wrong data
	valid, err = subtle.Verify("HMAC", key, signature, []byte("wrong data"))
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if valid {
		t.Fatal("Signature should be invalid for wrong data")
	}
}

func TestRSAOAEP(t *testing.T) {
	subtle := NewSubtleCrypto()

	// Generate key pair
	keyResult, err := subtle.GenerateKey(map[string]any{
		"name":   "RSA-OAEP",
		"length": 2048,
		"hash":   "SHA-256",
	}, true, []KeyUsage{UsageEncrypt, UsageDecrypt})
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	keyPair := keyResult.(*CryptoKeyPair)

	plaintext := []byte("Hello, RSA-OAEP!")

	// Encrypt with public key
	ciphertext, err := subtle.Encrypt(map[string]any{
		"name": "RSA-OAEP",
		"hash": "SHA-256",
	}, keyPair.PublicKey, plaintext)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	// Decrypt with private key
	decrypted, err := subtle.Decrypt(map[string]any{
		"name": "RSA-OAEP",
		"hash": "SHA-256",
	}, keyPair.PrivateKey, ciphertext)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Fatalf("Decrypted data doesn't match: got %q, want %q", decrypted, plaintext)
	}
}

func TestECDSA(t *testing.T) {
	subtle := NewSubtleCrypto()

	// Generate key pair
	keyResult, err := subtle.GenerateKey(map[string]any{
		"name": "ECDSA",
		"hash": "P-256", // namedCurve
	}, true, []KeyUsage{UsageSign, UsageVerify})
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	keyPair := keyResult.(*CryptoKeyPair)
	data := []byte("message to sign with ECDSA")

	// Sign
	signature, err := subtle.Sign(map[string]any{
		"name": "ECDSA",
		"hash": "SHA-256",
	}, keyPair.PrivateKey, data)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// Verify
	valid, err := subtle.Verify(map[string]any{
		"name": "ECDSA",
		"hash": "SHA-256",
	}, keyPair.PublicKey, signature, data)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if !valid {
		t.Fatal("Signature should be valid")
	}
}

func TestImportExportAESKey(t *testing.T) {
	subtle := NewSubtleCrypto()

	// Raw key data
	keyData := make([]byte, 32) // 256-bit key
	for i := range keyData {
		keyData[i] = byte(i)
	}

	// Import
	key, err := subtle.ImportKey("raw", keyData, "AES-CBC", true, []KeyUsage{UsageEncrypt, UsageDecrypt})
	if err != nil {
		t.Fatalf("ImportKey failed: %v", err)
	}

	// Export
	exported, err := subtle.ExportKey("raw", key)
	if err != nil {
		t.Fatalf("ExportKey failed: %v", err)
	}

	exportedBytes := exported.([]byte)
	if !bytes.Equal(exportedBytes, keyData) {
		t.Fatal("Exported key doesn't match original")
	}
}

func TestDeriveKey(t *testing.T) {
	subtle := NewSubtleCrypto()

	// Import password as PBKDF2 base key
	password := []byte("password")
	baseKey, err := subtle.ImportKey("raw", password, "PBKDF2", false, []KeyUsage{UsageDeriveKey, UsageDeriveBits})
	if err != nil {
		t.Fatalf("ImportKey failed: %v", err)
	}

	// Derive AES key
	derivedKey, err := subtle.DeriveKey(
		map[string]any{
			"name":       "PBKDF2",
			"salt":       []byte("salt"),
			"iterations": 10000,
			"hash":       "SHA-256",
		},
		baseKey,
		map[string]any{
			"name":   "AES-CBC",
			"length": 256,
		},
		true,
		[]KeyUsage{UsageEncrypt, UsageDecrypt},
	)
	if err != nil {
		t.Fatalf("DeriveKey failed: %v", err)
	}

	if derivedKey.Algorithm.Name != "AES-CBC" {
		t.Fatalf("Expected AES-CBC algorithm, got %s", derivedKey.Algorithm.Name)
	}

	// Test that the derived key works for encryption
	iv := make([]byte, 16)
	_, err = subtle.Encrypt(map[string]any{
		"name": "AES-CBC",
		"iv":   iv,
	}, derivedKey, []byte("test data"))
	if err != nil {
		t.Fatalf("Encrypt with derived key failed: %v", err)
	}
}

func TestBase64URL(t *testing.T) {
	tests := []struct {
		input    []byte
		expected string
	}{
		{[]byte{}, ""},
		{[]byte{0}, "AA"},
		{[]byte{0, 0}, "AAA"},
		{[]byte{0, 0, 0}, "AAAA"},
		{[]byte{255}, "_w"},
		{[]byte{255, 255}, "__8"},
	}

	for _, tc := range tests {
		encoded := base64URLEncode(tc.input)
		if encoded != tc.expected {
			t.Errorf("base64URLEncode(%v) = %q, want %q", tc.input, encoded, tc.expected)
		}

		decoded, err := base64URLDecode(encoded)
		if err != nil {
			t.Errorf("base64URLDecode(%q) failed: %v", encoded, err)
		}
		if !bytes.Equal(decoded, tc.input) {
			t.Errorf("base64URLDecode(%q) = %v, want %v", encoded, decoded, tc.input)
		}
	}
}
