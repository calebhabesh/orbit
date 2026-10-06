# W01 dependency and API audit

Selected `github.com/coder/websocket v1.8.15` from actual
`go list -m -json github.com/coder/websocket@latest` (reported release time
2026-06-15, module GoVersion 1.23). Installed with
`go get github.com/coder/websocket@v1.8.15`, then `go mod tidy`; only that direct
module and its two checksum records were added. Actual Go toolchain:
`go1.27.1-X:nodwarf5 linux/amd64`. No new transitive module.

Official [repository](https://github.com/coder/websocket) and
[NetConn API](https://pkg.go.dev/github.com/coder/websocket#NetConn) were read.
The exact installed `netconn.go`, `read.go`, `write.go`, `dial.go`, `accept.go` and
`LICENSE.txt` were inspected. ISC copyright Coder 2025; exact license text appended
to root NOTICE and dependency added to packaging/LICENSES.md. Module checksum:
`h1:6B2JPeOGlpff2Uz6vOEH1Vzpi0iUz20A+lPVhPHtNUA=`;
go.mod checksum `h1:NX3SzP+inril6yawo5CQXx8+fk145lPDC6pumgx0mVg=`.

Verified actual exports: `Dial`, `Accept`, `NetConn`, `Reader`, `Write`,
`SetReadLimit`, `CloseNow`, `CompressionDisabled` and net.Conn deadlines.
NetConn sets read limit -1, each Write is one binary message, active deadline
expiration cancels its read context, and client addresses are synthetic.
Orbit reads bounded frames itself, uses NetConn for writes/write deadlines,
supplies reversible read deadlines and logical relay admission addresses, and
closes/joins its pump. Default bufio readers/writers are finite (standard 4 KiB);
compression is disabled, so no flate pools/file-sized decompression. Fixed input
frame limit plus one byte detects overflow before any over-limit frame is exposed.

Both `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...` and arm64 counterpart
are recorded in results.json after execution. These compile the module/adapters;
only amd64 tests execute here, not native Pi load or real WAN. W04 adds service
admission/lifetime quotas. W09/W10 dependency/API proof remains independent.
