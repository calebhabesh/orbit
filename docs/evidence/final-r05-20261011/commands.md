# R05 commands (2026-10-11)

Owner's dev PC (x86_64, Arch Linux, kernel 7.2), base `8ef117d`. All new runs
are rootless user + network namespaces (`unshare -rn`) with synthetic
addresses and no external route, using production `orbit` and `orbit-net`
binaries from the R04 tree. Roots were marked disposable directories under
`~/.cache/orbit-r05-disposable` (removed after success). Nothing ran against
the VPS, the owner's folders or `orbit-trial`.

```sh
go build -trimpath -o <scratch>/bin/orbit ./cmd/orbit; go build -trimpath -o <scratch>/bin/orbit-net ./cmd/orbit-net
# Connectivity matrix: 5 cells x 10 repetitions, started 20 s apart (concurrent)
for cell in lan eim symmetric udp_blocked public_tcp; do
  python3 scripts/validation/measure_netns.py --orbit ... --orbit-net ... --cell $cell --reps 10 --settle 180 \
    --netem "delay 20ms" --keepalive-mib 2 --phases reconnect,resources,doctor --root ... --out raw/conn-$cell.json &
done
# Bulk: 3 concurrent runs
measure_netns.py --cell lan --reps 0 --phases throughput,chunks,resources --throughput-reps 3 --large-mib 64 --small-files 300 --chunk-mib 32 ...  # raw/bulk-lan.json
measure_netns.py --cell eim --reps 0 --phases throughput,faults --throughput-reps 3 --large-mib 64 --small-files 300 ...                         # raw/bulk-eim.json
measure_netns.py --cell udp_blocked --reps 0 --phases throughput --throughput-reps 3 --large-mib 64 --small-files 300 ...                      # raw/bulk-relay.json
python3 scripts/validation/summarize_r05.py raw/*.json > tables.md
```

Failed or discarded attempts (kept for the record):

- First matrix attempt: driver bugs found by smoke runs. Restarts raced
  launcher-started daemons ("state directory is already owned by another
  agent"), so restarts were not real. Holders and daemons were orphaned. LAN
  advertising was off in the self-hosted review. All were fixed in the
  drivers (`Device.start` stops launcher daemons and checks `.agent.pid`;
  holders are killed; `--lan-advertising`).
- Second matrix attempt: aborted by the scratch tmpfs quota ("Disk quota
  exceeded") with 8 MiB keep-alive files. Partial output is in
  `aborted-run1/` (no repetitions recorded). Rerun on disk with 2 MiB files.
- Without WAN delay, ICE never completed (`ICE_PROBE_DEFERRED`): relayed
  requests on loopback-speed links end before the request-scoped ICE attempt.
  All recorded cells use `tc netem delay 20ms` on home uplinks.
