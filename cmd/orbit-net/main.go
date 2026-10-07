// orbit-net is the separately operated rendezvous, relay and STUN process. It
// never owns folders, device identities or owner control, and it never invents
// hosted endpoints: every origin comes from an operator-signed profile.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "orbit-net:", err)
		os.Exit(1)
	}
}

func run(args []string, out, errOut io.Writer) error {
	// Flag-only invocations remain the W03 serve form.
	if len(args) == 0 || (len(args[0]) > 0 && args[0][0] == '-') {
		return serve(args, errOut)
	}
	switch args[0] {
	case "serve":
		return serve(args[1:], errOut)
	case "keygen":
		return keygen(args[1:], out, errOut)
	case "key":
		if len(args) < 2 || args[1] != "verify" {
			return errors.New("key requires verify")
		}
		return verifyKey(args[2:], out, errOut)
	case "profile":
		if len(args) < 2 {
			return errors.New("profile requires sign or verify")
		}
		switch args[1] {
		case "sign":
			return signProfile(args[2:], out, errOut)
		case "verify":
			return verifyProfile(args[2:], out, errOut)
		}
		return errors.New("profile requires sign or verify")
	case "alert":
		return alert(args[1:], out, errOut)
	case "version":
		fmt.Fprintf(out, "orbit-net %s (%s, %s)\n", version, commit, date)
		return nil
	case "help", "-h", "--help":
		fmt.Fprint(out, usage)
		return nil
	}
	return fmt.Errorf("unknown command %q", args[0])
}

const usage = `orbit-net operates Orbit's connection service.

Commands:
  serve --config FILE [--check]   run rendezvous/relay (and optional STUN)
  keygen --out FILE               create an Ed25519 key; prints its public key
  key verify --file FILE --authority HEX
                                 verify a restored authority key without changing it
  profile sign ...                sign a profile template with the authority key
  profile verify --profile FILE   show and validate a signed profile selection
  alert --config FILE --state FILE
                                 one monitoring check; notifies on firing/recovery
  alert --config FILE --test      send one test notification
  version                         print build information

SIGHUP reloads the TLS certificate and key; SIGINT/SIGTERM drain and stop.
See docs/orbit-net-operator.md for provisioning, rotation and monitoring.
`
