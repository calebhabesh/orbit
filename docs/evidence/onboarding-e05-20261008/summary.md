# E05 summary — long invitation display, copy and file transfer (2026-10-08)

**Complete.** F05 is fixed. [Commands](commands.md), [results](results.json).

The invitation screen no longer draws the code in a panel, so nothing can be
cut off. It shows the character count and three actions. `c` copies the code
with OSC 52, bounded, or says copying is not supported here. `v` prints it as
one unbroken line on the ordinary screen and clears that screen and its
scrollback on return. `s` saves it to a private default file in the state
folder and shows the `scp` and `orbit join --invitation-file` commands. The
join prompt accepts a code or a file path. A real PTY run at 80 and 200 columns
reconstructs the exact code, checks the OSC 52 payload and the private file,
and joins via the file path. A full uninterrupted `make check` passes.

Limitations: terminals do not acknowledge OSC 52. Copying a soft-wrapped line
as one line relies on the terminal emulator's selection, which is standard in
common emulators.
