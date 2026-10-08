package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"github.com/calebhabesh/orbit/internal/protocol"
	"io"
	"os"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/app"
	"github.com/calebhabesh/orbit/internal/config"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/launcher"
	"github.com/calebhabesh/orbit/internal/network"
)

func restartOrbitDaemon(ctx context.Context, dir string) error {
	if err := app.StopAgent(dir, 5*time.Second); err != nil {
		return err
	}
	_, err := launcher.EnsureDaemon(ctx, launcher.LaunchOptions{StateDir: dir, NoBrowser: true})
	return err
}

// networkReviewBytes bounds a reviewed network mutation carrying a signed
// profile and up to 16 KiB of custom service trust.
const networkReviewBytes = 64 << 10

// readServiceRoots reads owner-only PEM trust; the daemon validates it again.
func readServiceRoots(path string) ([]byte, error) {
	b, err := privateFile(path, config.MaxServiceRootsBytes)
	if err != nil {
		return nil, err
	}
	if _, _, err = config.ValidateServiceRoots(b, time.Now()); err != nil {
		return nil, errors.New("service roots must be 1–4 currently valid CA certificates in PEM")
	}
	return b, nil
}

func handleOrbitNetwork(args []string, out, errOut io.Writer) error {
	action := "status"
	if len(args) > 0 && args[0] != "" && args[0][0] != '-' {
		action = args[0]
		args = args[1:]
	}
	flags := flag.NewFlagSet("orbit network "+action, flag.ContinueOnError)
	flags.SetOutput(errOut)
	dirFlag := flags.String("state", "", "private state directory")
	lan := flags.Bool("lan-advertising", false, "broadcast signed device identity and direct listener on selected LAN interfaces")
	mode := flags.String("mode", "", "automatic, local_only, manual, self_hosted")
	profileFile := flags.String("profile-file", "", "private signed profile selection JSON; review authority and operator")
	rootsFile := flags.String("service-roots", "", "PEM CA trust for a self-hosted service (reviewed with --profile-file)")
	reviewFile := flags.String("review-file", "", "private mutation output for preview, input for apply")
	yes := flags.Bool("yes", false, "set/automatic/update: apply the shown review without asking")
	replace := flags.Bool("replace-operator", false, "confirm switching to a profile from a different operator")
	decline := flags.Bool("decline", false, "automatic: keep manual connections and stop offering Automatic")
	timings := map[string]*time.Duration{}
	for _, name := range []string{"direct-head-start", "connection-cycle", "direct-probe", "direct-cooldown", "network-poll", "network-quiet"} {
		timings[name] = flags.Duration(name, 0, "Advanced routing timing; zero restores finite default")
	}
	device := flags.String("device", "", "exact reviewed device ID for a single peer probe")
	timeout := flags.Duration("timeout", 20*time.Second, "finite explicit doctor budget (1s to 20s)")
	jsonOut := flags.Bool("json", false, "structured output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("network accepts no positional arguments")
	}
	dir, err := launcher.DiscoverState(*dirFlag)
	if err != nil {
		return err
	}
	oneStep := action == "set" || action == "automatic" || action == "update"
	if action == "preview" || action == "apply" || oneStep {
		if _, err = launcher.EnsureDaemon(context.Background(), launcher.LaunchOptions{StateDir: dir, NoBrowser: true}); err != nil {
			return err
		}
	}
	client := &controlclient.Client{StateDir: dir}
	ctx := context.Background()
	var r tc.Result
	switch action {
	case "status":
		r, err = client.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_status", ID: *device})
	case "doctor":
		if *timeout < time.Second || *timeout > 20*time.Second {
			return errors.New("doctor timeout must be between 1s and 20s")
		}
		probeCtx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		r, err = client.Query(probeCtx, tc.Query{Version: tc.Version, Kind: "network_doctor", ID: *device})
	case "preview", "set", "automatic", "update":
		if action == "preview" && *reviewFile == "" {
			return errors.New("network preview requires --review-file")
		}
		current, e := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_status"})
		if e != nil {
			return e
		}
		in := tc.NetworkIntent{Policy: current.Network.Policy, ReplaceOperator: *replace}
		explicit := map[string]bool{}
		changes := 0
		flags.Visit(func(f *flag.Flag) {
			explicit[f.Name] = true
			if f.Name != "state" && f.Name != "json" && f.Name != "yes" {
				changes++
			}
		})
		packaged := current.Network.Builtin
		switch action {
		case "automatic":
			if *mode != "" && *mode != "automatic" {
				return errors.New("network automatic selects mode automatic")
			}
			*mode = "automatic"
			if *decline {
				if current.Network.Policy.Mode != "manual" {
					return errors.New("--decline applies only to a manual install")
				}
				*mode = ""
				in.DeclineAutomaticOffer = true
				*yes = true
			} else if *profileFile == "" {
				if packaged == nil && !daemonHasCapability(ctx, client, tc.PackagedProfileCapability) {
					return errors.New("the running Orbit daemon predates this version; restart it (orbit service restart, or stop and rerun) and try again")
				}
				if packaged == nil || packaged.Expired {
					return errors.New("this Orbit build has no current packaged service profile; review one with --profile-file")
				}
				if current.Network.Policy.Mode == "automatic" && current.Network.Policy.Profile == packaged.Digest && changes == 0 {
					fmt.Fprintf(out, "Already using Automatic with %q.\n", packaged.Operator)
					return nil
				}
				in.Policy.Profile = packaged.Digest
			}
			if !*decline && !explicit["lan-advertising"] && current.Network.Policy.Mode == "manual" {
				in.Policy.LANAdvertising = true // fresh-setup default, shown in the review
			}
		case "update":
			if current.Network.ProfileUpdate != "available" || packaged == nil {
				fmt.Fprintln(out, "No packaged service profile update is waiting for review.")
				return nil
			}
			in.Policy.Profile = packaged.Digest
		}
		if *mode != "" {
			in.Policy.Mode = *mode
		}
		timingFields := map[string]*protocol.NetworkUint{"direct-head-start": &in.Policy.Timing.HeadStartMS, "connection-cycle": &in.Policy.Timing.CycleMS, "direct-probe": &in.Policy.Timing.ProbeMS, "direct-cooldown": &in.Policy.Timing.CooldownMS, "network-poll": &in.Policy.Timing.PollMS, "network-quiet": &in.Policy.Timing.QuietMS}
		var timingErr error
		flags.Visit(func(f *flag.Flag) {
			if dst, ok := timingFields[f.Name]; ok {
				d := *timings[f.Name]
				if d < 0 || d%time.Millisecond != 0 {
					timingErr = errors.New("timings require nonnegative whole milliseconds")
				} else {
					*dst = protocol.NetworkUint(d / time.Millisecond)
				}
			}
		})
		if timingErr != nil {
			return timingErr
		}
		flags.Visit(func(f *flag.Flag) {
			if f.Name == "lan-advertising" {
				in.Policy.LANAdvertising = *lan
			}
		})
		if in.Policy.Mode == "manual" {
			in.Policy.LANAdvertising = false
		}
		if in.Policy.Mode == "manual" || in.Policy.Mode == "local_only" {
			in.Policy.Profile = ""
		}
		if *profileFile != "" {
			var selection network.ProfileSelection
			if e = privateEnrollmentInput(*profileFile, &selection); e != nil {
				return e
			}
			if e = selection.Validate(uint64(time.Now().Unix())); e != nil {
				return e
			}
			if *rootsFile != "" {
				roots, e := readServiceRoots(*rootsFile)
				if e != nil {
					return e
				}
				in.ServiceRoots = string(roots)
			}
			in.Profile = &selection.Profile
			in.Authority = selection.Authority
			in.Environment = selection.Environment
			in.Policy.Profile, e = selection.Digest()
			if e != nil {
				return e
			}
		}
		if *rootsFile != "" && *profileFile == "" {
			return errors.New("--service-roots is reviewed together with --profile-file")
		}
		in.Policy.AwaitingProfile = in.Policy.Mode == "automatic" && in.Policy.Profile == ""
		r, err = client.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_preview", NetworkPlan: &in})
		if oneStep && !in.ReplaceOperator && operatorChange(r, err) {
			fmt.Fprintf(errOut, "This profile is from a different operator than the one this device uses (%q).\nPeers reach each other through Orbit services only while they use the same operator.\n", current.Network.Operator)
			if !*yes && !confirmCLI(errOut, "Replace the operator? [y/N] ", false) {
				return errors.New("operator not replaced")
			}
			in.ReplaceOperator = true
			r, err = client.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_preview", NetworkPlan: &in})
		}
		if err != nil {
			return err
		}
		if r.Error != nil {
			break
		}
		in.Policy = r.Network.Policy
		in.Review = *r.Review
		id, e := setupID()
		if e != nil {
			return e
		}
		m := tc.Mutation{Version: tc.Version, OperationID: id, Kind: "network", Network: &in}
		if action == "preview" {
			if err = writeSetupRequest(*reviewFile, m); err != nil {
				return err
			}
			break
		}
		if !in.DeclineAutomaticOffer {
			networkReviewSummary(errOut, current.Network, r.Network)
		}
		if !*yes {
			if !cliTTY() {
				return errors.New("network " + action + " asks for confirmation; pass --yes, or use preview/apply with --review-file")
			}
			if !confirmCLI(errOut, "Apply? [Y/n] ", true) {
				return errors.New("network change not applied")
			}
		}
		r, err = applyNetworkMutation(ctx, client, dir, m)
	case "apply":
		var m tc.Mutation
		if *reviewFile == "" {
			return errors.New("network apply requires a reviewed --review-file")
		}
		if err = privateBoundedInput(*reviewFile, networkReviewBytes, &m); err != nil {
			return err
		}
		if m.Kind != "network" {
			return errors.New("expected reviewed network mutation")
		}
		r, err = applyNetworkMutation(ctx, client, dir, m)
	default:
		return errors.New("network requires status, doctor, set, automatic, update, preview, or apply")
	}
	if err != nil {
		return err
	}
	if *jsonOut {
		if err = json.NewEncoder(out).Encode(r); err != nil {
			return err
		}
		if r.Error != nil {
			return &CLIExitError{Code: tc.ExitCode(r)}
		}
		return nil
	}
	if r.Network != nil {
		n := r.Network
		fmt.Fprintf(out, "Connection: %s; code=%s; ready=%v; restart_required=%v\n", n.Policy.Mode, n.Code, n.Ready, n.RestartRequired)
		effective := n.Policy.Timing.Effective()
		fmt.Fprintf(out, "Advanced timing: direct head start=%dms; cycle=%dms; probe=%dms; max cooldown=%dms; network poll=%dms; quiet=%dms\n", effective.HeadStartMS, effective.CycleMS, effective.ProbeMS, effective.CooldownMS, effective.PollMS, effective.QuietMS)
		if n.Operator != "" {
			fmt.Fprintf(out, "Operator: %q\nPrivacy: %q\n", n.Operator, n.Privacy)
		}
		if n.ProfileState != "" {
			fmt.Fprintf(out, "Profile: %s", n.ProfileState)
			if n.ProfileExpires != "" {
				fmt.Fprintf(out, "; expires=%s", n.ProfileExpires)
			}
			fmt.Fprintln(out)
		}
		if b := n.Builtin; b != nil {
			state := "current"
			if b.Expired {
				state = "expired"
			}
			fmt.Fprintf(out, "Packaged profile: %q epoch %d; expires=%s (%s)\n", b.Operator, b.Epoch, b.Expires, state)
		}
		fmt.Fprintln(out, "Orbit services see device addresses and connection metadata; file contents stay encrypted in transit.")
		fmt.Fprintf(out, "Next action: %s\n", n.Action)
		for _, probe := range n.Probes {
			fmt.Fprintf(out, "Probe %s: %s; observed=%s\n", probe.Kind, probe.Code, probe.ObservedAt)
		}
		for _, o := range n.Observations {
			fmt.Fprintf(out, "Device %s: %s; code=%s; observed=%s\n", o.Device, o.Route, o.Code, o.ObservedAt)
			fmt.Fprintf(out, "  Freshness: %s; candidates LAN=%d public=%d expired=%d; UDP=%s\n  Next action: %s\n", o.Freshness, o.LANCandidates, o.PublicCandidates, o.ExpiredCandidates, o.UDPCode, o.Action)
		}
	}
	if r.Operation != nil {
		fmt.Fprintf(out, "Policy operation: %s; state=%s\n", r.Operation.ID, r.State)
	}
	if r.Error != nil {
		return &CLIExitError{Code: tc.ExitCode(r), Err: errors.New(r.Error.Code)}
	}
	return nil
}

