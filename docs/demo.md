# Orbit 2.1 three-machine trial

Use a dedicated test folder for this rehearsal. Keep your personal folders and
existing Pi pilot untouched. All three devices need Orbit 2.1; short codes also
need the upgraded Orbit service. The deployment and host records are in the
[onboarding tracker](implementation/onboarding-status.md).

1. **Create on the PC.** Run `orbit`, choose Create, name the device PC and the
   folder Trial, and select an empty dedicated folder. Leave Connection at
   Automatic and Startup at the proposed desktop setting, login. Review the
   root and operator before confirming. If already set up, press `1`, then `c`
   to add a separate folder.
2. **Get a short code.** Press `a`, select Trial, review the folder and press
   Enter. Copy or type the `XXXX-XXXX` pairing code on the laptop. It expires
   after ten minutes and permits one claim. Press `r` for a fresh code after a
   mistake or expiry. `v` reveals the long invitation and `s` saves a private
   file for an older device or a service that has not been upgraded.
3. **Join on the laptop.** Run `orbit`, choose Join (or press `J`), review the
   named operator, and enter the code. Review the inviter and folder, choose a
   dedicated local folder, and accept the proposed connection and login startup.
   Submit the review. Keep the waiting screen open.
4. **Approve on the PC.** Press `w`, inspect the laptop request, compare the
   verification code on both screens, then approve the exact request. Wait for
   local readiness on the laptop. Approval remains mandatory after code exchange.
5. **Join the Pi.** Request a new code on the PC and join over SSH using `orbit`
   on the Pi. A headless host proposes unattended startup when lingering is
   already enabled. If it shows the lingering command, run that displayed command
   yourself, then use Ctrl-R to re-check. Without lingering, Orbit explicitly
   uses login startup. Approve this new Pi request separately on the PC.
6. **Browse and edit.** Reopen Orbit: a configured device with no attention items
   lands on Files. Press `5` to visit Files at any time; `1` opens Overview.
   Enter or Right opens a directory; Left or Backspace goes up. Use `/` to search,
   `]` and `[` to page, `o` to open, `e` for your configured editor, and `y` to copy
   a path. Edit a small text file on each device and check the bytes on the others.
   Saved here describes this device. Details show each peer's last report and
   its age; no label claims every device is current. New received files use 0600.
7. **Conflict and history.** Only in the dedicated trial folder, pause replication
   on two devices and edit the same existing file differently. Resume both, inspect
   the conflict with `c` in Files, review both versions, and resolve deliberately.
   Press `h` for history, review a historical version, and restore it as a new
   version. Preserve any concurrent version that arrives during review.
8. **Connection check.** Press `N` for connection details, then `d` for an explicit
   doctor check. Compare the reported direct/relay route and observation age.
   Exit the terminals and confirm the background services still transfer an edit.
   The monthly relay allowance affects relay data; direct connections remain
   available when the allowance is spent.

If joining fails, request a fresh short code. Keep long invitation files private;
remove only the temporary files you created when the trial ends. Do not reset
identities or reinitialize an existing folder to retry.

## Scripted campaign

The automated checks use fresh marked disposable state. Run:

```sh
make demo
go test -count=1 -v ./tests/terminal -run 'TestOnboardingE06|TestOnboardingE08|TestWANW07RealPTY|TestTerminalT11RealPTY|TestOnboardingE01EG3'
```

The code/approval/transfer, Files, conflict/history, and startup campaigns together
exercise the trial path. Host-class tests use a service-manager fixture; they do
not enable lingering on a personal machine. For the historical native campaign:

```sh
make build build-arm64
go run scripts/three_host_pilot.go --laptop laptop --pi rpi --vps vps
```

The wrapper creates fresh private roots and reports actual results. Run-owned
processes are identified by PID, start time, binary and state path. Existing
pilot directories and unrelated services are preserved. This automated
campaign is separate from [real personal use](evidence/release-20261001/personal-pilot/handoff.md).

Evidence-backed portfolio descriptions can describe the Go/SQLite sync engine,
independent causal model, verified resume, and scoped process/VM recovery.
Measured byte savings must name their workload, sample count and TLS/TCP
measurement boundary. Full release claims must follow the recorded technical
evidence. Personal use and comprehensive owner review are deferred until after
delivery under the 2026-10-04 scope amendment. Current terminal/native validation is tracked in
[the T13 report](evidence/terminal-t13-20261004/summary.md).
