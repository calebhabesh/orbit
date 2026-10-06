#!/usr/bin/env python3
"""Validate W03 local links, discovered execution, frozen source and preservation."""
import hashlib,json,re
from pathlib import Path
from urllib.parse import unquote
ROOT=Path(__file__).resolve().parents[3]
EVIDENCE=Path(__file__).resolve().parent
OWNED={
 'docs/implementation/wan-relay.md','docs/implementation/wan-status.md',
 'docs/orbit-wan-implementation-plan.md','docs/orbit-wan-architecture.md',
 'docs/orbit-wan-protocol.md','docs/operations.md','docs/persistence.md',
 'docs/verification.md','schemas/network-v1.md','docs/orbit-wan-design-gates.md',
}
def prose(path):return re.sub(r'```.*?```','',path.read_text(),flags=re.S)
def anchors(path):
 values,counts=set(),{}
 for heading in re.findall(r'^#{1,6}\s+(.+?)\s*#*$',prose(path),re.M):
  slug=re.sub(r'[^\w\- ]','',heading.lower()).replace(' ','-');n=counts.get(slug,0);counts[slug]=n+1;values.add(slug+(f'-{n}' if n else ''))
 return values
links=0
for name in sorted(OWNED)+[str(p.relative_to(ROOT)) for p in EVIDENCE.glob('*.md')]:
 path=ROOT/name
 for dst in re.findall(r'\[[^\]]+\]\(([^)]+)\)',prose(path)):
  if '://' in dst or dst.startswith('mailto:'):continue
  filename,_,anchor=unquote(dst).partition('#');target=path.parent/filename if filename else path
  assert target.exists(),(name,dst,'missing target')
  if anchor:assert anchor in anchors(target),(name,dst,'missing anchor')
  links+=1
manifest=json.loads((EVIDENCE/'manifest.json').read_text());preserved=0
for name,sha in manifest['initial_files'].items():
 if name in OWNED:continue
 assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==sha,('initial file changed',name)
 preserved+=1
for name,sha in manifest['final_source_files'].items():
 assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==sha,('frozen source changed',name)
names=set(re.findall(r'^TestWANW03\w+$',(EVIDENCE/'logs/final-discovery-complete.log').read_text(),re.M))
executed=set(re.findall(r'^=== RUN\s+(TestWANW03\w+)$',(EVIDENCE/'logs/header-final-race.log').read_text(),re.M))
assert len(names)==24,(len(names),names)
assert names<=executed,(names-executed)
assert 'FAIL' not in (EVIDENCE/'logs/header-final-race.log').read_text()
assert '--- PASS: TestWANW03TLSAndHeaderOrigins' in (EVIDENCE/'logs/header-specific-final-race.log').read_text()
assert '| W03 Rendezvous/profile | complete |' in (ROOT/'docs/implementation/wan-status.md').read_text()
assert 'First eligible packet: **W04**' in (ROOT/'docs/implementation/wan-status.md').read_text()
assert 'network_control_v1' not in (ROOT/'internal/control/terminal_lifecycle.go').read_text()
print(f'PASS: {links} local links/anchors; all {len(names)} discovered W03 tests executed with race; {preserved} initial files preserved; frozen Go/go.mod/go.sum/Makefile unchanged')
