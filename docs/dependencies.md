# Toolchain, dependencies and attribution

Baseline selected for P00 on 2026-09-20.

| Component | Pinned version | License | Reason / packaging consequence |
| --- | --- | --- | --- |
| [Go](https://go.dev/doc/devel/release) | 1.27.1 toolchain; module language 1.27.0 | BSD-3-Clause | Current supported Go release. Linux amd64 and arm64 are first-class ports. |
| [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) | v1.59.0 | BSD-3-Clause; bundled SQLite is public domain; bundled sqlite-vec notice is MIT | Maintained `database/sql` driver translated to Go. It builds with `CGO_ENABLED=0`, avoiding a target C toolchain and runtime libc dependency. The tradeoff is a larger Go dependency graph and generated translated code. |
| [`golang.org/x/sys`](https://pkg.go.dev/golang.org/x/sys) | v0.47.0 | BSD-3-Clause | Maintained Linux syscall definitions used only for the exclusive state lock. |

Transitive versions and integrity hashes are pinned by `go.mod` and `go.sum`.
Their license files remain in their Go modules and must be included in a release
notice generated from the final module graph in P15. P00 does not vendor code.

The production CLI build currently includes these transitive modules:

| Module | Version | License / notice |
| --- | --- | --- |
| `github.com/dustin/go-humanize` | v1.0.1 | MIT |
| `github.com/google/uuid` | v1.6.0 | BSD-3-Clause |
| `github.com/remyoudompheng/bigfft` | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |
| `modernc.org/libc` | v1.75.7 | BSD-3-Clause plus its `LICENSE-3RD-PARTY.md` notices |
| `modernc.org/mathutil` | v1.7.1 | BSD-3-Clause |
| `modernc.org/memory` | v1.12.1 | BSD-3-Clause plus bundled Go/mmap/logo notices |

This list was generated from `go list -deps` rather than the larger tool-only
module graph. Re-run it for release artifacts because transitive dependencies
and bundled notices can change when the SQLite driver changes.

The alternative [`github.com/mattn/go-sqlite3`](https://github.com/mattn/go-sqlite3)
is maintained and mature, but it
requires CGO and a C compiler. Cross-building it for the Pi therefore requires a
matching cross compiler and creates libc/linkage choices. The pure-Go driver was
selected to keep the first Linux artifacts portable; P03 still has to validate
connection-scoped PRAGMAs, WAL/checkpoint behavior, and the full failure model.

Architecture references and their influence are listed in
[`verification.md`](verification.md#primary-design-references). This project
does not copy Syncthing code or claim protocol compatibility.
