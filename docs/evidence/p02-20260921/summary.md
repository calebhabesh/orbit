# P02 independent model and history evidence

P02 passed locally on 2026-09-21. The production `internal/history` module
uses validated version vectors, while the test-only `model` oracle computes
heads through explicit parent reachability. The oracle does not import the
production causal package. Transport, SQL and filesystem behavior remain
outside both causal implementations.

The comparison campaign exhaustively enumerated all author schedules of one
through four events for two and three actors (150 histories) and ran four
deterministic 128-event schedules using seeds 2, 17, 101 and 20260921. The
longer schedules included explicit multi-head resolutions, reordered
topological delivery, duplicates, four authors and content availability
changes. Normalized production and oracle heads agreed.

Fixtures exercise empty files, executable-only successors, equal-byte
independent versions, tombstones, three authors, third-party forwarding and a
file/child structural conflict. Malformed ancestry, altered duplicate IDs,
invented vectors, noncanonical wire counters, duplicate/unknown JSON fields,
same-author stale bases and counter overflow boundaries are rejected. Reviewed
resolution planning requires the exact current head set and token.

The required mutation demonstration replaced causal comparison in the test
with a scalar “winner,” which dropped one of two concurrent heads. The
independent reachability oracle detected the mismatch. The mutant is confined
to the test and is not present in production code.

Why receipt is not review: delivery changes what a replica knows, but it does
not change the ancestry of bytes already visible in the working copy. An
ordinary save therefore extends its persisted working basis and required
same-author lineage only. Adding every received head as a parent would claim
the user reviewed remote content and silently resolve a genuine conflict.

Limitations: these finite schedules are not a general proof. No hosted CI or
native arm64 test execution was run. P03 must make counter allocation and
version/content readiness durable; P05 must complete endpoint-level wire
schemas and errors; P07/P08 must integrate reconciliation, restore and
resolution with persisted state.
