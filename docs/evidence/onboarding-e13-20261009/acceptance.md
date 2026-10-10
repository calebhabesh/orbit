# E13 acceptance map

| Requirement | Evidence |
| --- | --- |
| Leave keeps files/history, rejects stale root and fresh/retried work, persists across restart | `TestOnboardingE13LeavePreservesFilesHistoryAndStopsWorkAcrossRestart`; CLI stopped/live test; real participation PTY |
| Cancel/drain only the selected Orbit; refuse every peer data/membership endpoint without revealing state to an unauthorized identity | `TestOnboardingE13LeaveCancelsAndDrainsOnlyItsOrbit`; `TestOnboardingE13LeftOrbitRefusesEveryDataEndpointAndMembership` |
| Exact name, received-change count, immutable reviewed membership/history, cancel terminal actions without mutation; stale pending reviews require fresh confirmation | `TestOnboardingE13RemovalRequiresNameAndExactReceivedReview`; `TestOnboardingE13PendingRemovalRequiresFreshReviewBeforeContactingSurvivors`; `TestOnboardingE13RemovalNameAndExactRetry`; participation PTY |
| All survivors prepare; offline/divergent history does not advance; partial commit/restart resumes exact operation | `TestOnboardingE13ThreeDevicesOfflineDivergentAndInterruptedRollout` (three-device membership, two actual pinned mTLS servers); `TestOnboardingE13RetirementPrepareCommitAndLateHistory` |
| Deduplicated PEER_LEFT attention; learned removal stops the Orbit and shows persisted initiator or legacy reporter | `TestOnboardingE13RemovedScreenAndDeduplicatedPeerLeftAttention`; `TestOnboardingE13RemovalPickerArrowsAndRemovedScreen`; wire attribution assertions; participation PTY |
| Retain pending publication/recovery; stop restart publication; canceled work cannot revive; cleanup resumes after completion | `TestOnboardingE13ParticipationEndPreservesPendingRecoveryWithoutApplying` (marked ENOSPC fixture); control cancellation assertions; `TestGCSuspendedDuringMaintenance` and three-device cleanup assertions |
| Preserve schema 14 trial metadata on migration to 15; install both native architectures with prior data retained | All three `trial-verify-*.log` files; `trial-install.log`; `trial-after.log` |
| Retain E11/E12 polish and broader regression evidence | Final quick suite (five PTYs), focused/broad race and integration reruns, model/fault/harness checks, package manifest/payload checks; initial full terminal run plus the corrected sixteen-test rerun |

Actual dispositions and remaining limitations are in [results](results.json),
[commands](commands.md) and [summary](summary.md). A running or interrupted
check is not accepted as a pass.
