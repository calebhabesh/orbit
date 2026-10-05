# T12 — terminal entry, packages and compatible adoption

State: **complete for the T12 local package/entry/adoption acceptance bar**. Source is the existing dirty working tree rooted
at commit `61da64f`; existing T11/unrelated changes were retained. This is local
packet evidence, not clean-checkout/native three-host release acceptance.

Bare `orbit` and its long entry options now select the same TUI/status adapter
as `orbit tui`. Both input and output must be TTYs; pipes/JSON do not launch a
browser or daemon. Interactive entry starts/reuses the selected daemon and
restores terminal modes on quit/signals/tools. Legacy discovery no longer gets
bypassed by the TUI's former explicit default state flag. Competing state paths
remain an explicit --state choice. filesync commands/state/keys/root/scratch/wire
and schema 13 remain; explicit `legacy-browser`/`launch` freezes browser entry.

Packages include terminal desktop entry, one service with aliases, native-generated
Bash/Zsh/Fish completions, notices/licenses and operator runbooks. Standalone
upgrades retain customized units; uninstall is scoped to the selected user/system
installation and preserves private state/ordinary files. Debian removal disables
its service only on removal/deconfiguration, allowing upgrade postinstall to
try-restart an already active unit. RPM now has valid ordered/aligned v4 region
headers, header/package/payload digests, link/ownership metadata and lifecycle
scripts. Packages have checksums but no publisher signature; SHA256SUMS is not
an independent trust channel.

Four normally enabled TestTerminalT12 cases passed twice. Instrumented packet
race passed with GOFLAGS=-race, including built child binaries. Tests cover
legacy bare pipe/JSON discovery, unchanged identity/default-path avoidance,
reviewed missing-limit adoption replay, unchanged captured heads/working bytes,
continued same-author counter increase, old/new control capability refusal before
action dispatch under a held state lock, and bare-entry real PTY lifetime.
Compatibility regressions cover schema migrations/newer refusal, interrupted
migration rollback, fenced recovery/rekey and retained source relocation; the
pinned peer authorization/protocol mismatch matrix also passed.

Package runner checks every artifact checksum and each architecture/format's
contents. Native amd64 and QEMU 7.2 arm64 extracted binaries run initialization,
status, doctor and completions. Package-extracted bare PTY covers canonical/raw
editor return, error preservation, narrow/resize/plain input, reconnect, signals,
exact terminal restoration and subsequent real daemon capture. Standalone install,
two re-installs/upgrades and removal preserve custom units and legacy identity/state.
New Debian bookworm and Fedora 43 containers use actual dpkg/rpm transactions,
normal reviewed CLI setup, daemon startup/stop, captured history equality across
re-install and protected working/config/database preservation after removal.
No host user service, firewall, VPN or personal root was changed.

`make test` now includes cmd/filesync. `make check` includes ordinary terminal
and actual package-extracted PTY checks. Container transactions/QEMU are an explicit
optional target, avoiding a Docker dependency in ordinary checks. Retained browser
checks remain compatibility regressions; terminal acceptance is separate.

## Final validation

- Four ordinary T12 cases passed twice (22.700s); packet race with child
  instrumentation passed (42.155s), without skips or race warnings.
- `make check` passed (356.253s), including CLI, the complete ordinary terminal
  suite, package-extracted PTY, integration/model/fault checks and static packaging.
- Full `make test-race` passed (320.850s; terminal suite 319.017s), no race warnings.
- `make demo` passed; compatibility/peer/CLI and explicit retained browser
  checks passed. Final formatting and whitespace checks passed.
- Final `make package` passed (1.331s). The final native/emulated payload,
  standalone lifecycle, real distro transaction and archived bare-PTY campaign
  passed all 11 scenarios (16.786s). [Package results](package-results.json),
  [sanitized PTY frames/results](pty/results.json), [SHA256SUMS](SHA256SUMS),
  [release manifest](release-manifest.json), [actual commands](commands.md),
  [results](results.json) and [source/environment manifest](manifest.json)
  record the scoped observations and intermediate failures.

## Intermediate findings

The first pipe oracle expected a state path in concise human output; it was
restricted to structured output. Legacy setup had runtime.json overriding limits;
the legacy missing-limit fixture now removes both only under its disposable marker.
The status field is state_directory, not state_dir. Test capability versions are
strings and content query kind is conflicts; fixture compilation/query mistakes
were corrected. One broad run encountered the temporary version-type fixture error
while work was still changing; the stable final run is recorded separately.

Actual dpkg rejected missing parent directories despite extraction succeeding.
Actual rpm rejected out-of-order data offsets and an incorrect script tag, then
required v4 regions and integrity digests. The final reader validates these without
--nodigest; --nosignature acknowledges the unsigned publisher artifact. Upstream
[RPM tag definitions](https://github.com/rpm-software-management/rpm/blob/master/include/rpm/rpmtag.h)
and [header reader](https://github.com/rpm-software-management/rpm/blob/master/lib/header.cc)
guided the format correction. Fedora's minimal image omitted cmp; the harness
compares SHA256 hashes instead. The scratch QEMU image lacks cat/a default command;
the runner now copies the emulator from an exact newly labelled container and
validates that label before removing only that container.

## Limitations and next work

Native laptop/Pi/VPS arm64, actual LAN/Tailscale, installed systemd user-manager
startup/login/logout/boot/unattended persistence, clean-checkout release campaign,
VM reset/power loss and owner personal use/unaided explanation are unexecuted here
and remain T13. Container daemons establish manual background startup, not native
systemd behavior. QEMU instruction/runtime execution is not native Pi/storage
acceptance. Re-install of the current package establishes package transaction
compatibility, not arbitrary older-binary semantic rollback support.
P17 owner evidence remains outstanding. No agents were delegated.

Worker explanation: a binary rollback keeps current metadata and monotonic counters,
and requires schema plus runtime/operation compatibility. Restoring old metadata
loses knowledge of already authored counters, so it must rotate device/key identity
and reenroll. Restart may recover the accepted operation, but cannot legitimately
reuse a counter under the old author. This is worker explanation, not unaided owner
evidence.
