# T01 contracts and design gates

T01 freezes the additive terminal control boundary and resolves TG1–TG5 designs.
The contract is not yet mounted in the production binary. T00's opt-in failing
reproductions remain unchanged; production proof belongs to T02–T12. Historical
P/O evidence and existing relocation work are preserved. P17 actual owner use
and unaided explanation remain outstanding.

The [schema](../../../schemas/terminal-control-v1.md) defines typed setup/adoption,
invitation/join/sharing/approval, durable operation inspection, qualified status/
attention, named context, exact reads, editor sessions, reviewed conflict/restore,
finite settings and startup modes. It freezes strict decimal-string integers,
capabilities/versioning, request fingerprints, expired replay guards, explicit
reviews, error/retry semantics and human/JSON/script behavior. The
[source manifest](../../implementation/terminal-source-ownership.md) assigns exact
files and integration owners. Eleven synthetic presentation fixtures cover
success, empty, loading, awaiting approval, offline, stale, storage blocked,
root unavailable, partial, fork and conflict.

[Gate decisions](../../terminal-design-gates.md) record:

- TG1: isolated enrollment TLS listener preserves existing mandatory peer mTLS;
  exact inviter certificate/SPKI verification precedes disclosure; request/status
  proofs bind scope, key, attempt, nonce, expiry and endpoints. Approval still
  binds exact owner-reviewed key/folder/prior membership.
- TG2: descriptor/root generation review, bounded recursive preview and explicit
  capacity assumptions; persisted join phases and observed capture/membership/
  content/publication determine readiness. Bootstrap absence cannot delete.
- TG3: shared authenticated live or exclusively locked stopped controls;
  a failed live call never grants database ownership. Mutation identity and
  reviewed inputs survive disconnect, late responses, cancellation and restart.
- TG4: exact verified streams and pins, private editor sessions, stale-head
  rejection, staged upload fingerprints and recovery candidates. Historical
  source remains provenance; restore ancestry follows current reviewed heads.
- TG5: TTY versus pipe dispatch, separate client/daemon lifetime, terminal
  ownership transitions, honest login/unattended observations and legacy identity/
  state preservation. Actual library/PTY/package evidence is deferred explicitly.

Final discovered group: **17 tests** in three packages; **34 passes** with
`-count=2`. The targeted race run passed all 17 with no race report. Experiments
include actual prototype TLS pin/admission checks and the existing exclusive
state lock, plus bounded models for atomic capability admission, serialized
restart/replay, review/readiness, session pin expiry and terminal lifecycle.
Golden canonical transcript bytes are generated independently. A synthetic
8 MiB stream uses a 1 MiB transfer buffer and incremental digest; this is not
production RSS or editor evidence.

Final serial `make check` and `make test-race` both passed. Documentation/link
validation and `git diff --check` passed. Broad validation results are recorded in [results.json](results.json) and
[commands](commands.md), with full [transcripts](transcripts/targeted.txt).
Production endpoint integration, durability/failure claims, native networking,
PTY/service/package journeys and owner observations remain **unexecuted** here.
Model serialization is not a crash-durability proof; fixture rendering cannot
establish syncing or authorization. Schema 13, peer protocol 1 and dependencies
are unchanged. The finite metadata budget may block admission rather than prune
replay guards or protected history; no indefinite throughput claim is introduced.

Next eligible packet: **T02 shared client and daemon lifecycle**. Build the
frozen client seam, one finite initializer, exclusive startup/recovery and
network/listener settings. Lifecycle/settings mutations need a durable replay
ledger in T02; T04 later extends it to onboarding jobs. T03 follows with the
actual pinned enrollment listener and authenticated status/approval. Promote T00
cases only when the owning production-interface regressions pass.

Worker explanation: a screen can disappear after the daemon commits a request
but before its response arrives. Persisted operation identity/fingerprint lets
another client inspect or retry exactly that work; reviewed generations prevent
the retry from approving changed roots/heads. Screen-local drafts or new IDs
would lose that distinction and can duplicate effects or skip review. This is
a worker explanation, not the owner's required unaided exercise.
