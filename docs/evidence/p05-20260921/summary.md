# P05 peer/wire evidence summary

P05 is complete locally under the recorded loopback-process test environment.
Persistent Ed25519 identities, exact out-of-band SPKI pins, canonical approved
folder membership, TLS 1.3 mutual authentication and request-level folder
authorization are wired through the schema-v4 repository and peer listener.

The real TCP/TLS tests exchanged hello, a stable 129-entry two-page inventory,
and immutable envelopes. A version added between pages was excluded from the
old snapshot and visible after a clean restart. Expired tokens returned the
stable `SNAPSHOT_EXPIRED` code and succeeded from cursor zero. Tests rejected
unpaired, wrong-key, revoked, wrong-folder, membership-mismatched and
protocol-incompatible requests. Chunk access succeeded only by naming a ready
manifest version and valid position; knowing content bytes/digests alone did
not form an API request.

Wire parsing rejects duplicate/unknown fields, trailing JSON, noncanonical
integers and unsupported versions. Metadata requests and aggregate responses
are capped at 8 MiB, pages and version batches at 128, active requests at 32,
pre-parse admission at a 128-request burst and 64 requests/second, open
snapshots at four per peer/folder, and HTTP headers/timeouts at the documented
values. Golden fixtures and the closed field contract are under `schemas/`.

Limitations: the tests used loopback on one amd64 Linux/ext4 host. The arm64
binary was cross-built but not executed. Hosted CI, physical multi-host links,
certificate expiry/rotation, internet exposure, high-connection soak and a
partition/proxy campaign were not run. P05 exposes verified chunk serving but
does not implement transfer scheduling, persisted receive progress, receipts,
status, reconciliation, or publication; those remain P06 and later work.

Why per-request folder authorization remains necessary: mTLS proves possession
of an approved device key, not authority to read every folder or every object
known to the process. Membership can differ by folder and revision, and a
digest may be shared by content in multiple folders. Binding every inventory,
envelope and chunk request to the exact agreed folder membership prevents the
authenticated connection from becoming a global metadata or content oracle.
