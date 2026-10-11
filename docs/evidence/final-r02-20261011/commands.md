# R02 commands (2026-10-11)

Repository root, owner's dev PC, base `86678f2` plus the R02 working tree.
All services are local disposable fixtures; nothing ran against the VPS.

```sh
go test ./internal/rendezvous -run 'TestFinalR02' -count=1 -v             # logs/rendezvous.log
go test ./cmd/orbit-net -run 'TestFinalR02' -count=1 -v                    # logs/orbit-net.log (SIGHUP on the real binary)
go test ./tests/terminal -run 'TestFinalR02' -count=1 -v                   # logs/joiner.log
# 2.3 versus current client against an allowlisted orbit-net binary:
git worktree add --detach $SCRATCH/orbit-23 7be6a77                        # 2.3 publication commit; removed afterwards
# r02client.go.txt copied into each tree as cmd/zz-r02-client, built, then removed
ORBIT_FINAL_R02_CLIENTS="2.3=$SCRATCH/r02client/client-orbit-23,current=$SCRATCH/r02client/client-orbit" \
  go test ./cmd/orbit-net -run TestFinalR02OldClientAgainstAllowlist -count=1 -v -timeout 10m   # logs/old-client.log
go test -list '^TestFinal' ./internal/... ./cmd/... ./tests/...           # logs/test-list.log
make check                                                                  # logs/make-check-tail.log (milestone M1)
```
