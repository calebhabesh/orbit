package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/calebhabesh/file-sync/internal/network"
	"github.com/calebhabesh/file-sync/internal/protocol"
	"github.com/calebhabesh/file-sync/internal/rendezvous"
	"github.com/calebhabesh/file-sync/internal/state"
)

// serveConfig is the operator's reviewed service configuration. Paths name
// owner-only files; budgets are finite engineering ceilings, not capacity claims.
type serveConfig struct {
	Listen            string `json:"listen"`
	Origin            string `json:"origin"`
	Profile           string `json:"profile"`
	ServiceKey        string `json:"service_key"`
	OverlapProfile    string `json:"overlap_profile,omitempty"`
	OverlapServiceKey string `json:"overlap_service_key,omitempty"`
	TLSCert           string `json:"tls_cert"`
	TLSKey            string `json:"tls_key"`
	STUNListen        string `json:"stun_listen,omitempty"`
	// STUNBind is the local socket when the reviewed public STUN address is
	// reached through 1:1 NAT (for example cloud private addresses).
	STUNBind          string `json:"stun_bind,omitempty"`
	MetricsListen     string `json:"metrics_listen,omitempty"`
	RelayBPS          int64  `json:"relay_bps"`
	RelayDeviceBPS    int64  `json:"relay_device_bps"`
	RelaySessionBytes int64  `json:"relay_session_bytes"`
}

const (
	maxConfigBytes      = 16 << 10
	maxCertificateBytes = 64 << 10
	profileWarning      = 14 * 24 * time.Hour
	certificateWarning  = 14 * 24 * time.Hour
)

func serve(args []string, errOut io.Writer) error {
	flags := flag.NewFlagSet("orbit-net serve", flag.ContinueOnError)
	flags.SetOutput(errOut)
	configFile := flags.String("config", "", "private JSON service configuration; explicit flags override it")
	check := flags.Bool("check", false, "validate configuration, keys, profile and certificate, then exit")
	cfg := serveConfig{Listen: ":8443", RelayBPS: 20 << 20, RelayDeviceBPS: 5 << 20, RelaySessionBytes: 16 << 30}
	stunListen := flags.String("stun-listen", "", "optional separate numeric UDP STUN listener from reviewed profile")
	stunBind := flags.String("stun-bind", "", "local UDP address for --stun-listen behind 1:1 NAT (default: the listen address itself)")
	listen := flags.String("listen", "", "TLS listen address (default :8443)")
	profileFile := flags.String("profile", "", "private reviewed ProfileSelection JSON")
	overlapProfile := flags.String("overlap-profile", "", "optional adjacent epoch served during rotation")
	overlapKey := flags.String("overlap-service-key", "", "private hex Ed25519 key for --overlap-profile")
	origin := flags.String("origin", "", "HTTPS origin from profile")
	certFile := flags.String("tls-cert", "", "service certificate chain PEM")
	keyFile := flags.String("tls-key", "", "private service TLS key PEM")
	signer := flags.String("service-key", "", "private hex Ed25519 signing key file")
	metrics := flags.String("metrics-listen", "", "optional loopback HTTP address for /metrics and /healthz")
	relayRate := flags.Int64("relay-bps", 0, "aggregate ciphertext bytes/sec (finite, default 20 MiB/s)")
	deviceRate := flags.Int64("relay-device-bps", 0, "per-device ciphertext bytes/sec (finite, default 5 MiB/s)")
	sessionBytes := flags.Int64("relay-session-bytes", 0, "maximum ciphertext bytes per tunnel (default 16 GiB)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("serve accepts no positional arguments")
	}
	if *configFile != "" {
		data, err := readPrivatePath(*configFile, maxConfigBytes)
		if err != nil {
			return fmt.Errorf("read config: %w", err)
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&cfg); err != nil {
			return fmt.Errorf("decode config: %w", err)
		}
	}
	set := map[string]bool{}
	flags.Visit(func(f *flag.Flag) { set[f.Name] = true })
	for name, pair := range map[string]struct {
		dst *string
		src *string
	}{"stun-listen": {&cfg.STUNListen, stunListen}, "stun-bind": {&cfg.STUNBind, stunBind}, "listen": {&cfg.Listen, listen}, "profile": {&cfg.Profile, profileFile}, "overlap-profile": {&cfg.OverlapProfile, overlapProfile}, "overlap-service-key": {&cfg.OverlapServiceKey, overlapKey}, "origin": {&cfg.Origin, origin}, "tls-cert": {&cfg.TLSCert, certFile}, "tls-key": {&cfg.TLSKey, keyFile}, "service-key": {&cfg.ServiceKey, signer}, "metrics-listen": {&cfg.MetricsListen, metrics}} {
		if set[name] {
			*pair.dst = *pair.src
		}
	}
	for name, pair := range map[string]struct{ dst, src *int64 }{"relay-bps": {&cfg.RelayBPS, relayRate}, "relay-device-bps": {&cfg.RelayDeviceBPS, deviceRate}, "relay-session-bytes": {&cfg.RelaySessionBytes, sessionBytes}} {
		if set[name] {
			*pair.dst = *pair.src
		}
	}
	p, err := prepare(cfg, time.Now())
	if err != nil {
		return err
	}
	defer p.service.Close()
	for _, warning := range p.warnings {
		fmt.Fprintln(errOut, "orbit-net: warning:", warning)
	}
	if *check {
		fmt.Fprintf(errOut, "orbit-net: configuration valid for %s, profile epoch %d (%s) until %s\n", p.cfg.Origin, p.selection.Profile.Epoch, p.selection.Environment, time.Unix(int64(p.selection.Profile.Expires), 0).UTC().Format(time.RFC3339))
		return nil
	}
	return p.run(errOut)
}

