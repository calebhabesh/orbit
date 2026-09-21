# Separate causal history, immutable content and working copies

Accepted 2026-09-20. Keep recorded causal state and immutable retained content distinct from the editable working folder, with a recovery journal bridging publication and metadata. This lets conflicts, pending transfers and failed publication remain visible without treating filesystem replacement as a database transaction; it costs storage and requires explicit retention and recovery protocols.
