# Orbit

The next owner-selected expansion is [native WAN connectivity and simpler setup](docs/orbit-wan-implementation-plan.md),
with preconfigured Orbit services and optional self-hosting. Its [W tracker](docs/implementation/wan-status.md)
records planned work; the networking capabilities in that plan are not implemented.
The existing product and validation evidence below retain their stated scope.

Orbit is a background Linux file sync daemon with a keyboard interface and
independent CLI commands. Ordinary files stay in local folders. SQLite records
causal history and recovery journals; verified content is retained in a private
content store and transferred over pinned mutual TLS. Concurrent edits require
an explicit review. Stored receipts describe dated observations, not a promise
that an offline device currently has your latest working bytes.

Run `orbit` in a terminal to create or join a folder and manage everyday sync.
Closing it leaves the daemon running. In a pipe, `orbit` prints concise status;
`orbit --json` returns structured status without prompts or terminal escapes.
No browser, GUI runtime, Node or C compiler is required for ordinary operation.
Trusted external editors/diff tools are optional; their bounded invocation uses
util-linux `prlimit`.

```sh
make build build-arm64
./bin/orbit
./bin/orbit status --json
make check
make test-race
make demo
make package
make test-terminal-packages
```

Archives, Debian and RPM packages for amd64/arm64 are written to `dist/`, with
`SHA256SUMS`, dependency notices, service aliases, a terminal desktop entry,
Bash/Zsh/Fish completions and operator runbooks. Cross builds are not native
Pi execution. [Install and upgrade](docs/runbooks/install.md) explains startup
modes and preserved legacy state/services. `filesync` engine commands retain
their vocabulary, identities, wire format and `.filesync-internal` scratch names.
`orbit legacy-browser` (retained alias `orbit launch`) explicitly opens the
frozen browser compatibility interface; keep its bootstrap URL private.

Use the TUI's Create/Join forms to review existing contents, finite budgets and
reachable LAN or existing Tailscale addresses. On the inviter, Add device creates
a private invitation; the receiver submits a request, and the owner compares the
exact verification code before approval. Sharing a second folder requires its
own consent and reuses the device key. Keep invitations in private input/files,
never shell arguments or shared transcripts.

```sh
orbit status
orbit folders
orbit devices
orbit conflicts
orbit history notes.txt
orbit deleted
orbit doctor
```

File commands infer the registered folder from the current directory; use
`--folder <name|id>` when needed and `--state <absolute-path>` for multiple
installations. Conflicts and restores require exact current reviews; unavailable
historical bytes cannot be restored. The interface exposes session recovery and
separate-copy restore without silently choosing a conflict winner.

- [Terminal operator guide](docs/runbooks/terminal-operator.md)
- [Keyboard onboarding and sharing](docs/runbooks/terminal-onboarding.md)
- [Conflicts, editor recovery and restore](docs/runbooks/terminal-recovery.md)
- [LAN/Tailscale prerequisites](docs/runbooks/private-network.md)
- [Backup and identity recovery](docs/runbooks/database-recovery.md)
- [Binary rollback](docs/runbooks/rollback.md)
- [Uninstall preserving files/state](docs/runbooks/uninstall.md)

The [terminal release report](docs/evidence/terminal-t13-20261004/summary.md)
records native journeys, resource measurements, failures and remaining checks.
The [case study](docs/case-study.md) explains the design and measured tradeoffs;
[portfolio bullets](docs/portfolio-bullets.md) link concrete supporting evidence.
Historical P/O evidence remains dated. Personal use and comprehensive owner
review are deferred until after delivery. The [scope](docs/portfolio-scope.md),
[protocol](docs/protocol.md), [persistence](docs/persistence.md),
[operations](docs/operations.md) and [verification](docs/verification.md)
own the guarantees and failure model.
