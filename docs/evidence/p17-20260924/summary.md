# Historical P17 demonstration — corrected assessment

The original run reported a 37.53-second scripted workstation/Pi/VPS campaign.
Its [raw results](pilot_results.json) remain historical self-reported output.
The harness did not actually interrupt its resumption scenario, asserted some
outcomes incompletely, deleted existing roots before checking markers, and
used process-name-wide termination. It must not be used as release acceptance
or as a personal-use pilot.

The [old benchmark](benchmarks.json) contains calculated/hard-coded byte counts
and percentages. Its discard-only HTTP baseline does not match File Sync's
verification/storage work. The numeric savings and timing comparisons are
withdrawn, including the internally inconsistent unchanged-tree comparison.
Those raw files are retained to make the correction auditable.

Use the [2026-10-01 report](../release-20261001/summary.md) and the replacement
harnesses under `scripts/validation`. P17 remains in progress until the laptop
campaign and actual personal-use evidence meet their acceptance criteria.
