# W06 — Automatic CLI setup and scripting

Run date: 2026-10-05. **W06 is complete.** Production controls/CLI, focused acceptance and
the final clean `make check` aggregate pass.

Fresh guided setup reviews device/folder names, root/capacity, finite budgets,
startup and Automatic connection without requesting peer addresses. It initializes
and launches the daemon itself. Existing manual installations and explicit legacy
settings-file setup keep their policy until reviewed opt-in. Missing hosted
configuration preserves local capture and explicitly reports networking unavailable.
Local-only is available before public announcements and reports that LAN discovery
is still W08 work, rather than implying implemented discovery.

Network policy/profile changes use the existing authenticated typed controller,
private review and durable operation ledger. Exact intents, independent profile
trust, current generation, expiration and operation identity are bound. Accepted
writes recover before activation; older setup jobs cannot roll back newer intent.
The daemon owns service clients, transport and synchronization. Cached status
separates desired/active policy, restart requirements and service readiness from
actual dated routes and existing version/copy observations.

Named invitations and request approvals retain the existing enrollment semantics.
V2 and v3 private files/codes coexist; stdin and deliberate private file transfer
avoid secret argv. Ordinary invitation JSON/status omits the capability. Exact
request/device/key/folder/transcript/membership reviews support scripts and the
real TTY CLI. Request/operation/root identity survives delayed or quota-blocked
approval and client relaunch. Additional folders receive their own roots and
approval without replacing device identities or the inviter certificate.

Executed focused evidence:

- Controller/configuration review, idempotent replay, changed-input rejection,
  stale generation rejection, monotone writes, identity preservation and
  accepted-before-effect recovery pass, including race instrumentation.
- Production-binary guided first-device CLI in a real PTY passes. The signed
  local-development relay journey passes scripted create/invite/join/approval,
  delayed/relaunched enrollment, second-folder admission and guided approval.
- The journey asserts actual verified files in both pull directions, exact version
  authorship and the saved inviter certificate/pin, not only operation IDs.
- Wrong-pin/expired fresh invite rejection, private stdin input, owner-only transfer
  permissions, JSON/error redaction, non-TTY review requirements, unsupported root
  rejection, missing profile and service outage/local capture pass.
- The clean final `make check` passes the complete terminal campaign, packaged
  binary checks, formatting, vet, CLI/internal tests, integration, model campaign,
  design-gate/fault tests and all twelve Python validation-harness tests.
- Shared controller/config/network/CLI race tests, network/control canonical
  fixture checks and `go vet ./...` pass. Final race CLI fixtures pass; their
  ordinary launched production binaries are built without race instrumentation.

Initial failures and repairs are retained in command/log records. A later aggregate
passed the whole terminal campaign, packages and internal checks, then found a
manual pairing fixture which needed explicit Manual review under the new fresh
Automatic default. The repaired manual fixture and retained invite alias pass
focused integration/race checks; their transfer labels preserve historical v2 and
explicit routed v3 behavior. The original
full check hit the ten-minute package timeout before queued terminal tests ran;
it receives no credit. The terminal target now has a finite thirty-minute aggregate
budget. Individual process waits and destructive-environment marker checks remain.
A first reverse-transfer test exposed the five-minute legacy CLI cadence; routed
modes now use five seconds by default, with explicit overrides and manual defaults
preserved. Second-folder admission encountered a real retryable rate-limit state;
the accepted request was retained and later completed under the same operation.

Limits: all network fixtures run on one Linux development host with independent
signed service/TLS trust. This is neither native WAN/NAT evidence nor hosted-default
operation. LAN discovery, reachable public direct paths, QUIC/ICE/STUN, roaming,
full diagnostics, operated profiles and distribution remain later packets. W11
still owns native timing/fairness/Pi measurements. T13 login/logout/unattended checks
and deferred P17 owner use/explanation remain unchanged. W07 is the next sequential
packet.
