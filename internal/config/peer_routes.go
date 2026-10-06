package config

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"

	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/state"
)

type PeerRoute struct {
	Folder         string `json:"folder"`
	Device         string `json:"device"`
	Pin            string `json:"pin"`
	Profile        string `json:"profile"`
	CertificateDER string `json:"certificate_der"`
}

func validateDurableRoute(r PeerRoute) error {
	for _, v := range []string{r.Folder, r.Device, r.Pin, r.Profile} {
		if err := protocol.NetworkHex(v, 32); err != nil {
			return err
		}
	}
	der, err := base64.StdEncoding.Strict().DecodeString(r.CertificateDER)
	if err != nil {
		return err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return err
	}
	_, _, err = protocol.NetworkCertificate(r.CertificateDER, r.Pin, uint64(cert.NotBefore.Unix()))
	return err
}
func LoadPeerRoutes(dir string) ([]PeerRoute, error) {
	b, err := state.ReadPrivate(dir, "peer-routes.json", 1<<20)
	if errors.Is(err, os.ErrNotExist) {
		return []PeerRoute{}, nil
	}
	if err != nil {
		return nil, err
	}
	var doc struct {
		Version string      `json:"version"`
		Routes  []PeerRoute `json:"routes"`
	}
	if err = protocol.NetworkDecode(b, &doc); err != nil {
		return nil, err
	}
	if doc.Version != "1" || len(doc.Routes) > MaxPeerEndpoints {
		return nil, errors.New("INVALID_REQUEST")
	}
	seen := map[string]bool{}
	pins := map[string]string{}
	for _, r := range doc.Routes {
		// NetworkCertificate enforces validity at the supplied time; use DER's
		// actual NotBefore for shape checks so expired intent remains reloadable.
		if err = validateDurableRoute(r); err != nil {
			return nil, err
		}
		key := r.Folder + "/" + r.Device
		if seen[key] || (pins[r.Device] != "" && pins[r.Device] != r.Pin) {
			return nil, errors.New("IDENTITY_MISMATCH")
		}
		seen[key] = true
		pins[r.Device] = r.Pin
	}
	return doc.Routes, nil
}
func SetPeerRoute(dir string, route PeerRoute) error {
	if err := validateDurableRoute(route); err != nil {
		return err
	}
	routes, err := LoadPeerRoutes(dir)
	if err != nil {
		return err
	}
	replaced := false
	for i, r := range routes {
		if r.Device == route.Device && r.Pin != route.Pin {
			return errors.New("IDENTITY_MISMATCH")
		}
		if r.Folder == route.Folder && r.Device == route.Device {
			if r.Profile != route.Profile {
				return errors.New("PROFILE_MISMATCH")
			}
			if r == route {
				return nil
			}
			routes[i] = route
			replaced = true
		}
	}
	if !replaced {
		if len(routes) >= MaxPeerEndpoints {
			return errors.New("NETWORK_BUSY")
		}
		routes = append(routes, route)
	}
	b, err := json.Marshal(struct {
		Version string      `json:"version"`
		Routes  []PeerRoute `json:"routes"`
	}{"1", routes})
	if err != nil {
		return err
	}
	return WritePrivate(dir, "peer-routes.json", b)
}

// RebindPeerRoutes moves durable routes to the active reviewed profile digest.
// Profile review only advances an epoch under the same authority/environment,
// so a rotation must not strand existing pairings on the superseded digest.
// Device IDs, pins and certificates are unchanged. Callers hold exclusive state.
func RebindPeerRoutes(dir, profile string) error {
	if err := protocol.NetworkHex(profile, 32); err != nil {
		return err
	}
	routes, err := LoadPeerRoutes(dir)
	if err != nil {
		return err
	}
	changed := false
	for i := range routes {
		if routes[i].Profile != profile {
			routes[i].Profile = profile
			changed = true
		}
	}
	if !changed {
		return nil
	}
	b, err := json.Marshal(struct {
		Version string      `json:"version"`
		Routes  []PeerRoute `json:"routes"`
	}{"1", routes})
	if err != nil {
		return err
	}
	return WritePrivate(dir, "peer-routes.json", b)
}
