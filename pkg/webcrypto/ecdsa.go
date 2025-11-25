package webcrypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"errors"
	"fmt"
	"math/big"
)

// getCurve returns the elliptic curve for the given name.
func getCurve(name string) (elliptic.Curve, error) {
	switch name {
	case "P-256":
		return elliptic.P256(), nil
	case "P-384":
		return elliptic.P384(), nil
	case "P-521":
		return elliptic.P521(), nil
	default:
		return nil, fmt.Errorf("unsupported curve: %s", name)
	}
}

// ecdsaSign signs data using ECDSA.
func ecdsaSign(key *CryptoKey, hashAlg string, data []byte) ([]byte, error) {
	privKey, err := key.ECDSAPrivateKey()
	if err != nil {
		return nil, err
	}

	h, _, err := getHashForRSA(hashAlg) // Reuse hash helper
	if err != nil {
		return nil, err
	}

	h.Write(data)
	hashed := h.Sum(nil)

	r, s, err := ecdsa.Sign(rand.Reader, privKey, hashed)
	if err != nil {
		return nil, err
	}

	// Encode r and s as IEEE P1363 format (fixed-size concatenation)
	curveBytes := (privKey.Curve.Params().BitSize + 7) / 8
	signature := make([]byte, 2*curveBytes)
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	copy(signature[curveBytes-len(rBytes):curveBytes], rBytes)
	copy(signature[2*curveBytes-len(sBytes):], sBytes)

	return signature, nil
}

// ecdsaVerify verifies an ECDSA signature.
func ecdsaVerify(key *CryptoKey, hashAlg string, signature, data []byte) (bool, error) {
	pubKey, err := key.ECDSAPublicKey()
	if err != nil {
		return false, err
	}

	h, _, err := getHashForRSA(hashAlg)
	if err != nil {
		return false, err
	}

	h.Write(data)
	hashed := h.Sum(nil)

	// Decode IEEE P1363 format signature
	curveBytes := (pubKey.Curve.Params().BitSize + 7) / 8
	if len(signature) != 2*curveBytes {
		return false, errors.New("invalid signature length")
	}

	r := new(big.Int).SetBytes(signature[:curveBytes])
	s := new(big.Int).SetBytes(signature[curveBytes:])

	return ecdsa.Verify(pubKey, hashed, r, s), nil
}

// generateECKeyPair generates an ECDSA/ECDH key pair.
func generateECKeyPair(algorithm, namedCurve string, extractable bool, usages []KeyUsage) (*CryptoKeyPair, error) {
	curve, err := getCurve(namedCurve)
	if err != nil {
		return nil, err
	}

	privKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		return nil, err
	}

	// Split usages between public and private key
	var pubUsages, privUsages []KeyUsage
	for _, u := range usages {
		switch u {
		case UsageVerify:
			pubUsages = append(pubUsages, u)
		case UsageSign:
			privUsages = append(privUsages, u)
		}
	}

	return &CryptoKeyPair{
		PublicKey:  NewPublicKey(Algorithm{Name: algorithm}, true, pubUsages, &privKey.PublicKey),
		PrivateKey: NewPrivateKey(Algorithm{Name: algorithm}, extractable, privUsages, privKey),
	}, nil
}

// importECKey imports an EC key from various formats.
func importECKey(format string, keyData any, algorithm, namedCurve string, extractable bool, usages []KeyUsage) (*CryptoKey, error) {
	switch format {
	case "spki":
		data, ok := keyData.([]byte)
		if !ok {
			return nil, errors.New("spki key data must be []byte")
		}
		pub, err := x509.ParsePKIXPublicKey(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse SPKI: %w", err)
		}
		ecPub, ok := pub.(*ecdsa.PublicKey)
		if !ok {
			return nil, errors.New("not an ECDSA public key")
		}
		return NewPublicKey(Algorithm{Name: algorithm}, extractable, usages, ecPub), nil

	case "pkcs8":
		data, ok := keyData.([]byte)
		if !ok {
			return nil, errors.New("pkcs8 key data must be []byte")
		}
		priv, err := x509.ParsePKCS8PrivateKey(data)
		if err != nil {
			return nil, fmt.Errorf("failed to parse PKCS8: %w", err)
		}
		ecPriv, ok := priv.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("not an ECDSA private key")
		}
		return NewPrivateKey(Algorithm{Name: algorithm}, extractable, usages, ecPriv), nil

	case "jwk":
		jwk, ok := keyData.(map[string]any)
		if !ok {
			return nil, errors.New("jwk must be an object")
		}
		return importECFromJWK(jwk, algorithm, extractable, usages)

	case "raw":
		// Raw format for public keys only (uncompressed point)
		data, ok := keyData.([]byte)
		if !ok {
			return nil, errors.New("raw key data must be []byte")
		}
		curve, err := getCurve(namedCurve)
		if err != nil {
			return nil, err
		}
		x, y := elliptic.Unmarshal(curve, data)
		if x == nil {
			return nil, errors.New("invalid raw EC public key")
		}
		pubKey := &ecdsa.PublicKey{Curve: curve, X: x, Y: y}
		return NewPublicKey(Algorithm{Name: algorithm}, extractable, usages, pubKey), nil

	default:
		return nil, fmt.Errorf("unsupported key format: %s", format)
	}
}

