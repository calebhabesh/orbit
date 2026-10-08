package main

import (
	"fmt"
	"io"
	"strings"
)

func printOrbitHelp(stdout io.Writer) {
	fmt.Fprintln(stdout, `Orbit - Local file synchronization and everyday workspace management

Usage:
  orbit                    Keyboard interface in a TTY; concise status in pipes
  orbit [command] [options]
  orbit [command] --help

Everyday Commands:
  tui                      Keyboard interface (TTY); concise status in pipes
  status                   Concise folder/copy progress, attention, daemon and startup facts
  context [path]           Inspect folder identity, registered root, and current path context
  folders                  List synced folders and registered roots
  devices                  List enrolled devices, connection mode, and last contact
  conflicts                List files requiring version or structure review
  history <path>           List known versions and content availability
  deleted                  Find deleted files and available restore candidates
  restore <path>           Review and restore available history or a separate copy
  export <path>            Recover an exact version to an external destination

Setup & Sharing:
  setup                    Create or resume first-device setup
  join                     Join an existing Orbit using a private invitation
  folders add <root>       Preview and register an additional synced folder
  folders share            Submit a private reviewed folder/device share request
  devices invite           Show a one-line invitation code (or save a private file)
  devices approve          Apply an exact reviewed device/folder approval
  devices add              Retained invitation compatibility command
  devices requests         Review, approve, or decline enrollment requests

Management & Diagnostics:
  folders pause <name>     Pause local synchronization for a folder
  folders resume <name>    Resume local synchronization for a folder
  folders relocate <name>  Change the local filesystem path for a folder
  retry --task ID | --all  Retry work that ran out of attempts (works while Orbit runs)
  service                  Manage background service (status, enable, start, stop, restart)
  stop                     Stop a background daemon started outside the service
  storage                  Inspect storage usage, retention, and maintenance
  network status           Cached policy, service readiness and observed peer routes
  network doctor           Explicit bounded DNS/TLS/directory/relay/direct/UDP checks
  network automatic        Turn on Automatic with the packaged service profile (asks once)
  network update           Review a newer packaged service profile (asks once)
  network set              Change mode/profile/LAN advertising with one confirmation
  network preview/apply    Two-step review files for scripting
  doctor                   Actionable diagnostics and safe recovery advice
  legacy-browser           Explicit frozen browser compatibility launcher
  completion [shell]       Generate shell completions (bash, zsh, fish)
  version                  Display product version, schema, and build metadata
  engine <command>         Low-level engine commands (scan, sync, membership, work, resolve; see orbit engine help)

Options:
  --state <path>           Explicit agent state directory
  --folder <name|id>       Target a specific synced folder by name or 64-hex ID
  --json                   Structured JSON output for automation
  --help, -h               Show command help

Run 'orbit help <command>' for detailed command documentation.`)
}

