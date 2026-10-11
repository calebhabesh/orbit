package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
)

// Personal budgets written by init: finite and small enough for a home or
// free-tier host. Operators raise them deliberately.
const (
	initRelayBPS       = 4 << 20
	initRelayDeviceBPS = 2 << 20
	initSessionBytes   = 16 << 30
	initMonthBytes     = 200 << 30
)

const initPrivacy = "Self-hosted Orbit service for the operator's own devices. It sees device addresses and connection metadata, kept in memory for at most ten minutes; relayed file data stays end-to-end encrypted and is never stored."

// initService creates a complete self-hosted service in one owner-only
// directory: offline authority key (in offline/, to move off the host), service
// key, signed self_hosted profile, TLS pair (self-signed unless supplied), the
// CA file devices review, and serve.json with personal budgets and allowlist
// admission with member sponsorship. It prints the device and allowlist steps.
func initService(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("orbit-net init", flag.ContinueOnError)
	flags.SetOutput(errOut)
	dir := flags.String("dir", "", "new owner-only directory to create (must not exist)")
	origin := flags.String("origin", "", "https://<host-or-ip>[:port] devices will use")
	stun := flags.String("stun", "", "optional public numeric STUN address IP:port served by this host")
	stunBind := flags.String("stun-bind", "", "local UDP address for --stun behind 1:1 NAT")
	listen := flags.String("listen", "", "TLS listen address (default: all addresses on the origin's port)")
	operator := flags.String("operator", "Self-hosted Orbit service", "operator name devices review")
	privacy := flags.String("privacy", initPrivacy, "privacy statement devices review")
	validFor := flags.Duration("valid-for", 365*24*time.Hour, "profile (and generated certificate) validity")
	certFile := flags.String("tls-cert", "", "existing certificate chain PEM (default: generate a self-signed certificate)")
	keyFile := flags.String("tls-key", "", "existing private TLS key PEM, with --tls-cert")
	configDir := flags.String("config-dir", "", "directory serve.json will live in on the host (default: --dir, run in place)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *dir == "" || *origin == "" {
		return errors.New("init requires --dir and --origin")
	}
	if (*certFile == "") != (*keyFile == "") {
		return errors.New("--tls-cert and --tls-key go together")
	}
	if *validFor < 24*time.Hour || *validFor > 3*365*24*time.Hour {
		return errors.New("--valid-for must be between 24h and three years")
	}
	u, err := url.Parse(*origin)
	if err != nil || u.Scheme != "https" || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.User != nil {
		return errors.New("--origin must look like https://host[:port]")
	}
	host, port := u.Hostname(), u.Port()
	if port == "" || port == "443" {
		port = "443"
		u.Host = host
		if strings.Contains(host, ":") {
			u.Host = "[" + host + "]"
		}
	}
	httpsOrigin := "https://" + u.Host
	if *listen == "" {
		*listen = ":" + port
	}
	var stunList []string
	if *stun != "" {
		if a, e := netip.ParseAddrPort(*stun); e != nil || a.Port() == 0 {
			return errors.New("--stun must be a numeric IP:port")
		}
		stunList = []string{*stun}
	} else if *stunBind != "" {
		return errors.New("--stun-bind needs --stun")
	}
	abs, err := filepath.Abs(*dir)
	if err != nil {
		return err
	}
	if _, err = os.Stat(abs); err == nil {
		return fmt.Errorf("%s already exists; init never overwrites keys", abs)
	}
	target := abs
	if *configDir != "" {
		if target, err = filepath.Abs(*configDir); err != nil {
			return err
		}
	}
	for _, d := range []string{abs, filepath.Join(abs, "offline"), filepath.Join(abs, "tls")} {
		if err = os.Mkdir(d, 0700); err != nil {
			return err
		}
	}
	authority, authorityKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	service, serviceKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err = writeExclusive(filepath.Join(abs, "offline", "authority.key"), []byte(hex.EncodeToString(authorityKey)+"\n")); err != nil {
		return err
	}
	if err = writeExclusive(filepath.Join(abs, "service.key"), []byte(hex.EncodeToString(serviceKey)+"\n")); err != nil {
		return err
	}
	now := time.Now()
	profile := protocol.NetworkProfile{Version: "1", Operator: *operator, Authority: hex.EncodeToString(authority), ServiceKey: hex.EncodeToString(service), Epoch: 1, Expires: protocol.NetworkUint(now.Add(*validFor).Unix()), Origins: []string{httpsOrigin, "wss://" + strings.TrimPrefix(httpsOrigin, "https://")}, STUN: append([]string{}, stunList...), Privacy: *privacy}
	canonical, err := profile.Canonical(true)
	if err != nil {
		return fmt.Errorf("profile rejected (%v): check --origin, --stun, --operator and --privacy", err)
	}
	profile.Signature = hex.EncodeToString(ed25519.Sign(authorityKey, canonical))
	selection, err := network.ReviewProfile(nil, profile, profile.Authority, "self_hosted", uint64(now.Unix()))
	if err != nil {
		return fmt.Errorf("signed profile does not validate: %w", err)
	}
	encoded, _ := json.MarshalIndent(selection, "", "  ")
	if err = writeExclusive(filepath.Join(abs, "profile.json"), append(encoded, '\n')); err != nil {
		return err
	}
	certName, keyName := "tls/cert.pem", "tls/key.pem"
	if *certFile != "" {
		chain, e := readBounded(*certFile, maxCertificateBytes)
		if e != nil {
			return fmt.Errorf("read --tls-cert: %w", e)
		}
		key, e := readPrivatePath(*keyFile, maxCertificateBytes)
		if e != nil {
			return fmt.Errorf("read --tls-key: %w", e)
		}
		if err = writeExclusive(filepath.Join(abs, certName), chain); err == nil {
			err = writeExclusive(filepath.Join(abs, keyName), key)
		}
	} else {
		err = selfSigned(filepath.Join(abs, certName), filepath.Join(abs, keyName), filepath.Join(abs, "service-ca.pem"), host, now, now.Add(*validFor))
	}
	if err != nil {
		return err
	}
	cfg := serveConfig{Listen: *listen, Origin: httpsOrigin, Profile: filepath.Join(target, "profile.json"), ServiceKey: filepath.Join(target, "service.key"), TLSCert: filepath.Join(target, certName), TLSKey: filepath.Join(target, keyName), STUNListen: *stun, STUNBind: *stunBind, MetricsListen: "127.0.0.1:9464", RelayBPS: initRelayBPS, RelayDeviceBPS: initRelayDeviceBPS, RelaySessionBytes: initSessionBytes, RelayMonthBytes: initMonthBytes, Admission: "allowlist", AllowedDevicesFile: filepath.Join(target, "allowed-devices.json"), Sponsorship: true}
	if target == abs {
		// Run in place: keep the monthly relay count beside the configuration.
		// Under the packaged unit, $STATE_DIRECTORY holds it instead.
		cfg.RelayState = filepath.Join(abs, "relay-month.json")
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if err = writeExclusive(filepath.Join(abs, "serve.json"), append(b, '\n')); err != nil {
		return err
	}
	// Validate everything serve checks except the not-yet-written allowlist.
	check := cfg
	check.Admission, check.AllowedDevicesFile, check.RelayState, check.Sponsorship = "", "", "", false
	for _, name := range []*string{&check.Profile, &check.ServiceKey, &check.TLSCert, &check.TLSKey} {
		*name = filepath.Join(abs, strings.TrimPrefix(*name, target+string(filepath.Separator)))
	}
	p, err := prepare(check, now)
	if err != nil {
		return fmt.Errorf("generated service does not validate: %w", err)
	}
	_ = p.service.Close()
	digest, _ := selection.Digest()
	roots := filepath.Join(abs, "service-ca.pem")
	rootsNote := "service-ca.pem (the self-signed certificate devices trust for this service only)"
	if *certFile != "" {
		roots, rootsNote = "<your CA certificate PEM>", "your certificate's CA (omit --service-roots for a publicly trusted certificate)"
	}
	fmt.Fprintf(out, `Created a self-hosted Orbit service in %s
  profile.json          signed self_hosted profile, epoch 1, digest %s, expires %s
  service.key           online service key (stays on the host)
  %-21s TLS pair for %s
  service-ca.pem        %s
  serve.json            personal budgets (4 MiB/s relay, 200 GiB/month), admission allowlist
                        with member sponsorship
  offline/authority.key profile authority: move it OFF this host and back it up (needed for the next epoch)

Ports: %s/tcp for HTTPS/WSS%s. Devices reach the service at %s.

1. On each of your devices (profile.json and the CA file copied, owner-only):
     orbit network set --mode self_hosted --profile-file profile.json --service-roots %s
   then read its identity line ("device=... pin=...") from: orbit status

2. Allow your first device before starting:
     orbit-net devices add --config %s 'device=<64 hex> pin=<64 hex>'
   Devices that later join its orbits are admitted automatically (member
   sponsorship); "orbit-net devices remove" revokes one.

3. Check and start:
     orbit-net serve --config %s --check
     orbit-net serve --config %s
   (or install serve.json and its files under /etc/orbit-net and use the orbit-net unit;
    see "Run your own Orbit service" in orbit-net-operator.md)
`, abs, digest, time.Unix(int64(profile.Expires), 0).UTC().Format("2006-01-02"), "tls/cert.pem, key.pem", host, rootsNote, port, stunNote(*stun), httpsOrigin, filepath.Base(roots), filepath.Join(target, "serve.json"), filepath.Join(target, "serve.json"), filepath.Join(target, "serve.json"))
	return nil
}

func stunNote(stun string) string {
	if stun == "" {
		return " (no STUN: devices use the service relay when direct connections fail)"
	}
	_, port, _ := net.SplitHostPort(stun)
	return " and " + port + "/udp for STUN"
}

// selfSigned writes a P-256 certificate for host that is its own CA, so
// devices can review it with --service-roots. The key is owner-only.
func selfSigned(certPath, keyPath, caPath, host string, notBefore, notAfter time.Time) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return err
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: host, Organization: []string{"Self-hosted Orbit service"}}, NotBefore: notBefore.Add(-time.Hour), NotAfter: notAfter, IsCA: true, BasicConstraintsValid: true, MaxPathLenZero: true, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	if ip := net.ParseIP(host); ip != nil {
		template.IPAddresses = []net.IP{ip}
	} else {
		template.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err = writeExclusive(certPath, certPEM); err != nil {
		return err
	}
	if err = writeExclusive(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})); err != nil {
		return err
	}
	return writeExclusive(caPath, certPEM)
}
