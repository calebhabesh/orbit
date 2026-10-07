# Public repository preparation — 2026-10-07

The repository was private during development and became public on 2026-10-07
as a portfolio project.

## What was checked

- **Credentials:** Gitleaks 8.30.1 (official release, checksum verified) scanned
  all history with default rules. Every finding is the `generic-api-key`
  heuristic matching public values: SHA-256 file digests in evidence manifests,
  public device key pins, conflict head digests, the public service key and
  local preview tokens of deleted test folders. The operator's authority and
  service private keys, the TLS/ACME credentials and the alert channel were
  checked directly against every commit and are absent. No private key,
  cloud token or WireGuard key exists in history.
- **Personal data:** workstation home paths, account names, machine hostnames,
  home LAN/ULA and tailnet addresses, NIC MAC addresses and SSH key
  fingerprints recorded in evidence logs were replaced throughout history with
  neutral placeholders (`/home/owner`, `owner-desktop`, `owner-pi`,
  `192.168.88.x`, `100.101.0.x`, `02:00:00:00:00:00`, `redacted`). The
  connection service's public address, operator name and contact email stay,
  because the signed release profile publishes them deliberately.

## Effect on history

All 63 commits are kept, with identical authors, dates and messages; only
their contents (and therefore their IDs) changed. Evidence written before this
date cites the original IDs, including version strings of deployed binaries.
Use this table to translate them. The original history is kept privately by
the owner.

