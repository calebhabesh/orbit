import hashlib,json,pathlib
root=pathlib.Path(__file__).resolve().parents[3]
evidence=pathlib.Path(__file__).resolve().parent
old=json.loads((evidence/'initial-evidence.json').read_text())
for name,digest in old.items():
 p=root/name
 assert p.is_file() and not p.is_symlink(),(name,'missing or replaced')
 assert hashlib.sha256(p.read_bytes()).hexdigest()==digest,(name,'historical evidence changed')
initial=json.loads((evidence/'initial-source.json').read_text())['files']
final=json.loads((evidence/'final-source.json').read_text())['files']
allowed={'internal/network/relay_runtime.go','tests/terminal/wan_direct_test.go','docs/orbit-wan-protocol.md','internal/testkit/disposable.go','internal/testkit/disposable_test.go','internal/replication/ice_test.go','tests/terminal/wan_roaming_test.go','tests/terminal/wan_enrollment_process_test.go','docs/verification.md','docs/implementation/wan-status.md','docs/implementation/wan-release.md','docs/orbit-wan-implementation-plan.md'}
changed=[name for name,digest in initial.items() if final.get(name)!=digest]
assert set(changed)<=allowed,('unrelated changed files',set(changed)-allowed)
print(json.dumps({'historical_evidence_files':len(old),'preserved':True,'changed_inherited_files':changed,'added_files':[n for n in final if n not in initial]},indent=2))
