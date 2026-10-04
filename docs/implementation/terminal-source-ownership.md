# Terminal source ownership manifest

T01 freezes these exact default file assignments. No parallel workers were
started. Assignment is by packet/task, independent of worker model. Existing
engine methods stay in their owning modules; listed new files are defaults,
not claims that they already exist. A follow-up changing a path updates this
manifest and its packet evidence. Shared-file work proceeds sequentially.

| Packet / task | Exact owned source paths / integration responsibility |
| --- | --- |
| T01 contract definitions | `internal/control/terminalcontract/{types,validate,invitation,contracts_test}.go`; `internal/protocol/terminal_enrollment{,_wire,_test}.go`; `schemas/terminal-control-v1.md`; `schemas/fixtures/terminal-v1/*.json`; `tests/designgates/terminal_t01_test.go`; `tests/designgates/testdata/terminal-enrollment-v2.hex`; `docs/terminal-design-gates.md`; this manifest |
| T02 live/stopped client | New `internal/controlclient/{client,live,stopped,stream,errors,client_test}.go`; extract helpers from `cmd/filesync/main.go`; integrate `internal/control/{server,control}.go` and new `terminal_http.go` |
| T02 initializer/lifecycle | `internal/{app/app,launcher/launcher,config/storage,state/state}.go`; new `internal/launcher/terminal_lifecycle{,_test}.go`; `cmd/filesync/main.go`; network settings/lifecycle integration belongs to this packet |
| T03 invitation/proof/admission | New `internal/replication/enrollment{,_test}.go`; `internal/replication/{server,client,identity}.go`; `internal/control/orbit_control.go`; new `internal/control/terminal_enrollment{,_test}.go`; new `internal/repository/terminal_enrollment{,_test}.go`; migrations in `internal/repository/repository.go` |
| T04 preview/setup/join | New `internal/workspace/root_preview{,_test}.go`; `internal/workspace/workspace.go`; new `internal/repository/terminal_operations{,_test}.go`; new `internal/control/terminal_setup{,_test}.go`; `internal/control/orbit_control.go`; `cmd/filesync/terminal_setup{,_test}.go`; integrates durable operation ledger for all mutations |
| T05 multi-folder/rollout | T03 enrollment/controller/repository files; new `internal/replication/terminal_membership.go`; `internal/replication/client.go`; `internal/scheduler/scheduler.go`; new `cmd/filesync/terminal_share{,_test}.go`; rollout integration owner T05 |
| T06 names/context/rendering | New `internal/control/terminal_context{,_test}.go`; `internal/repository/browse.go`; new `cmd/filesync/terminal_{context,output,help,completion}.go`; `cmd/filesync/main.go`; live/stopped adapter equivalence integration |
| T07 status/attention | New `internal/control/terminal_status{,_test}.go`, `internal/repository/terminal_attention{,_test}.go`, `cmd/filesync/terminal_status{,_test}.go`; `internal/repository/browse.go`; `internal/control/doctor.go` |
| T08 exact content/editor/recovery | New `internal/control/terminal_content{,_test}.go`; `internal/repository/{content_read,terminal_sessions}.go`; `internal/workspace/file_mutations.go`; new `cmd/filesync/terminal_editor{,_test}.go`; integrates history/GC/publication methods without moving ancestry into presentation |
| T09 shell/library/lifetime | New `internal/terminal/{app,keys,render,library,app_test}.go`; new `scripts/terminal_pty_test.py`; `go.mod`, `go.sum`; `cmd/filesync/main.go`; selected Charm v2 stack pins/adapters and actual client lifetime integration |
| T10 setup screens | New `internal/terminal/{setup,join,approval,share,setup_test}.go`; adapter integration in `app.go`; uses real T04/T05 controls |
| T11 everyday screens | New `internal/terminal/{overview,status,conflicts,history,restore,settings,editor,management_test}.go`; adapter integration in `app.go`; real status/content integration |
| T12 entry/packages | `scripts/build_packages.go`; `packaging/systemd/orbit.service`; `cmd/filesync/main.go`; `README.md`; new `docs/terminal-runbook.md`; package/legacy/adoption integration |
| T13 campaign | New `tests/terminal/release_test.go`; `scripts/validation/{three_host,service_lifecycle,harness}.py`; evidence/report/status reconciliation; production fixes assigned back to their owning module |

