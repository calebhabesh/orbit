# Commands and results

Executed 2026-10-06 from `<repo>`; no delegation.

| Command | Result |
| --- | --- |
| `go test -race -count=1 -v ./cmd/orbit-net` | Exit 0; four tests pass; [log](logs/operator-race.log) |
| `go vet ./cmd/orbit-net` | Exit 0 |
| `git diff --check` | Exit 0 before evidence finalization; repeated afterward |
| `make package-orbit-net` | Exit 0; amd64 and arm64 archives built; [log](logs/operator-packages.log) |
| `bin/orbit-net-linux-amd64 key verify --file /home/owner/.config/orbit-operator/authority.key --authority 9af3cf8a979f1b635a56831259d7645a62fb7c19db2de8be51e6afb0ce423b36` | Exit 0; [public-only result](logs/authority-verification.log) |
| `stat -c '%a %U %n' /home/owner/.config/orbit-operator` and bounded `find` for file modes/names | Directory 0700; authority key 0600; no key contents emitted |
| `ssh -o BatchMode=yes -o ConnectTimeout=8 laptop 'stat -c "%a %U %n" ~/.config/orbit-operator; find ~/.config/orbit-operator -maxdepth 1 -type f -printf "%m %f\n"; lsblk -o NAME,TYPE,FSTYPE,MOUNTPOINTS,RM'` | Recorded directory absent; no mounted removable drive observed; terminal command exit 0 comes from `lsblk`, not key custody |
| `ssh -o BatchMode=yes -o ConnectTimeout=8 laptop 'id; sudo -n true; find ~/.config -maxdepth 2 -type d -iname "*orbit*" -print'` | User caleb2002; sudo needs authentication; no Orbit directory found; not proof of absent root custody |
| `ssh -o BatchMode=yes -o ConnectTimeout=8 vps 'systemctl is-active orbit-net; systemctl list-unit-files "*prometheus*" "*alert*" "*monitor*" --no-pager; command -v prometheus; command -v alertmanager; curl --max-time 5 -fsS http://127.0.0.1:9464/healthz; curl --max-time 5 -fsS http://127.0.0.1:9464/metrics'` | Service active, health ok, metrics available; monitoring executables not found; [log](logs/live-health.log) |
| `ssh -o BatchMode=yes -o ConnectTimeout=8 vps 'command -v sendmail; command -v msmtp; command -v mail; systemctl list-unit-files "*postfix*" "*exim*" "*smtp*" --no-pager; ls /etc/orbit-net'` | No mail executables/units found; configuration directory access denied; final exit 2; no private configuration read |
| `bash -n /tmp/orbit-wg6-offline-backup.sh` | Exit 0; wizard not executed |
| `command -v shellcheck` | Unavailable; no ShellCheck result claimed |

No actual backup, notification delivery, hosted fault injection, W15 campaign,
full aggregate or full race run occurred in this follow-up.
