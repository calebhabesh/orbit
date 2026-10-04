package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/calebhabesh/file-sync/internal/control"
	tc "github.com/calebhabesh/file-sync/internal/control/terminalcontract"
	"github.com/calebhabesh/file-sync/internal/controlclient"
	"github.com/calebhabesh/file-sync/internal/launcher"
)

// Retain legacy service verbs, with admission/replay supplied by the shared client.
func terminalServiceAction(ctx context.Context, dir, action, mode, requestFile string, jsonOutput bool, out io.Writer) error {
	c := &controlclient.Client{StateDir: dir}
	var r tc.Result
	var err error
	if action == "review" {
		r, err = c.Query(ctx, tc.Query{Version: tc.Version, Kind: "service"})
	} else {
		var m tc.Mutation
		if requestFile != "" {
			info, e := os.Lstat(requestFile)
			if e != nil {
				return e
			}
			if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > tc.MaxMetadata {
				return fmt.Errorf("request file must be private regular terminal metadata")
			}
			b, e := os.ReadFile(requestFile)
			if e != nil {
				return e
			}
			if e := tc.Decode(b, &m); e != nil {
				return e
			}
			if m.Kind != "service" {
				return fmt.Errorf("service request file requires a service intent")
			}
		} else {
			if action == "apply" {
				return fmt.Errorf("apply requires --request-file")
			}
			review, e := c.Query(ctx, tc.Query{Version: tc.Version, Kind: "service"})
			if e != nil {
				return e
			}
			var id [32]byte
			if _, e := rand.Read(id[:]); e != nil {
				return e
			}
			m = tc.Mutation{Version: tc.Version, Kind: "service", OperationID: hex.EncodeToString(id[:]), Service: &tc.ServiceIntent{Action: action, Mode: mode, Review: *review.Review}}
		}
		r, err = c.Mutate(ctx, m)
	}
	if err != nil {
		return err
	}
	if jsonOutput {
		if err := json.NewEncoder(out).Encode(r); err != nil {
			return err
		}
	} else if r.Operation != nil {
		fmt.Fprintf(out, "Service %s: %s (operation %s)\n", action, r.Operation.State, r.Operation.ID)
	} else {
		fmt.Fprintf(out, "Running: %v; enabled at login: %v; mode: %s\n", r.Service.Running, r.Service.Enabled, r.Service.Mode)
	}
	if r.Error != nil {
		return &control.ControlError{Code: r.Error.Code, Message: r.Error.Message, Retryable: r.Error.Retryable, Action: r.Error.Action}
	}
	return nil
}

// Reviewed runtime settings are separate from legacy presentation preferences.
func handleTerminalRuntime(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("orbit settings runtime", flag.ContinueOnError)
	flags.SetOutput(stderr)
	dirFlag := flags.String("state", "", "selected state directory")
	request := flags.String("request-file", "", "private reviewed settings mutation (same file for replay)")
	operation := flags.String("operation", "", "inspect a durable operation ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("runtime accepts no positional arguments")
	}
	dir, err := launcher.DiscoverState(*dirFlag)
	if err != nil {
		return err
	}
	c := &controlclient.Client{StateDir: dir}
	var r tc.Result
	if *request != "" {
		info, e := os.Lstat(*request)
		if e != nil {
			return e
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > tc.MaxMetadata {
			return fmt.Errorf("request file requires private regular bounded metadata")
		}
		b, e := os.ReadFile(*request)
		if e != nil {
			return e
		}
		var m tc.Mutation
		if err := tc.Decode(b, &m); err != nil {
			return err
		}
		if m.Kind != "settings" {
			return fmt.Errorf("runtime requires a settings mutation")
		}
		r, err = c.Mutate(context.Background(), m)
	} else if *operation != "" {
		r, err = c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "operation", ID: *operation})
	} else {
		r, err = c.Query(context.Background(), tc.Query{Version: tc.Version, Kind: "settings"})
	}
	if err != nil {
		return err
	}
	if err := json.NewEncoder(stdout).Encode(r); err != nil {
		return err
	}
	if r.Error != nil {
		return &control.ControlError{Code: r.Error.Code, Message: r.Error.Message, Action: r.Error.Action}
	}
	return nil
}
