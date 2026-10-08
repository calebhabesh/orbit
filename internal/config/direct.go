package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"os"

	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/state"
)

// LoadDirectSettings reads advanced private local settings, never remote metadata.
func LoadDirectSettings(dir string) (network.DirectSettings, error) {
	var settings network.DirectSettings
	b, err := state.ReadPrivate(dir, "direct-network.json", 4096)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return settings, err
	}
	if err = protocol.DecodeStrict(b, &settings); err != nil {
		return settings, err
	}
	// Advanced local settings retain the W08 required fields; W09 UDP fields
	// are additive optional fields, unlike all-required signed network envelopes.
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(b, &fields); err != nil {
		return settings, err
	}
	for _, key := range []string{"interfaces", "listen", "disabled"} {
		if _, ok := fields[key]; !ok {
			return settings, errors.New("INVALID_ENCODING")
		}
	}
	for _, value := range fields {
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return settings, errors.New("INVALID_ENCODING")
		}
	}
	if len(settings.Interfaces) > network.MaxDiscoveryInterfaces {
		return settings, errors.New("INVALID_INTERFACE")
	}
	seen := map[string]bool{}
	for _, name := range settings.Interfaces {
		if name == "" || len(name) > 64 || seen[name] {
			return settings, errors.New("INVALID_INTERFACE")
		}
		seen[name] = true
	}
	for _, address := range []string{settings.Listen, settings.UDPListen} {
		if address == "" {
			continue
		}
		host, port, err := net.SplitHostPort(address)
		if err != nil || port == "" || (host != "" && net.ParseIP(host) == nil) {
			return settings, errors.New("INVALID_DIRECT_LISTENER")
		}
	}
	return settings, nil
}
