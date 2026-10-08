# E01 summary — daemon lifecycle and service defaults (2026-10-08)

**Complete.** F01, F04, F14 and the F12 startup default are fixed, and EG3 is closed.
[Commands](commands.md), [results](results.json).

- **F01:** the packaged unit and the unit `orbit service enable` writes now listen
  on `127.0.0.1:0`, and every client reads `control.addr`. `install.sh` replaces
  an earlier packaged unit that differs only by the 8080 port. A service action
  rewrites a byte-exact earlier generated user unit; edited units and drop-ins are
  kept. A real transient user unit started with 8080 held by Java: status
  connected and the journal held daemon output.
- **One daemon owner (F04/F14):** the launcher starts `orbit.service` when it
  serves the selected state. If the unit fails to take over, the launcher stops it
  (so it can't crash-loop) and runs a detached daemon that logs to
  `<state>/daemon.log`. Status reports `owner` (service/terminal/manual) and
  `unit_state` separately from the startup mode, in the TUI header, `orbit status`
  and `orbit service status`. `orbit service start/restart` hand a terminal or
  manual daemon over to the unit through the graceful stop path, keeping identity
  and saved settings. `orbit service stop` also stops a daemon running outside the unit.
- **Startup default (F12, EG3):** a desktop is `graphical.target` or an active
  `graphical-session.target`, and proposes login. A headless host proposes
  unattended only with lingering on; otherwise it proposes login and shows
  `sudo loginctl enable-linger USER` with a Ctrl-R re-check. With no manager the
  proposal is manual. The proposal applies only to states the user unit can serve.
  Orbit never runs sudo or enable-linger: a grep finds no such call, and the test
  stand-ins fail the test if either is called.
- **Safety:** unit selection trusts `systemctl --user show -p ExecStart`, so a process with a
  disposable `$HOME` cannot select the owner's real unit. The first `make check`
  showed why the proposal must be state-scoped; nothing on the host was modified.

Remaining limitations: one uninterrupted `make check` after the last
test-expectation fix was not run; the targets it would cover were each run and
pass. Real upgrades (install.sh on PC/laptop, deb on Pi) and drop-in removal are E10.
The F12 selector widget is E03.
