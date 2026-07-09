package federation

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

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

func secretHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func secretFingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:8])
}
