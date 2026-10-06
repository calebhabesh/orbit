#!/usr/bin/env python3
"""Check W02 links, test coverage and preservation against initial dirty tree."""
import hashlib,json,re
from pathlib import Path
from urllib.parse import unquote
ROOT=Path(__file__).resolve().parents[3]
EVIDENCE=Path(__file__).resolve().parent
OWNED={
 'internal/app/app.go','internal/control/control.go',
 'internal/control/enrollment_compatibility.go','internal/control/terminal_setup.go',
 'internal/replication/client.go','internal/replication/enrollment.go',
 'internal/replication/transfer_test.go','internal/scheduler/retry.go',
 'internal/network/transport.go','internal/network/limits.go',
 'docs/implementation/wan-foundations.md','docs/implementation/wan-status.md',
 'docs/orbit-wan-implementation-plan.md','docs/orbit-wan-architecture.md',
 'docs/orbit-wan-design-gates.md','docs/operations.md','docs/persistence.md',
 'docs/verification.md',
}
def prose(path):return re.sub(r'```.*?```','',path.read_text(),flags=re.S)
def anchors(path):
 values,counts=set(),{}
 for heading in re.findall(r'^#{1,6}\s+(.+?)\s*#*$',prose(path),re.M):
  slug=re.sub(r'[^\w\- ]','',heading.lower()).replace(' ','-');n=counts.get(slug,0);counts[slug]=n+1;values.add(slug+(f'-{n}' if n else ''))
 return values
links=0
for name in sorted(n for n in OWNED if n.endswith('.md'))+[str(p.relative_to(ROOT)) for p in EVIDENCE.glob('*.md')]:
 path=ROOT/name
 for dst in re.findall(r'\[[^\]]+\]\(([^)]+)\)',prose(path)):
  if '://' in dst or dst.startswith('mailto:'):continue
  filename,_,anchor=unquote(dst).partition('#');target=path.parent/filename if filename else path
  assert target.exists(),(name,dst,'missing target')
  if anchor:assert anchor in anchors(target),(name,dst,'missing anchor')
  links+=1
manifest=json.loads((EVIDENCE/'manifest.json').read_text());preserved=0
for name,sha in manifest['initial_files'].items():
 if name in OWNED or name.startswith('docs/evidence/wan-w02-20261005/'):continue
 assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==sha,('initial file changed',name)
 preserved+=1
for name,sha in manifest['final_source_files'].items():
 assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==sha,('frozen source changed',name)
status=(ROOT/'docs/implementation/wan-status.md').read_text()
assert '| W02 Manager/HTTPS | complete |' in status
assert 'First eligible packet: **W03**' in status
names=set(re.findall(r'^TestWANW02\w+$',(EVIDENCE/'logs/final-discovery.log').read_text(),re.M))
executed=set(re.findall(r'^=== RUN\s+(TestWANW02\w+)$',(EVIDENCE/'logs/final-focused-race.log').read_text(),re.M))
assert len(names)==12,(len(names),names)
assert names<=executed,(names-executed)
assert 'FAIL' not in (EVIDENCE/'logs/final-focused-race.log').read_text()
assert 'network_control_v1' not in (ROOT/'internal/control/terminal_lifecycle.go').read_text()
print(f'PASS: {links} local links/anchors; all {len(names)} W02 tests executed with race; {preserved} initial files preserved including P/O/T and W00/W01 evidence, dependencies and fixtures')
