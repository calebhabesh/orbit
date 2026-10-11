# R00 commands (2026-10-11)

All commands ran from the repository root on the owner's dev PC unless marked
`vps` (read-only, over the WireGuard SSH alias). Nothing on the VPS changed.

## Baseline

```sh
git rev-parse HEAD            # b2172e21e1534a146e0bf0b1af39738bbe7b195c (plan commit, pushed 2026-10-11)
go version                    # go1.27.2-X:nodwarf5 linux/amd64
```

## VPS, read-only (`logs/vps-snapshot.log`)

```sh
ssh vps 'date -u; /usr/local/bin/orbit-net version; systemctl is-active orbit-net;
  systemctl show orbit-net -p ActiveEnterTimestamp -p NRestarts;
  sudo -n cat /etc/orbit-net/serve.json | <redact key/secret path values>;
  sudo -n cat /var/lib/orbit-net/relay-month.json;
  curl -s http://127.0.0.1:9464/metrics | grep -v "^#"'
```

## Owner trial devices, read-only

```sh
scripts/trial/trial.sh status   # PC orbit-trial running and set up; laptop and Pi stopped, not set up
```

## Local fixture (`logs/r00-fixture.log`)

```sh
ORBIT_FINAL_R00_WINDOW=300s go test ./internal/rendezvous/ -run 'TestFinalR00' -count=1 -v
go test -list '^TestFinal' ./internal/... ./cmd/... ./tests/...
make quick
```

An earlier development run used `ORBIT_FINAL_R00_WINDOW=30s`. It was too short
to cross a renewal, which is why the evidence run uses 300 s.

## Interim relay cap

Not applied. See `summary.md`.
