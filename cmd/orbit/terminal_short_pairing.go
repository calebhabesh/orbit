package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	tc "github.com/calebhabesh/orbit/internal/control/terminalcontract"
	"github.com/calebhabesh/orbit/internal/controlclient"
)

func resolvePairingCLI(client *controlclient.Client, code, digest string, n *tc.NetworkStatus, interactive bool, ask func(string, string) (string, error), out io.Writer) (tc.Invitation, error) {
	if n == nil {
		return tc.Invitation{}, errors.New("PAIRING_UNAVAILABLE: operator information unavailable")
	}
	operator, privacy := n.Operator, n.Privacy
	if digest == "" {
		digest = n.Policy.Profile
	}
	if digest == "" && n.Builtin != nil && !n.Builtin.Expired && interactive {
		digest, operator, privacy = n.Builtin.Digest, n.Builtin.Operator, n.Builtin.Privacy
	}
	if digest == "" {
		return tc.Invitation{}, errors.New("short-code input requires a selected operator or --pairing-profile REVIEWED_DIGEST; use orbit network status to inspect the operator/privacy text")
	}
	if interactive {
		fmt.Fprintf(out, "Pairing through %s. %s\n", EscapeTerminal(operator), EscapeTerminal(privacy))
		answer, e := ask("Contact this operator once to retrieve the invitation? (yes/no)", "yes")
		if e != nil {
			return tc.Invitation{}, e
		}
		if answer != "yes" {
			return tc.Invitation{}, errors.New("pairing canceled")
		}
	}
	id, e := setupID()
	if e != nil {
		return tc.Invitation{}, e
	}
	r, e := client.Mutate(context.Background(), tc.Mutation{Version: tc.Version, Kind: "pairing", OperationID: id, Pairing: &tc.PairingIntent{Code: code, Profile: digest}})
	if e != nil {
		return tc.Invitation{}, e
	}
	if r.Error != nil {
		return tc.Invitation{}, fmt.Errorf("%s: %s", r.Error.Code, r.Error.Action)
	}
	if r.Invitation == nil {
		return tc.Invitation{}, errors.New("PAIRING_UNAVAILABLE: ask the inviting device for a new code")
	}
	return *r.Invitation, nil
}
