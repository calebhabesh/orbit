# Conservative membership and causal metadata retention

Design baseline 2026-09-20, subject to D3/D4 experiments. Use owner-distributed matching membership revisions and retain accepted causal/tombstone metadata while implementing safe historical-content cleanup. This favors auditable retirement and deletion safety over seamless membership changes and metadata compaction; finite metadata budgets can pause admission and require a later explicit migration rather than silently dropping ancestry.