type prepared struct {
	cfg       serveConfig
	selection network.ProfileSelection
	service   *rendezvous.Service
	cert      *certificateHolder
	warnings  []string
}

// prepare validates every operator input before any socket is opened.
func prepare(cfg serveConfig, now time.Time) (*prepared, error) {
	if cfg.RelayBPS <= 0 || cfg.RelayDeviceBPS <= 0 || cfg.RelaySessionBytes <= 0 || cfg.RelayDeviceBPS > cfg.RelayBPS {
		return nil, errors.New("relay limits must be positive and finite, with per-device rate at most the aggregate")
	}
	if cfg.Profile == "" || cfg.ServiceKey == "" || cfg.TLSCert == "" || cfg.TLSKey == "" || cfg.Listen == "" {
		return nil, errors.New("missing service configuration: listen, profile, service_key, tls_cert and tls_key are required")
	}
	if (cfg.OverlapProfile == "") != (cfg.OverlapServiceKey == "") {
		return nil, errors.New("overlap_profile and overlap_service_key must be configured together")
	}
	selection, key, err := loadServed(cfg.Profile, cfg.ServiceKey)
	if err != nil {
		return nil, err
	}
	if cfg.Origin == "" {
		for _, o := range selection.Profile.Origins {
			if strings.HasPrefix(o, "https://") {
				if cfg.Origin != "" {
					return nil, errors.New("profile has several HTTPS origins; configure origin explicitly")
				}
				cfg.Origin = o
			}
		}
	}
	var overlap []rendezvous.ServedProfile
	if cfg.OverlapProfile != "" {
		s, k, e := loadServed(cfg.OverlapProfile, cfg.OverlapServiceKey)
		if e != nil {
			return nil, fmt.Errorf("overlap profile: %w", e)
		}
		overlap = append(overlap, rendezvous.ServedProfile{Selection: s, ServiceKey: k})
	}
	if cfg.STUNListen != "" {
		allowed := false
		for _, address := range selection.Profile.STUN {
			allowed = allowed || address == cfg.STUNListen
		}
		if !allowed {
			return nil, errors.New("STUN listener not in reviewed profile")
		}
	}
	if cfg.STUNBind != "" {
		bind, e := netip.ParseAddrPort(cfg.STUNBind)
		if cfg.STUNListen == "" || e != nil || bind.Port() == 0 {
			return nil, errors.New("stun_bind must be a numeric address and port, used together with stun_listen")
		}
	}
	if cfg.MetricsListen != "" {
		address, e := netip.ParseAddrPort(cfg.MetricsListen)
		if e != nil || !address.Addr().IsLoopback() {
			return nil, errors.New("metrics_listen must be a numeric loopback address and port")
		}
	}
	u, err := url.Parse(cfg.Origin)
	if err != nil || u.Scheme != "https" {
		return nil, errors.New("origin must be an HTTPS origin from the profile")
	}
	cert, err := loadCertificate(cfg.TLSCert, cfg.TLSKey, u.Hostname(), now)
	if err != nil {
		return nil, err
	}
	service, err := rendezvous.New(rendezvous.Options{Selection: selection, Origin: cfg.Origin, ServiceKey: key, Overlap: overlap, Relay: rendezvous.RelayLimits{ServiceBytesPerSecond: cfg.RelayBPS, DeviceBytesPerSecond: cfg.RelayDeviceBPS, SessionBytes: cfg.RelaySessionBytes}})
	if err != nil {
		return nil, fmt.Errorf("profile, origin or service key rejected: %w", err)
	}
	p := &prepared{cfg: cfg, selection: selection, service: service, cert: &certificateHolder{}}
	p.cert.store(cert)
	if left := time.Unix(int64(selection.Profile.Expires), 0).Sub(now); left < profileWarning {
		p.warnings = append(p.warnings, fmt.Sprintf("profile epoch %d expires in %s; sign and distribute the next epoch", selection.Profile.Epoch, left.Round(time.Minute)))
	}
	if left := cert.Leaf.NotAfter.Sub(now); left < certificateWarning {
		p.warnings = append(p.warnings, fmt.Sprintf("TLS certificate expires in %s; renew and send SIGHUP", left.Round(time.Minute)))
	}
	if selection.Environment != "release" {
		p.warnings = append(p.warnings, "serving a "+selection.Environment+" profile; clients must review it explicitly with custom trust")
	}
	return p, nil
}

