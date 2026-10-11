# R06 summary: write-up, release and private service

Date: 2026-10-11, base `5224a14` (R05), release commit `11b17da` (Orbit 2.4.0,
orbit-net 1.2.0).

## What shipped

- **Case study** (`docs/case-study.md`):
  - the final-scope decision;
  - a dated correction of the "profile shipped inside every build" claim;
  - "A private service and honest route numbers" (admission design, the 2.3
    retry measurement, route accounting, self-hosting);
  - "Measurements" (R05 tables, what they show, untested networks);
  - "What Orbit does not guarantee".
- **README:** what Orbit is now, a "Try it" section (LAN, then
  `orbit-net init` self-hosting), the emulated matrix result, qualified timing
  scope. **Portfolio bullets:** a final-scope bullet; hosted service marked
  private.
- **Release:** Orbit 2.4.0 public packages (no packaged service) and
  orbit-net 1.2.0.
- **Hosted VPS** switched to allowlist admission (owner-approved):
  `vps-deployment.md`.
- **Installs:**
  - public 2.4.0 as the default `orbit` on PC and laptop (tarball
    `install.sh user`) and Pi (`dpkg`), all not set up;
  - the owner `orbit-trial` build on all three hosts;
  - the PC trial stays `SERVICE_READY` on the allowlisted VPS
    (`logs/installs.log`, `logs/trial-install.log`).

## Final checks

| Check | Result |
| --- | --- |
| `go clean -testcache; make check` | **pass** (full terminal suite, terminal packages, integration, model, faults, harness, amd64/arm64 builds, packaging) after one fix. The first run failed `TestWANW06BinaryGuidedFirstDevice`, which still expected the pre-R04 Automatic default; it was updated to Local network (`logs/make-check.log`) |
| `make test-race-core` | first run **failed**: a data race in the R02 test harness (`bytes.Buffer` read while `os/exec` copied the child's output), not product code. Fixed with a mutex buffer; rerun **pass**, 22 packages (`logs/race-core-first.log`, `logs/race-core-rerun.log`) |
| `make test-terminal-release` (race, T13, count=2) | **pass** |
| `make demo` | **pass** |
| VPS deployment | listed PC trial `SERVICE_READY`; a disposable unlisted device `NOT_ADMITTED`; healthz 200; month count kept |

## Limitations

- `make test-race` over the whole `tests/terminal` suite under race was not
  run. Race coverage is `test-race-core` plus the T13 race subset.
- The laptop and Pi trials are not set up. Their future identities must be
  added to the VPS allowlist (`vps-deployment.md`).
- Nothing was pushed or published; GitHub release artifacts were not created.
