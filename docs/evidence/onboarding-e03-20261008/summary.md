# E03 summary — keyboard, form and paste conventions (2026-10-08)

**Complete.** F06, F12 (selector) and F15 are fixed, and the key convention is
recorded in the [terminal UX](../../orbit-terminal-ux.md#owner-amendment-2026-10-08).
[Commands](commands.md), [results](results.json).

- Forms: ↑/↓ and Tab move between fields; Enter advances and confirms on the
  last field. Startup and Connection are `‹ value ›` selectors, typing is
  ignored and ←/→ change them. A failed preview focuses the field at fault (root
  errors focus the root). Main screen: `1`–`4` and Tab switch views, and `/` searches.
- F06: a paste into the hidden invitation replaces it, and a line shows how many
  characters arrived. Ctrl-U clears it, and Enter on an empty field does
  nothing. After a failed attempt the value is kept (the W07 rule), but the next
  typed character starts over, so an unbracketed re-paste replaces it.
- F15: review and invitation files in a folder others can read get an error
  naming the folder, its mode and `mkdir -m 700` fix.
- A new real-PTY campaign (`make test-terminal-keys-pty`) drives setup with
  arrows, Tab and Enter only, pastes invitations bracketed and unbracketed after
  a failed attempt, and approves with arrows. A negative build proves it detects
  appending.

Limitations: the line-mode `orbit setup` prompts remain typed. The first full
check was stopped by host memory pressure; the second found four PTY harnesses
using the old keys (now updated), and every target passed afterwards, but not in
one uninterrupted run.
