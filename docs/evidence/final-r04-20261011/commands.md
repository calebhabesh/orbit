# R04 commands (2026-10-11)

Repository root, owner's dev PC, base `dbfacd4` plus the R04 working tree.
`<scratch>` is the session scratch directory.

```sh
go build ./... && go build -tags orbit_hosted ./...
go test ./internal/network -run TestWANW14 -count=1; go test -tags orbit_hosted ./internal/network -run TestWANW14 -count=1
go test ./internal/control/terminalcontract ./internal/terminal ./internal/control -run TestFinalR04 -count=1 -v
# 2.3 binary from 7be6a77 (temporary worktree, removed), public and hosted builds of the R04 tree:
<scratch>/bin/{orbit-2.3,orbit-public,orbit-hosted} version --json          # packaged_profile available true/false/true
scripts/validation/r04_upgrade_rollback.sh <scratch>/bin/orbit-2.3 <scratch>/bin/orbit-public <scratch>/r04up   # logs/upgrade-rollback.log (unshare -rn: loopback only)
unshare -rn bash <scratch>/adopt.sh                                          # logs/owner-adopt-from-file.log
go test ./tests/terminal -run 'TestWANW06BinaryLocalCaptureAndPrivateInputs|TestWANW14|TestWANW07' -count=1 -v   # logs/terminal-affected.log
PTY=all make quick                                                           # logs/make-quick.log
go test -tags orbit_hosted -short ./cmd/... ./internal/... -count=1          # all ok
TRIAL_TAGS= scripts/trial/trial.sh install local; scripts/trial/trial.sh install local   # logs/owner-rehearsal.log
```

The first terminal run failed `TestWANW07RealPTYRelayOnboarding`, which still
expected the old fresh "automatic" default. The test was updated (Local network
default; Ctrl-N ×3 to Automatic; `NO_SERVICE_CONFIGURED`) and rerun: pass.
Two unit tests that pinned the old default were also adjusted: W07 TUI now
supplies a packaged profile, and the W06 CLI test expects `local_only`. A
`JoinPolicy` change that would have broken v2 direct invitations was reverted
before commit.
