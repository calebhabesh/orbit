package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/calebhabesh/file-sync/internal/launcher"
	"io"
	"path/filepath"
	"strings"

	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/state"
)

func privateEnrollmentInput(path string, out any) error {
	dir, name := filepath.Split(path)
	if dir == "" {
		dir = "."
	}
	b, err := state.ReadPrivate(dir, name, 16384)
	if err != nil {
		return err
	}
	if strings.HasPrefix(strings.TrimSpace(string(b)), "orbit-invitation:v2:") {
		b, err = base64.RawURLEncoding.DecodeString(strings.TrimPrefix(strings.TrimSpace(string(b)), "orbit-invitation:v2:"))
		if err != nil {
			return err
		}
	}
	return tc.Decode(b, out)
}
func reviewedEnrollmentCLI(dir, action, request, alias, path, operation string, asJSON bool, out io.Writer) error {
	var reviewed tc.ApprovalIntent
	if err := privateEnrollmentInput(path, &reviewed); err != nil {
		return err
	}
	if reviewed.Request != request || reviewed.Decision != action {
		return fmt.Errorf("STALE_VIEW: review/request/decision mismatch")
	}
	if operation == "" {
		var id [32]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		operation = hex.EncodeToString(id[:])
	}
	client := &controlclient.Client{StateDir: dir}
	ctx := context.Background()
	if action == "decline" {
		req := control.DeclineEnrollmentRequest{RequestID: request, Reviewed: &reviewed, OperationID: operation}
		err := client.WithController(ctx, func() error { return client.Call(ctx, "POST", "/api/v1/enrollment/decline", req, nil) }, func(c *control.Controller) error { return c.DeclineEnrollmentRequest(ctx, req) })
		if err != nil {
			return err
		}
		if asJSON {
			return json.NewEncoder(out).Encode(map[string]string{"status": "declined", "operation_id": operation})
		}
		fmt.Fprintf(out, "Enrollment request declined: %s\nOperation: %s\n", request, operation)
		return nil
	}
	folder, err := parseID(reviewed.Folder)
	if err != nil {
		return err
	}
	req := control.ApproveEnrollmentRequest{RequestID: request, Folder: folder, Reviewed: &reviewed, OperationID: operation, SuggestedLabel: alias}
	var result control.ApproveEnrollmentResult
	err = client.WithController(ctx, func() error { return client.Call(ctx, "POST", "/api/v1/enrollment/approve", req, &result) }, func(c *control.Controller) error {
		r, e := c.ApproveEnrollmentRequest(ctx, req)
		if r != nil {
			result = *r
		}
		return e
	})
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(out).Encode(result)
	}
	fmt.Fprintf(out, "Enrollment request approved:\n  Request ID: %s\n  Revision:   %d\n  Digest:     %x\n  Operation:  %s\n", result.RequestID, result.Revision, result.Digest, operation)
	return nil
}

func shareFolderCLI(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("orbit folders share", flag.ContinueOnError)
	flags.SetOutput(errOut)
	dirFlag := flags.String("state", "", "selected private state")
	path := flags.String("request-file", "", "private reviewed share mutation with exact folder/device/membership")
	asJSON := flags.Bool("json", false, "structured explicit invitation transfer")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" {
		return fmt.Errorf("folders share requires --request-file with reviewed folder, device and membership")
	}
	var m tc.Mutation
	if err := privateEnrollmentInput(*path, &m); err != nil {
		return err
	}
	if m.Kind != "share" {
		return fmt.Errorf("INVALID_REQUEST: expected share mutation")
	}
	dir, err := launcher.DiscoverState(*dirFlag)
	if err != nil {
		return err
	}
	r, err := (&controlclient.Client{StateDir: dir}).Mutate(context.Background(), m)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(r)
	}
	b, err := json.Marshal(r.Invitation)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "Folder invitation (receiver must review its local root and request approval):")
	fmt.Fprintln(out, "orbit-invitation:v2:"+base64.RawURLEncoding.EncodeToString(b))
	return nil
}

func refreshPeerEndpointCLI(dir, folder, device, url, certificate string, asJSON bool, out io.Writer) error {
	if folder == "" || device == "" || url == "" {
		return fmt.Errorf("endpoint requires --folder, --device and --url")
	}
	ctx := context.Background()
	client := &controlclient.Client{StateDir: dir}
	req := control.SetPeerEndpointRequest{Folder: folder, Device: device, URL: url, Certificate: certificate}
	err := client.WithController(ctx, func() error { return client.Call(ctx, "POST", "/api/v1/settings/peers", req, nil) }, func(c *control.Controller) error { return c.SetPeerEndpoint(ctx, req) })
	if err != nil {
		return err
	}
	if asJSON {
		return json.NewEncoder(out).Encode(map[string]string{"status": "configured", "device": device, "folder": folder, "url": url})
	}
	fmt.Fprintf(out, "Endpoint configured: device=%s url=%s\n", device, url)
	return nil
}
