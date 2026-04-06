package callhook

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func newID(prefix string) (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	value := base64.RawURLEncoding.EncodeToString(raw)
	if prefix == "" {
		return value, nil
	}

	return fmt.Sprintf("%s_%s", prefix, value), nil
}
