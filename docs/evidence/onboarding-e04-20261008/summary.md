# E04 summary — join and setup defaults, actionable onboarding errors (2026-10-08)

**Complete.** F07, F08 and F11 are fixed and F16 is documented. [Commands](commands.md), [results](results.json).

- **F08:** `tc.JoinPolicy` proposes the joiner's connection, the same way in the
  CLI and the TUI. For a routed invitation it is the inviter's operator:
  Automatic when that is the packaged profile, self-hosted otherwise, using the
  profile carried in the invitation. The review shows the operator and its
  privacy text, and confirming installs that profile through the ordinary
  profile review. When the running daemon started with another policy, the join
  records phase `network_restart`, the client restarts the daemon and the job
  continues. A fresh disposable device joined a fresh inviter over a local
  service fixture by accepting every default.
- **F07:** a cut-short or damaged code is `INVITATION_INCOMPLETE`: "copy the
  whole line again". Other classes keep their specific codes.
- **F11:** on approval, the inviter names the device with the label the joiner
  chose, minus non-printable characters; text, JSON and TUI listings show it.
  The joiner's wait screen explains the approval and shows the code to compare.
- **F16:** received files are owner-only, as documented in persistence and on
  the setup form.

Limitations: self-hosted operators with private CA roots still need
`orbit network set --service-roots` first. The wait screen cannot name the
inviter. One uninterrupted `make check` after the last fix was not run; every
target and the WAN/onboarding subset passed afterwards.