func loadServed(profilePath, keyPath string) (network.ProfileSelection, ed25519.PrivateKey, error) {
	var selection network.ProfileSelection
	data, err := readPrivatePath(profilePath, protocol.NetworkMaxBytes)
	if err != nil {
		return selection, nil, fmt.Errorf("read profile: %w", err)
	}
	if err = protocol.NetworkDecode(data, &selection); err != nil {
		return selection, nil, fmt.Errorf("decode profile: %w", err)
	}
	if err = selection.Validate(uint64(time.Now().Unix())); err != nil {
		return selection, nil, fmt.Errorf("profile invalid or expired: %w", err)
	}
	key, err := readKey(keyPath)
	if err != nil {
		return selection, nil, fmt.Errorf("read service key: %w", err)
	}
	if hex.EncodeToString(key.Public().(ed25519.PublicKey)) != selection.Profile.ServiceKey {
		return selection, nil, errors.New("service key does not match the profile's service_key")
	}
	return selection, key, nil
}

func readKey(path string) (ed25519.PrivateKey, error) {
	data, err := readPrivatePath(path, 256)
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(data)))
	if err != nil || len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("key file must hold one hex Ed25519 private key")
	}
	return ed25519.PrivateKey(key), nil
}

func readPrivatePath(path string, max int64) ([]byte, error) {
	return state.ReadPrivate(filepath.Dir(path), filepath.Base(path), max)
}

// loadCertificate requires an owner-only key and a currently valid leaf for the
// profile host, so a reload can never install a wrong-host or expired chain.
func loadCertificate(certPath, keyPath, host string, now time.Time) (*tls.Certificate, error) {
	chain, err := readBounded(certPath, maxCertificateBytes)
	if err != nil {
		return nil, fmt.Errorf("read TLS certificate: %w", err)
	}
	key, err := readPrivatePath(keyPath, maxCertificateBytes)
	if err != nil {
		return nil, fmt.Errorf("read TLS key: %w", err)
	}
	cert, err := tls.X509KeyPair(chain, key)
	if err != nil {
		return nil, errors.New("TLS certificate and key do not form a valid pair")
	}
	if cert.Leaf == nil {
		if cert.Leaf, err = x509.ParseCertificate(cert.Certificate[0]); err != nil {
			return nil, err
		}
	}
	if now.Before(cert.Leaf.NotBefore) || !now.Before(cert.Leaf.NotAfter) {
		return nil, errors.New("TLS certificate is not currently valid")
	}
	if err = cert.Leaf.VerifyHostname(host); err != nil {
		return nil, errors.New("TLS certificate does not cover the profile origin host")
	}
	return &cert, nil
}

