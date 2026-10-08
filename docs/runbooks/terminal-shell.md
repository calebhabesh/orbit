# Terminal shell

Run `orbit` or `orbit tui --state /absolute/state`. Both use the same shell and
authenticated controls, reusing/starting the selected daemon before terminal
ownership. Pipes and --json use status without starting a daemon. Closing the
interface leaves synchronization running. Setup and everyday screens are documented
in [onboarding](terminal-onboarding.md) and [recovery](terminal-recovery.md).

Use j/k or arrows to select, left/right or o/f/n/d to change section, Tab or /
to focus search, Enter to inspect, Esc to return, and ? for help. Text entry
treats j/k/q/? as ordinary characters. Search filters the current page; ] loads
the next page and [ returns to the first page. Refresh retains selection by
identity and preserves search/focus. Attention appears before folders on the
overview. Folder roots are registrations; their presence alone does not assert
captured files or successful device copies. Use `orbit status` for qualified
copy observations and `orbit doctor` for recovery diagnostics.

Use `--no-color`, `NO_COLOR=1` or `TERM=dumb` for text focus without color.
Piped output prints concise status; `--json` prints structured status without
a TUI, terminal escapes or prompts, including when invoked in a TTY.
q and Ctrl-C close only the interface. Committed operations belong to the
daemon; explicit operation cancellation and `orbit service stop` are separate
controls. SIGTERM restores the terminal and may report a canceled client exit.

For a development tool handoff on a scratch file, configure a trusted owner
program explicitly:

```sh
bin/orbit tui --state /absolute/state --tool 'vim -n' \
  --tool-file /absolute/private/scratch.txt --tool-limit 1048576
```

e yields terminal input/output to that direct argv program, bounded by util-linux
`prlimit`; paths are appended as separate arguments. On success or failure the
shell restores navigation and reports the return. This scratch-tool command
does not commit a conflict resolution. T11 integrates the adapter with T08's
verified exports, admitted sessions, staged upload and fresh reviewed commit.

Run `make test-terminal-pty` for the actual-binary PTY campaign. To retain
sanitized transcripts, use a new empty output directory:

```sh
python3 scripts/terminal_pty_test.py --binary bin/orbit \
  --output /new/empty/evidence-directory
```

The Linux runner requires Python 3, `/proc`, PTYs and util-linux `prlimit`.
It copies the binary into a fresh private marked root, initializes synthetic
state, starts only its own children, and checks marker, canonical paths,
process start time, executable and argv/state before signals. It pauses/resumes
that daemon to exercise unavailable control while its state lock is held. No
root, service, network policy or daemon supplied by the user is a fault target.
The runner removes its root only after validated child cleanup. Evidence covers
80x24, 40x16, resize, keyboard/paste/search, tool success/failure/cancellation,
quit/Ctrl-C/SIGTERM, exact terminal restoration, pipes/JSON and actual captured
file content after client exit. Native host, package/login/unattended, physical
faults and owner learning are separate release evidence.

T10 adds actual [keyboard onboarding and device management](terminal-onboarding.md),
including reviewed create/join, scoped invitations, exact requests and resumable
progress. T11 retains ownership of the reviewed content/editor screens.
