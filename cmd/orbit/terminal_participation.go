package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"crypto/rand"
	"encoding/hex"
	"github.com/calebhabesh/orbit/internal/control"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
	"github.com/calebhabesh/orbit/internal/launcher"
	statestore "github.com/calebhabesh/orbit/internal/state"
)

func handleOrbitLeave(args []string, stdout, stderr io.Writer) error {
	f := flag.NewFlagSet("orbit leave", flag.ContinueOnError)
	f.SetOutput(stderr)
	state := f.String("state", "", "agent state directory")
	folder := f.String("folder", "", "Orbit name or ID")
	yes := f.Bool("yes", false, "confirm Leave; stop all sync for this Orbit here and keep files")
	jsonOut := f.Bool("json", false, "structured JSON")
	pos, rest := parsePositionalFolder(args, "state", "folder")
	if err := f.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *folder == "" {
		*folder = pos
	}
	dir, err := launcher.DiscoverState(*state)
	if err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: dir}
	ctx := context.Background()
	resolved, err := ResolveContext(ctx, client, ContextOptions{StateDir: dir, Folder: *folder, AllowEmptyPath: true})
	if err != nil {
		return err
	}
	if resolved.Result.Error != nil {
		return &CLIExitError{Code: RenderResult(stdout, stderr, resolved.Result, *jsonOut)}
	}
	if !*yes {
		if *jsonOut {
			return json.NewEncoder(stdout).Encode(map[string]any{"state": "preview", "folder": resolved.Folder, "root": resolved.Root, "action": "repeat with --yes"})
		}
		fmt.Fprintf(stdout, "Leave %s: stop sending and receiving this Orbit's data here. Files stay in %s.\nUnsynced edits may exist only here. Other devices must remove this one; rejoining needs fresh enrollment.\nRepeat with --yes to confirm.\n", EscapeTerminal(resolved.FolderName), EscapeTerminal(resolved.Root))
		return nil
	}
	id, err := parseID(resolved.Folder)
	if err != nil {
		return err
	}
	if err = client.LeaveOrbit(ctx, control.LeaveOrbitRequest{Folder: id, ExpectedRoot: resolved.Root}); err != nil {
		return &CLIExitError{Code: RenderError(stderr, err, *jsonOut)}
	}
	if *jsonOut {
		return json.NewEncoder(stdout).Encode(map[string]string{"state": "left", "folder": resolved.Folder, "files_kept": resolved.Root})
	}
	fmt.Fprintf(stdout, "Left %s; files kept in %s.\n", EscapeTerminal(resolved.FolderName), EscapeTerminal(resolved.Root))
	return nil
}

func handleOrbitRemoveDevice(args []string, stdout, stderr io.Writer) error {
	f := flag.NewFlagSet("orbit remove-device", flag.ContinueOnError)
	f.SetOutput(stderr)
	state := f.String("state", "", "agent state directory")
	folder := f.String("folder", "", "Orbit name or ID")
	device := f.String("device", "", "device name or ID")
	reviewFile := f.String("review-file", "", "write a private reviewed request here")
	requestFile := f.String("request-file", "", "execute/resume this exact reviewed request")
	confirm := f.String("confirm-name", "", "type the device name exactly")
	jsonOut := f.Bool("json", false, "structured JSON")
	pos, rest := parsePositionalFolder(args, "state", "folder", "device", "review-file", "request-file", "confirm-name")
	if err := f.Parse(rest); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *device == "" {
		*device = pos
	}
	dir, err := launcher.DiscoverState(*state)
	if err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: dir}
	ctx := context.Background()
	if *requestFile != "" {
		absolute, err := filepath.Abs(*requestFile)
		if err != nil {
			return err
		}
		raw, err := controlclient.PrivateInvitation(ctx, absolute)
		if err != nil {
			return err
		}
		var req control.RemoveDeviceRequest
		if err = tc.Decode(raw, &req); err != nil {
			return err
		}
		if *confirm == "" {
			return errors.New("--confirm-name is required; type the reviewed device name exactly")
		}
		req.ConfirmName = *confirm
		out, err := client.RemoveDevice(ctx, req)
		if err != nil {
			return &CLIExitError{Code: RenderError(stderr, err, *jsonOut)}
		}
		if *jsonOut {
			err = json.NewEncoder(stdout).Encode(out)
		} else {
			fmt.Fprintf(stdout, "%s\nOperation: %s (%s)\n", EscapeTerminal(out.Message), out.OperationID, out.State)
		}
		if err != nil {
			return err
		}
		if out.State != "completed" {
			return &CLIExitError{Code: 3}
		}
		return nil
	}
	resolved, err := ResolveContext(ctx, client, ContextOptions{StateDir: dir, Folder: *folder, AllowEmptyPath: true})
	if err != nil {
		return err
	}
	if resolved.Result.Error != nil {
		return &CLIExitError{Code: RenderResult(stdout, stderr, resolved.Result, *jsonOut)}
	}
	q := tc.Query{Version: tc.Version, Kind: "devices"}
	if isHex64(*device) {
		q.ID = *device
	} else {
		q.Name = *device
	}
	res, err := client.Query(ctx, q)
	if err != nil {
		return err
	}
	if len(res.Items) != 1 {
		return errors.New("select one exact device name or ID with --device")
	}
	preview, err := client.RetirementPreview(ctx, resolved.Folder, res.Items[0].ID)
	if err != nil {
		return err
	}
	var random [32]byte
	if _, err = rand.Read(random[:]); err != nil {
		return err
	}
	op := hex.EncodeToString(random[:])
	req := control.RemoveDeviceRequest{Folder: preview.Folder, DeviceID: preview.DeviceID, OperationID: op, MembershipDigest: preview.MembershipDigest, SnapshotDigest: preview.SnapshotDigest}
	if *reviewFile != "" {
		b, err := json.MarshalIndent(req, "", "  ")
		if err != nil {
			return err
		}
		absolute, err := filepath.Abs(*reviewFile)
		if err != nil {
			return err
		}
		if err := statestore.ValidatePrivateFileDir(filepath.Dir(absolute)); err != nil {
			return err
		}
		if err := writeContentReviewFile(ctx, client, absolute, append(b, '\n')); err != nil {
			return err
		}
	}
	if *jsonOut {
		return json.NewEncoder(stdout).Encode(preview)
	}
	fmt.Fprintf(stdout, "Remove %s from %s\nReceived here: %d recorded changes. Total on that device: unknown.\n%s\nEvery surviving device must agree on this history. Other Orbits keep syncing.\nSave with --review-file PATH, then execute/resume with --request-file PATH --confirm-name %q.\n", EscapeTerminal(preview.DeviceName), EscapeTerminal(resolved.FolderName), preview.ReceivedChanges, EscapeTerminal(preview.Disclaimer), EscapeTerminal(preview.DeviceName))
	return nil
}
