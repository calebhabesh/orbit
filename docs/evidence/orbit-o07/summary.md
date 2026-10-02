# O07 — Hierarchical browse, search and authenticated content

Completed on 2026-10-02 in the existing development worktree. O02 and the O01
G03 read-lifetime decision were checked; existing O06 work was preserved.
No release/pilot completion is inferred from this packet.

Implemented immediate-child directory navigation, literal workspace path search,
stable sorting and paginated history/Deleted files. SQL includes accepted heads
without projections, empty directories/scaffolds, safe implicit ancestors,
pending content and structural/conflicting history. Strict relative paths and
workspace/query-bound cursors fail explicitly; a durable schema-12 generation
invalidates pages after captured/received/projection/content changes.

Authenticated exact-version GET downloads use a verified, pinned, seekable
repository reader. They support native attachments and byte ranges without
whole-file allocation, and use a per-write idle timeout for progressing large
files. UTF-8 text and PNG/JPEG previews enforce encoded byte and pixel budgets,
with safe MIME/CSP/cache/disposition headers. GIF/APNG/SVG/HTML content cannot
execute as preview application content. Existing CLI export also uses the
pinned reader; new browse/search/details commands share controller operations
and have tested stopped/live-daemon parity.

Production tests prove active stream pins survive concurrent GC and read-record
expiry, cancellation releases pins, GC can subsequently reclaim superseded
bytes, restart clears abandoned stream pins, and missing/expired/corrupt exact
versions never select replacement content. Repeated manifest chunks acquire
one pin. Acquisition checks GC intents at the serialized mutation boundary.
Corrupt reads invoke existing quarantine and return a diagnosed error.

All 13 targeted tests passed in [the final log](logs/targeted.log), including
actual HTTP, browser-cookie authentication, CLI controller/API parity,
strict paths, budget/active-format rejection, ranges and cancellation/GC.
`make check` passed [all pipeline targets](logs/make-check.log), including model,
fault/harness, both architectures and tar/deb/rpm packages for each.
`make test-race` passed [all packages](logs/test-race.log). The initial race run
failed timing assertions, with no data-race report; these observations are
retained and timing is reported as measurements rather than a universal SLA.
Documentation validation checked 95 Markdown files and 433 local links, with
zero missing targets; [link results](logs/document-links.json). See
[commands and failure corrections](commands.md).

Synthetic local measurements, one sample per operation, warm filesystem cache:

| Fixture / measurement | Observed result |
| --- | --- |
| 10,000 paths, 100 directories: root page, 50 entries | 109.31 ms in standalone resource run |
| Same fixture: directory page, 50 entries | 152.75 ms in standalone resource run |
| Same fixture: substring search, 50 results of 100 | 102.66 ms in standalone resource run |
| Root-page JSON | 8,754 bytes |
| Three queries: Go allocations | approximately 0.59 MiB |
| Entire standalone test process, including fixture creation | 28,944 KiB maximum RSS |
| 32 MiB repeated-chunk pinned read plus GC: Go allocations | approximately 1.2 MiB |
| 64 MiB actual TLS HTTP download: Go allocations | 1,726,760 bytes |

Exact timings and resource scope are in [resource log](logs/scaling-resource.log)
and [targeted log](logs/targeted.log). SQLite scans/groups locally known paths
within a selected workspace; a folder index is not a substring/full-text index.
Only bounded pages are materialized in Go. These synthetic observations are not
latency guarantees, tail percentiles, or benchmarks on laptop/Pi/VPS hardware.

Limitations: the browser's hierarchy/preview/details UX and keyboard scenarios
are O08 work and were **unexecuted** for O07. O09 file mutations are also open.
Raster header validation bounds declared single-frame dimensions; malformed
images can still fail browser decoding. Native installation/network performance,
new power-loss experiments and final product pilot remain unexecuted here.
Persistent causal metadata is not compacted. Global cursor generation can
invalidate a page because a different workspace changed. Copy/contact data is
local history, never a claim of global synchronization or perpetual availability.
P17 owner-use and unaided explanation remain outstanding.

Source provenance, hashes and measured outcomes are in [manifest](manifest.json)
and [results](results.json). Base commit is recorded with dirty-worktree hashes;
this is not a clean committed release reproduction.
