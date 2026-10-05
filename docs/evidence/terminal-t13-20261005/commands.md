# T13 follow-up commands and evidence index

Run from the repository root. Preserve completed outputs: each journey/reproduction
requires a new empty output directory. Native runners prepare private marked
disposable roots and stop only workers they own. They do not provision VPNs,
modify host policy or reset existing services.

## Production regressions

```sh
go test -count=1 -v ./tests/terminal \
  -run '^TestTerminalT13RetiredIdentityApprovalHasRecoveryAction$'
go test -count=1 -v ./tests/terminal \
  -run '^TestTerminalT13ReplacementBootstrapsRetiredHistory$'
go test -race -count=1 ./internal/repository ./internal/control/... ./cmd/filesync
GOFLAGS=-race go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT13'
go test ./tests/terminal -list TestTerminalT13
```

Both new regressions failed before their owning-module fixes. Retired approval
returned a generic error; replacement lacked the canonical artifacts required to
admit known retired-author histories. [Red approval](retired-approval-before.log),
[red replacement](replacement-before.log), [green approval](retired-approval-after-03.log)
and [green fresh replacement](replacement-after-02.log) retain actual output.
The [final eight-case race run](t13-hydration-race.log) includes the enhanced
replacement case: seed an approved membership without artifacts, close/reopen the
joining owner, then recover the same reviewed attempt. The
[repository/control/CLI race run](hydration-repository-control-race.log) checks
hash mismatch refusal, atomic unchanged-revision hydration, replay and continued
refusal of unknown retired-author history.

## Current packaged journeys

```sh
make package
python3 scripts/validation/terminal_private.py --dist dist \
  --hosts local local local \
  --addresses 192.168.88.171 192.168.88.171 192.168.88.171 --network lan \
  --output docs/evidence/terminal-t13-20261005/local-private-05
python3 scripts/validation/terminal_private.py --dist dist \
  --hosts laptop rpi rpi \
  --addresses 192.168.88.83 192.168.88.63 192.168.88.63 --network lan \
  --output docs/evidence/terminal-t13-20261005/native-lan-05
```

[Current package build](package-hydration.log), [local report](local-private-05/terminal-private.json)
and [native report](native-lan-05/terminal-private.json) record the successful
campaigns. Reports include runner/payload hashes, original version IDs, exact
membership facts, elapsed time and scoped cleanup. The local run uses one physical
host; the native run uses two. Neither is laptop/Pi/VPS or Tailscale acceptance.

Earlier `local-private-01`–`04` and `native-lan-01`–`04` directories retain their
unsuccessful results. Causes include an incorrect pending-request oracle,
unreachable workstation enrollment, real per-IP throttling/attempt expiry,
missing retirement artifacts and the observation worker's shorter timeout.
Corrections never bypass approval, regenerate retained attempts, relax product
limits or remove history/membership/byte assertions. Quiet time precedes creation
of the final replacement attempt. One exact read timeout is retried by the final
native runner. No failing run has been relabeled as successful.

## Supporting checks and isolated release

```sh
python3 scripts/terminal_package_test.py --dist dist \
  --output docs/evidence/terminal-t13-20261005/package-scenarios.json
python3 -m unittest discover -s scripts/validation -p 'test_*.py'
python3 -m py_compile scripts/validation/host_agent.py scripts/validation/terminal_private.py
git diff --check
python3 scripts/validation/snapshot_release.py --kernel /boot/vmlinuz-linux \
  --output docs/evidence/terminal-t13-20261005/reproduction-hydration
```

The nine [package scenarios](package-scenarios.json) ran on the earlier
error-reporting-only packages. Containers and arm64 emulation were not selected;
native Pi execution is instead established by the final ordinary journey.
[Twelve safety/VT tests](python-safety-final.log) passed with the final runner.

The isolated candidate is `00992f6d8b6afcde7dfaffe2c8aad8c27c847602`.
[Snapshot inputs](reproduction-hydration/snapshot-manifest.json) and the
[production source audit](source-audit.json) establish source identity without
committing to the original branch/index. The clean reproduction report records
exact subprocess argv, checkout, outcomes, timings and package hashes. Its stages
are `make check`, uncached full race, demo, 16 virtual-machine reset cases, five
storage exhaustion cases, checksum verification and identical repeated packaging.
All stages passed; the candidate ended with no tracked changes.
Native development package labels/hashes are separate from clean candidate ones.
Final read-only `systemctl --user --no-pager --plain list-units --type=service
'filesync-pilot-*.service'` queries via each existing SSH alias confirmed all three
preserved pilots are still active/running. Logs are linked in [results](results.json).

## Authenticated Tailscale and actual three-host acceptance

The owner installed/authenticated Tailscale after the initial
[missing-prerequisite preflight](tailscale-preflight/terminal-private.json).
The following actual commands then passed:

```sh
python3 scripts/validation/terminal_private.py \
  --dist /tmp/filesync-reproduction-e3jo0307/checkout/dist \
  --hosts laptop rpi vps --addresses 100.101.0.12 100.101.0.14 100.101.0.11 \
  --network tailscale --preflight-only \
  --output docs/evidence/terminal-t13-20261005/tailscale-authenticated-preflight-01
python3 scripts/validation/terminal_private.py \
  --dist /tmp/filesync-reproduction-e3jo0307/checkout/dist \
  --hosts laptop rpi vps --addresses 100.101.0.12 100.101.0.14 100.101.0.11 \
  --network tailscale \
  --output docs/evidence/terminal-t13-20261005/tailscale-three-host-01
```

[Authenticated preflight](tailscale-authenticated-preflight-01/terminal-private.json)
observed running/authenticated clients and peer routes through `tailscale0`.
[The complete journey](tailscale-three-host-01/terminal-private.json) passed in
277.636 s on three physical hosts with the exact clean candidate packages:
onboarding, preserved existing files, edits, original-author forwarding, reviewed
retirement/export/import/replay, retired-key refusal and fresh replacement.
Scoped cleanup reported no errors. SSH carries control; Orbit traffic uses the
recorded Tailscale endpoints. No VPN/host policy or unrelated service changes
were made by the agent.

## Unexecuted acceptance and resumption

Native login/logout/unattended boot still needs a designated disposable native
environment. The existing hosts' unrelated workloads are not reboot targets.
Follow [startup modes](../../runbooks/install.md#startup-modes) and preserve
unexecuted labels until those events actually occur. Personal use and comprehensive
owner review are deferred and do not block completion. T13 remains open.
