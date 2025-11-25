package webcrypto

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
)

// digest computes a cryptographic hash of the data.
func digest(algorithm string, data []byte) ([]byte, error) {
	switch algorithm {
	case "SHA-1":
		h := sha1.Sum(data)
		return h[:], nil
	case "SHA-256":
		h := sha256.Sum256(data)
		return h[:], nil
	case "SHA-384":
		h := sha512.Sum384(data)
		return h[:], nil
	case "SHA-512":
		h := sha512.Sum512(data)
		return h[:], nil
	default:
		return nil, fmt.Errorf("unsupported digest algorithm: %s", algorithm)
	}
}
