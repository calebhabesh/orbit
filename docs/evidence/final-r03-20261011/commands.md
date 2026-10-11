# R03 commands (2026-10-11)

Repository root, owner's dev PC, base `618a886` plus the R03 working tree.
Everything ran inside rootless user + network namespaces (`unshare -rn`, no
sudo) with private 10.77.0.0/16 addresses; no VPS, owner folder or
`orbit-trial` install was involved. `<scratch>` is the session scratch directory.

```sh
go build -trimpath -o <scratch>/bin/orbit ./cmd/orbit
go build -trimpath -o <scratch>/bin/orbit-net ./cmd/orbit-net
go test ./cmd/orbit-net -run 'TestFinalR03' -count=1 -v                       # logs/init-tests.log
python3 scripts/validation/selfhost_netns.py --orbit <scratch>/bin/orbit --orbit-net <scratch>/bin/orbit-net \
  --root <scratch>/r03run1 --out <scratch>/r03-run1.json --block-direct --keep  # netns-run1.json (direct A<->B dropped)
python3 scripts/validation/selfhost_netns.py --orbit <scratch>/bin/orbit --orbit-net <scratch>/bin/orbit-net \
  --root <scratch>/r03run2 --out <scratch>/r03-run2.json                        # netns-run2.json (no block, control run)
make quick                                                                       # logs/make-quick.log
```

The first attempts failed in the driver itself before any Orbit process ran:
`nft` rejected the chain name `fwd` (a keyword), and a fresh device state needed
the launcher (`network preview`) to initialize it. Both are fixed in the script.
