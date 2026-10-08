# E10 commands

Validation (uncached; logs in `logs/`, step results in `logs/summary.txt`):

```sh
go test -timeout=45m ./tests/terminal/...                  # make-check-run2-terminal-pass.log
go clean -testcache
make test-terminal-packages fmt-check vet test test-integration test-model \
     test-faults test-harness build build-arm64 package     # make-check-rest.log
make test-race-core                                         # test-race-core.log
go test -race -count=1 ./internal/network/                  # test-race-network-rerun.log
make test-terminal-pty test-terminal-onboarding-pty \
     test-terminal-keys-pty test-terminal-everyday-pty      # pty-*.log
make demo                                                   # demo.log
make test-orbit-net-rehearsal                               # orbit-net-rehearsal.log
make package package-orbit-net && (cd dist && sha256sum -c SHA256SUMS && sha256sum -c orbit-net-SHA256SUMS)
```

VPS (owner-confirmed 2026-10-08, 2 TiB/month), over WireGuard as `ssh vps`:

```sh
# backups: /usr/local/bin/orbit-net.bak-20261008,
#          /usr/local/lib/orbit-net-alert/orbit-net.bak-20261008,
#          /etc/orbit-net/serve.json.bak-20261008
# serve.json: "relay_month_bytes": 2199023255552
# /etc/systemd/system/orbit-net.service.d/state.conf:
#   [Service] StateDirectory=orbit-net, StateDirectoryMode=0700
sudo -u orbit-net /tmp/orbit-net-1.1.0 serve --config /etc/orbit-net/serve.json --check
sudo install -m 0755 /tmp/orbit-net-1.1.0 /usr/local/bin/orbit-net
sudo install -m 0755 /tmp/orbit-net-1.1.0 /usr/local/lib/orbit-net-alert/orbit-net
sudo systemctl daemon-reload && sudo systemctl restart orbit-net
curl -s http://127.0.0.1:9464/metrics | grep orbit_net_relay_month
```

Rollback: restore the three `.bak-20261008` files, remove `state.conf`,
`daemon-reload`, restart. No Docker, Caddy, tunnel or firewall change.

Hosts:

```sh
# PC and laptop: tarball, user install (rewrites the 8080 unit to port 0)
tar -xzf orbit-v2.1.0-linux-amd64.tar.gz && ./install.sh user
# Pi
sudo dpkg -i orbit_2.1.0_arm64.deb
# PC and Pi: remove the F01 workaround
rm ~/.config/systemd/user/orbit.service.d/control-port.conf && systemctl --user daemon-reload
```
