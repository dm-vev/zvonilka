package config

import (
	"strings"
	"time"
)

// MeshCoreConfig defines the local MeshCore bridge adapter settings.
type MeshCoreConfig struct {
	InterfaceKind    string
	Device           string
	HelperPython     string
	HelperScriptPath string
	ReceiveTimeout   time.Duration
	TextPrefix       string
	Destination      string
}

// normalize canonicalizes MeshCore adapter settings.
func (c *MeshCoreConfig) normalize() {
	if c == nil {
		return
	}

	c.InterfaceKind = strings.ToLower(strings.TrimSpace(c.InterfaceKind))
	c.Device = strings.TrimSpace(c.Device)
	c.HelperPython = strings.TrimSpace(c.HelperPython)
	c.HelperScriptPath = strings.TrimSpace(c.HelperScriptPath)
	c.TextPrefix = strings.TrimSpace(c.TextPrefix)
	c.Destination = strings.TrimSpace(c.Destination)
}