func handleOrbitHelp(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printOrbitHelp(stdout)
		return nil
	}

	cmd := strings.ToLower(args[0])
	switch cmd {
	case "tui":
		fmt.Fprintln(stdout, `orbit tui - Keyboard interface

Usage: orbit tui [--state <path>] [--no-color]

j/k or arrows select; Tab and / focus page search; Enter inspects;
Esc returns; ? shows help; q and Ctrl-C close the interface.
Closing the interface leaves the daemon and committed work running.
Bare orbit uses the same entry. Pipes print concise status; --json prints JSON.
An explicitly configured --tool and --tool-file exercise external tool
handoff on scratch files. --editor and --diff configure reviewed conflict tools.`)
	case "launch", "legacy-browser":
		fmt.Fprintln(stdout, "orbit legacy-browser [--state <path>] [--no-browser]\nExplicit browser compatibility entry; launch is its retained alias.\nKeep bootstrap URLs private. Ordinary Orbit entry uses the terminal.")
	case "status":
		fmt.Fprintln(stdout, `orbit status - Display concise folder and daemon synchronization status

Usage:
  orbit status [options]

Options:
  --state <path>   Explicit agent state directory
  --json           Structured JSON output

Description:
  Shows background daemon state, service configuration, and summary of
  registered synced folders and attention items.`)

	case "context":
		fmt.Fprintln(stdout, `orbit context - Inspect folder identity and path resolution

Usage:
  orbit context [path] [options]
  orbit context -- [path]

Options:
  --folder <name|id>  Target synced folder by name or 64-hex ID
  --state <path>      Explicit agent state directory
  --json              Structured JSON output

Description:
  Resolves the current working directory or explicit path into a registered
  folder identity, root directory, relative path, and context generation.
  If the current directory is outside a synced folder, candidates are listed.
  Use '--' to pass literal paths with leading dashes or special characters.`)

	case "folders":
		fmt.Fprintln(stdout, `orbit folders - Manage synced folders and local roots

Usage:
  orbit folders [options]
  orbit folders list [options]
  orbit folders add <path> [options]
  orbit folders share --request-file <private-share.json> [options]
  orbit folders pause <name> [options]
  orbit folders resume <name> [options]
  orbit folders relocate <name> [options]

Options:
  --state <path>      Explicit agent state directory
  --json              Structured JSON output

Description:
  Lists registered synced folders or manages folder configuration.
  Sharing requires a private typed share mutation with exact folder, device,
  key pin, membership revision and controller review. The TUI s workflow
  obtains that review interactively; transfer output stays private.
  'orbit folders' with no subcommand defaults to listing all folders.`)

	case "devices":
		fmt.Fprintln(stdout, `orbit devices - Manage enrolled devices and enrollment requests

Usage:
  orbit devices [options]
  orbit devices list [options]
  orbit devices invite --folder <name> --code
  orbit devices invite --folder <name> --out <private-file>
  orbit devices requests show --device <name> --review-file <private-review>
  orbit devices approve --review-file <private-review>
  orbit devices add [options]
  orbit devices requests [options]
  orbit devices endpoint [options]

Options:
  --folder <name|id>  Folder scope for device actions
  --state <path>      Explicit agent state directory
  --json              Structured JSON output

Description:
  Lists known devices or manages device invitations and approvals.
  'orbit devices' with no subcommand defaults to listing all devices.`)

	case "history":
		fmt.Fprintln(stdout, `orbit history - List known versions for a file

Usage:
  orbit history <path> [options]
  orbit history -- <path> [options]

Options:
  --folder <name|id>  Target synced folder by name or 64-hex ID
  --state <path>      Explicit agent state directory
  --json              Structured JSON output

Description:
  Lists a bounded page of known versions and their actual content states.
  Use --limit and --cursor to continue; a changed history invalidates the cursor.`)

	case "deleted":
		fmt.Fprintln(stdout, `orbit deleted - List deleted files available for restore

Usage:
  orbit deleted [options]

Options:
  --folder <name|id>  Target synced folder by name or 64-hex ID
  --state <path>      Explicit agent state directory
  --json              Structured JSON output

Description:
  Lists a bounded page of deleted paths and actual historical candidates.
  Availability can be ready, pending, expired or corrupt; missing bytes disable restore.`)

	case "restore", "export", "conflicts":
		fmt.Fprintln(stdout, `Orbit reviewed conflict and recovery commands

Usage:
  orbit conflicts [--limit 50] [--cursor <cursor>]
  orbit conflicts show <path> --out <private-review.json>
  orbit conflicts select <path> --selected <author:counter> --review-file <review>
  orbit conflicts edit <path> --review-file <review> --tool 'editor --wait'
  orbit conflicts diff <path> --review-file <review> --tool 'diff -u'
  orbit conflicts upload <path> --review-file <review> --session <id> --file <result>
  orbit conflicts merge <path> --review-file <review> --session <id> --upload <id> --digest <sha256> --bytes <size>
  orbit conflicts keep-copies <path> --selected <author:counter> --review-file <review> --copies-file <plans.json>
  orbit conflicts session --session <id>
  orbit conflicts renew <path> --session <id> --review-file <review>
  orbit conflicts cancel --session <id>
  orbit conflicts discard <path> --session <id> --review-file <fresh-review>
  orbit restore <path> --version <author:counter> --out <private-review.json>
  orbit restore <path> --version <author:counter> --review-file <review>
  orbit restore <path> --version <author:counter> --to <relative-copy> --out <review>
  orbit export <path> --version <author:counter> --out <external-file>

Options:
  --folder <name|id>  Explicit folder; relative paths remain relative to the shell cwd
  --state <path>      Explicit state directory
  --operation <id>    Reuse this 64-hex identity after a lost mutation response
  --json             Structured output; tool output goes to stderr

Restore/select without --review-file only preview. New heads or working-byte changes
require a new review; no current heads are silently selected. Source bytes must be
locally available and verified. Original-path restore parents are the reviewed
current heads; the historical source is provenance. Separate-copy previews bind an
absent destination. Copy plans contain exact source, destination and its review.
External export never overwrites a destination or writes inside a synced root.

Editors use direct quoted argv and private source-NN/result paths, with util-linux
prlimit bounding result file size. Sessions last 300 seconds and can be explicitly
renewed before expiry. Inspect retained results after tool exit, interruption or
expiry. Cancel preserves candidates; discard requires a fresh explicit review.
See docs/runbooks/terminal-recovery.md for the complete copy/editor walkthrough.`)

	case "doctor":
		fmt.Fprintln(stdout, `orbit doctor - Run actionable diagnostics and recovery checks

Usage:
  orbit doctor [options]

Options:
  --state <path>   Explicit agent state directory
  --json           Structured JSON output

Description:
  Checks local daemon health, database integrity, root directory availability,
  network listener, and storage limits, reporting safe next actions for any issues.`)

	case "setup":
		fmt.Fprintln(stdout, `orbit setup - Create or adopt a synced folder

Usage:
  orbit setup [options]

Options:
  --root <path>    Folder to create or adopt (default ~/Orbit)
  --name <name>    Display name for folder
  --label <label>  Device name
  --join           Join an existing Orbit via invitation
  --preview        Measure root directory and output private preview
  --state <path>   Explicit agent state directory
  --json           Structured JSON output

Description:
  Fresh setup reviews Automatic connection; missing services leave local capture usable.
  Orbit services see addresses and connection metadata; content stays encrypted in transit.
  Choose --connection local_only before announcing. LAN discovery arrives in W08.
  Scripts use --preview --review-file FILE, then --request-file FILE.
  Existing manual installations keep their policy until reviewed opt-in.`)

	case "join":
		fmt.Fprintln(stdout, `orbit join - Join an existing Orbit using a private invitation

Usage:
  orbit join [options]
  orbit join --invitation-file <path> [options]

Options:
  --root <path>              Local directory for joined folder
  --invitation-file <path>   Path to private v2/v3 invitation file
  --invitation-stdin         Read bounded invitation from stdin
  --preview --review-file   Save the reviewed root/name/settings operation
  --request-file <path>     Apply/resume the exact private reviewed operation
  --state <path>             Explicit agent state directory
  --json                     Structured JSON output

Description:
  Authenticates an inviting device and submits an enrollment request to join a synced folder.`)

	case "network":
		fmt.Fprintln(stdout, `orbit network status [--state PATH] [--json]
orbit network doctor [--device EXACT_ID] [--timeout 20s] [--state PATH] [--json]
orbit network automatic [--lan-advertising=BOOL] [--yes] [--decline]
orbit network update [--yes]
orbit network set [--mode MODE] [--profile-file PRIVATE_SELECTION] [--service-roots PEM] [--replace-operator] [--yes]
orbit network preview [--mode MODE] --review-file PRIVATE_FILE [--profile-file PRIVATE_SELECTION]
orbit network apply --review-file PRIVATE_FILE

Modes: automatic, local_only, manual, self_hosted. Every Orbit build carries a
signed release profile for the hosted Orbit service; Automatic uses it. automatic,
update and set show the operator, privacy text and mode change, then ask
"Apply? [Y/n]" once (--yes for scripts). Upgraded manual installs are offered
Automatic once; automatic --decline keeps manual and stops the offer.
A newer packaged profile from the same operator with unchanged privacy text is
applied when the daemon starts; changed text waits for orbit network update.
Switching to another operator (for example self-hosting) asks for explicit
replacement; devices pair and relay through Orbit services only on one operator.
Profile selection includes an independently reviewed authority, environment and
signed operator/privacy text. Apply restarts the daemon when required.
Status uses cached observations; doctor explicitly probes the reviewed active policy.
Select one exact device ID for separate direct/relay identity checks; no peer fan-out.
Doctor times out within 20s and never infers NAT type from an unavailable result.
Advanced timing flags for set/preview: --direct-head-start, --connection-cycle,
--direct-probe, --direct-cooldown, --network-poll, --network-quiet (Go durations).
Zero restores a finite default; bounds and the exact intent are reviewed before apply.
Omit --mode to keep the current connection mode.
Advanced manual endpoints remain available through settings.`)
	case "service":
		fmt.Fprintln(stdout, `orbit service - Manage background daemon service

Usage:
  orbit service [status|start|stop|restart|enable|disable] [options]

Options:
  --mode <login|unattended>  Startup mode (login or unattended)
  --state <path>             Explicit agent state directory
  --json                     Structured JSON output

Description:
  Manages systemd user service for background syncing.`)

	case "completion":
		fmt.Fprintln(stdout, `orbit completion - Generate shell completion scripts

Usage:
  orbit completion [bash|zsh|fish]

Description:
  Generates shell completion scripts for tab-completion of commands and flags.
  Example:
    source <(orbit completion bash)`)

	case "storage":
		fmt.Fprintln(stdout, `orbit storage - Inspect storage usage, retention, and maintenance

Usage:
  orbit storage [options]
  orbit storage usage [options]
  orbit storage gc run [options]
  orbit storage check [options]
  orbit storage repair [options]
  orbit storage prune [options]

Options:
  --state <path>   Explicit agent state directory
  --json           Structured JSON output

Description:
  Reports workspace storage metrics, budgets, and maintenance actions.`)

	case "version":
		fmt.Fprintln(stdout, `orbit version - Display version and build information

Usage:
  orbit version [options]

Options:
  --json           Structured JSON output

Description:
  Displays product version, schema version, commit, build date, and runtime platform.`)

	default:
		return fmt.Errorf("unknown command %q for help; run 'orbit help' for available commands", cmd)
	}

	return nil
}
