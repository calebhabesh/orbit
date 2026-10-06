package main

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/launcher"
)

func cliTTY() bool {
	_, err := unix.IoctlGetTermios(int(os.Stdin.Fd()), unix.TCGETS)
	return err == nil
}
func cliAnswer(reader *bufio.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	line, err := reader.ReadString('\n')
	return strings.TrimSpace(line), err
}
func wanFolder(ctx context.Context, c *controlclient.Client, name string) (string, error) {
	r, err := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "folders"})
	if err != nil {
		return "", err
	}
	var matches []tc.NamedItem
	for _, item := range r.Items {
		if name == "" || name == item.ID || name == item.Name {
			matches = append(matches, item)
		}
	}
	if len(matches) > 1 && name == "" && cliTTY() {
		for i, item := range matches {
			fmt.Fprintf(os.Stderr, "%d. Folder %q; root=%q\n", i+1, item.Name, item.Root)
		}
		answer, e := cliAnswer(bufio.NewReader(os.Stdin), os.Stderr, "Select folder number: ")
		if e != nil {
			return "", e
		}
		n, e := strconv.Atoi(answer)
		if e == nil && n > 0 && n <= len(matches) {
			return matches[n-1].ID, nil
		}
	}
	if len(matches) != 1 {
		return "", errors.New("select one folder with --folder NAME (names must be unambiguous)")
	}
	return matches[0].ID, nil
}
func handleWANInvite(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("orbit devices invite", flag.ContinueOnError)
	flags.SetOutput(errOut)
	dirFlag := flags.String("state", "", "private state directory")
	folder := flags.String("folder", "", "folder name or identity")
	transfer := flags.String("out", "", "absolute private invitation transfer file")
	showCode := flags.Bool("code", false, "print a one-line invitation code to paste on the other device (a secret: share it only with that device)")
	requestFile := flags.String("request-file", "", "private reviewed invitation mutation")
	reviewFile := flags.String("review-file", "", "private mutation output during preview")
	preview := flags.Bool("preview", false, "review folder/membership before issuing invitation")
	ttl := flags.Int64("ttl", 86400, "invitation seconds (maximum 86400)")
	asJSON := flags.Bool("json", false, "redacted structured result")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("invite accepts no positional arguments")
	}
	dir, err := launcher.DiscoverState(*dirFlag)
	if err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: dir}
	ctx := context.Background()
	var m tc.Mutation
	if *requestFile != "" {
		if err = privateEnrollmentInput(*requestFile, &m); err != nil {
			return err
		}
		if m.Kind != "invite" && m.Kind != "share" {
			return errors.New("expected invitation or share mutation")
		}
	} else {
		f, e := wanFolder(ctx, client, *folder)
		if e != nil {
			return e
		}
		fid, e := parseID(f)
		if e != nil {
			return e
		}
		var membership control.MembershipExportResult
		err = client.WithController(ctx, func() error {
			return client.Call(ctx, "POST", "/api/v1/membership/export?folder="+f, nil, &membership)
		}, func(c *control.Controller) error { var e error; membership, e = c.MembershipExport(ctx, fid); return e })
		if err != nil {
			return err
		}
		if *ttl <= 0 || *ttl > 86400 {
			return errors.New("ttl must be 1–86400 seconds")
		}
		id, e := setupID()
		if e != nil {
			return e
		}
		m = tc.Mutation{Version: tc.Version, Kind: "invite", OperationID: id, Invite: &tc.InviteIntent{Folder: f, ExpectedMembership: hex.EncodeToString(membership.Digest[:]), ExpiresAt: time.Now().Add(time.Duration(*ttl) * time.Second).UTC().Format(time.RFC3339Nano)}}
		if *preview {
			if *reviewFile == "" {
				return errors.New("invite preview requires --review-file")
			}
			if err = writeSetupRequest(*reviewFile, m); err != nil {
				return err
			}
			if *asJSON {
				return json.NewEncoder(out).Encode(m)
			}
			fmt.Fprintf(out, "Invitation review: folder=%s; membership=%s; expires=%s\n", f, m.Invite.ExpectedMembership, m.Invite.ExpiresAt)
			return nil
		}
		// --out deliberately transfers a scoped request capability; no hidden picker.
	}
	if *transfer == "" && !*showCode && cliTTY() {
		value, e := cliAnswer(bufio.NewReader(os.Stdin), errOut, "Press Enter to show a one-line code to paste on the other device, or type an absolute private file path: ")
		if e != nil {
			return e
		}
		*transfer = value
		*showCode = value == ""
	}
	if *transfer != "" && *showCode {
		return errors.New("choose either --code or --out")
	}
	if *transfer == "" && !*showCode {
		return errors.New("invite requires --code or --out PRIVATE_FILE; secrets are omitted from normal output")
	}
	r, err := client.Mutate(ctx, m)
	if err != nil {
		return err
	}
	if r.Invitation == nil {
		return errors.New("invitation was not issued")
	}
	if *showCode {
		code, e := tc.InvitationCode(*r.Invitation)
		if e != nil {
			return e
		}
		fmt.Fprintf(errOut, "Invitation for this folder only; expires %s. On the other device run `orbit join` and paste:\n", r.Invitation.ExpiresAt)
		fmt.Fprintln(out, code)
		return nil
	}
	if err = client.SaveInvitation(ctx, *transfer, *r.Invitation); err != nil {
		return err
	}
	r.Invitation = nil // normal status/JSON never contains the capability
	if *asJSON {
		return json.NewEncoder(out).Encode(r)
	}
	fmt.Fprintf(out, "Invitation saved privately to %q; operation=%s\n", *transfer, m.OperationID)
	return nil
}
func handleWANRequests(action string, args []string, out, errOut io.Writer) error {
	if len(args) > 0 && args[0] == "show" {
		args = args[1:]
	}
	flags := flag.NewFlagSet("orbit devices "+action, flag.ContinueOnError)
	flags.SetOutput(errOut)
	dirFlag := flags.String("state", "", "private state directory")
	folder := flags.String("folder", "", "folder name or identity")
	request := flags.String("request", "", "exact reviewed request identity")
	device := flags.String("device", "", "requesting device name or identity")
	reviewFile := flags.String("review-file", "", "private reviewed approval input/output")
	operation := flags.String("operation", "", "durable approval identity")
	decision := flags.String("decision", "approve", "approval review decision: approve or decline")
	asJSON := flags.Bool("json", false, "structured output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("requests accept no positional arguments")
	}
	dir, err := launcher.DiscoverState(*dirFlag)
	if err != nil {
		return err
	}
	client := &controlclient.Client{StateDir: dir}
	ctx := context.Background()
	if action != "show" {
		if *reviewFile == "" {
			if !cliTTY() {
				return errors.New("approval requires --review-file from devices requests show")
			}
			pending, e := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "requests", Limit: 200})
			if e != nil {
				return e
			}
			var choices []tc.EnrollmentRequest
			for _, req := range pending.Requests {
				if req.State == "pending_approval" && (*request == "" || *request == req.ID) && (*device == "" || *device == req.Label || *device == req.Requester) {
					choices = append(choices, req)
				}
			}
			if len(choices) == 0 {
				return errors.New("no matching pending requests")
			}
			reader := bufio.NewReader(os.Stdin)
			selected := 0
			for i, req := range choices {
				fmt.Fprintf(errOut, "%d. Device %q; folder=%s; key pin=%s; verification code=%s\n", i+1, req.Label, req.Folder, req.KeyPin, req.VerificationCode)
			}
			if len(choices) > 1 {
				answer, e := cliAnswer(reader, errOut, "Select request number: ")
				if e != nil {
					return e
				}
				n, e := strconv.Atoi(answer)
				if e != nil || n < 1 || n > len(choices) {
					return errors.New("invalid request selection")
				}
				selected = n - 1
			}
			req := choices[selected]
			answer, e := cliAnswer(reader, errOut, "Compare the verification code on both devices. Type "+action+" to confirm: ")
			if e != nil {
				return e
			}
			if answer != action {
				return errors.New("approval not confirmed")
			}
			id, e := setupID()
			if e != nil {
				return e
			}
			m := tc.Mutation{Version: tc.Version, OperationID: id, Kind: "approval", Approval: &tc.ApprovalIntent{Request: req.ID, Folder: req.Folder, Requester: req.Requester, KeyPin: req.KeyPin, TranscriptDigest: req.TranscriptDigest, ExpectedMembership: req.ExpectedMembership, Decision: action}}
			path := filepath.Join(dir, "approval-request-"+id+".json")
			if e = writeSetupRequest(path, m); e != nil {
				return e
			}
			*reviewFile = path
		}
		var m tc.Mutation
		if err = privateEnrollmentInput(*reviewFile, &m); err != nil {
			return err
		}
		if m.Kind != "approval" || m.Approval == nil {
			return errors.New("expected exact reviewed approval")
		}
		if m.Approval.Decision != action || (*request != "" && m.Approval.Request != *request) {
			return errors.New("STALE_VIEW: decision/request mismatch")
		}
		if *operation != "" {
			m.OperationID = *operation
		}
		r, e := client.Mutate(ctx, m)
		if e != nil {
			return e
		}
		code := RenderResult(out, errOut, r, *asJSON)
		if code != 0 {
			return &CLIExitError{Code: code}
		}
		return nil
	}
	f := ""
	if *folder != "" {
		f, err = wanFolder(ctx, client, *folder)
		if err != nil {
			return err
		}
	}
	r, err := client.Query(ctx, tc.Query{Version: tc.Version, Kind: "requests", Folder: f, Limit: 200})
	if err != nil {
		return err
	}
	selected := []tc.EnrollmentRequest{}
	for _, req := range r.Requests {
		if req.State == "pending_approval" && (*request == "" || req.ID == *request) && (*device == "" || req.Requester == *device || req.Label == *device) {
			selected = append(selected, req)
		}
	}
	r.Requests = selected
	if *reviewFile != "" {
		if len(selected) != 1 || (*request == "" && *device == "") {
			return errors.New("select one exact request with --request or unambiguous --device")
		}
		req := selected[0]
		id, e := setupID()
		if e != nil {
			return e
		}
		m := tc.Mutation{Version: tc.Version, Kind: "approval", OperationID: id, Approval: &tc.ApprovalIntent{Request: req.ID, Folder: req.Folder, Requester: req.Requester, KeyPin: req.KeyPin, TranscriptDigest: req.TranscriptDigest, ExpectedMembership: req.ExpectedMembership, Decision: *decision}}
		if err = writeSetupRequest(*reviewFile, m); err != nil {
			return err
		}
	}
	if *asJSON {
		return json.NewEncoder(out).Encode(r)
	}
	for _, req := range selected {
		fmt.Fprintf(out, "Device %q; folder=%s; request=%s\nKey pin: %s; verification code: %s; state=%s\n", req.Label, req.Folder, req.ID, req.KeyPin, req.VerificationCode, req.State)
	}
	if r.Cursor != "" {
		fmt.Fprintln(out, "More requests exist; use the existing paginated requests control.")
	}
	return nil
}
