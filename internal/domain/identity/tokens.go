package identity

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
)

// newID generates a compact opaque identifier with an optional prefix.
func newID(prefix string) (string, error) {
	token, err := randomToken(12)
	if err != nil {
		return "", err
	}

	if prefix == "" {
		return token, nil
	}

	return fmt.Sprintf("%s_%s", prefix, token), nil
}

// newSecret generates a numeric one-time secret with the requested length.
func newSecret(length int) (string, error) {
	if length <= 0 {
		return "", ErrInvalidInput
	}

	max := big.NewInt(10)
	digits := make([]byte, length)

	for idx := 0; idx < length; idx++ {
		value, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}

		digits[idx] = byte('0' + value.Int64())
	}

	return string(digits), nil
}

// randomToken returns a base64url-encoded random token of the requested size.
func randomToken(size int) (string, error) {
	if size <= 0 {
		return "", ErrInvalidInput
	}

	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}
