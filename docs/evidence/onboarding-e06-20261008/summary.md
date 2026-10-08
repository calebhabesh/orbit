# E06 summary — short pairing code through the Orbit service (2026-10-08)

**Complete; EG1 closed.** Code at `0da2e7d`. [Commands](commands.md),
[results](results.json), [test log](logs/e06-tests.log).

- **Construction:** CPaceRistretto255/SHA-512, initiator/responder,
  pinned to draft-irtf-cfrg-cpace-21 on `gtank/ristretto255 v0.2.0`. The
  published Appendix B.3 generator, message and ISK vectors pass. A private
  scalar is consumed once; a wrong password or reused exchange fails.
- **Code:** `XXXX-XXXX`, eight Crockford base32 characters. The first half
  names a public mailbox (20 bits) and the second is the password (20 bits):
  one guess per code, so an online guess succeeds with probability 2^-20.
- **Service:** in-memory mailboxes (128 total, 4 per owner key, 10 minutes),
  5 create/claim requests per source with 1/10 s refill. A wrong code burns
  the mailbox and tells the inviter; a restart loses mailboxes ("ask for a new
  code"). The service sees mailbox, device IDs/pins, timing, public PAKE
  elements and invitation ciphertext only. The synthetic capture holds neither
  the invitation nor the code.
- **Clients:** `orbit devices invite --code` prints the short code when the
  service advertises `short_pairing_v1`, and `--long` prints the long one.
  The TUI Add device screen shows the code, its state and `r` for a new one,
  with `v`/`s` for the long invitation. The join prompt and `orbit join` take
  a code, a long invitation or a file path. The decrypted invitation must match
  the CPace inviter's device and pin and the selected operator. Approval
  is still required.
- **Privacy text:** the profile statement stays accurate; no epoch change.
- **Docs:** [WAN protocol](../../orbit-wan-protocol.md), [WAN UX](../../orbit-wan-ux.md),
  [operator guide](../../orbit-net-operator.md#short-pairing-codes-e06),
  [control schema](../../../schemas/terminal-control-v1.md).

Found and fixed during acceptance (E10 session): the full headless short-code
join planned `manual` startup on a fresh headless host with lingering, because
`systemctl --user show -p ExecStart --value` prints an empty line for a missing
unit, which the service selection read as "another unit". It is now trimmed.

Deployment: orbit-net 1.1.0 is live on the hosted service since 2026-10-08
([E10](../onboarding-e10-20261008/summary.md)). Not executed: a short-code join
between physical devices through the hosted service (the owner's trial).
