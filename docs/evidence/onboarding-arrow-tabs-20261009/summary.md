# Files tab arrow navigation — 2026-10-09

Complete; installed and verified as orbit-trial on PC, laptop and Pi. Existing
uncommitted E11–E13 work is retained. No commit or trial reset was performed.

The owner reported reaching tab 5 and being unable to switch views with arrows.
`TestTopLevelArrowsCycleThroughAllFiveViews` reproduced both directions:
right from tab 5 stayed on 5 instead of wrapping to 1; left from tab 5 stayed
on 5 instead of selecting 4 (`logs/repro.log`). Files handled both keys before
the shared tab dispatcher. Removing those two Files bindings restores the
normal dispatcher. Enter/`l` open directories, Backspace/Esc go up; focused
search and modal forms retain their arrow handling. Explicit arrow navigation
also suppresses automatic first-load landing. Help, footers and owning E03/E08
UX/specification entries now describe these bindings.

Checks with `TMPDIR=/home/ethioking/.cache/orbit-work-tmp`, `GOFLAGS=-p=2`:

| Actual command | Result |
| --- | --- |
| `go test -count=1 ./internal/terminal -run '^TestTopLevelArrowsCycleThroughAllFiveViews$'` before fix | Expected failure, both directions, 0.017 s |
| Same command after fix | Pass, 0.027 s |
| `go test -count=1 ./internal/terminal` | Pass, 3.994 s; Files directory navigation also verified with Enter/Backspace |
| `go test -race -count=1 ./internal/terminal` | Pass, 14.019 s |
| `make build test-terminal-keys-pty` | Pass; real arrow escape sequences cycle right and left through every tab, including Files, plus existing form/paste/approval checks |
| `make fmt-check vet` | Pass |
| `make trial-status`, private backup helpers, `make trial-install`, verification helpers, `make trial-status` | Pass; native amd64/arm64, install 18 s, PC/laptop restarted, Pi remains unconfigured/stopped |

Logs retain each result. The existing private backup/verification helpers are
`/home/ethioking/.cache/orbit-work-tmp/e13-trial-{backup,verify}.py`; they also
run on laptop/rpi over SSH stdin. They read the marked trial only, keep owner
control tokens private, and compare identity, root, membership, recorded history
and ordinary-file manifests. Private previous binaries and consistent metadata
backups remain under each host's `~/.local/lib/orbit-trial/backups/e13-*/`.
Final native binary checksums and host states are in [results](results.json).

Already-running TUI processes need closing and reopening to load the new key
handling. Broader integration, packaging, full terminal and physical WAN checks
were not repeated for this isolated UI binding fix; prior E13 evidence is
preserved separately. No fault injection or personal-folder mutation was used.
