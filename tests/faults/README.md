# Fault harness

Fault schedules begin in P01 and P03. Every destructive test must create and
validate a `.filesync-disposable` marker through `internal/testkit`; ordinary
`make check` never runs destructive or privileged fault injection.
