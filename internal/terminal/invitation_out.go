package terminal

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
)

// maxClipboardCode bounds the OSC 52 payload (base64 of the code). Terminals
// commonly cap OSC 52 near 100 KB; invitations are ~1.7 KB, at most 16 KiB.
const maxClipboardCode = 16 << 10

// plainReveal shows the invitation outside the panel UI (F05). The program
// pauses, the code is written to the ordinary screen as one unbroken line
// that the terminal wraps itself, so a selection copies it as one line with
// no borders or ellipsis. Enter returns; the screen and its scrollback are
// then cleared so the capability does not stay on screen.
type plainReveal struct {
	code string
	in   io.Reader
	out  io.Writer
}

func (r *plainReveal) SetStdin(in io.Reader)   { r.in = in }
func (r *plainReveal) SetStdout(out io.Writer) { r.out = out }
func (r *plainReveal) SetStderr(io.Writer)     {}
func (r *plainReveal) Run() error {
	fmt.Fprint(r.out, "\x1b[2J\x1b[H")
	fmt.Fprintf(r.out, "Orbit invitation (%s characters). Select the whole line below to copy it;\r\nit is one line even where your terminal wraps it.\r\n\r\n", humanCount(len(r.code)))
	fmt.Fprint(r.out, r.code)
	fmt.Fprint(r.out, "\r\n\r\nPress Enter to return (the screen is cleared).")
	_, err := bufio.NewReader(r.in).ReadString('\n')
	fmt.Fprint(r.out, "\x1b[2J\x1b[3J\x1b[H")
	if err == io.EOF {
		err = nil
	}
	return err
}

type revealDone struct{ err error }

// revealInvitation pauses the interface to show the code (v).
func (m *model) revealInvitation() tea.Cmd {
	code, err := tc.InvitationCode(m.flow.invitation)
	if err != nil {
		m.flow.err = "Invitation unavailable for display."
		return nil
	}
	return tea.Exec(&plainReveal{code: code}, func(err error) tea.Msg { return revealDone{err} })
}

// clipboardSupported is false where OSC 52 cannot work at all.
func clipboardSupported() bool {
	t := os.Getenv("TERM")
	return t != "" && t != "dumb" && t != "linux"
}

// copyInvitation sends the code to the terminal clipboard with OSC 52 (c).
// Terminals do not acknowledge OSC 52, so success is stated as sent.
func (m *model) copyInvitation() tea.Cmd {
	f := m.flow
	code, err := tc.InvitationCode(f.invitation)
	if p := f.result.Pairing; p != nil && p.Code != "" {
		code, err = p.Code, nil
	}
	switch {
	case err != nil:
		f.err = "Invitation unavailable for copying."
		return nil
	case !clipboardSupported():
		f.notice = "Copy not supported here. Press v to show the code, or s to save it to a file."
		return nil
	case len(code) > maxClipboardCode:
		f.notice = "The code is too long for the terminal clipboard. Press s to save it to a file."
		return nil
	}
	f.notice = "Sent " + humanCount(len(code)) + " characters to the clipboard (OSC 52). If pasting gives nothing, this terminal blocks it: press v or s."
	return tea.SetClipboard(code)
}

// defaultInvitationPath is a private file in the state directory (0700).
func (m *model) defaultInvitationPath() string {
	if m.opts.StateDir == "" {
		return ""
	}
	inv := m.flow.invitation
	// A digest of the whole invitation tells invitations apart without
	// putting any part of the capability in the file name.
	code, _ := tc.InvitationCode(inv)
	sum := sha256.Sum256([]byte(code))
	name := "invitation-" + safeFilePart(inv.Folder, 8) + "-" + hex.EncodeToString(sum[:4]) + ".json"
	return filepath.Join(m.opts.StateDir, name)
}

func safeFilePart(s string, n int) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= n {
			break
		}
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// savedTransferLines say how to move a saved file and join with it.
func savedTransferLines(path string) []string {
	host, _ := os.Hostname()
	if host == "" {
		host = "<this-device>"
	}
	return []string{
		"Saved: " + safe(path),
		"On the other device: scp " + safe(host) + ":" + safe(path) + " ~/orbit-invitation.json",
		"then: orbit join --invitation-file ~/orbit-invitation.json (or type that path in the join screen).",
	}
}
