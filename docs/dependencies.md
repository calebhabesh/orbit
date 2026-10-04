# Toolchain, dependencies and attribution

Baseline selected for P00 on 2026-09-20.

| Component | Pinned version | License | Reason / packaging consequence |
| --- | --- | --- | --- |
| [Go](https://go.dev/doc/devel/release) | 1.27.1 toolchain; module language 1.27.0 | BSD-3-Clause | Current supported Go release. Linux amd64 and arm64 are first-class ports. |
| [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) | v1.59.0 | BSD-3-Clause; bundled SQLite is public domain; bundled sqlite-vec notice is MIT | Maintained `database/sql` driver translated to Go. It builds with `CGO_ENABLED=0`, avoiding a target C toolchain and runtime libc dependency. The tradeoff is a larger Go dependency graph and generated translated code. |
| [`golang.org/x/sys`](https://pkg.go.dev/golang.org/x/sys) | v0.47.0 | BSD-3-Clause | Maintained Linux syscall definitions used only for the exclusive state lock. |

Transitive versions and integrity hashes are pinned by `go.mod` and `go.sum`.
Their license files remain in their Go modules and are compiled into the root
[`NOTICE`](../NOTICE) and [`packaging/LICENSES.md`](../packaging/LICENSES.md)
files for binary and package distributions.

Release audit (2026-10-01): `go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...`
reported no vulnerabilities for the checked Go build. `npm ci` and `npm audit`
reported zero dependency findings for the locked frontend graph. These are
point-in-time tool results, not a guarantee of no future or unknown defects.
Module integrity is checked separately with `go mod verify`. See the
[release evidence](evidence/release-20261001/summary.md).

The production CLI build includes these direct and transitive modules:

| Module | Version | License / notice |
| --- | --- | --- |
| `golang.org/x/sys` | `v0.47.0` | BSD-3-Clause |
| `modernc.org/sqlite` | `v1.59.0` | BSD-3-Clause; bundled SQLite public domain; sqlite-vec MIT |
| `github.com/dustin/go-humanize` | `v1.0.1` | MIT |
| `github.com/google/uuid` | `v1.6.0` | BSD-3-Clause |
| `github.com/remyoudompheng/bigfft` | `v0.0.0-20230129092748-24d4a6f8daec` | BSD-3-Clause |
| `modernc.org/libc` | `v1.75.7` | BSD-3-Clause plus its `LICENSE-3RD-PARTY.md` notices |
| `modernc.org/mathutil` | `v1.7.1` | BSD-3-Clause |
| `modernc.org/memory` | `v1.12.1` | BSD-3-Clause plus bundled Go/mmap/logo notices |

All licenses are permissive and compatible with static distribution without runtime CGO or libc dependencies.

## T09 terminal dependency decision — 2026-10-04

The selected stable v2 family is [Bubble Tea v2.0.10](https://github.com/charmbracelet/bubbletea/releases/tag/v2.0.10),
[Bubbles v2.2.1](https://github.com/charmbracelet/bubbles/releases/tag/v2.2.1) and
[Lip Gloss v2.0.6](https://github.com/charmbracelet/lipgloss/releases/tag/v2.0.6).
Their downloaded module sources declare MIT licenses; exact texts/copyrights
are included in `NOTICE`, with the compiled terminal graph listed in
`packaging/LICENSES.md`. Go module sums pin both sources and dependencies.
Bubbles requires Bubble Tea >=2.0.8 and Lip Gloss >=2.0.5; the resolved pins
meet those requirements. Bubble Tea's Go 1.26 requirement fits the project's
pinned Go 1.27.1 toolchain. The stack remains CGO-free.

The [official v2 API](https://pkg.go.dev/charm.land/bubbletea/v2) and downloaded
`tea.go`, `options.go`, `key.go` and `exec.go` establish the actual `View`,
`KeyPressMsg`, window/paste events, color-profile options and `ExecProcess`
interfaces used by the adapter. Bubbles textinput supplies input admission/focus;
Lip Gloss and ANSI helpers supply semantic styling and grapheme-width layout.
Terminal restoration and daemon independence are application PTY assertions,
not inferred from framework documentation. T09 evidence records actual builds
and process checks; arm64 compilation is not native Raspberry Pi execution.

The alternative [`github.com/mattn/go-sqlite3`](https://github.com/mattn/go-sqlite3)
is maintained and mature, but it
requires CGO and a C compiler. Cross-building it for the Pi therefore requires a
matching cross compiler and creates libc/linkage choices. The pure-Go driver was
selected to keep the first Linux artifacts portable; P03 still has to validate
connection-scoped PRAGMAs, WAL/checkpoint behavior, and the full failure model.

Architecture references and their influence are listed in
[`verification.md`](verification.md#primary-design-references). This project
does not copy Syncthing code or claim protocol compatibility.