// importECFromJWK imports an EC key from JWK format.
func importECFromJWK(jwk map[string]any, algorithm string, extractable bool, usages []KeyUsage) (*CryptoKey, error) {
	kty, _ := jwk["kty"].(string)
	if kty != "EC" {
		return nil, errors.New("jwk kty must be 'EC'")
	}

	crvName, _ := jwk["crv"].(string)
	curve, err := getCurve(crvName)
	if err != nil {
		return nil, err
	}

	xStr, _ := jwk["x"].(string)
	yStr, _ := jwk["y"].(string)

	x, err := base64URLDecode(xStr)
	if err != nil {
		return nil, fmt.Errorf("invalid x coordinate: %w", err)
	}
	y, err := base64URLDecode(yStr)
	if err != nil {
		return nil, fmt.Errorf("invalid y coordinate: %w", err)
	}

	pubKey := &ecdsa.PublicKey{
		Curve: curve,
		X:     new(big.Int).SetBytes(x),
		Y:     new(big.Int).SetBytes(y),
	}

	// Check if private key component is present
	dStr, hasD := jwk["d"].(string)
	if !hasD {
		return NewPublicKey(Algorithm{Name: algorithm}, extractable, usages, pubKey), nil
	}

	d, err := base64URLDecode(dStr)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	privKey := &ecdsa.PrivateKey{
		PublicKey: *pubKey,
		D:         new(big.Int).SetBytes(d),
	}

	return NewPrivateKey(Algorithm{Name: algorithm}, extractable, usages, privKey), nil
}

// exportECKey exports an EC key to the specified format.
func exportECKey(format string, key *CryptoKey) (any, error) {
	switch format {
	case "spki":
		pubKey, err := key.ECDSAPublicKey()
		if err != nil {
			return nil, err
		}
		return x509.MarshalPKIXPublicKey(pubKey)

	case "pkcs8":
		privKey, err := key.ECDSAPrivateKey()
		if err != nil {
			return nil, err
		}
		return x509.MarshalPKCS8PrivateKey(privKey)

	case "jwk":
		return exportECToJWK(key)

	case "raw":
		pubKey, err := key.ECDSAPublicKey()
		if err != nil {
			return nil, err
		}
		return elliptic.Marshal(pubKey.Curve, pubKey.X, pubKey.Y), nil

	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// exportECToJWK exports an EC key to JWK format.
func exportECToJWK(key *CryptoKey) (map[string]any, error) {
	var pubKey *ecdsa.PublicKey
	var privKey *ecdsa.PrivateKey

	if key.Type == KeyTypePrivate {
		var err error
		privKey, err = key.ECDSAPrivateKey()
		if err != nil {
			return nil, err
		}
		pubKey = &privKey.PublicKey
	} else {
		var err error
		pubKey, err = key.ECDSAPublicKey()
		if err != nil {
			return nil, err
		}
	}

	// Determine curve name
	var crvName string
	switch pubKey.Curve {
	case elliptic.P256():
		crvName = "P-256"
	case elliptic.P384():
		crvName = "P-384"
	case elliptic.P521():
		crvName = "P-521"
	default:
		return nil, errors.New("unsupported curve")
	}

	curveBytes := (pubKey.Curve.Params().BitSize + 7) / 8

	jwk := map[string]any{
		"kty":     "EC",
		"crv":     crvName,
		"x":       base64URLEncode(padBytes(pubKey.X.Bytes(), curveBytes)),
		"y":       base64URLEncode(padBytes(pubKey.Y.Bytes(), curveBytes)),
		"key_ops": usagesToStrings(key.Usages),
		"ext":     key.Extractable,
	}

	if privKey != nil {
		jwk["d"] = base64URLEncode(padBytes(privKey.D.Bytes(), curveBytes))
	}

	return jwk, nil
}

// padBytes pads bytes to the specified length.
func padBytes(b []byte, length int) []byte {
	if len(b) >= length {
		return b
	}
	padded := make([]byte, length)
	copy(padded[length-len(b):], b)
	return padded
}
