package config

import (
	"strings"
	"time"
)

// MeshtasticConfig defines the local Meshtastic bridge adapter settings.
type MeshtasticConfig struct {
	InterfaceKind    string
	Device           string
	HelperPython     string
	HelperScriptPath string
	ReceiveTimeout   time.Duration
	TextPrefix       string
}

// normalize canonicalizes Meshtastic adapter settings.
func (c *MeshtasticConfig) normalize() {
	if c == nil {
		return
	}

	c.InterfaceKind = strings.ToLower(strings.TrimSpace(c.InterfaceKind))
	c.Device = strings.TrimSpace(c.Device)
	c.HelperPython = strings.TrimSpace(c.HelperPython)
	c.HelperScriptPath = strings.TrimSpace(c.HelperScriptPath)
	c.TextPrefix = strings.TrimSpace(c.TextPrefix)
}
