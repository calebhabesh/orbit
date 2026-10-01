# Named invariant and scenario evidence

The clean source checkout at `e13e53a` passed its named Go checks, process
faults, local demo, 16-case VM-reset matrix and five storage-failure cases.
[Raw reproduction](release-candidate/reproduction.json) and
[verbose fault output](release-candidate/check.log) are authoritative.
The scenario catalog test checks descriptions only; it is not execution
evidence. These outcomes are scoped tests rather than universal proofs.

| Invariant | Named check | Result |
| --- | --- | --- |
| I01: Immutable version ID has exactly one envelope; duplicate delivery creates no logical duplicate | `TestP16InvariantI01_ImmutableVersionIDOneEnvelope` | PASS in clean-checkout fault suite |
| I02: Same valid causal history yields equivalent heads regardless of delivery order | `TestP16InvariantI02_SameValidHistoryEquivalentHeads` | PASS in clean-checkout fault suite |
| I03: Concurrent content and edit/delete heads survive until explicitly covered by a reviewed resolution | `TestP16InvariantI03_ConcurrentHeadsSurviveUntilResolution` | PASS in clean-checkout fault suite |
| I04: Ordinary capture does not implicitly resolve received but unreviewed heads | `TestP16InvariantI04_OrdinaryCaptureDoesNotResolveReceivedHeads` | PASS in clean-checkout fault suite |
| I05: No stored receipt without durable metadata and required verified content | `TestP16InvariantI05_NoStoredReceiptWithoutDurableMetadataAndContent` | PASS in clean-checkout fault suite |
| I06: Partial or corrupt content is never published as complete | `TestP16InvariantI06_PartialOrCorruptContentNeverPublished` | PASS in clean-checkout fault suite |
| I07: Recovery preserves previously durable protected versions and reports ambiguity | `TestP16InvariantI07_RecoveryPreservesProtectedVersionsAndReportsAmbiguity` | PASS in clean-checkout fault suite |
| I08: Counter and event creation are atomic; identity rollback is never knowingly reused | `TestP16InvariantI08_AtomicCounterAndNoIdentityRollbackReuse` | PASS in clean-checkout fault suite |
| I09: Peer input cannot escape its authorized folder or request unauthorized objects | `TestP16InvariantI09_PeerInputCannotEscapeAuthorizedFolder` | PASS in clean-checkout fault suite |
| I10: GC never removes protected content; expiry never erases causal knowledge | `TestP16InvariantI10_GCNeverRemovesProtectedContent` | PASS in clean-checkout fault suite |
| I11: Unavailable roots/incomplete scans/bootstrap absence do not create deletions | `TestP16InvariantI11_UnavailableRootsIncompleteScansNeverDelete` | PASS in clean-checkout fault suite |
| I12: Concurrent structural operations preserve incompatible histories without recursive destructive replacement | `TestP16InvariantI12_StructuralOperationsPreserveChildBytes` | PASS in clean-checkout fault suite |
| I13: Work and resource use are bounded; large-file work eventually progresses when resources are available | `TestP16InvariantI13_BoundedWorkAndResourceLimits` | PASS in clean-checkout fault suite |
| I14: Third-party forwarding preserves author/ancestry; hub availability is not final-device receipt | `TestP16InvariantI14_ThirdPartyForwardingPreservesAuthorAncestry` | PASS in clean-checkout fault suite |
| I15: Membership disagreement/retirement cannot silently admit excluded old histories or resurrect deletion | `TestP16InvariantI15_MembershipRetirementRejectionAndStaleRejoin` | PASS in clean-checkout fault suite |
| I16: Restore/resolution replay is idempotent and stale reviewed state is rejected | `TestP16InvariantI16_RestoreResolutionIdempotentAndStaleRejected` | PASS in clean-checkout fault suite |
| I17: Scan after apply/restart does not fabricate local edits | `TestP16InvariantI17_ScanAfterApplyDoesNotFabricateEdits` | PASS in clean-checkout fault suite |
| I18: Corruption yields unavailable state or verified repair, never substitute contents | `TestP16InvariantI18_CorruptionYieldsUnavailableOrVerifiedRepair` | PASS in clean-checkout fault suite |
| I19: UI and CLI issue the same operations and display qualified progress | `TestP16InvariantI19_UICLIParityAndQualifiedProgress` | PASS in clean-checkout fault suite |
| I20: Limits, schema/protocol incompatibility and failed migrations preserve recoverable state | `TestP16InvariantI20_IncompatibleSchemaLimitsPreserveRecoverableState` | PASS in clean-checkout fault suite |

