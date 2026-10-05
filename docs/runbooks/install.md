# Linux installation, legacy adoption and upgrade

Use the amd64 or arm64 archive, Debian or RPM package for your Linux host. The
binary is static and includes pure-Go SQLite and dependency license notices.
No browser/Node/GUI runtime is needed. Optional trusted editor/diff commands use
util-linux `prlimit`. A systemd user manager is required for managed startup;
manual daemon startup is supported separately.

Verify the downloaded set before installing:

```sh
sha256sum -c SHA256SUMS
```

For standalone installation, extract into a new directory and run `bash install.sh
user`. It installs binaries under `~/.local/bin`, a terminal desktop launcher,
completions/runbooks under `~/.local/share`, and user service aliases. Add
`~/.local/bin` to PATH. `bash install.sh system` installs shared binaries/assets
and requires privilege. Install `.deb` with `sudo dpkg -i <package>` or `.rpm`
with `sudo rpm -Uvh <package>`. No format enables startup automatically.
Standalone upgrades preserve existing user unit content; review/customize units
explicitly. Existing active user services may be restarted once by the installer.
Debian/RPM units use filesync.service with orbit.service as its alias, not a
second daemon. Package-manager scripts cannot configure another user's manager.

Run `orbit` for keyboard create/join; the client starts/reuses a daemon for its
selected state. Scripts use `orbit status`/`orbit --json`. The desktop entry
requires a terminal emulator chosen by your desktop. Independent headless commands
and `filesync serve` need neither desktop nor terminal emulator.

## Existing installations

Preserve `config.json`, peer identity/certificates, SQLite/WAL, object store,
operation spools, registered roots and `.filesync-internal` scratch. Do not copy
old metadata over a running/current identity. Orbit discovers
`$XDG_STATE_HOME/filesync` (default `~/.local/state/filesync`) or `~/.filesync`.
If both exist, use `--state /absolute/selected/state`; it will not guess or merge.
The selected path stays in place; switching entry names is not an identity reset.

Before upgrading, stop the selected daemon and obtain a consistent backup:

```sh
filesync stop --state /absolute/selected/state
filesync maintenance backup --state /absolute/selected/state --out /private/pre-upgrade.sqlite
filesync maintenance preflight --state /absolute/selected/state
```

Keep a protected copy of private configuration/keys/runtime settings and content
objects as well; a SQLite backup alone is not a content backup. Install the binary,
then inspect `orbit maintenance check --state /absolute/selected/state` and
`orbit doctor --state /absolute/selected/state`. Supported additive schema migrations
occur under exclusive state ownership when opened. Newer schemas are refused;
reinstall a compatible binary. Partial identity-recovery markers fence replication;
follow [recovery](database-recovery.md), never manually clear them. Binary rollback
requires compatible schema **and** terminal operation/runtime support.

Missing finite limits produce `LIMITS_REVIEW_REQUIRED`, not an invented budget.
Query `orbit settings runtime --state <state>`; review the returned settings/review,
and save a private typed settings mutation with a fresh operation ID through
`orbit settings runtime --request-file <private-request> --state <state>`.
Use the same file after a lost response. The controller commits intent before
writing runtime.json and recovers interrupted accepted work; completed replay
returns the same effect. Budget review does not recapture history, reset counters,
move roots or change device keys. A request has `version: "1"`, `kind: "settings"`, a random 64-hex
`operation_id`, and `settings: {review: <returned review>, settings: <reviewed
settings object>}`. Budget/concurrency integers are decimal JSON strings. Retain
all existing listener/advertisement/startup fields when reviewing limits; leave
retention_seconds at zero. The private file must be a regular file with mode 0600.
The typed format is documented in
[terminal control schema](../../schemas/terminal-control-v1.md).

Older peers that lack enrollment/terminal capabilities require compatible upgrades
or the documented legacy controls, rather than a verification bypass. Peer wire
versions/membership checks remain unchanged. Relocation still uses reviewed source
and destination; a cross-filesystem move retains the original as a safety copy.
Do not remove retained source/staging roots until the recorded operation is inspected.

## Startup modes

Manual: run `filesync serve --state /absolute/selected/state` in a supervised
session, or let the TUI start its separate daemon. Managed login startup uses:

```sh
orbit service enable --mode login --state /absolute/selected/state
orbit service start --state /absolute/selected/state
orbit service status --state /absolute/selected/state
```

The shipped unit selects `~/.local/state/filesync`; a legacy/custom state needs
an operator-reviewed unit/drop-in with matching ExecStart and ExecStop paths.
Keep customized listener flags, profile, resource limits and startup policy.
Never enable an alias for a second state expecting it to adopt a different daemon.

Unattended startup also needs a working user manager after logout/boot. Review
`loginctl show-user "$USER" -p Linger`; deliberately configure lingering under
your host policy, then `orbit service enable --mode unattended`. Orbit never
changes that privileged policy. Verify actual logout/login/boot and subsequent
capture on the intended host; local containers do not establish those behaviors.

Configure reachable LAN/Tailscale peer and enrollment addresses in Setup Advanced
or reviewed runtime settings, then restart the selected daemon if listeners change.
Owner control stays on loopback. See [network prerequisites](private-network.md),
[terminal operator guide](terminal-operator.md) and [safe uninstall](uninstall.md).
