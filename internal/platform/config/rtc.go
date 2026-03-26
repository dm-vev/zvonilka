package config

import (
	"strings"
	"time"

	domaincall "github.com/dm-vev/zvonilka/internal/domain/call"
)

// RTCConfig defines in-server RTC runtime and ICE settings.
type RTCConfig struct {
	PublicEndpoint string
	CredentialTTL  time.Duration
	STUNURLs       []string
	TURNURLs       []string
	TURNSecret     string
}

// ToDomain converts configuration into the call-domain RTC shape.
func (c RTCConfig) ToDomain() domaincall.RTCConfig {
	return domaincall.RTCConfig{
		PublicEndpoint: strings.TrimSpace(c.PublicEndpoint),
		CredentialTTL:  c.CredentialTTL,
		STUNURLs:       append([]string(nil), c.STUNURLs...),
		TURNURLs:       append([]string(nil), c.TURNURLs...),
		TURNSecret:     strings.TrimSpace(c.TURNSecret),
	}
}
