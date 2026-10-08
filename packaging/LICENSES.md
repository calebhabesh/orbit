# Third-Party Licenses and Legal Notices

This repository and the compiled `orbit` binary include code from external Go modules and third-party projects. All included code is distributed under permissive open-source licenses compatible with the project license.

See the root [`NOTICE`](../NOTICE) file for complete copyright assertions and license texts.

## Direct Dependencies

| Module | Version | License | Direct Purpose |
| --- | --- | --- | --- |
| `github.com/coder/websocket` | `v1.8.15` | ISC | WSS binary-stream transport adapter (W01; daemon integration W02/W04) |
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

## T09 Terminal Dependencies

Exact license texts and copyright notices are retained in `NOTICE` and included in every archive/package.

| Module | Version | License |
| --- | --- | --- |
| `charm.land/bubbles/v2` | `v2.2.1` | MIT |
| `charm.land/bubbletea/v2` | `v2.0.10` | MIT |
| `charm.land/lipgloss/v2` | `v2.0.6` | MIT |
| `github.com/atotto/clipboard` | `v0.1.4` | BSD-3-Clause |
| `github.com/charmbracelet/colorprofile` | `v0.4.3` | MIT |
| `github.com/charmbracelet/ultraviolet` | `v0.0.0-20260811164956-006e29f97886` | MIT |
| `github.com/charmbracelet/x/ansi` | `v0.11.8` | MIT |
| `github.com/charmbracelet/x/term` | `v0.2.2` | MIT |
| `github.com/charmbracelet/x/termios` | `v0.1.1` | MIT |
| `github.com/charmbracelet/x/windows` | `v0.2.2` | MIT |
| `github.com/clipperhouse/displaywidth` | `v0.11.0` | MIT |
| `github.com/clipperhouse/uax29/v2` | `v2.7.0` | MIT |
| `github.com/lucasb-eyer/go-colorful` | `v1.4.1` | MIT |
| `github.com/mattn/go-runewidth` | `v0.0.27` | MIT |
| `github.com/muesli/cancelreader` | `v0.2.2` | MIT |
| `github.com/rivo/uniseg` | `v0.4.7` | MIT |
| `github.com/xo/terminfo` | `v0.0.0-20220910002029-abceb7e1c41e` | MIT |
| `golang.org/x/sync` | `v0.22.0` | BSD-3-Clause |

## W09 QUIC and ICE API dependencies

Pion is imported by the adapter compatibility tests; production ICE establishment remains W10. HTTP/3 is linked into the daemon. Exact license texts are included in `NOTICE`.

| Module | Version | License |
| --- | --- | --- |
| `github.com/pion/dtls/v3` | `v3.1.9` | MIT |
| `github.com/pion/ice/v4` | `v4.4.6` | MIT |
| `github.com/pion/logging` | `v0.2.4` | MIT |
| `github.com/pion/mdns/v2` | `v2.2.2` | MIT |
| `github.com/pion/randutil` | `v0.1.0` | MIT |
| `github.com/pion/stun/v4` | `v4.0.1` | MIT |
| `github.com/pion/transport/v5` | `v5.0.1` | MIT |
| `github.com/pion/turn/v5` | `v5.1.2` | MIT |
| `github.com/quic-go/go-ossfuzz-seeds` | `v0.1.0` | MIT |
| `github.com/quic-go/qpack` | `v0.6.0` | MIT |
| `github.com/quic-go/quic-go` | `v0.63.0` | MIT |
| `github.com/wlynxg/anet` | `v0.0.5` | BSD-3-Clause |
| `golang.org/x/crypto` | `v0.54.0` | BSD-3-Clause |
| `golang.org/x/net` | `v0.56.0` | BSD-3-Clause |
| `golang.org/x/text` | `v0.40.0` | BSD-3-Clause |
| `golang.org/x/time` | `v0.14.0` | BSD-3-Clause |

## E06 CPace curve arithmetic

| Module | Version | License |
| --- | --- | --- |
| `github.com/gtank/ristretto255` | `v0.2.0` | BSD-3-Clause |
| `filippo.io/edwards25519` | `v1.1.0` | BSD-3-Clause |

Orbit pins CPace to draft-irtf-cfrg-cpace-21 and tests the published vectors.
These dependency licenses are reproduced in NOTICE.
