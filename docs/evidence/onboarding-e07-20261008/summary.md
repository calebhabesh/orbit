# E07 summary — approval waiting within service limits (2026-10-08)

**Complete.** F10 is fixed. [Commands](commands.md), [results](results.json).

A joining device checks its status every 30–36 s while it awaits approval,
instead of every 15 s. Each check costs two requests against the inviter's
5-per-minute source bucket, so a wait now averages about 3.6 requests a minute.
A `RATE_LIMITED` reply during a join pauses it and is shown as "the inviting
device asked us to slow down", not as a blocking error. Two real daemons waited
30 minutes unapproved against production limits with no refusal; approval was
noticed after 8 s. In-process fixtures that advance by sleeping 25 s set the
old 15 s spacing explicitly through `control.Options.ApprovalPoll`.

Limitations: approval is polled rather than pushed, so it is noticed within
one interval (at most 36 s).
