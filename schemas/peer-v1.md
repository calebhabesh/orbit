# Peer wire schema v1

All peer endpoints use HTTPS with mutual TLS and `POST`. JSON objects reject
unknown fields, duplicate keys, trailing values, noncanonical decimal strings,
uppercase/non-hex IDs, and bodies over 8 MiB. Optional fields are omitted, not
`null`. Responses add a trailing newline only as JSON whitespace.

| Endpoint | Request | Success response |
| --- | --- | --- |
| `/peer/v1/hello` | `HelloRequest` | `HelloResponse` |
| `/peer/v1/inventory` | `InventoryRequest` | `InventoryResponse` |
| `/peer/v1/versions/get` | `VersionsRequest` | `VersionsResponse` containing v1 envelope objects |
| `/peer/v1/chunks/get` | `ChunkRequest` | raw bytes with exact `Content-Length` and `X-FileSync-Chunk-SHA256` |

The authoritative field spellings are the Go structs in
`internal/replication/wire.go` and the golden examples in `schemas/fixtures`.
Every JSON struct is closed (`additionalProperties: false` semantics).
Identifiers and digests are 64 lowercase hexadecimal characters. Counters,
revisions, limits, indexes, and cursors are canonical unsigned decimal strings.

Inventory tokens are 32 random bytes encoded as 64 lowercase hexadecimal
characters. A cursor is meaningful only with its token. `SNAPSHOT_EXPIRED`
requires a fresh request with no token and cursor `0`; it never licenses the
client to continue an old cursor against current state.

Errors use `ErrorResponse`: `protocol_version`, stable `code`, human `message`,
boolean `retryable`, and `action`. V1 handler codes are `UNAUTHORIZED`,
`MEMBERSHIP_MISMATCH`, `INCOMPATIBLE_VERSION`, `INVALID_REQUEST`,
`SNAPSHOT_EXPIRED`, `CONTENT_UNAVAILABLE`, `RETRY_EXHAUSTED`, and `IO_ERROR`.
