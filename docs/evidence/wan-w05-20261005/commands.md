# Commands

```text
go test ./internal/replication -run '^TestWANW05' -count=1 -v
go test ./internal/rendezvous -run '^TestWANW05' -count=1 -v
go test ./internal/config -run '^TestWANW05' -count=1
go test -race ./internal/replication ./internal/rendezvous ./internal/config -run '^TestWANW05' -count=1
go test ./internal/config ./internal/replication ./internal/rendezvous ./internal/network ./internal/control ./internal/app -count=1
go test ./tests/terminal -run '^TestWANW05DaemonSIGKILL' -count=1 -v
go test ./tests/terminal -run '^TestWANW05ThirdDeviceOfflineRolloutForkAndRetiredBootstrap' -count=1 -v
go test ./tests/terminal -run '^TestWANW05RoutedJoinTwoFoldersRestartAndTwoWayData' -count=1 -v
git diff --check
go vet ./...
```

The daemon fault command validates every signal target against the marked
disposable root and `.agent.pid` before signaling. No personal directory or
existing service was targeted.
