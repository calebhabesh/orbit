package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/state"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/config"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/launcher"
	"github.com/calebhabesh/orbit/internal/pairing"
	"golang.org/x/sys/unix"
)

func setupID() (string, error) {
	var b [32]byte
	_, err := rand.Read(b[:])
	return hex.EncodeToString(b[:]), err
}
func handleOrbitSetup(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit setup", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dirFlag := flags.String("state", "", "selected state directory")
	root := flags.String("root", "", "folder to create or adopt (default ~/Orbit)")
	label := flags.String("label", "", "device name")
	name := flags.String("name", "", "synced folder name")
	preview := flags.Bool("preview", false, "measure root and write a private reviewed request")
	resume := flags.Bool("resume", false, "inspect/resume the operation in --request-file or --operation")
	joining := flags.Bool("join", false, "join the folder in a private invitation")
	invitationFile := flags.String("invitation-file", "", "private transferred invitation")
	invitationCode := flags.String("invitation", "", "deprecated; use private file or stdin")
	pairingProfile := flags.String("pairing-profile", "", "reviewed operator profile digest for short-code stdin")
	invitationStdin := flags.Bool("invitation-stdin", false, "read bounded private invitation from stdin")
	connection := flags.String("connection", "", "automatic, local_only, manual, or self_hosted")
	remote := flags.String("remote", "", "inviter endpoint (must match invitation)")
	folder := flags.String("folder", "", "folder identity (must match invitation)")
	reviewFile := flags.String("review-file", "", "private request output during --preview; reviewed request input otherwise")
	requestFile := flags.String("request-file", "", "private durable reviewed mutation; reuse this file for retries")
	operation := flags.String("operation", "", "inspect durable setup/join operation")
	runtimeFile := flags.String("settings-file", "", "private finite/network/startup settings JSON for the review")
	timeout := flags.Int("timeout", 60, "seconds to wait for Ready (0 returns current phase)")
	jsonOut := flags.Bool("json", false, "structured output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *invitationCode != "" {
		return errors.New("invitation secrets must use a private file, stdin, or prompt")
	}
	if flags.NArg() != 0 || *timeout < 0 {
		return errors.New("invalid setup arguments")
	}
	dir, err := launcher.DiscoverState(*dirFlag)
	if err != nil {
		return err
	}
	_, priorErr := os.Stat(filepath.Join(dir, "config.json"))
	freshInstall := os.IsNotExist(priorErr)
	// Daemon owns initialization and work even if the prompt is closed.
	if _, err = launcher.EnsureDaemon(context.Background(), launcher.LaunchOptions{StateDir: dir, NoBrowser: true}); err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: dir}
	output := func(r tc.Result) error {
		if r.Operation != nil {
			status, e := client.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"})
			if e == nil {
				r.Network = status.Network
			}
		}
		if *jsonOut {
			if err := json.NewEncoder(stdout).Encode(r); err != nil {
				return err
			}
		} else if r.Operation != nil {
			if r.Operation.Kind == "join" && r.State == "completed" {
				fmt.Fprintln(stdout, "Workspace joined successfully")
			}
			fmt.Fprintf(stdout, "Setup: %s; phase=%s; operation=%s\n", r.State, r.Operation.Phase, r.Operation.ID)
			if r.Join != nil {
				fmt.Fprintf(stdout, "Folder: %s; root: %q; request: %s\n", r.Join.Folder, r.Join.Root, r.Join.Request)
			}
		} else if r.Preview != nil {
			p := r.Preview
			fmt.Fprintf(stdout, "Root: %q; files=%d; directories=%d; bytes=%d; unsupported=%d; unreadable=%d; complete=%v; capacity known=%v\n", p.Root, p.Files, p.Directories, p.Bytes, p.Unsupported, p.Unreadable, p.Complete, p.CapacityKnown)
		}
		if !*jsonOut && r.Network != nil {
			fmt.Fprintf(stdout, "Networking: %s; code=%s; ready=%v (peer copies are reported separately by status)\n", r.Network.Policy.Mode, r.Network.Code, r.Network.Ready)
		}

		if r.Error != nil {
			if !*jsonOut {
				fmt.Fprintf(stderr, "Error [%s]: %s; action: %s\n", r.Error.Code, EscapeTerminal(r.Error.Message), EscapeTerminal(r.Error.Action))
			}
			return &CLIExitError{Code: tc.ExitCode(r)}
		}
		return nil
	}
	if *operation != "" {
		r, e := client.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: *operation})
		if e != nil {
			return e
		}
		return output(r)
	}
	file := *requestFile
	if file == "" && !*preview {
		file = *reviewFile
	}
	var mutation tc.Mutation
	if file != "" {
		if err = privateEnrollmentInput(file, &mutation); err != nil {
			return err
		}
		if mutation.Kind != "setup" && mutation.Kind != "adopt" && mutation.Kind != "join" {
			return errors.New("request must be setup, adopt, or join")
		}
	} else {
		if *resume {
			return errors.New("resume requires --operation or --request-file; pending authorization is never inferred from a root")
		}
		if *root == "" {
			home, e := os.UserHomeDir()
			if e != nil {
				return e
			}
			*root = filepath.Join(home, "Orbit")
		}
		*root, err = filepath.Abs(*root)
		if err != nil {
			return err
		}
		interactive := false
		if info, e := os.Stdin.Stat(); e == nil {
			interactive = info.Mode()&os.ModeCharDevice != 0
			if _, e = unix.IoctlGetTermios(int(os.Stdin.Fd()), unix.TCGETS); e != nil {
				interactive = false
			}
		}
		if !interactive && !*preview {
			return errors.New("noninteractive setup requires a reviewed --request-file; use --preview --review-file PATH first")
		}
		reader := bufio.NewReader(os.Stdin)
		ask := func(prompt, old string) (string, error) {
			fmt.Fprintf(stderr, "%s [%q]: ", prompt, old)
			line, e := reader.ReadString('\n')
			if e != nil {
				return old, e
			}
			line = strings.TrimSpace(line)
			if line == "" {
				line = old
			}
			return line, nil
		}
		if *label == "" {
			host, _ := os.Hostname()
			*label = host
		}
		if *label == "" {
			*label = "Orbit Device"
		}
		if *name == "" {
			*name = "Orbit"
		}
		settings, err := client.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "settings"})
		if err != nil {
			return err
		}
		desired := *settings.Settings
		networkResult, err := client.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "network_status"})
		if err != nil {
			return err
		}
		policy := networkResult.Network.Policy
		connectionAnswered := false
		noFolders := false
		if folders, e := client.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "folders", Limit: 1}); e == nil {
			noFolders = len(folders.Items) == 0
		}
		if *connection != "" {
			policy.Mode = *connection
			policy.LANAdvertising = policy.Mode != "manual"
		} else if freshInstall && *runtimeFile == "" {
			policy.Mode = "automatic"
			policy.LANAdvertising = true
		}
		if policy.Mode == "manual" || policy.Mode == "local_only" {
			policy.Profile = ""
		}
		usePackagedProfile(&policy, networkResult.Network)
		if policy != networkResult.Network.Policy {
			policy.Generation++
		}
		if err = policy.Validate(); err != nil {
			return err
		}
		if *runtimeFile != "" {
			if err = privateEnrollmentInput(*runtimeFile, &desired); err != nil {
				return err
			}
		}

		for {
			if interactive && !*preview {
				*label, err = ask("Device name", *label)
				if err != nil {
					return err
				}
				*name, err = ask("Folder name", *name)
				if err != nil {
					return err
				}
				*root, err = ask("Root to create or adopt", *root)
				if err != nil {
					return err
				}
				*root, err = filepath.Abs(*root)
				if err != nil {
					return err
				}
			}
			if interactive && !*preview {
				choice, e := ask("Connection (automatic/local_only/manual/self_hosted)", policy.Mode)
				if e != nil {
					return e
				}
				connectionAnswered = choice != policy.Mode
				if choice != policy.Mode {
					policy.Mode = choice
					policy.LANAdvertising = choice != "manual"
					policy.Generation = networkResult.Network.Policy.Generation + 1
				}
				if policy.Mode == "manual" || policy.Mode == "local_only" {
					policy.Profile = ""
				}
				usePackagedProfile(&policy, networkResult.Network)
				if err = policy.Validate(); err != nil {
					fmt.Fprintln(stderr, err)
					continue
				}
				fmt.Fprintln(stderr, "Orbit services see device addresses and connection metadata; file contents stay encrypted in transit.")
				if policy.Mode == "manual" {
					desired.PeerListen, err = ask("Peer listen address (numeric LAN or Tailscale IP:port)", desired.PeerListen)
					if err != nil {
						return err
					}
					desired.EnrollmentListen, err = ask("Enrollment listen address", desired.EnrollmentListen)
					if err != nil {
						return err
					}
					desired.AdvertisedPeer, err = ask("Reachable peer address", desired.AdvertisedPeer)
					if err != nil {
						return err
					}
					desired.AdvertisedEnrollment, err = ask("Reachable enrollment address", desired.AdvertisedEnrollment)
					if err != nil {
						return err
					}
				}
				numeric := func(prompt string, value *tc.Uint) error {
					for {
						line, e := ask(prompt, strconv.FormatUint(uint64(*value), 10))
						if e != nil {
							return e
						}
						n, e := strconv.ParseUint(line, 10, 64)
						if e == nil && n > 0 {
							*value = tc.Uint(n)
							return nil
						}
						fmt.Fprintln(stderr, "Enter a positive byte/count value.")
					}
				}
				for _, field := range []struct {
					label string
					value *tc.Uint
				}{{"Data budget bytes", &desired.DataBudget}, {"Metadata budget bytes", &desired.MetadataBudget}, {"Free-space reserve bytes", &desired.ReserveBytes}, {"Transfer concurrency (1–32)", &desired.Concurrency}} {
					if err = numeric(field.label, field.value); err != nil {
						return err
					}
				}
				if h := settings.Host; h != nil && h.Note != "" {
					fmt.Fprintln(stderr, h.Note)
				}
				desired.Startup, err = ask("Startup (manual/login/unattended)", desired.Startup)
				if err != nil {
					return err
				}
			}
			if err = config.ValidateRuntimeSettings(desired); err != nil {
				if interactive && !*preview {
					fmt.Fprintln(stderr, err)
					continue
				}
				return err
			}
			plan := tc.SetupIntent{DeviceName: *label, FolderName: *name, Root: *root, Settings: desired, Network: &policy}
			kind := "setup"
			var inv tc.Invitation
			if *joining {
				kind = "join"
				if interactive && *invitationFile == "" && !*invitationStdin {
					input, e := ask("Private invitation file path or pasted invitation", "")
					if e != nil {
						return e
					}
					_, shortErr := pairing.Normalize(input)
					if strings.HasPrefix(input, "orbit-invitation:v") || shortErr == nil {
						*invitationCode = input
					} else {
						*invitationFile = input
					}
				}

				if *invitationStdin {
					b, e := io.ReadAll(io.LimitReader(os.Stdin, 16385))
					if e != nil {
						return e
					}
					if len(b) > 16384 {
						return errors.New("invitation exceeds 16 KiB")
					}
					if _, shortErr := pairing.Normalize(strings.TrimSpace(string(b))); shortErr == nil {
						inv, e = resolvePairingCLI(client, strings.TrimSpace(string(b)), *pairingProfile, networkResult.Network, interactive, ask, stderr)
					} else {
						e = decodeInvitationInput(b, &inv)
					}
					if e != nil {
						return e
					}
				} else if *invitationFile != "" {
					if err = privateEnrollmentInput(*invitationFile, &inv); err != nil {
						return err
					}
				} else {
					code := strings.TrimSpace(*invitationCode)
					if _, shortErr := pairing.Normalize(code); shortErr == nil {
						inv, err = resolvePairingCLI(client, code, *pairingProfile, networkResult.Network, interactive, ask, stderr)
						if err != nil {
							return err
						}
					} else {
						if !strings.HasPrefix(code, "orbit-invitation:v") {
							return errors.New("join requires a private invitation file, stdin, or prompt")
						}
						if e := decodeInvitationInput([]byte(code), &inv); e != nil {
							return e
						}
					}
				}
				if err = inv.ExpandPackaged(); err != nil {
					return err
				}
				if err = tc.ShapeError(inv.Validate()); err != nil {
					return err
				}
				expires, _ := time.Parse(time.RFC3339Nano, inv.ExpiresAt)
				if !time.Now().Before(expires) {
					return &control.ControlError{Code: "INVITATION_EXPIRED", Message: "invitation expired", Action: "request a new invitation; retain the reviewed root and device name"}
				}
				if (*remote != "" && *remote != inv.EnrollmentEndpoint) || (*folder != "" && *folder != inv.Folder) {
					return errors.New("invitation identity/scope mismatch")
				}
				if *connection == "" && !connectionAnswered {
					// The invitation decides the operator unless the owner chose (F08).
					joinPolicy := tc.JoinPolicy(inv, networkResult.Network.Policy, networkResult.Network.Builtin, freshInstall || noFolders)
					plan.Network = &joinPolicy
					if inv.Profile != nil {
						fmt.Fprintf(stderr, "Connection: %s through %s, taken from the invitation. %s\n", joinPolicy.Mode, EscapeTerminal(inv.Profile.Operator), EscapeTerminal(inv.Profile.Privacy))
					}
				}
			}
			q := tc.Query{Version: tc.Version, Kind: "root_preview", Path: *root, Name: kind, RootPlan: &plan}
			var result tc.Result
			for {
				result, err = client.Query(context.Background(), q)
				if err != nil {
					break
				}
				if result.Preview.Complete {
					break
				}
				q.Cursor = result.Cursor
				if !*jsonOut {
					fmt.Fprintf(stderr, "Measured so far: %d files, %d bytes; enumeration continues\n", result.Preview.Files, result.Preview.Bytes)
				}
			}
			if err != nil {
				if interactive && !*preview {
					fmt.Fprintln(stderr, err)
					continue
				}
				return err
			}
			if result.Error != nil || result.Preview.Unsupported != 0 || result.Preview.Unreadable != 0 {
				if interactive && !*preview {
					fmt.Fprintln(stderr, "Root requires correction before adoption.")
					continue
				}
				if result.Error == nil {
					result.Error = &tc.Error{Code: "ROOT_REVIEW_INCOMPLETE", Message: "unsupported or unreadable objects block adoption"}
				}
				return output(result)
			}
			plan.Preview = *result.Review
			id, e := setupID()
			if e != nil {
				return e
			}
			mutation = tc.Mutation{Version: tc.Version, OperationID: id, Kind: kind, Setup: &plan}
			if *joining {
				attempt, e := setupID()
				if e != nil {
					return e
				}
				mutation.Setup = nil
				mutation.Join = &tc.JoinIntent{Invitation: inv, Attempt: attempt, DeviceName: plan.DeviceName, FolderName: plan.FolderName, Root: plan.Root, Preview: plan.Preview, Settings: plan.Settings, Network: plan.Network}
			}
			if *preview {
				if *reviewFile != "" {
					if err = writeSetupRequest(*reviewFile, mutation); err != nil {
						return err
					}
				}
				return output(result)
			}
			fmt.Fprintf(stderr, "Review: device=%q; folder=%q; root=%q; files=%d; bytes=%d\nFinite/startup settings: %+v\n", plan.DeviceName, plan.FolderName, plan.Root, result.Preview.Files, result.Preview.Bytes, desired)
			fmt.Fprintf(stderr, "Connection: %s; profile: %s\nLAN advertising: %t (signed device identity and listener addresses).\nOrbit services see device addresses and connection metadata; file contents stay encrypted in transit.\n", policy.Mode, policy.Profile, policy.LANAdvertising)
			if b := networkResult.Network.Builtin; b != nil && policy.Profile == b.Digest {
				fmt.Fprintf(stderr, "Operator: %q (packaged profile, expires %s)\nPrivacy: %s\n", b.Operator, b.Expires, b.Privacy)
			}
			answer, e := ask("Create/adopt this reviewed folder? (yes/edit)", "edit")
			if e != nil {
				return e
			}
			if answer != "yes" {
				continue
			}
			file = filepath.Join(dir, "setup-request-"+id+".json")
			if err = writeSetupRequest(file, mutation); err != nil {
				return err
			}
			fmt.Fprintf(stderr, "Resume with --request-file %q\n", file)
			break
		}
	}
	client.RestartDaemon = func(ctx context.Context) error {
		if err := app.StopAgent(dir, 5*time.Second); err != nil {
			return err
		}
		_, err := launcher.EnsureDaemon(ctx, launcher.LaunchOptions{StateDir: dir, NoBrowser: true})
		return err
	}
	r, err := client.Setup(context.Background(), mutation)
	if err != nil {
		return err
	}
	deadline := time.Now().Add(time.Duration(*timeout) * time.Second)
	r, err = waitSetupOperation(r, mutation.OperationID, deadline, client.Query)
	if err != nil {
		return err
	}
	return output(r)
}
func writeSetupRequest(path string, m tc.Mutation) error {
	// O_EXCL keeps an existing reviewed request intact; private parent is verified
	// by the same input path helper before later consumption.
	if err := state.ValidatePrivateFileDir(filepath.Dir(path)); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return err
	}
	parent, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer parent.Close()
	return parent.Sync()
}

func waitSetupOperation(r tc.Result, id string, deadline time.Time, query func(context.Context, tc.Query) (tc.Result, error)) (tc.Result, error) {
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	for r.State == "running" && time.Now().Before(deadline) {
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return r, nil
		case <-timer.C:
		}
		next, err := query(ctx, tc.Query{Version: tc.Version, Kind: "operation", ID: id})
		if err != nil {
			// Queries only observe the original durable operation. A busy/reconnecting
			// daemon can time out while still completing it; never resubmit here.
			if ctx.Err() != nil {
				return r, nil
			}
			var networkError net.Error
			if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
				continue
			}
			return r, err
		}
		r = next
	}
	return r, nil
}

// usePackagedProfile makes a reviewed Automatic setup select this build's
// packaged release profile; without one, Automatic waits for a profile review.
func usePackagedProfile(policy *tc.NetworkPolicy, status *tc.NetworkStatus) {
	if policy.Mode == "automatic" && policy.Profile == "" && status != nil && status.Builtin != nil && !status.Builtin.Expired {
		policy.Profile = status.Builtin.Digest
	}
	policy.AwaitingProfile = policy.Mode == "automatic" && policy.Profile == ""
}
