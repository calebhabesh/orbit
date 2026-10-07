# W16 actual command index

Original executions are preserved. Exit 1 on hosted-gate-refusal is expected refusal, not native acceptance. Failed/refactoring attempts are uncredited.

| Record | Exact argv | Exit | Seconds | Source unchanged |
| --- | --- | --- | --- | --- |
| [rehearsal](rehearsal.json) | `go test -count=1 -v -timeout=8m ./tests/terminal -run '^TestWANW16NativeRunnerRehearsal$'` | 1 | 1.923 | True |
| [native-inventory](native-inventory.json) | `python3 scripts/validation/wan_native.py --topology docs/evidence/wan-w16-20261006/topology.json --route-targets 132.145.111.200 --inventory-only --output docs/evidence/wan-w16-20261006/native-inventory` | 0 | 1.127 | True |
| [rehearsal-version-fixed](rehearsal-version-fixed.json) | `go test -count=1 -v -timeout=8m ./tests/terminal -run '^TestWANW16NativeRunnerRehearsal$'` | 1 | 210.629 | False |
| [rehearsal-receipt-fixed](rehearsal-receipt-fixed.json) | `go test -count=1 -v -timeout=8m ./tests/terminal -run '^TestWANW16NativeRunnerRehearsal$'` | 1 | 3.712 | True |
| [rehearsal-identity-fixed](rehearsal-identity-fixed.json) | `go test -count=1 -v -timeout=8m ./tests/terminal -run '^TestWANW16NativeRunnerRehearsal$'` | 1 | 55.92 | False |
| [rehearsal-final](rehearsal-final.json) | `env ORBIT_W16_REHEARSAL_REPORT=docs/evidence/wan-w16-20261006/rehearsal-report.json go test -count=1 -v -timeout=10m ./tests/terminal -run '^TestWANW16NativeRunnerRehearsal$'` | 1 | 56.827 | True |
| [runner-race](runner-race.json) | `env ORBIT_W16_REHEARSAL_REPORT=docs/evidence/wan-w16-20261006/runner-race-report.json go test -race -count=1 -v -timeout=12m ./tests/terminal -run '^TestWANW16NativeRunnerRehearsal$'` | 1 | 113.97 | False |
| [harness](harness.json) | `python3 -O -m unittest discover -s scripts/validation -p 'test_*.py' -v` | 0 | 0.069 | True |
| [native-inventory-final](native-inventory-final.json) | `python3 scripts/validation/wan_native.py --topology docs/evidence/wan-w16-20261006/topology.json --route-targets 132.145.111.200 --inventory-only --output docs/evidence/wan-w16-20261006/native-inventory-final` | 0 | 1.058 | True |
| [vet](vet.json) | `go vet ./tests/terminal` | 0 | 0.439 | True |
| [runner-process-race](runner-process-race.json) | `env GOFLAGS=-race ORBIT_W16_REHEARSAL_REPORT=<repo>/docs/evidence/wan-w16-20261006/runner-process-race-report.json go test -race -count=1 -v -timeout=12m ./tests/terminal -run '^TestWANW16NativeRunnerRehearsal$'` | 0 | 119.935 | True |
| [discovery](discovery.json) | `go test -list '^TestWANW16' ./tests/terminal` | 0 | 0.295 | True |
| [harness-final](harness-final.json) | `python3 -O -m unittest discover -s scripts/validation -p 'test_*.py' -v` | 0 | 0.064 | True |
| [hosted-gate-refusal](hosted-gate-refusal.json) | `python3 scripts/validation/wan_native.py --topology docs/evidence/wan-w16-20261006/topology.json --route-targets 132.145.111.200 --output docs/evidence/wan-w16-20261006/hosted-gate-refusal` | 1 | 0.038 | True |
| [native-pi-inventory](native-pi-inventory.json) | `python3 scripts/validation/wan_native.py --topology docs/evidence/wan-w16-20261006/topology-pi.json --route-targets 132.145.111.200 --inventory-only --output docs/evidence/wan-w16-20261006/native-pi-inventory` | 0 | 1.167 | True |
| [vet-final](vet-final.json) | `go vet ./tests/terminal` | 0 | 0.065 | True |
| [whitespace](whitespace.json) | `git diff --check` | 0 | 0.007 | True |
| [handoff-final](handoff-final.json) | `python3 docs/evidence/wan-w16-20261006/verify_handoff.py` | 0 | 0.074 | True |