func readBounded(path string, max int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err == nil && int64(len(data)) > max {
		err = errors.New("file too large")
	}
	return data, err
}

type certificateHolder struct {
	current           atomic.Pointer[tls.Certificate]
	reloads, failures atomic.Uint64
}

func (h *certificateHolder) store(c *tls.Certificate) { h.current.Store(c) }
func (h *certificateHolder) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return h.current.Load(), nil
}

func (p *prepared) reload(errOut io.Writer) {
	u, _ := url.Parse(p.cfg.Origin)
	cert, err := loadCertificate(p.cfg.TLSCert, p.cfg.TLSKey, u.Hostname(), time.Now())
	if err != nil {
		p.cert.failures.Add(1)
		fmt.Fprintln(errOut, "orbit-net: certificate reload failed; keeping current certificate:", err)
		return
	}
	p.cert.store(cert)
	p.cert.reloads.Add(1)
	fmt.Fprintf(errOut, "orbit-net: certificate reloaded; valid until %s\n", cert.Leaf.NotAfter.UTC().Format(time.RFC3339))
}

func (p *prepared) run(errOut io.Writer) error {
	var stun *rendezvous.STUNServer
	if p.cfg.STUNListen != "" {
		address := p.cfg.STUNListen
		if p.cfg.STUNBind != "" {
			address = p.cfg.STUNBind
		}
		socket, err := net.ListenPacket("udp", address)
		if err != nil {
			return fmt.Errorf("STUN listener: %w", err)
		}
		if stun, err = rendezvous.NewSTUNServer(socket); err != nil {
			_ = socket.Close()
			return err
		}
		defer stun.Close()
	}
	server := p.service.Server()
	server.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS13, GetCertificate: p.cert.get, NextProtos: []string{"http/1.1"}}
	raw, err := net.Listen("tcp", p.cfg.Listen)
	if err != nil {
		return fmt.Errorf("TLS listener: %w", err)
	}
	listener := rendezvous.BoundedListener(raw)
	defer listener.Close()
	var metricsServer *http.Server
	var metricsDone sync.WaitGroup
	if p.cfg.MetricsListen != "" {
		ml, e := net.Listen("tcp", p.cfg.MetricsListen)
		if e != nil {
			return fmt.Errorf("metrics listener: %w", e)
		}
		metricsServer = &http.Server{Handler: p.metricsHandler(listener, stun), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8 << 10}
		metricsDone.Add(1)
		go func() { defer metricsDone.Done(); _ = metricsServer.Serve(ml) }()
	}
	stopSignals, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	hup := make(chan os.Signal, 1)
	signal.Notify(hup, syscall.SIGHUP)
	defer signal.Stop(hup)
	done := make(chan error, 1)
	go func() { done <- server.ServeTLS(listener, "", "") }()
	fmt.Fprintf(errOut, "orbit-net: serving %s on %s (profile epoch %d, %s)\n", p.cfg.Origin, raw.Addr(), p.selection.Profile.Epoch, p.selection.Environment)
	for {
		select {
		case err = <-done:
			if !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		case <-hup:
			p.reload(errOut)
			continue
		case <-stopSignals.Done():
		}
		break
	}
	fmt.Fprintln(errOut, "orbit-net: stopping; closing controls and relays, then draining HTTP")
	_ = p.service.Close()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if metricsServer != nil {
		_ = metricsServer.Shutdown(shutdown)
		metricsDone.Wait()
	}
	if err = server.Shutdown(shutdown); err != nil {
		_ = server.Close()
		return err
	}
	return nil
}
