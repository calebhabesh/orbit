# Owner trial loop

For hands-on trials on the owner's PC, laptop and Pi: change → check → install
→ try within minutes. Not a release gate; run `make check` (or CI) once at the
end of a batch.

## The trial instance

Each host runs a second, disposable Orbit beside the owner's own:

| | Owner's Orbit | Trial |
|---|---|---|
| Command | `orbit` | `orbit-trial` (`~/.local/bin`) |
| Binary | packaged install | `~/.local/lib/orbit-trial/orbit` |
| Home seen by Orbit | `~` | `~/orbit-trial` (state in `.local/state/orbit`, folders default to `~/orbit-trial/<name>`) |
| User unit | `orbit.service` | `orbit-trial.service` (via `ORBIT_SERVICE_UNIT`) |

`orbit-trial` is the full product: setup, joining, short codes, startup choice
(login/unattended install and run `orbit-trial.service`), Files, everything.
Device identities are fresh after each reset. The trial never touches
`orbit.service`, `~/.local/state/orbit`, `~/Orbit` or the Pi's `filesync-pilot`
unit, and the owner's Orbit keeps running throughout.

To drop test files into a trial folder: `~/orbit-trial/<folder name>`. Because
the trial's HOME is not the account's home, the trial shows absolute paths and
proposes `~/orbit-trial/<name>`; a typed `~` means the real home, as in the
shell.
Choosing a folder outside `~/orbit-trial` works but reset leaves it in place
(it says so). Never point the trial at `~/Orbit`: Orbit would take over that
root's registration from the owner's instance.

## Commands (run on the PC)

```
make quick            # build, gofmt, vet + short tests of touched packages, affected PTY suites
make trial-install    # build amd64/arm64 once, install on local, laptop, rpi in parallel
make trial-reset      # stop and wipe the trial on all hosts (unit, state, ~/orbit-trial)
make trial-fresh      # reset, then install
make trial-status     # version, daemon, unit, folders and devices per host
```

`scripts/trial/trial.sh <install|reset|fresh|status|stop> [host...]` targets
single hosts (`local`, or ssh names); `TRIAL_HOSTS` changes the default set.
Install restarts a running trial unit, so the new build takes over without
losing the trial. `make quick` compares against `HEAD` (uncommitted work), or
`HEAD~1` when the tree is clean; `QUICK_BASE=<rev>` and `PTY="onboarding keys"`
(or `all`/`none`) override.

Typical cycle: edit → `make quick` → `make trial-install` → try in
`orbit-trial` on each machine. For a new joining/sync scenario:
`make trial-fresh`, then create the Orbit on one host and join from the others.
