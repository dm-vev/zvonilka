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
	NodeID         string
	CandidateHost  string
	UDPPortMin     int
	UDPPortMax     int
	STUNURLs       []string
	TURNURLs       []string
	TURNSecret     string
	Nodes          []RTCNodeConfig
}

// RTCNodeConfig defines one logical media-plane node.
type RTCNodeConfig struct {
	ID              string
	Endpoint        string
	ControlEndpoint string
}

// ToDomain converts configuration into the call-domain RTC shape.
func (c RTCConfig) ToDomain() domaincall.RTCConfig {
	return domaincall.RTCConfig{
		PublicEndpoint: strings.TrimSpace(c.PublicEndpoint),
		CredentialTTL:  c.CredentialTTL,
		NodeID:         strings.TrimSpace(c.NodeID),
		CandidateHost:  strings.TrimSpace(c.CandidateHost),
		UDPPortMin:     c.UDPPortMin,
		UDPPortMax:     c.UDPPortMax,
		STUNURLs:       append([]string(nil), c.STUNURLs...),
		TURNURLs:       append([]string(nil), c.TURNURLs...),
		TURNSecret:     strings.TrimSpace(c.TURNSecret),
		Nodes:          toDomainRTCNodes(c.Nodes),
	}
}

func toDomainRTCNodes(values []RTCNodeConfig) []domaincall.RTCNode {
	if len(values) == 0 {
		return nil
	}

	result := make([]domaincall.RTCNode, 0, len(values))
	for _, value := range values {
		result = append(result, domaincall.RTCNode{
			ID:              strings.TrimSpace(value.ID),
			Endpoint:        strings.TrimSpace(value.Endpoint),
			ControlEndpoint: strings.TrimSpace(value.ControlEndpoint),
		})
	}

	return result
}
