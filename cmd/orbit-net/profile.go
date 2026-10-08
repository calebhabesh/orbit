package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/calebhabesh/orbit/internal/network"
	"github.com/calebhabesh/orbit/internal/protocol"
	"github.com/calebhabesh/orbit/internal/state"
)

// keygen creates the offline profile authority or an online service key. The
// private key never leaves its owner-only file; only the public key is printed.
func keygen(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("orbit-net keygen", flag.ContinueOnError)
	flags.SetOutput(errOut)
	path := flags.String("out", "", "new private key file in an owner-only directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" || flags.NArg() != 0 {
		return errors.New("keygen requires --out")
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	if err = writeExclusive(*path, []byte(hex.EncodeToString(private)+"\n")); err != nil {
		return err
	}
	fmt.Fprintln(out, hex.EncodeToString(public))
	return nil
}

// verifyKey checks restored custody material locally. In addition to matching
// the stored public half, it proves the private seed can sign for that authority.
// It produces neither a profile nor a transferable signature.
func verifyKey(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("orbit-net key verify", flag.ContinueOnError)
	flags.SetOutput(errOut)
	path := flags.String("file", "", "restored owner-only private key file")
	authority := flags.String("authority", "", "expected public authority key (hex)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" || *authority == "" || flags.NArg() != 0 {
		return errors.New("key verify requires --file and --authority")
	}
	public, err := hex.DecodeString(*authority)
	if err != nil || len(public) != ed25519.PublicKeySize {
		return errors.New("authority must be a hex Ed25519 public key")
	}
	key, err := readKey(*path)
	if err != nil {
		return fmt.Errorf("read restored authority key: %w", err)
	}
	if !bytes.Equal(key.Public().(ed25519.PublicKey), public) {
		return errors.New("restored key differs from the expected authority")
	}
	challenge := make([]byte, 32)
	if _, err := rand.Read(challenge); err != nil {
		return err
	}
	challenge = append([]byte("orbit-net offline backup verification v1\x00"), challenge...)
	if !ed25519.Verify(public, challenge, ed25519.Sign(key, challenge)) {
		return errors.New("restored key cannot sign for the expected authority")
	}
	fmt.Fprintf(out, "verified authority: %s\n", hex.EncodeToString(public))
	return nil
}

// writeExclusive never replaces an existing key, profile or archive.
func writeExclusive(path string, data []byte) error {
	if err := state.ValidateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}

// signProfile turns an operator template into a signed ProfileSelection. The
// authority key is the offline root of client trust; --previous enforces the
// same authority/environment and a strictly higher epoch, like clients do.
func signProfile(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("orbit-net profile sign", flag.ContinueOnError)
	flags.SetOutput(errOut)
	authorityKey := flags.String("authority-key", "", "private hex Ed25519 profile authority key")
	template := flags.String("template", "", "JSON profile template (no authority or signature)")
	environment := flags.String("environment", "", "release, self_hosted or development")
	validFor := flags.Duration("valid-for", 0, "set expires to now plus this duration instead of the template value")
	previous := flags.String("previous", "", "optional previously distributed selection; the new epoch must supersede it")
	path := flags.String("out", "", "new signed selection file")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *authorityKey == "" || *template == "" || *environment == "" || *path == "" || flags.NArg() != 0 {
		return errors.New("profile sign requires --authority-key, --template, --environment and --out")
	}
	key, err := readKey(*authorityKey)
	if err != nil {
		return fmt.Errorf("read authority key: %w", err)
	}
	data, err := readBounded(*template, protocol.NetworkMaxBytes)
	if err != nil {
		return err
	}
	var profile protocol.NetworkProfile
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&profile); err != nil {
		return fmt.Errorf("decode template: %w", err)
	}
	if profile.Signature != "" || (profile.Authority != "" && profile.Authority != hex.EncodeToString(key.Public().(ed25519.PublicKey))) {
		return errors.New("template must not carry a signature or a different authority")
	}
	now := time.Now()
	if *validFor > 0 {
		if profile.Expires != 0 {
			return errors.New("use either template expires or --valid-for")
		}
		profile.Expires = protocol.NetworkUint(now.Add(*validFor).Unix())
	}
	profile.Authority = hex.EncodeToString(key.Public().(ed25519.PublicKey))
	canonical, err := profile.Canonical(*environment != "release")
	if err != nil {
		return fmt.Errorf("template rejected (%v): release profiles need public HTTPS/WSS origins and public numeric STUN addresses", err)
	}
	profile.Signature = hex.EncodeToString(ed25519.Sign(key, canonical))
	var old *network.ProfileSelection
	if *previous != "" {
		prior, e := readSelection(*previous)
		if e != nil {
			return fmt.Errorf("previous selection: %w", e)
		}
		if profile.Epoch <= prior.Profile.Epoch {
			return errors.New("epoch must be higher than the previous selection; clients refuse rollback to an older epoch")
		}
		old = &prior
	}
	selection, err := network.ReviewProfile(old, profile, profile.Authority, *environment, uint64(now.Unix()))
	if err != nil {
		return fmt.Errorf("signed profile does not validate: %w", err)
	}
	encoded, err := json.MarshalIndent(selection, "", "  ")
	if err != nil {
		return err
	}
	if err = writeExclusive(*path, append(encoded, '\n')); err != nil {
		return err
	}
	return describe(out, selection)
}

func readSelection(path string) (network.ProfileSelection, error) {
	var selection network.ProfileSelection
	data, err := readBounded(path, protocol.NetworkMaxBytes)
	if err != nil {
		return selection, err
	}
	return selection, protocol.NetworkDecode(data, &selection)
}

func verifyProfile(args []string, out, errOut io.Writer) error {
	flags := flag.NewFlagSet("orbit-net profile verify", flag.ContinueOnError)
	flags.SetOutput(errOut)
	path := flags.String("profile", "", "signed selection file")
	authority := flags.String("authority", "", "optional expected authority public key (hex)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *path == "" || flags.NArg() != 0 {
		return errors.New("profile verify requires --profile")
	}
	selection, err := readSelection(*path)
	if err != nil {
		return err
	}
	if *authority != "" && !strings.EqualFold(*authority, selection.Authority) {
		return errors.New("profile authority differs from the expected authority")
	}
	if err = selection.Validate(uint64(time.Now().Unix())); err != nil {
		return fmt.Errorf("profile invalid or expired: %w", err)
	}
	return describe(out, selection)
}

func describe(out io.Writer, s network.ProfileSelection) error {
	digest, err := s.Digest()
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "operator:    %s\nenvironment: %s\nepoch:       %d\nexpires:     %s\nauthority:   %s\nservice key: %s\ndigest:      %s\n", s.Profile.Operator, s.Environment, s.Profile.Epoch, time.Unix(int64(s.Profile.Expires), 0).UTC().Format(time.RFC3339), s.Authority, s.Profile.ServiceKey, digest)
	for _, o := range s.Profile.Origins {
		fmt.Fprintf(out, "origin:      %s\n", o)
	}
	for _, a := range s.Profile.STUN {
		fmt.Fprintf(out, "stun:        %s\n", a)
	}
	return nil
}
