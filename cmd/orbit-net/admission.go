package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/calebhabesh/orbit/internal/rendezvous"
)

// RG1 admission configuration. "open" (the default) serves every device;
// "allowlist" serves the devices in allowed_devices or allowed_devices_file
// and lets others only pair toward them. SIGHUP re-reads both.

const maxAllowlistBytes = 64 << 10

// allowedDevice is one allowlist file entry; Label is the operator's note.
type allowedDevice struct {
	Device string `json:"device"`
	Pin    string `json:"pin"`
	Label  string `json:"label,omitempty"`
}

type allowlistFile struct {
	Version string          `json:"version"`
	Devices []allowedDevice `json:"devices"`
}

// loadAdmission builds the policy the configuration names.
func loadAdmission(cfg serveConfig) (*rendezvous.Admission, error) {
	switch cfg.Admission {
	case "", "open":
		if len(cfg.AllowedDevices) > 0 || cfg.AllowedDevicesFile != "" {
			return nil, errors.New(`allowed_devices and allowed_devices_file need "admission": "allowlist"`)
		}
		return rendezvous.OpenAdmission(), nil
	case "allowlist":
	default:
		return nil, errors.New(`admission must be "open" or "allowlist"`)
	}
	if (len(cfg.AllowedDevices) > 0) == (cfg.AllowedDevicesFile != "") {
		return nil, errors.New("allowlist admission needs exactly one of allowed_devices or allowed_devices_file")
	}
	devices := cfg.AllowedDevices
	if cfg.AllowedDevicesFile != "" {
		f, err := readAllowlist(cfg.AllowedDevicesFile)
		if err != nil {
			return nil, err
		}
		devices = f.Devices
	}
	list := make([]rendezvous.AllowedDevice, 0, len(devices))
	for _, d := range devices {
		if len(d.Label) > 64 || strings.ContainsAny(d.Label, "\n\r\t") {
			return nil, errors.New("allowlist labels are at most 64 characters on one line")
		}
		list = append(list, rendezvous.AllowedDevice{Device: d.Device, Pin: d.Pin})
	}
	a, err := rendezvous.NewAllowlist(list)
	if err != nil {
		return nil, fmt.Errorf("allowlist: %w", err)
	}
	return a, nil
}

func readAllowlist(path string) (allowlistFile, error) {
	var f allowlistFile
	data, err := readPrivatePath(path, maxAllowlistBytes)
	if err != nil {
		return f, fmt.Errorf("read allowed_devices_file: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&f); err != nil {
		return f, fmt.Errorf("decode allowed_devices_file: %w", err)
	}
	if f.Version != "1" {
		return f, errors.New(`allowed_devices_file needs "version": "1"`)
	}
	return f, nil
}

// writeAllowlist replaces the file atomically with owner-only permissions.
func writeAllowlist(path string, f allowlistFile) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".allowlist-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(append(data, '\n')); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

var (
	identityDevice = regexp.MustCompile(`(?:^|\s)device=([0-9a-f]{64})(?:\s|$)`)
	identityPin    = regexp.MustCompile(`(?:^|\s)pin=([0-9a-f]{64})(?:\s|$)`)
	bareHex        = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// parseIdentity accepts the identity line `orbit devices` and `orbit status`
// print ("... device=<64 hex> pin=<64 hex> ...") or two bare 64-hex values.
func parseIdentity(text string) (allowedDevice, error) {
	text = strings.ToLower(strings.TrimSpace(text))
	d, p := identityDevice.FindStringSubmatch(text), identityPin.FindStringSubmatch(text)
	if d != nil && p != nil {
		return allowedDevice{Device: d[1], Pin: p[1]}, nil
	}
	if fields := strings.Fields(text); len(fields) == 2 && bareHex.MatchString(fields[0]) && bareHex.MatchString(fields[1]) {
		return allowedDevice{Device: fields[0], Pin: fields[1]}, nil
	}
	return allowedDevice{}, errors.New(`give the device's identity line ("device=<64 hex> pin=<64 hex>", as printed by orbit devices or orbit status) or "<device> <pin>"`)
}

// devices edits the allowlist file a serve configuration names.
func devices(args []string, out, errOut io.Writer) error {
	if len(args) == 0 {
		return errors.New("devices requires list, add or remove")
	}
	action := args[0]
	flags := flag.NewFlagSet("orbit-net devices "+action, flag.ContinueOnError)
	flags.SetOutput(errOut)
	configFile := flags.String("config", "", "serve configuration naming allowed_devices_file")
	label := flags.String("label", "", "add: the operator's note for this device (at most 64 characters)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if *configFile == "" {
		return errors.New("devices requires --config")
	}
	data, err := readPrivatePath(*configFile, maxConfigBytes)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var cfg serveConfig
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&cfg); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	if cfg.AllowedDevicesFile == "" {
		return errors.New("the configuration has no allowed_devices_file; set it (and \"admission\": \"allowlist\") to manage devices here")
	}
	path := cfg.AllowedDevicesFile
	f, err := readAllowlist(path)
	if errors.Is(err, os.ErrNotExist) && action == "add" {
		f, err = allowlistFile{Version: "1", Devices: []allowedDevice{}}, nil
	}
	if err != nil {
		return err
	}
	switch action {
	case "list":
		if flags.NArg() != 0 {
			return errors.New("devices list accepts no arguments")
		}
		fmt.Fprintf(out, "%d allowed device(s) in %s\n", len(f.Devices), path)
		for _, d := range f.Devices {
			fmt.Fprintf(out, "device=%s pin=%s %s\n", d.Device, d.Pin, d.Label)
		}
		return nil
	case "add":
		d, err := parseIdentity(strings.Join(flags.Args(), " "))
		if err != nil {
			return err
		}
		d.Label = *label
		for _, old := range f.Devices {
			if old.Device == d.Device {
				return fmt.Errorf("device %s is already listed; remove it first to change its pin", d.Device)
			}
		}
		f.Devices = append(f.Devices, d)
	case "remove":
		if flags.NArg() != 1 || !bareHex.MatchString(strings.ToLower(flags.Arg(0))) {
			return errors.New("devices remove needs one 64-hex device ID")
		}
		id, kept := strings.ToLower(flags.Arg(0)), f.Devices[:0]
		for _, d := range f.Devices {
			if d.Device != id {
				kept = append(kept, d)
			}
		}
		if len(kept) == len(f.Devices) {
			return fmt.Errorf("device %s is not listed", id)
		}
		f.Devices = kept
	default:
		return errors.New("devices requires list, add or remove")
	}
	// Validate the result as serve would before replacing the file.
	if len(f.Devices) > 0 {
		list := make([]rendezvous.AllowedDevice, 0, len(f.Devices))
		for _, d := range f.Devices {
			list = append(list, rendezvous.AllowedDevice{Device: d.Device, Pin: d.Pin})
		}
		if _, err = rendezvous.NewAllowlist(list); err != nil {
			return fmt.Errorf("allowlist: %w", err)
		}
	} else if cfg.Admission == "allowlist" {
		return errors.New("refusing to remove the last allowed device; switch admission to open first")
	}
	if err = writeAllowlist(path, f); err != nil {
		return err
	}
	fmt.Fprintf(out, "%d allowed device(s) in %s; reload the service (systemctl reload orbit-net) to apply\n", len(f.Devices), path)
	return nil
}
