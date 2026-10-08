# Fault harness

Fault schedules begin in P01 and P03. Every destructive test must create and
validate a `.orbit-disposable` marker through `internal/testkit`; ordinary
`make check` runs disposable helper-process termination but never machine reset
or privileged injection. The explicit `scripts/validation/abrupt_reset.py`
campaign resets only newly created, marked VM images; see the release report.
