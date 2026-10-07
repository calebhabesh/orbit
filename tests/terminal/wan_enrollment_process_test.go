package terminal_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"github.com/calebhabesh/file-sync/internal/replication"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/calebhabesh/file-sync/internal/app"
	"github.com/calebhabesh/file-sync/internal/config"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/testkit"
)

// The child runs the real daemon lifecycle, scheduler and authenticated owner
// control. Hooks stop only at committed production boundaries in marked roots.
func TestWANW05DaemonChild(t *testing.T) {
	dir := os.Getenv("ORBIT_W05_CHILD_STATE")
	if dir == "" {
		t.Skip("child process helper")
	}
	base := os.Getenv("ORBIT_W05_CHILD_ROOT")
	if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(os.Getenv("ORBIT_W05_CA"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(data) {
		t.Fatal("invalid disposable CA")
	}
	stopSamples := startW11DaemonSampling(t)
	defer stopSamples()
	boundary := os.Getenv("ORBIT_W05_BOUNDARY")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	var lostReceipt atomic.Bool
	var armedChunks atomic.Int32
	err = app.ServeWithOptions(ctx, dir, app.ServeOptions{TransferFaultHook: func(name string) error {
		if os.Getenv("ORBIT_W15_NAMESPACE") == "isolated-marked-namespace" && boundary != "" {
			if arm, e := os.ReadFile(filepath.Join(dir, "w15-arm")); e == nil && string(arm) == boundary {
				if name == replication.HookChunkVerified {
					armedChunks.Add(1)
				}
				if name != boundary || (boundary == replication.HookAfterReceipt && armedChunks.Load() < 16) {
					return nil
				}
				if e = testkit.ValidateNetworkNamespace(os.Getenv("TMPDIR"), os.Getenv("ORBIT_W11_PARENT_NETNS")); e != nil {
					return e
				}
				if e = testkit.ValidateDestructiveTarget(base, dir); e != nil {
					return e
				}
				if e = config.WritePrivate(dir, "w05-boundary", []byte(name)); e != nil {
					return e
				}
				select {} // direct parent owns this exact child and SIGKILLs it
			}
		}
		if os.Getenv("ORBIT_W11_NAMESPACE") == "isolated-marked-namespace" && name == replication.HookAfterReceipt && lostReceipt.CompareAndSwap(false, true) {
			if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
				return err
			}
			if err := config.WritePrivate(dir, "w11-receipt-loss", []byte("after authenticated receipt send; original task retries\n")); err != nil {
				return err
			}
			return fmt.Errorf("marked receipt response loss: %w", io.ErrUnexpectedEOF)
		}
		return nil
	}, ControlAddress: "127.0.0.1:0", NetworkRoots: roots, Ready: os.Stdout, NoWatch: true, SyncInterval: time.Second, ControllerFaultHook: func(name string) error {
		if name != boundary {
			return nil
		}
		if err := testkit.ValidateDestructiveTarget(base, dir); err != nil {
			return err
		}
		if err := config.WritePrivate(dir, "w05-boundary", []byte(name)); err != nil {
			return err
		}
		select {} // parent validates this exact child before SIGKILL
	}})
	if err != nil {
		t.Fatal(err)
	}
}

type w05Process struct {
	cmd  *exec.Cmd
	done chan error
	f    *fixture
	log  string
}

func startW05Process(t *testing.T, f *fixture, ca, boundary string) *w05Process {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(f.state, "w05-boundary")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	log := filepath.Join(f.root, "w05-child-"+enrollmentRandom(t)+".log")
	out, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestWANW05DaemonChild$", "-test.v")
	if os.Getenv("ORBIT_W15_NAMESPACE") == "isolated-marked-namespace" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
	}
	cmd.Env = append(os.Environ(), "ORBIT_W05_CHILD_STATE="+f.state, "ORBIT_W05_CHILD_ROOT="+f.root, "ORBIT_W05_CA="+ca, "ORBIT_W05_BOUNDARY="+boundary)
	cmd.Stdout = out
	cmd.Stderr = out
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &w05Process{cmd: cmd, done: make(chan error, 1), f: f, log: log}
	t.Cleanup(func() {
		if p.cmd != nil {
			p.stop(t, false)
		}
	})
	go func() { p.done <- cmd.Wait(); _ = out.Close() }()
	deadline := time.Now().Add(20 * time.Second)
	for {
		data, _ := os.ReadFile(log)
		if strings.Contains(string(data), "agent ready:") {
			break
		}
		select {
		case err := <-p.done:
			p.cmd = nil
			t.Fatalf("daemon startup: %v %s", err, data)
		default:
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatalf("daemon did not start: %s", data)
		}
		time.Sleep(50 * time.Millisecond)
	}
	return p
}
func (p *w05Process) stop(t *testing.T, kill bool) {
	t.Helper()
	if p.cmd == nil {
		return
	}
	if err := testkit.ValidateDestructiveTarget(p.f.root, p.f.state); err != nil {
		t.Fatal(err)
	}
	pid, err := os.ReadFile(filepath.Join(p.f.state, ".agent.pid"))
	if err != nil || strings.TrimSpace(string(pid)) != strconv.Itoa(p.cmd.Process.Pid) {
		t.Fatal("child identity mismatch before signal", err)
	}
	if kill {
		err = p.cmd.Process.Kill()
	} else {
		err = p.cmd.Process.Signal(syscall.SIGTERM)
	}
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-p.done:
		if !kill && err != nil {
			data, _ := os.ReadFile(p.log)
			t.Fatalf("daemon exit: %v %s", err, data)
		}
	case <-time.After(15 * time.Second):
		_ = p.cmd.Process.Kill()
		t.Fatal("daemon did not join")
	}
	p.cmd = nil
}
func waitW05Boundary(t *testing.T, p *w05Process, boundary string) {
	t.Helper()
	deadline := time.Now().Add(100 * time.Second)
	for {
		data, _ := os.ReadFile(filepath.Join(p.f.state, "w05-boundary"))
		if string(data) == boundary {
			return
		}
		if time.Now().After(deadline) {
			log, _ := os.ReadFile(p.log)
			t.Fatalf("boundary %s not reached: %s", boundary, log)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
func TestWANW05DaemonSIGKILLRoutedEnrollmentBoundaries(t *testing.T) {
	for _, phase := range []string{"request_prepared", "request_accepted", "approval", "membership_received"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			_, selection, origin, roots, _ := w05Service(t)
			a, b := newW05Node(t, selection, origin, roots), newW05Node(t, selection, origin, roots)
			folder := w05Create(t, a, "source")
			inv := w05Invite(t, a, folder, "")
			m := w05Join(t, b, inv, "join")
			// The only CA is generated for this local fixture and transferred independently
			// of the invitation. Real operator/default profiles remain W13.
			// CertPool does not export DER; obtain the service leaf through verified TLS.
			connection, err := tlsDialW05(origin, roots)
			if err != nil {
				t.Fatal(err)
			}
			leaf := connection.ConnectionState().PeerCertificates[0]
			_ = connection.Close()
			ca := filepath.Join(a.f.root, "service-ca.pem")
			if err = os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw}), 0600); err != nil {
				t.Fatal(err)
			}
			a.stop()
			b.stop()
			a.stop = func() {}
			b.stop = func() {}
			a.f.close()
			b.f.close()
			ownerBoundary := ""
			joinBoundary := "terminal.setup." + phase
			if phase == "approval" {
				ownerBoundary = "terminal.enrollment.approved"
				joinBoundary = ""
			}
			owner := startW05Process(t, a.f, ca, ownerBoundary)
			joiner := startW05Process(t, b.f, ca, joinBoundary)
			ac, bc := &controlclient.Client{StateDir: a.f.state}, &controlclient.Client{StateDir: b.f.state}
			mutated := make(chan struct{})
			go func() { _, _ = bc.Mutate(context.Background(), m); close(mutated) }()
			if phase == "request_prepared" || phase == "request_accepted" {
				waitW05Boundary(t, joiner, joinBoundary)
				joiner.stop(t, true)
				<-mutated
				joiner = startW05Process(t, b.f, ca, "")
			} else {
				<-mutated
			}
			deadline := time.Now().Add(100 * time.Second)
			var request tc.EnrollmentRequest
			for {
				r, e := ac.Query(context.Background(), tc.Query{Version: "1", Kind: "requests", Folder: inv.Folder, Limit: 200})
				if e == nil && len(r.Requests) == 1 {
					request = r.Requests[0]
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("request not recovered: %v", e)
				}
				time.Sleep(200 * time.Millisecond)
			}
			approval := tc.Mutation{Version: "1", Kind: "approval", OperationID: enrollmentRandom(t), Approval: &tc.ApprovalIntent{Request: request.ID, Folder: request.Folder, Requester: request.Requester, KeyPin: request.KeyPin, TranscriptDigest: request.TranscriptDigest, ExpectedMembership: request.ExpectedMembership, Decision: "approve"}}
			approved := make(chan struct{})
			go func() { _, _ = ac.Mutate(context.Background(), approval); close(approved) }()
			if phase == "approval" {
				waitW05Boundary(t, owner, ownerBoundary)
				owner.stop(t, true)
				<-approved
				owner = startW05Process(t, a.f, ca, "")
			} else {
				<-approved
			}
			if phase == "membership_received" {
				waitW05Boundary(t, joiner, joinBoundary)
				joiner.stop(t, true)
				joiner = startW05Process(t, b.f, ca, "")
			}
			deadline = time.Now().Add(120 * time.Second)
			for {
				r, e := bc.Query(context.Background(), tc.Query{Version: "1", Kind: "operation", ID: m.OperationID})
				if e == nil && r.Operation.State == "completed" {
					if r.Join.Request != request.ID || r.Join.Attempt != m.Join.Attempt || r.Join.Root != m.Join.Root || !r.Readiness.Ready() {
						t.Fatal("crash replaced reviewed identity/root or invented readiness")
					}
					break
				}
				if time.Now().After(deadline) {
					log, _ := os.ReadFile(joiner.log)
					t.Fatalf("setup did not resume: %v %+v %s", e, r.Error, log)
				}
				time.Sleep(250 * time.Millisecond)
			}
			// Scheduler performs the reverse pull after restoring the approved route.
			deadline = time.Now().Add(60 * time.Second)
			for {
				bytes, e := os.ReadFile(filepath.Join(a.f.root, "source", "joining-file"))
				if e == nil && string(bytes) == "requester protected bytes join" {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("reverse data route did not recover")
				}
				time.Sleep(250 * time.Millisecond)
			}
			joiner.stop(t, false)
			owner.stop(t, false)
			a.f.open()
			b.f.open()
			if got := hex.EncodeToString(b.f.device[:]); got != request.Requester {
				t.Fatal("identity changed after kill")
			}
			w05AssertFiles(t, a, b, folder, filepath.Join(a.f.root, "source"), m.Join.Root)
			regs, e := b.f.db.RegisteredFolders(context.Background())
			if e != nil || len(regs) != 1 {
				t.Fatal("duplicate root registration")
			}
			t.Logf("SIGKILL %s recovered exact attempt/root and two-way heads/hashes", phase)
		})
	}
}

func tlsDialW05(origin string, roots *x509.CertPool) (*tls.Conn, error) {
	return tls.Dial("tcp", strings.TrimPrefix(origin, "https://"), &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS13})
}
