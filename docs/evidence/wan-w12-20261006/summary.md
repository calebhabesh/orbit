# W12 — qualified network status, diagnostics and controls

Date: 2026-10-06. Worker: active Codex session; no agent delegation. The
repository began with the completed W00–W11 implementation and a dirty tree;
the [source snapshot](initial-source.json) records that starting state. Existing
packet changes and evidence were preserved.

W12 implements a shared passive status and explicit doctor path. Status reports
desired/active policy, profile state and expiry, service readiness, typed next
action, candidate counts, freshness and dated per-peer route observations. It
keeps daemon, capture, peer-stored/applied and membership state separate. Doctor
is bounded and explicit: it checks service DNS/TLS, authenticated directory,
pinned direct/relay TLS and actual UDP STUN response. It never sends peer HTTP,
mutates route observations or infers a NAT/firewall type. Unknown transport text
is reduced to stable codes.

Reviewed policy changes enforce the privacy boundary. Local-only/Manual closes
global service and relay clients, invalidates cached WAN candidates and drains
WAN pools before completion while preserving device identity, keys, history,
membership, roots and files. Existing installations remain manual until reviewed;
first setup records explicit consent. Self-host trust uses the same TLS and pin
validation. CLI JSON/human output, status output, TUI details, keyboard `d`/`r`/
`Esc`, completions and support export use the shared contract.

Validation passed:

- focused W12/control/network/terminal/client checks, including cancellation,
  admission, passive candidate freshness, actual STUN, pinned no-HTTP probes,
  typed status/action, local-only privacy and support redaction;
- owning packages under the race detector;
- production binary/PTY journey with healthy-service/offline-peer distinction,
  relay quota truth, keyboard doctor cancellation/refresh, local-only zero
  service requests, reviewed apply/replay/restart, identity/file retention and
  cleanup.

The full uncached Go package run is retained in `logs/all-packages-final.log`.
It passes every package through integration and then reproduces the documented
original aggregate terminal failure (`TestWANW08BinaryOptionalCollisionFreshRelayOnboarding`
at the 10-minute test alarm). It receives no W12 acceptance credit; the focused
W12 packages, owning race run and dedicated binary/PTY run pass independently.
Initial failed/repair logs are retained beside the passing logs. No physical WAN,
privileged namespace, personal-folder or existing-VPS fault injection was used.

Remaining work is W13 operated hosted defaults/self-host deployment, W14
migration/packages, W15 integrated failure/resource campaign, W16 physical WAN
and ordinary setup, and the inherited T13/P17 items. W12 does not imply those
acceptance claims.
