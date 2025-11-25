package webcrypto

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"errors"
	"fmt"
	"hash"
	"math/big"
)

// getHashForRSA returns the hash function and crypto.Hash for RSA operations.
func getHashForRSA(hashAlg string) (hash.Hash, crypto.Hash, error) {
	switch hashAlg {
	case "SHA-1":
		return sha1.New(), crypto.SHA1, nil
	case "SHA-256":
		return sha256.New(), crypto.SHA256, nil
	case "SHA-384":
		return sha512.New384(), crypto.SHA384, nil
	case "SHA-512":
		return sha512.New(), crypto.SHA512, nil
	default:
		return nil, 0, fmt.Errorf("unsupported hash algorithm: %s", hashAlg)
	}
}

// rsaOAEPEncrypt encrypts data using RSA-OAEP.
func rsaOAEPEncrypt(key *CryptoKey, hashAlg string, label, plaintext []byte) ([]byte, error) {
	pubKey, err := key.RSAPublicKey()
	if err != nil {
		return nil, err
	}

	h, _, err := getHashForRSA(hashAlg)
	if err != nil {
		return nil, err
	}

	return rsa.EncryptOAEP(h, rand.Reader, pubKey, plaintext, label)
}

// rsaOAEPDecrypt decrypts data using RSA-OAEP.
func rsaOAEPDecrypt(key *CryptoKey, hashAlg string, label, ciphertext []byte) ([]byte, error) {
	privKey, err := key.RSAPrivateKey()
	if err != nil {
		return nil, err
	}

	h, _, err := getHashForRSA(hashAlg)
	if err != nil {
		return nil, err
	}

	return rsa.DecryptOAEP(h, rand.Reader, privKey, ciphertext, label)
}

// rsassaPKCS1Sign signs data using RSASSA-PKCS1-v1_5.
func rsassaPKCS1Sign(key *CryptoKey, hashAlg string, data []byte) ([]byte, error) {
	privKey, err := key.RSAPrivateKey()
	if err != nil {
		return nil, err
	}

	h, cryptoHash, err := getHashForRSA(hashAlg)
	if err != nil {
		return nil, err
	}

	h.Write(data)
	hashed := h.Sum(nil)

	return rsa.SignPKCS1v15(rand.Reader, privKey, cryptoHash, hashed)
}

// rsassaPKCS1Verify verifies a signature using RSASSA-PKCS1-v1_5.
func rsassaPKCS1Verify(key *CryptoKey, hashAlg string, signature, data []byte) (bool, error) {
	pubKey, err := key.RSAPublicKey()
	if err != nil {
		return false, err
	}

	h, cryptoHash, err := getHashForRSA(hashAlg)
	if err != nil {
		return false, err
	}

	h.Write(data)
	hashed := h.Sum(nil)

	err = rsa.VerifyPKCS1v15(pubKey, cryptoHash, hashed, signature)
	return err == nil, nil
}

// rsaPSSSign signs data using RSA-PSS.
func rsaPSSSign(key *CryptoKey, hashAlg string, data []byte) ([]byte, error) {
	privKey, err := key.RSAPrivateKey()
	if err != nil {
		return nil, err
	}

	h, cryptoHash, err := getHashForRSA(hashAlg)
	if err != nil {
		return nil, err
	}

	h.Write(data)
	hashed := h.Sum(nil)

	return rsa.SignPSS(rand.Reader, privKey, cryptoHash, hashed, nil)
}

// rsaPSSVerify verifies a signature using RSA-PSS.
func rsaPSSVerify(key *CryptoKey, hashAlg string, signature, data []byte) (bool, error) {
	pubKey, err := key.RSAPublicKey()
	if err != nil {
		return false, err
	}

	h, cryptoHash, err := getHashForRSA(hashAlg)
	if err != nil {
		return false, err
	}

	h.Write(data)
	hashed := h.Sum(nil)

	err = rsa.VerifyPSS(pubKey, cryptoHash, hashed, signature, nil)
	return err == nil, nil
}

// generateRSAKeyPair generates an RSA key pair.
func generateRSAKeyPair(algorithm string, modulusLength int, hashAlg string, extractable bool, usages []KeyUsage) (*CryptoKeyPair, error) {
	if modulusLength == 0 {
		modulusLength = 2048
	}

	privKey, err := rsa.GenerateKey(rand.Reader, modulusLength)
	if err != nil {
		return nil, err
	}

	// Split usages between public and private key
	var pubUsages, privUsages []KeyUsage
	for _, u := range usages {
		switch u {
		case UsageEncrypt, UsageVerify, UsageWrapKey:
			pubUsages = append(pubUsages, u)
		case UsageDecrypt, UsageSign, UsageUnwrapKey:
			privUsages = append(privUsages, u)
		}
	}

	return &CryptoKeyPair{
		PublicKey:  NewPublicKey(Algorithm{Name: algorithm}, true, pubUsages, &privKey.PublicKey),
		PrivateKey: NewPrivateKey(Algorithm{Name: algorithm}, extractable, privUsages, privKey),
	}, nil
}

