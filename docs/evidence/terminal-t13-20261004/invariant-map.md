# T13 invariant evidence map

Each row names executable assertions. Final clean-checkout results are recorded
in [the passing reproduction](reproduction-final-candidate/clean-release/reproduction.json);
no pending command is treated as passed. The scenario catalog is not execution
evidence. Historical [engine scenario mapping](../release-20261001/invariant-map.md)
remains dated. Physical-host and service limitations are explicit below.

| Invariant | Named current check | Additional evidence / limit |
| --- | --- | --- |
| I01 | `TestP16InvariantI01_ImmutableVersionIDOneEnvelope` | Scoped clean Go/model/process assertions |
| I02 | `TestP16InvariantI02_SameValidHistoryEquivalentHeads` | Scoped clean Go/model/process assertions |
| I03 | `TestP16InvariantI03_ConcurrentHeadsSurviveUntilResolution` | Scoped clean Go/model/process assertions |
| I04 | `TestP16InvariantI04_OrdinaryCaptureDoesNotResolveReceivedHeads` | Scoped clean Go/model/process assertions |
| I05 | `TestP16InvariantI05_NoStoredReceiptWithoutDurableMetadataAndContent` | VM transfer/publication boundaries; stored receipts are observations |
| I06 | `TestP16InvariantI06_PartialOrCorruptContentNeverPublished` | Native interrupted 12-MiB transfer and whole-file hashes |
| I07 | `TestP16InvariantI07_RecoveryPreservesProtectedVersionsAndReportsAmbiguity` | 16 reset boundaries and five storage cases; virtual ext4 device, no physical power-loss claim |
| I08 | `TestP16InvariantI08_AtomicCounterAndNoIdentityRollbackReuse` | Scoped clean Go/model/process assertions |
| I09 | `TestP16InvariantI09_PeerInputCannotEscapeAuthorizedFolder` | Scoped clean Go/model/process assertions |
| I10 | `TestP16InvariantI10_GCNeverRemovesProtectedContent` | Scoped clean Go/model/process assertions |
| I11 | `TestP16InvariantI11_UnavailableRootsIncompleteScansNeverDelete` | Scoped clean Go/model/process assertions |
| I12 | `TestP16InvariantI12_StructuralOperationsPreserveChildBytes` | Scoped clean Go/model/process assertions |
| I13 | `TestP16InvariantI13_BoundedWorkAndResourceLimits` | Also TestBackgroundSyncFromPersistedPeerEndpoints (actual 16-MiB transfer under continuing small edits); sampled 1,024-file CLI and 8/32-MiB merge resources |
| I14 | `TestP16InvariantI14_ThirdPartyForwardingPreservesAuthorAncestry` | Native laptop/Pi/VPS forwarding retains original author; engine fixtures use SSH relays/manual membership |
| I15 | `TestP16InvariantI15_MembershipRetirementRejectionAndStaleRejoin` | Scoped clean Go/model/process assertions |
| I16 | `TestP16InvariantI16_RestoreResolutionIdempotentAndStaleRejected` | Scoped clean Go/model/process assertions |
| I17 | `TestP16InvariantI17_ScanAfterApplyDoesNotFabricateEdits` | Scoped clean Go/model/process assertions |
| I18 | `TestP16InvariantI18_CorruptionYieldsUnavailableOrVerifiedRepair` | Scoped clean Go/model/process assertions |
| I19 | `TestP16InvariantI19_UICLIParityAndQualifiedProgress` | Shared CLI/TUI controls; native PTY and JSON checks |
| I20 | `TestP16InvariantI20_IncompatibleSchemaLimitsPreserveRecoverableState` | Exclusive WAL reopen and busy readiness; stopped migration/identity fencing |
| I21 | `TestTerminalT02ConcurrentProcessLaunchAndLifetime`; `TestTerminalT12BareRealPTY` | Packaged real PTY quit leaves daemon running; unique native user units |
| I22 | `TestTerminalT04ResumeAtDurablePhases`; `TestTerminalT04DelayedJoinRestartAndTransfer`; `TestTerminalT10RealPTYOnboarding` | Direct LAN ordinary joining preserves files/IDs across delayed approval and daemon restart; boot/logout unexecuted |
| I23 | `TestTerminalT03InviterPinBeforeDisclosure`; `TestTerminalT03ScopeAndProofRejections`; `TestTerminalT03IsolationAndDataAuthorization` | Native PTY expired/wrong-pin refusals; local real-network scope/replay/authority tests |
| I24 | `TestTerminalT05MembershipPinAndSequentialGate`; `TestTerminalT05ForkRecoveryPreservesOriginalGroup`; `TestTerminalT05ThreeProcessSharingOfflineRolloutAndEndpointRefresh` | Local three-process ordinary rollout/fork proof; native ordinary three-host private route unexecuted |
| I25 | `TestTerminalT08ExactReadsLiveStoppedAndGC`; `TestTerminalT07_Status_BoundedPaginationAndCancellation`; `TestTerminalT13LargeDirectoryAndStreamedMergeResources` | Live SQLite readers use authenticated owner control; direct reads require stop/consistent backup |
| I26 | `TestTerminalT08SessionStreamedMergeAndStaleArrival`; `TestTerminalT08AtomicReplayAfterCommitAndCopyPartial`; `TestTerminalT11RealPTYEveryday`; `TestTerminalT13OperationObservesPublicationWithoutReauthoring` | Pending observation completes without reauthoring; explicit retry retains exact operation |
| I27 | `TestTerminalT10RealPTYOnboarding`; `TestTerminalT11RealPTYEveryday`; `TestTerminalT12BareRealPTY` | Real keyboard PTYs, termios restoration, narrow/colorless terminals and editor return; owner walkthrough deferred |
| I28 | `TestOrbitPruning_BoundedLifecycleRecords`; `TestOrbitPruning_IdempotencyExpiryAndReplaySafety`; `TestTerminalT04ExpiredPreparedRequestRecovery` | Pruning preserves pending work, journals, pins and causal knowledge |

Current native outputs: [LAN ordinary journey](lan-final-candidate/terminal-native.json),
[packaged PTY/service campaign](native-hosts-final-candidate-03/terminal-hosts.json),
[three-host engine](native-engine-final-candidate/three-host.json). Until each result
file reports success it is pending. An existing Tailscale path and native
login/logout/boot persistence remain unexecuted; private SSH relays and service
restarts cannot substitute for those network/host observations.