| Original | Public | Date | Subject |
| --- | --- | --- | --- |
| `0179bb5` | `32656c1` | 2026-09-12 | Initialize file sync project scope and implementation roadmap |
| `43e89b2` | `2e4d34d` | 2026-09-12 | Record repository initialization status |
| `0b81bde` | `4569aa3` | 2026-09-21 | Implement P00 project foundation |
| `eff887b` | `279a9c7` | 2026-09-21 | Complete P00 with hosted CI evidence |
| `7f0cb81` | `4bf8e62` | 2026-09-23 | Implement Packets P01–P06: Foundations and Two-Peer Sync Slice |
| `b86b7c5` | `e71ed46` | 2026-09-23 | feat(web): add embedded web management console and assets (P14) |
| `9d559ac` | `d478b2c` | 2026-09-23 | feat(core): implement replication, operations, scheduler, and control engines (P07–P13) |
| `80bda3b` | `203a58c` | 2026-09-23 | feat(cli): wire unified CLI commands, daemon runner, and build tooling |
| `80e6c8d` | `e0b3e8c` | 2026-09-23 | feat(packaging): add Linux release packaging, systemd service, and license notices (P15) |
| `f04bd9a` | `05940d2` | 2026-09-23 | docs(runbooks): add operator runbooks for maintenance and disaster recovery |
| `b1cabe2` | `5324a5a` | 2026-09-23 | test: add integration suites (P07–P15) and P16 reproducible fault campaign |
| `51b33be` | `7496aaf` | 2026-09-23 | docs(status): record P07–P16 verification evidence and update specifications |
| `916e951` | `045ca8a` | 2026-10-01 | Fix release evidence harnesses and inventory scaling |
| `90b75ce` | `c2d9656` | 2026-10-01 | Enable bounded background peer sync and persistent storage limits |
| `e3e5c5d` | `2437b99` | 2026-10-01 | Align measured TLS baseline and proxy TCP settings |
| `5861f93` | `4e75786` | 2026-10-01 | Recover pending queue work beyond completed history |
| `9cb690e` | `feb96c2` | 2026-10-01 | Admit bounded inventory pages without per-entry storage scans |
| `aace5ec` | `d2ddba3` | 2026-10-01 | Preserve evidence directories and upgrade only prepared pilot services |
| `9d62f50` | `fc93d0a` | 2026-10-01 | Record benchmark dimensions and varied mixed object workloads |
| `884c370` | `e86c81e` | 2026-10-01 | Measure comparable TLS transfers over the actual VPS route |
| `e13e53a` | `5a87bbe` | 2026-10-01 | Share bounded chunk backoff across parallel transfer workers |
| `0a16d84` | `a3cdfb7` | 2026-10-01 | docs: record final release validation and fixture cleanup |
| `1180f89` | `60b8f48` | 2026-10-02 | feat(engine): implement storage migrations, file mutations journal, and design gates (O00–O02) |
| `5259f86` | `ff17c03` | 2026-10-02 | feat(web): build monochrome Orbit Web UI with device and file management (O04, O06, O08, O10, O11) |
| `de0d3f5` | `813eeab` | 2026-10-02 | feat(control): add Orbit control server, launcher, and enrollment workflows (O03, O05) |
| `6e94d9f` | `1683611` | 2026-10-02 | feat(cli): wire Orbit CLI commands for browsing, file operations, and service control (O07, O09) |
| `7a44ab0` | `7ac8d66` | 2026-10-02 | feat(packaging): provide Linux release packages, desktop integration, and systemd service (O12) |
| `d3cdc1d` | `e2707ba` | 2026-10-02 | docs: update specifications, architecture, and operator runbooks for Orbit revamp |
| `86ae552` | `ae7def8` | 2026-10-02 | test(validation): record O04–O13 verification evidence and release acceptance campaign |
| `16eb350` | `60f1f8c` | 2026-10-04 | feat(engine): implement folder relocation, adoption journal, and terminal enrollment protocol (O14, T01–T05) |
| `403ce99` | `a0b6818` | 2026-10-04 | feat(control): add terminal control plane, client foundations, and relocation APIs (O14, T01–T08) |
| `206aa47` | `f258e07` | 2026-10-04 | feat(cli): build Orbit terminal commands, interactive TUI shell, and relocation CLI (O14, T04, T06, T09, T10) |
| `6385412` | `a13b0d5` | 2026-10-04 | docs: specify Orbit terminal redesign architecture, UX, and operator runbooks |
| `61da64f` | `575934f` | 2026-10-04 | test(validation): record T00–T10 terminal and O14 relocation verification evidence |
| `d3951ba` | `6e2280c` | 2026-10-05 | feat(engine): harden retirement snapshot persistence, prepared history queries, and scheduler concurrency |
| `d76fd70` | `38c1eb0` | 2026-10-05 | feat(control): add everyday operations, session upload handling, and status contract extensions (T11) |
| `ef462f2` | `abe85ca` | 2026-10-05 | feat(terminal): build everyday interactive screens, file review workflows, and shell completion (T11) |
| `ca3e2cc` | `e692c7f` | 2026-10-05 | feat(packaging): enhance package build automation, desktop entry, and system installer (T12) |
| `ebcd9ca` | `cbfdfdd` | 2026-10-05 | test(terminal): add T11–T13 validation suites, PTY tests, and release verification evidence |
| `2a0a7b7` | `04bb6b8` | 2026-10-05 | docs: update terminal status, operator runbooks, and portfolio specifications (T11–T13) |
| `ee461a7` | `6462d9b` | 2026-10-05 | docs(wan): define native WAN expansion architecture, protocol, and implementation plan |
| `73b6007` | `93c28a5` | 2026-10-06 | feat(protocol): define WAN wire formats, models, and schema fixtures (W01, W05) |
| `0d63f8a` | `9533bcd` | 2026-10-06 | feat(network): implement multi-transport engine, rendezvous coordination, and encrypted relay (W02–W04, W08–W10) |
| `18542cd` | `595f815` | 2026-10-06 | feat(replication): add routed enrollment, multi-transport transfers, and scheduler fairness (W02, W05, W11) |
| `17c4dd0` | `8c003bc` | 2026-10-06 | feat(control): add network control plane APIs, diagnostics doctor, and app orchestration (W05, W06, W12) |
| `2a71b5f` | `5cc925d` | 2026-10-06 | feat(terminal): build WAN CLI commands, pairing workflows, and TUI onboarding screens (W06, W07, W12) |
| `3e9882a` | `b13e273` | 2026-10-06 | feat(orbit-net): build operated rendezvous and relay service daemon (W13) |
| `d8bdc2a` | `dfbb52f` | 2026-10-06 | feat(packaging): embed default profiles, update licenses, and package automation (W14) |
| `82474ef` | `225cb6c` | 2026-10-06 | test(wan): add end-to-end integration suites, PTY workflows, and namespace tests (W00–W14) |
| `d253825` | `8d71e4c` | 2026-10-06 | docs(wan): record W00–W14 completion status, technical evidence, and updated specifications |
| `e03bd75` | `e297fef` | 2026-10-07 | feat(orbit-net): add alert check, timer units and key verification (WG6) |
| `2bbb41d` | `d557c45` | 2026-10-07 | fix(network): pace signed service operations and recover relay after path loss (W16) |
| `43dd36b` | `3c303cf` | 2026-10-07 | test(wan): add W15/W16 suites, native runner with TUI, restart and three-host drills |
| `a8ef7e6` | `8835e6a` | 2026-10-07 | docs(wan): record W15/W16 evidence and close W16 for reachable networks |
| `4bc4ea1` | `d0e2ded` | 2026-10-07 | fix(service): accept packaged %h unit and require unit ownership for start (W17) |
| `b3f3af2` | `ef1dd74` | 2026-10-07 | test(service): add packaged-unit regressions, KVM lifecycle drill and longer terminal timeout |
| `77c099e` | `1a08af4` | 2026-10-07 | docs(wan): record W17 combined release, networking guide and T13 closeout |
| `1d1fa44` | `d737412` | 2026-10-07 | fix(ci): run packaging and package tests on arm64 hosts, bound race run |
| `5874887` | `87a50db` | 2026-10-07 | feat(network): exchange signed LAN records between approved peers |
| `5aadff5` | `f3a8ea5` | 2026-10-07 | feat(release): package profile epoch 2 and tolerate an expired overlap epoch |
| `f65b623` | `513a96d` | 2026-10-07 | docs: remove P17 requirement, record LAN exchange and profile epoch 2 |
| `a34ed2f` | `f2a4bc7` | 2026-10-07 | perf(join): poll enrollment status every 15 s after submission |
| `e7f0382` | `2bf7bf9` | 2026-10-07 | docs: prepare public repository (MIT license, security policy, README intro) |
