# R01 commands (2026-10-11)

Repository root, owner's dev PC, base `4de12fd` plus the R01 working tree.
No VPS, owner folder or `orbit-trial` install was touched.

```sh
go test ./internal/network/ ./internal/config/ ./internal/control/... ./internal/replication/ ./internal/terminal/ ./cmd/orbit/ -run 'TestFinalR01' -count=1 -v   # logs/r01-tests.log
go test ./internal/network/ -run '^$' -bench 'FinalR01' -benchtime 2s -count 3                                                                                # logs/bench.log
ORBIT_FINAL_R01_GIB=1 go test ./internal/replication/ -run TestFinalR01OneGiBPull -count=1 -v -timeout 20m                                                 # logs/one-gib-pull.log
go test ./tests/terminal/ -run 'TestWANW08BinaryLocalOnlyLANAfterApproval|TestWANW09BinaryLocalOnlyQUICAfterApproval' -count=1 -v                            # logs/binary-daemons.log
go test -list '^TestFinal' ./internal/... ./cmd/... ./tests/...                                                                                              # logs/test-list.log
make quick                                                                                                                                                    # logs/make-quick.log
```
