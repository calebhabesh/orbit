# Terminal conflict and recovery walkthrough

Run these commands inside a registered folder, or use absolute paths with
`--folder <name>`. Relative paths remain relative to your shell's current directory.
Keep review files outside synced roots. All commands accept `--state` and `--json`.

## Select a version or restore history

```sh
orbit conflicts --folder Documents --limit 50
orbit conflicts show report.txt --out /private/review.json --json
orbit conflicts select report.txt --selected AUTHOR:COUNTER --review-file /private/review.json --operation OPERATION_ID
orbit history report.txt --limit 50 --json
orbit restore report.txt --version AUTHOR:COUNTER --out /private/restore.json --json
orbit restore report.txt --version AUTHOR:COUNTER --review-file /private/restore.json --operation OPERATION_ID
```

Use actual IDs from the query; `OPERATION_ID` is a fresh random 32-byte lowercase
hex value. Retain it for response-loss replay. Without `--review-file`, select and
restore only preview. Preview shows captured working state, exact heads, replacement
path and source availability. Uncaptured working edits require capture and a new
review; they are never silently replaced. New versions invalidate the old review.
Publication can remain pending even after a new version has durably committed.
Inspect the returned operation ID; replay resumes its existing publication.

Restore creates a new version whose parents are the reviewed current heads.
The historical source identifies the recovered bytes, rather than replacing
current ancestry. Display timestamps neither establish causality nor choose winners.
Deleted paths have history candidates, with locally observed content states.
Pending, expired, missing and corrupt content cannot be substituted or restored.
This interface offers no unverified peer-fetch shortcut or fixed deletion timer.

## Keep copies or recover a separate synced copy

Preview each explicit destination before committing:

```sh
orbit conflicts show report.txt --version AUTHOR_A:COUNTER_A --to report-a.txt --out /private/copy-a.json --json
orbit conflicts show report.txt --version AUTHOR_B:COUNTER_B --to report-b.txt --out /private/copy-b.json --json
```

Create a private JSON array of `CopyPlan` objects. Each object has `source` from
its preview's `content_review.source.version`, `destination` from the preview,
and `review` from `content_review.destination_review`. Supply that file with:

```sh
orbit conflicts keep-copies report.txt --selected AUTHOR_A:COUNTER_A --review-file /private/review.json --copies-file /private/copies.json --operation OPERATION_ID
```

Copies commit individually before the original resolution. The operation ledger
retains exact destination versions through interruption, and replay resumes without
duplicating them. There is no atomic visibility across paths. Collisions require a
new destination plan. Structural conflicts remain separate path reviews; selecting
an ancestor does not silently resolve or erase a descendant.

For a separate recovery copy, use `restore --to recovered.txt` for both preview
and commit. For external recovery, use the exact verified export operation:

```sh
orbit export report.txt --version AUTHOR:COUNTER --out /external/recovered.txt
```

External export publishes only a completed verified stream and refuses existing
destinations or paths inside registered roots. Export failures leave the destination
absent. Source content is protected against GC for the response's complete lifetime.

## Editor and merge sessions

```sh
orbit conflicts edit report.txt --review-file /private/review.json --tool 'editor --wait' --json
```

Tools run through direct argv parsing, never shell expansion. `diff` appends the
private `source-NN` files; `edit` appends the private `result` path. Source exports
are verified exact versions and read-only. Sessions reserve exports, an editable
result, and one immutable upload before writes. Result capacity is the largest
source size, with a 1 MiB minimum. The Linux util-linux `prlimit` adapter applies
that file-size bound before launching an editor; missing tooling refuses launch.
Tools are trusted owner programs, rather than a general host sandbox.

`edit` or `upload --file <result>` streams the output into immutable staging and
returns `upload.id`, `upload.session`, `upload.digest` and `upload.bytes`. Inspect
those bytes/digest before issuing a separate merge commit:

```sh
orbit conflicts merge report.txt --review-file /private/review.json --session SESSION_ID --upload UPLOAD_ID --digest SHA256 --bytes SIZE --operation OPERATION_ID
```

Merge rechecks original review, session, upload size/digest, membership, heads and
working bytes. It preserves the first source's executable setting. A new arrival
or working edit rejects the old review; the adapter never refreshes and resubmits
on the user's behalf. One upload is admitted per session. Interrupted staging
retains a budgeted recovery spool and cannot become a captured version.

Sessions expire after 300 seconds from creation or explicit renewal. Renew before
expiry using the original review and `conflicts renew`. `conflicts session` shows
retained result paths after client close or daemon restart. Cancel closes further
commits and preserves candidates. After inspecting them, obtain a fresh content
review and use `conflicts discard` for explicit cleanup. Unknown auxiliary files
prevent complete cleanup and remain for inspection. GC can release expired session
pins; independent active read pins remain protected. Review and replay metadata
retain expired-ID guards rather than recycling identities.

Interactive PTY editor suspend/resume belongs to T11/T13. Cross-host LAN/Tailscale,
native boot/logout and P17 actual owner use and unaided explanation remain separate
release evidence.
