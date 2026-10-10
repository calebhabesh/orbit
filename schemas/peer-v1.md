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
| `/peer/v1/chunks/get` | `ChunkRequest` | raw bytes with exact `Content-Length` and `X-Orbit-Chunk-SHA256` |

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

`POST /peer/v1/lan` carries `LANExchange` (`version` `"1"`, `records`: at most
eight signed `orbit-lan-v1` records) in both directions between approved
peers; see [the WAN protocol](../docs/orbit-wan-protocol.md#peer-lan-exchange-post-w17-2026-10-07).
Peers without LAN advertising, or that predate it, answer `404 INVALID_REQUEST`.

T05 membership inspection/rollout uses `POST /peer/v1/membership/get` with
`MembershipGetRequest`/`MembershipGetResponse`. It authenticates the claimed
member key and denies retired/nonmember clients. Optional `from_revision` is a
positive canonical decimal string; when present, `expected_digest` must match
that exact predecessor. Return one successor (or the same current revision);
`MEMBERSHIP_FORK` (409, nonretryable) identifies disagreement, while a requester
ahead of the server receives `MEMBERSHIP_MISMATCH`. Without these optional fields,
legacy latest-membership inspection remains. Older closed decoders reject the
new fields safely; automatic rollout needs a T05-capable peer, while exact-agreement
legacy data exchange remains compatible. Membership's existing canonical encoding
and exact-revision data authorization are unchanged.


## E13 participation/retirement extension — 2026-10-09

`PEER_LEFT` (403, nonretryable) is returned only after member/key authentication
when the serving device left this folder. `DEVICE_REMOVED` (403, nonretryable)
is returned for the requesting retired ID authenticated by its historical pin.
Its additive optional `removed_by` is a 64-hex device ID. Older closed decoders
may reject the enriched error safely; upgrade survivors before using removal.

`POST /peer/v1/membership/retirement` accepts strict `RetirementRequest`:
`protocol_version`, 64-hex `device_id`, `action` (`prepare` or `commit`) and
`proposal`. The proposal fields are `initiator`, `device_name`, `membership`,
`snapshot`; membership/snapshot use the existing native artifact JSON also used
by `MembershipGetResponse` (byte arrays/native revisions), not the decimal-string
base data wrappers. Canonical membership/retirement digest encodings are unchanged.
The exact structs are in `replication/retirement.go`,
`repository/retirement_review.go` and `protocol/membership.go`; normal peer body
limits apply. Success has `protocol_version`, `state` (`prepared`/`completed`)
and 64-hex `digest`. Mismatched reviews return `RETIREMENT_CHANGED` (409,
nonretryable); competing intents return `MEMBERSHIP_FORK` (409, nonretryable).

The TLS requester must be the proposal initiator, a pinned active member of
its immutable predecessor who remains active now. Preparation alone gives no
data authorization. Commit requires the persisted exact proposal and an atomic
accepted-retiree-set recheck. Duplicate exact commits succeed. Missing support on
an older survivor leaves removal pending rather than falling back to unreviewed
retirement. Only an already prepared matching successor can use automatic
membership-get catch-up. Protocol/persistence own these semantics.