T09 actual files: `internal/terminal/{library,app,keys,render,app_test}.go`,
`cmd/filesync/terminal_tui.go`, dispatch/help/completion integration,
`internal/controlclient/tools.go`, bounded selector/cursor pages in
`internal/repository/terminal_names.go` and `internal/control/terminal_context.go`,
`tests/terminal/tui_test.go`, `scripts/terminal_{pty_test,vt}.py`, the VT oracle
unit tests, `Makefile`, dependency pins/licenses, shell runbook and owning docs.
The shell query seam and bounded direct-argv tool adapter are frozen for T10/T11.

Every packet also owns its evidence directory, its tracker entry and necessary
owning-spec clarification. Existing T00 tests remain untouched until the named
repair packet promotes them; preserve P/O evidence and unrelated relocation
files. T02/T03/T04 may introduce migrations sequentially, but T01 makes none.
The operation ledger is defined in T01; T02 must at minimum persist lifecycle/
settings mutation identities before reporting those controls complete. T04
extends the shared ledger to durable setup jobs; it does not license T02/T03
to defer their own replay requirements. CLI and TUI call the same controller
family implementations; integration completion requires their actual interfaces.

T03 actual integration paths: `internal/replication/{enrollment,enrollment_limits_test}.go`,
`internal/repository/enrollment.go` and shared membership transaction in `peers.go`;
`internal/control/{terminal_enrollment,enrollment_compatibility}.go` with
`{terminal_lifecycle,orbit_control,server,types}.go` integration; peer listener
wiring in `internal/app/app.go`; canonical decoder in
`internal/protocol/membership_decode{,_test}.go`; CLI compatibility in
`cmd/filesync/terminal_enrollment.go` and `main.go`; production tests in
`tests/terminal/enrollment_test.go` and migrated legacy pairing/enrollment fixtures.
Legacy local invitation consumption now checks recorded scope in
`internal/repository/product_records.go`. Existing schema 13 is unchanged;
private metadata prefixes provide additive records. T04 retains setup/root
jobs rather than inheriting a completed readiness claim from this compatibility
adapter. No parallel workers were started.

## T04 production ownership

T04 adds `internal/workspace/adoption.go` (descriptor-rooted measured continuation
and reviewed root creation/registration), `internal/repository/onboarding.go`
(capacity admission and atomic job/operation/review admission),
`internal/control/terminal_setup{,_compatibility}.go` (durable orchestration and
legacy adapters), `cmd/filesync/terminal_setup.go` (reviewed CLI forms/scripts),
and `tests/terminal/setup{,_cli}_test.go` (production acceptance).
Existing workspace capture, terminal lifecycle/contract, app/scheduler,
enrollment endpoint persistence, legacy setup/control tests and CLI pairing
fixtures are integrated at their owning seams. T04 evidence manifest records the
exact task file hashes. Existing relocation source and P/O evidence remain intact.

## T05 production ownership

T05 reuses `internal/control/{terminal_enrollment,terminal_lifecycle,orbit_control}.go`,
`internal/repository/enrollment.go` and `internal/replication/enrollment.go` for
restricted known-device invitations and reviewed replay. Rollout stays in
`internal/replication/{transfer,server,wire}.go`; error observation/classification
is in `internal/scheduler/retry.go`. Runtime endpoint reload already implemented
by T04 is reused, with address-only saved-anchor refresh in the owning controller.
CLI adapters use `cmd/filesync/{main,terminal_enrollment}.go`; ordinary production
checks use `tests/terminal/{sharing,sharing_process}_test.go`. No new wrapper
module, dependency, schema version, agent delegation or relocation edit was needed.

## T10 production ownership

T10 adds `internal/terminal/{setup,join,approval,share,setup_render,setup_test}.go`,
`internal/control/terminal_management.go`, `internal/controlclient/onboarding.go`,
`tests/terminal/tui_onboarding_test.go`, `scripts/terminal_onboarding_pty_test.py`
and the onboarding runbook. It integrates T09 app/keys/render, CLI setup/TUI entry,
terminal lifecycle/setup/status and typed query/result codecs, repository bounded
operation pages, VT oracle/tests and Makefile. The shared setup adapter owns exact
private retry and listener-restart records; causal/membership/root/relocation
semantics remain in their original owners. No new dependency, schema migration,
agent delegation or personal-root/service action was introduced.
