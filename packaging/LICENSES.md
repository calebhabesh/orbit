# Third-Party Licenses and Legal Notices

This repository and the compiled `filesync` binary include code from external Go modules and third-party projects. All included code is distributed under permissive open-source licenses compatible with the project license.

See the root [`NOTICE`](../NOTICE) file for complete copyright assertions and license texts.

## Direct Dependencies

| Module | Version | License | Direct Purpose |
| --- | --- | --- | --- |
| `golang.org/x/sys` | `v0.47.0` | BSD-3-Clause | Linux flock / statfs system call interface |
| `modernc.org/sqlite` | `v1.59.0` | BSD-3-Clause (SQLite in Public Domain, sqlite-vec MIT) | Pure-Go CGO-free SQLite database engine |

## Transitive Dependencies

| Module | Version | License | Purpose |
| --- | --- | --- | --- |
| `github.com/dustin/go-humanize` | `v1.0.1` | MIT | Byte/size formatting |
| `github.com/google/uuid` | `v1.6.0` | BSD-3-Clause | Unique identifier generation |
| `github.com/remyoudompheng/bigfft` | `v0.0.0-20230129092748-24d4a6f8daec` | BSD-3-Clause | Big integer arithmetic for pure-Go libc |
| `modernc.org/libc` | `v1.75.7` | BSD-3-Clause | C standard library emulation layer |
| `modernc.org/mathutil` | `v1.7.1` | BSD-3-Clause | Mathematical utilities |
| `modernc.org/memory` | `v1.12.1` | BSD-3-Clause | Memory management primitives |

## Embedded Web UI Dependencies

The single binary embeds pre-compiled Web UI assets. **Zero Node.js runtime or npm is required to run Orbit.**

| Package | Version | License | Purpose |
| --- | --- | --- | --- |
| `react` | `19.3.0` | MIT | Component UI library |
| `react-dom` | `19.3.0` | MIT | DOM rendering engine |
