# Commands — scale, churn and demo (2026-10-13)

All runs use fresh marked disposable roots in rootless namespaces
(`unshare -rn`, `unshare -rnm` for the demo). No personal folder, hosted
service, Pi pilot or VPS was touched. `bin/orbit` is release 2.5.1
(`make build` at `247216f`); "fixed" is the same tree plus the batched-receipt
change (`go build -trimpath`, version `2.5.1+receipts`).

```sh
make build build-orbit-net

# Idle sync-round cost (unit fixture, 20 ms injected per request)
ORBIT_ROUND_COST=1 go test -count=1 -run TestSyncRoundCost -v ./internal/replication/   # before and after the change
go test -count=1 -run TestSyncerBatchesReceiptReassertion ./internal/replication/

# End-to-end small-file before/after (R05 driver, NAT cell, 20 ms)
python3 scripts/validation/measure_netns.py --orbit BIN --orbit-net bin/orbit-net --cell eim --netem "delay 20ms" \
  --reps 0 --phases throughput --throughput-reps 3 --large-mib 8 --small-files 300 --root NEW --out raw/small-{old,fixed}.json

# Scale-out and churn (release 2.5.1)
python3 scripts/validation/scale_netns.py --orbit bin/orbit --orbit-net bin/orbit-net --devices 5  --root NEW --out raw/n5.json
python3 scripts/validation/scale_netns.py --orbit bin/orbit --orbit-net bin/orbit-net --devices 10 --soak 1800 --converge-timeout 1200 --root NEW --out raw/n10.json
python3 scripts/validation/scale_netns.py --orbit bin/orbit --orbit-net bin/orbit-net --devices 16 --root NEW --out raw/n16-hub-saturation.json   # stopped at fanout rep 3 (600 s timeout)
python3 scripts/validation/scale_netns.py --orbit bin/orbit --orbit-net bin/orbit-net --devices 20 --root NEW --out raw/n20-membership-cap.json  # refused at member 17

# Charts
python3 scripts/validation/plot_scale.py --scale raw/n5.json raw/n10.json --scale-note "16 devices: …" --soak raw/n10.json \
  --round-cost raw/round-cost.json --small raw/small-old.json raw/small-fixed.json \
  --r05 docs/evidence/final-r05-20261011/raw --out charts

# Demo video (docs/assets/orbit-demo.mp4)
python3 scripts/demo/record_demo.py --orbit BIN --orbit-net bin/orbit-net --root NEW --out orbit-demo.mp4
```
