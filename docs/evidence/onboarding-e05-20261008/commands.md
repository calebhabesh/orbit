# E05 commands — 2026-10-08

Base revision `176da46`, Go `go1.27.1-X:nodwarf5 linux/amd64`, dev PC; all
state in marked disposable roots.

```sh
go test -list '^TestOnboardingE' ./internal/... ./cmd/orbit/... ./tests/...   # logs/test-list.txt: 33 names
go test ./internal/terminal -run '^TestOnboardingE05' -count=1 -v              # logs/e05-tests.log: 3 PASS
make test-terminal-keys-pty      # logs/keys-pty.log: passed, including at 80 and 200 columns:
                                 #   v reveals one line with no CR/LF/ESC inside, decoding to the
                                 #   same invitation as the saved file; screen+scrollback cleared
                                 #   (ESC[3J); c emits OSC 52 whose payload base64-decodes to the
                                 #   exact code and is within the 16 KiB bound; s saves to the
                                 #   default 0600 file in the 0700 state folder; the join prompt
                                 #   accepts that file's path
GOFLAGS=-p=4 make check          # exit 0, uninterrupted (24 min)
make test-terminal-pty test-terminal-onboarding-pty test-terminal-everyday-pty test-terminal-keys-pty   # all exit 0
```

During development the 200-column run found two invitations created in the
same minute sharing a default file name (IDEMPOTENCY_CONFLICT on save). The
name now carries a 4-byte digest of the whole invitation. The WAN PTY harness's
hide step was updated for the new reveal.
