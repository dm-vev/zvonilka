package bot

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

func fanoutWorkerName() string {
	return "bot_updates"
}

func newID(prefix string) (string, error) {
	raw := make([]byte, 12)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	token := base64.RawURLEncoding.EncodeToString(raw)
	if prefix == "" {
		return token, nil
	}

	return fmt.Sprintf("%s_%s", prefix, token), nil
}