// applyNetworkMutation commits a reviewed network mutation and restarts the
// daemon when the new policy needs it.
func applyNetworkMutation(ctx context.Context, client *controlclient.Client, dir string, m tc.Mutation) (tc.Result, error) {
	r, err := client.Mutate(ctx, m)
	if err != nil || r.Error != nil || r.State != "completed" {
		return r, err
	}
	status, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "network_status"})
	if err != nil {
		return r, err
	}
	if status.Network.RestartRequired {
		err = restartOrbitDaemon(ctx, dir)
	}
	return r, err
}

func operatorChange(r tc.Result, err error) bool {
	if err != nil {
		return strings.Contains(err.Error(), "PROFILE_OPERATOR_CHANGE")
	}
	return r.Error != nil && r.Error.Code == "PROFILE_OPERATOR_CHANGE"
}

// confirmCLI asks one question on the terminal; empty input takes the default.
func confirmCLI(errOut io.Writer, prompt string, def bool) bool {
	answer, err := cliAnswer(bufio.NewReader(os.Stdin), errOut, prompt)
	if err != nil {
		return false
	}
	switch strings.ToLower(answer) {
	case "":
		return def
	case "y", "yes":
		return true
	}
	return false
}

// networkReviewSummary shows what a one-step network change will do.
func networkReviewSummary(w io.Writer, before, after *tc.NetworkStatus) {
	fmt.Fprintf(w, "Connection: %s -> %s\n", connectionLabel(before.Policy.Mode), connectionLabel(after.Policy.Mode))
	if after.Operator != "" {
		fmt.Fprintf(w, "Operator: %q", after.Operator)
		if after.ProfileExpires != "" {
			fmt.Fprintf(w, " (profile expires %s)", after.ProfileExpires)
		}
		fmt.Fprintf(w, "\nPrivacy: %s\n", after.Privacy)
	}
	if after.Policy.Mode != "manual" {
		fmt.Fprintf(w, "LAN advertising: %t (signed device identity and listener addresses on this network)\n", after.Policy.LANAdvertising)
	}
	if after.Policy.Mode == "automatic" || after.Policy.Mode == "self_hosted" {
		fmt.Fprintln(w, "Orbit services see device addresses and connection metadata; file contents stay encrypted in transit.")
	}
	fmt.Fprintln(w, "Identity, keys, approvals, history and files are unchanged.")
}

func connectionLabel(mode string) string {
	switch mode {
	case "automatic":
		return "Automatic"
	case "local_only":
		return "Local network only"
	case "self_hosted":
		return "Self-hosted"
	}
	return "Manual"
}

func daemonHasCapability(ctx context.Context, client *controlclient.Client, capability string) bool {
	r, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "capabilities"})
	if err != nil {
		return false
	}
	for _, c := range r.Capabilities {
		if c == capability {
			return true
		}
	}
	return false
}
