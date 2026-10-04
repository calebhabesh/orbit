# T06 — Commands and context

State: **complete**, 2026-10-04. All seven ordinary tests passed twice (1.159s).
Final serial `make check` and `make test-race` exited 0. The uncached packet
race run passed (21.252s). Physical cross-host LAN/Tailscale, native boot/logout,
and P17 actual owner evidence remain unexecuted/outstanding. P/O history and
relocation changes are preserved.

Production changes implement the unified command catalog, context inference,
and adapter parity:
1. **Named and Contextual Resolution**: `orbit context` infers the active folder
   and root-relative path when invoked inside any registered root or subdirectory.
   Outside a registered root, an explicit folder name or 64-character hex ID is
   resolved. Duplicate display names or ambiguous targets return `AMBIGUOUS_CONTEXT`
   (exit code 2) with candidate suggestions. Unmatched folders return `FOLDER_NOT_FOUND`
   (exit code 2) with active folder candidates.
2. **Root Health Safety**: Missing roots report `ROOT_UNAVAILABLE` (exit code 5),
   and relocated/mismatched dev/inode roots report `STALE_ROOT` (exit code 4),
   requiring explicit review rather than silent misdirection.
3. **Path Safety and Boundaries**: Shell-relative paths are converted to validated
   root-relative targets anchored within the root descriptor. Traversal escapes
   via `..` or reserved namespaces (`.filesync`, `.orbit-*`) are rejected with
   `INVALID_PATH` (exit code 2). The `--` separator allows literal arguments,
   protecting leading dashes and spaces.
4. **Terminal Escaping and Structured JSON**: Human output sanitizes ANSI escape
   sequences (`\x1b`), carriage returns, line feeds, and control runes via
   `EscapeTerminal`, neutralizing terminal spoofing. With `--json`, raw structured
   data adheres to frozen contract fields.
5. **Transport and Lifecycle Parity**: Command adapters route through
   `controlclient.Client.WithController`, querying the live daemon HTTP endpoint
   when running and falling back to direct SQLite controller operations when
   stopped, eliminating database lock contention.
6. **Completions and Grouped Help**: Fast shell completions for bash, zsh, and fish
   discover command names and active folder names without disclosing credentials,
   tokens, or private material. Grouped help cleanly maps everyday and advanced
   commands.

Commands, intermediate failures and limitations are in [commands](commands.md).
Gate dispositions are in [results](results.json), with task hashes and dirty-tree
provenance in [manifest](manifest.json). Next eligible work is **T07 — status,
attention, and diagnostics**.

A convenient context lookup is a navigation helper, not an authorization or safety
check. Context maps `cwd` or a partial name to a candidate folder descriptor and
relative path. Authorization requires local authenticated credentials and verified
cryptographic membership in the folder's DAG. Path safety requires descriptor-rooted
containment, lexical validation, and traversal rejection at the filesystem boundary.
Finding a folder does not authorize access to its data; resolving a path does not
prove the target is safe to read or write.
This is the worker's explanation, not P17's unaided owner explanation.