## Scenario evidence and limits

All Go references below exist and passed in the clean-checkout command. Native
campaigns and VM experiments are separately linked; local/model execution is
not described as physical-host fault proof.

| Scenario | Actual evidence | Scope / limitation |
| --- | --- | --- |
| Three offline edits / reconnect orders | Independent bounded model; `TestP16InvariantI02_SameValidHistoryEquivalentHeads`; packaged native three-head campaign | Enumerated model bounds and one native schedule, not every possible network trace |
| Resolve A/B then receive C | `TestD2ResolutionCoversOnlyReviewedHeads`; native late-C and stale-token assertions | Explicit reviewed heads only |
| Equal-byte independent edits | `TestReconciliationEqualByteConflict`, `TestD2EqualBytesDoNotEraseAncestry` | Distinct ancestry remains |
| Same-author stale basis | `TestD2SameAuthorStaleBasisBlocksCandidate` | Safe refusal, no invented order |
| Delete/edit, repeat delete and restore | `TestReconciliationOfflineEditDelete`, `TestReconciliationRepeatedDelete`, `TestP08CLIResolutionRestoreControlReplay` | Restore creates new ancestry |
| Divergent enrollment | `TestD5DivergentEnrollmentCreatesIndependentHistories` | Bootstrap absence creates no tombstones |
| Parent/child and file/directory collision | `TestD5ParentDeleteOrFileVsChildIsStructuralConflict`, workspace structural tests | No recursive destructive replacement |
| Editor races during capture/publication | `TestD1ExchangePreservesObservedOverwriteAndSaveByRename`, `TestOpenDescriptorAfterExchangeWritesRecoveryCandidate` | Arbitrary continuing descriptor writes remain outside guarantee |
| Replaced/unreadable roots | `TestRootReplacementPausesScan`, `TestIncompleteSubtreeCannotInferDeletion`, `TestP12RootUnavailablePauseFolderNoDeletions` | Logical unavailable-root scenarios; no personal filesystem unmount |
| Durable-boundary crashes | P03/P04/P06/P16 actual helper-process kills; clean VM matrix | Selected VM boundaries under recorded kernel/ext4/virtio assumptions; no physical Pi reset |
| Mid-file/mid-chunk drop and lost receipt | `TestSyncerInterruptedResume`, `TestSyncerLostReceiptReplaySafe`, packaged native interruption | Durable objects can precede progress rows; all recorded chunks reused |
| Full storage during write/SQLite/checkpoint/stage/flush | Five clean VM disk-full cases | Four actual exhaustion cases; fsync syscall error injected by VM-child seccomp |
| Corruption and shared-chunk repair | `TestP11IntegrityScanAndQuarantineAffectedVersions`, `TestRepairSharedChunkRestoresBothVersions` | Authorized available copy required |
| GC interleavings and restart | D4 reference-set model, `TestD4ReferenceCreationInterleavingsNeverCommitMissingProtectedContent`, `TestP16GCBoundaries` | Coordinated tested interleavings |
| Expiry and long-offline peers | `TestP10LongOfflinePeerNoResurrectedDeletions` | Metadata retained; historical bytes may expire |
| Retirement / configuration / stale rejoin | `TestD3RetirementRejectsOldEpochResurrection`, `TestP09ThreePeerForwardingAndMembershipLifecycle` | No automatic retirement |
| Forwarding without author/final-device overlap | Packaged native A→VPS→B and explicit no-false-B-receipt assertion | Run-owned author listener stopped; workstation routing bridge stays available |
| Unauthorized input / path / symlink / size limits | Authentication matrix, `TestParentSymlinkSwapCannotPublishOutsideRoot`, `TestMalformedAndOverLimitInputsAreBounded` | Authenticated input still validated |
| Repeated small edits plus archive | `TestBackgroundSyncFromPersistedPeerEndpoints`: actual 16-MiB transfer while producer keeps editing; synthetic 10,000-file/1-GiB run | Local sustained producer, not a long-term native stress claim; sampled resources have a 20-ms interval |
| Migration interruption / newer schema | `TestP15InterruptedMigrationRollback`, `TestP15UpgradePreflight`, rollback counter tests | Refusal/recovery; no counter reuse |

Owner personal use and explanation remain P17 acceptance gates. They cannot
be established by this map. The prepared personal folder is explicitly
excluded from fault experiments.
