# Equal replicas with explicit conflict preservation

Accepted 2026-09-20. Use independently writable replicas with causal version histories and owner-reviewed conflict resolution; the always-on VPS is a forwarding/storage replica, not an authority. This supports disconnected work without inventing a consensus requirement, while accepting that unresolved working copies may differ and that convergence depends on eventual communication and available content.
