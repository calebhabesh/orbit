# T06 commands and observations

Working directory: `<repo>`. Revision
`86ae55280b22a5839258d3cca40210c6e2613025` plus the existing dirty T00–T05,
relocation and planning tree. Go `go1.27.1-X:nodwarf5 linux/amd64`, Linux
`7.2.8-arch1-2 x86_64`. No delegation, dependency or schema increment.

Commands are actually executed; logs omit private tokens and credentials.
Test roots are marker-protected temporary disposable directories.

| Command | Actual result / record |
| --- | --- |
| `go test ./tests/terminal -list '^TestTerminalT06'` | Exit 0; seven ordinary top-level tests; [discovery](discovery.log) |
| `go test -v ./internal/control -run '^TestTerminalContext\|^TestTerminalFoldersAndDevices'` | Exit 0; 6 unit tests passed (0.428s); [control unit](control-unit.log) |
| `go test -v ./cmd/filesync/...` | Exit 0; 3 packages passed; [cmd filesync](cmd-filesync.log) |
| `go test -count=2 -v ./tests/terminal -run '^TestTerminalT06'` | Exit 0; all seven tests passed twice (1.159s); [packet](packet.log) |
| `go test -race -count=1 -v ./tests/terminal -run '^TestTerminalT06'` | Exit 0; all seven tests passed with race detector enabled (21.252s); [packet race](packet-race.log) |
| `go vet ./tests/terminal ./cmd/filesync` | Exit 0; clean vet checks; [scoped vet](scoped-vet.log) |
| `make check` | Exit 0; unit, model, integration, packaging (amd64/arm64 tar/deb/rpm), manifest; [make check](make-check.log) |
| `make test-race` | Exit 0; full repository race detector pass; [make test race](make-test-race.log) |
| `python3 docs/evidence/terminal-t06-20261004/validate_docs.py` and `git diff --check` | Exit 0; local links, anchors, balanced fences, trailing whitespace clean; [doc validation](documentation-validation.log) |

### Intermediate failures and resolutions

1. **Folder Disambiguation Candidates**: In `internal/control/terminal_context.go`, when an explicit folder query did not match an active folder, candidate suggestions were initially omitted. Corrected to return candidate items via `tc.Result.Items` populated with active folders for user disambiguation (`FOLDER_NOT_FOUND`, exit code 2).
2. **Help Header Backward Compatibility**: The root help message header initially changed to `"Orbit - Workspace and file synchronization"`, which tripped an existing launch integration test expecting the substring `"Orbit - Local file synchronization"`. Resolved by adopting `"Orbit - Local file synchronization and everyday workspace management"`, satisfying both the redesigned T06 grouped catalog and preexisting regression assertions.
3. **Daemon Database Lock Collision in Folder Adapters**: In `cmd/filesync/main.go`, `handleFoldersPause`, `handleFoldersResume`, `handleFoldersRemove`, and `handleFoldersRevalidate` previously opened the workspace database directly via `app.WithWorkspace`. When the daemon process was already running, this failed with an exclusive SQLite lock error (`database is locked`). Resolved by routing these commands through `controlclient.Client.WithController`, which routes calls to the live daemon HTTP API when running and falls back to direct controller access only when stopped.

### Path safety vs Context vs Authorization

- **Context Lookup**: Syntactic convenience that maps `cwd` and command-line inputs to candidate folders and root-relative targets. It infers folder context when `cwd` resides inside a registered root, derives root-relative file paths, and disambiguates names. Context resolution does not authorize access or validate filesystem safety.
- **Authorization**: Governed strictly by local authentication credentials and folder membership cryptographic keys. A caller cannot view or modify synced files without valid credentials and cryptographic membership within that folder's DAG.
- **Path Safety**: Enforced via descriptor-rooted filesystem boundaries (`filepath.Rel`, boundary traversal rejection, and reserved namespace barriers blocking `.filesync`, `.orbit-*`, or `..` escapes returning `INVALID_PATH`, exit code 2). No lexical path resolution or context match is trusted for raw filesystem IO.

### Limitations and unexecuted checks

Cross-host LAN/Tailscale, native boot/logout/login, physical power loss, and P17
actual owner use/unaided explanation remain **unexecuted** here. Streamed editor
sessions and interactive conflict resolution remain assigned to T08. Status
attention aggregation remains assigned to T07.
