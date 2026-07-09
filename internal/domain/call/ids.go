package call

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"
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

func turnCredential(secret string, accountID string, expiresAt time.Time) (string, string, error) {
	secret = strings.TrimSpace(secret)
	accountID = strings.TrimSpace(accountID)
	if secret == "" || accountID == "" || expiresAt.IsZero() {
		return "", "", ErrInvalidInput
	}

	username := strconv.FormatInt(expiresAt.UTC().Unix(), 10) + ":" + accountID
	mac := hmac.New(sha1.New, []byte(secret))
	if _, err := mac.Write([]byte(username)); err != nil {
		return "", "", err
	}

	return username, base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}
