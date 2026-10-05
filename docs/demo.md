# Reproducible demonstration outline

This is an agent-assisted guide to a 3–5 minute scripted demonstration.
It does not certify owner understanding or substitute for the personal pilot.
Use a fresh dedicated validation folder for scripted conflicts and transfers.

| Time | Show | Evidence/oracle |
| --- | --- | --- |
| 0:00–0:30 | Initialize identities, register roots and approve the same membership | Approved participants and verified roots |
| 0:30–1:00 | Ordinary edit arrives through background services | Native pilot automated setup hashes; no manual scan/sync |
| 1:00–1:45 | Three offline versions survive reconnect; review the conflict | Three matching heads/tokens and retained contents |
| 1:45–2:20 | Resolve A/B; receive late C; reject old token; review again | C remains concurrent; stale request rejected |
| 2:20–3:00 | VPS forwards while author's listener is stopped | Original author retained; author does not invent final-device receipt |
| 3:00–3:40 | Stop only the disposable receiving sync process mid-file; resume | Recorded verified chunks reused; remaining chunks verified; whole hash matches |
| 3:40–4:10 | Restore historical bytes | New version/current reviewed ancestry; hashes match |
| 4:10–4:40 | Show scoped reset trace and measured positive/negative workloads | Guest cache discarded; selected protected hashes survive; no universal power-loss/speedup claim |

Run `make demo` for a local demonstration. For the native scripted campaign:

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
