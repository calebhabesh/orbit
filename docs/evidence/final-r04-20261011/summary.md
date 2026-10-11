# R04 summary: public builds without a default service (RG3)

Date: 2026-10-11, base `dbfacd4`. RG3 is closed (candidate 1).

## What shipped

- `internal/network/release_hosted.go` (`orbit_hosted`) embeds
  `release-profile.json`; `release_public.go` embeds nothing. Public builds,
  including `make build` and `make package` by default, carry no service.
  `make trial-*` builds owner (hosted) binaries; `ORBIT_BUILD_TAGS=orbit_hosted`
  makes owner packages, and the manifest's `packaged_profile` is `null` for
  public packages.
- Fresh setup (CLI `orbit setup`, TUI Create) proposes **Local network** when
  no packaged service exists (`tc.FreshConnectionMode`). The form hint names
  Self-hosted with `orbit-net init`.
- Automatic with no stored and no packaged profile reports
  `NO_SERVICE_CONFIGURED` with `tc.NoServiceAction` (self-hosting steps) in
  status, doctor, the TUI and the daemon's network error.
- `orbit network automatic` and short pairing codes explain the no-service case
  and point to the full invitation code or the inviter's service.

## Evidence

| Acceptance item | Result |
| --- | --- |
| Fresh-build setup, CLI | `TestWANW06BinaryLocalCaptureAndPrivateInputs`: fresh review `local_only`, status `LOCAL_ONLY` |
| Fresh-build setup, TUI | `TestFinalR04FreshSetupDefaults` (unit) and W07 TUI unit with a packaged profile still Automatic |
| Fresh-build setup, PTY | `TestWANW07RealPTYRelayOnboarding`: fresh form shows `local_only` and the `orbit-net init` hint; deliberate Automatic shows `NO_SERVICE_CONFIGURED` in Connection Details; Local network shows `LOCAL_ONLY` |
| No-service status and pairing | `TestFinalR04NoServiceConfigured`: no builtin, no offer; Automatic → `NO_SERVICE_CONFIGURED` with the action; a short code → `NO_SERVICE_CONFIGURED` |
| Upgrade from 2.3 with a stored release selection, and rollback | `r04_upgrade_rollback.sh` in an empty network namespace: 2.3 adopts (Automatic, verified, Caleb Habesh). The public build (`packaged_profile.available=false`) shows the same mode, verified profile and digest `9138a478…`; rollback to 2.3 the same; device identity unchanged. `SERVICE_UNAVAILABLE` only because the namespace has no route |
| Owner-path rehearsal on `orbit-trial` | PC `orbit-trial` (Automatic via the VPS): public build installed → `SERVICE_READY`, same operator, digest and device. Owner build restored → `SERVICE_READY`. A fresh public-build device adopts the hosted profile from an owner-only file copy (`owner-adopt-from-file.log`) |
| Both build variants | `TestWANW14PackagedReleaseProfile` passes untagged (asserts no builtin) and tagged (asserts the embedded selection); `go test -tags orbit_hosted -short ./cmd/... ./internal/...` all ok; W14 CLI test passes with a hosted binary |
| Regression | `PTY=all make quick`: pass (pty, onboarding, everyday, keys, participation) |

## Limitations

- 2.x installs already in the wild keep their embedded hosted profile until
  epoch 2 expires (2027-10-07). The R02 allowlist in R06 is what stops them.
- Owner epoch advances now come from owner builds or a reviewed profile file,
  not public packages.
- The laptop and Pi trials (not set up) were not reinstalled. Owner trial
  builds stay hosted by default.
- `TestWANW14MixedVersionUpgradeAndRollback` skips because its legacy revision
  predates the public history rewrite (pre-existing). The new upgrade script
  covers 2.3 → public instead.
