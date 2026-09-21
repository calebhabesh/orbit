# P00 local evidence — 2026-09-20

Revision baseline: `43e89b295dbfa53a0c7f5919b6017432931d2b1e` plus the
uncommitted P00 and planning worktree. Development host: Arch Linux amd64,
kernel `7.2.4-arch1-2`, Go `1.27.1-X:nodwarf5`, SQLite CLI `3.53.4`.

## Result

- A temporary committed snapshot was cloned to a fresh checkout and
  `make check` passed there.
- Unit tests exercised deterministic config creation, private permissions,
  cross-process state locking, schema refusal, required SQLite PRAGMAs,
  persistence after close/reopen, and disposable-root validation.
- `make test-race` passed for all current packages on linux/amd64.
- Both amd64 and arm64 CLI artifacts built with `CGO_ENABLED=0` and were
  reported as statically linked ELF executables.
- The arm64 artifact ran through QEMU user-mode emulation. Its real `init`
  command created a schema-version-1 SQLite database and private config/state
  files. This establishes emulated instruction/runtime compatibility, not Pi
  kernel, filesystem, storage, or hardware compatibility.
- `actionlint` v1.7.7 accepted the workflow, but no hosted GitHub Actions run
  has occurred for the uncommitted worktree. P00 therefore remains
  `in_progress` rather than complete.

No destructive fault injection was run. The arm64 state was created beneath
`/tmp/filesync-p00-arm64.hzD0e8`; it contains only generated P00 test data.

## Owner explanation notes

Driver choice affects deployment because a CGO SQLite driver needs a target C
compiler and a compatible target libc/link strategy. The selected translated
Go driver permits ordinary Go cross-compilation and static Linux artifacts,
at the cost of a larger dependency graph and generated implementation code.

A compiled arm64 ELF proves only that the compiler emitted a target artifact.
Running `init` under QEMU additionally exercises arm64 instructions and the
SQLite path, but still does not test the Raspberry Pi's kernel, filesystem,
storage flush behavior, memory pressure, or service packaging. Real-device
execution remains later release evidence.