// importRSAKey imports an RSA key from various formats.
func importRSAKey(format string, keyData any, algorithm, hashAlg string, extractable bool, usages []KeyUsage) (*CryptoKey, error) {
	switch format {
	case "spki":
		// Import public key from SubjectPublicKeyInfo format
		data, ok := keyData.([]byte)
		if !ok {
			return nil, errors.New("spki key data must be []byte")
		}
		pub, err := x509.ParsePKIXPublicKey(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SPKI: %w", err)
		}
		rsaPub, ok := pub.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("not an RSA public key")
		}
		return NewPublicKey(Algorithm{Name: algorithm}, extractable, usages, rsaPub), nil

	case "pkcs8":
		// Import private key from PKCS#8 format
		data, ok := keyData.([]byte)
		if !ok {
			return nil, errors.New("pkcs8 key data must be []byte")
		}
		priv, err := x509.ParsePKCS8PrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS8: %w", err)
		}
		rsaPriv, ok := priv.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an RSA private key")
		}
		return NewPrivateKey(Algorithm{Name: algorithm}, extractable, usages, rsaPriv), nil

	case "jwk":
		jwk, ok := keyData.(map[string]any)
		if !ok {
			return nil, errors.New("jwk must be an object")
		}
		return importRSAFromJWK(jwk, algorithm, extractable, usages)

	default:
		return nil, fmt.Errorf("unsupported key format: %s", format)
	}
}

// importRSAFromJWK imports an RSA key from JWK format.
func importRSAFromJWK(jwk map[string]any, algorithm string, extractable bool, usages []KeyUsage) (*CryptoKey, error) {
	kty, _ := jwk["kty"].(string)
	if kty != "RSA" {
		return nil, errors.New("jwk kty must be 'RSA'")
	}

	// Decode modulus (n) and exponent (e)
	nStr, _ := jwk["n"].(string)
	eStr, _ := jwk["e"].(string)

	n, err := base64URLDecode(nStr)
	if err != nil {
		return nil, fmt.Errorf("invalid modulus: %w", err)
	}
	e, err := base64URLDecode(eStr)
	if err != nil {
		return nil, fmt.Errorf("invalid exponent: %w", err)
	}

	pubKey := &rsa.PublicKey{
		N: new(big.Int).SetBytes(n),
		E: int(new(big.Int).SetBytes(e).Int64()),
	}

	// Check if private key components are present
	dStr, hasD := jwk["d"].(string)
	if !hasD {
		return NewPublicKey(Algorithm{Name: algorithm}, extractable, usages, pubKey), nil
	}

	// Import private key
	d, err := base64URLDecode(dStr)
	if err != nil {
		return nil, fmt.Errorf("invalid private exponent: %w", err)
	}

	privKey := &rsa.PrivateKey{
		PublicKey: *pubKey,
		D:         new(big.Int).SetBytes(d),
	}

	// Optional: import primes if present
	if pStr, ok := jwk["p"].(string); ok {
		if p, err := base64URLDecode(pStr); err == nil {
			privKey.Primes = append(privKey.Primes, new(big.Int).SetBytes(p))
		}
	}
	if qStr, ok := jwk["q"].(string); ok {
		if q, err := base64URLDecode(qStr); err == nil {
			privKey.Primes = append(privKey.Primes, new(big.Int).SetBytes(q))
		}
	}

	if err := privKey.Validate(); err != nil {
		// Try to precompute if validation fails
		privKey.Precompute()
	}

	return NewPrivateKey(Algorithm{Name: algorithm}, extractable, usages, privKey), nil
}

// exportRSAKey exports an RSA key to the specified format.
func exportRSAKey(format string, key *CryptoKey) (any, error) {
	switch format {
	case "spki":
		pubKey, err := key.RSAPublicKey()
		if err != nil {
			return nil, err
		}
		return x509.MarshalPKIXPublicKey(pubKey)

	case "pkcs8":
		privKey, err := key.RSAPrivateKey()
		if err != nil {
			return nil, err
		}
		return x509.MarshalPKCS8PrivateKey(privKey)

	case "jwk":
		return exportRSAToJWK(key)

	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// exportRSAToJWK exports an RSA key to JWK format.
func exportRSAToJWK(key *CryptoKey) (map[string]any, error) {
	jwk := map[string]any{
		"kty":     "RSA",
		"key_ops": usagesToStrings(key.Usages),
		"ext":     key.Extractable,
	}

	if key.Type == KeyTypePrivate {
		privKey, err := key.RSAPrivateKey()
		if err != nil {
			return nil, err
		}
		jwk["n"] = base64URLEncode(privKey.N.Bytes())
		jwk["e"] = base64URLEncode(big.NewInt(int64(privKey.E)).Bytes())
		jwk["d"] = base64URLEncode(privKey.D.Bytes())
		if len(privKey.Primes) >= 2 {
			jwk["p"] = base64URLEncode(privKey.Primes[0].Bytes())
			jwk["q"] = base64URLEncode(privKey.Primes[1].Bytes())
		}
	} else {
		pubKey, err := key.RSAPublicKey()
		if err != nil {
			return nil, err
		}
		jwk["n"] = base64URLEncode(pubKey.N.Bytes())
		jwk["e"] = base64URLEncode(big.NewInt(int64(pubKey.E)).Bytes())
	}

	return jwk, nil
}
