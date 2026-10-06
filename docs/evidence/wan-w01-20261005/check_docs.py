#!/usr/bin/env python3
"""Validate W01 links/contracts/discovery and preserve initial W00/other files."""
import hashlib,json,re
from pathlib import Path
from urllib.parse import unquote
ROOT=Path(__file__).resolve().parents[3]
EVIDENCE=Path(__file__).resolve().parent
OWNED={
 'docs/implementation/wan-foundations.md','docs/implementation/wan-status.md',
 'docs/orbit-wan-implementation-plan.md','docs/orbit-wan-architecture.md',
 'docs/orbit-wan-design-gates.md','docs/orbit-wan-protocol.md',
 'schemas/terminal-control-v1.md','go.mod','go.sum','NOTICE','packaging/LICENSES.md',
}
DOCS=sorted(p for p in OWNED if p.endswith('.md'))+['docs/implementation/wan-contracts.md','schemas/network-v1.md']
DOCS += [str(p.relative_to(ROOT)) for p in EVIDENCE.glob('*.md')]
def prose(path):return re.sub(r'```.*?```','',path.read_text(),flags=re.S)
def anchors(path):
 values,counts=set(),{}
 for heading in re.findall(r'^#{1,6}\s+(.+?)\s*#*$',prose(path),re.M):
  slug=re.sub(r'[^\w\- ]','',heading.lower()).replace(' ','-');n=counts.get(slug,0);counts[slug]=n+1;values.add(slug+(f'-{n}' if n else ''))
 return values
links=0
for name in DOCS:
 path=ROOT/name
 for dst in re.findall(r'\[[^\]]+\]\(([^)]+)\)',prose(path)):
  if '://' in dst or dst.startswith('mailto:'):continue
  filename,_,anchor=unquote(dst).partition('#');target=path.parent/filename if filename else path
  assert target.exists(),(name,dst,'missing target')
  if anchor:assert anchor in anchors(target),(name,dst,'missing anchor')
  links+=1
plan=(ROOT/'docs/orbit-wan-implementation-plan.md').read_text();status=(ROOT/'docs/implementation/wan-status.md').read_text()
rows=re.findall(r'^\| (W\d\d) \| [^|]+ \| ([^|]+) \|$',plan,re.M);assert len(rows)==18
for packet,deps in rows:
 assert re.search(r'^\| '+packet+r' [^|]+ \|',status,re.M),packet
 for dep in re.findall(r'W\d\d',deps):assert int(dep[1:])<int(packet[1:])
assert '| W01 Contracts/gates | complete |' in status
assert 'First eligible packet: **W02**' in status
manifest=json.loads((EVIDENCE/'manifest.json').read_text());preserved=0
for name,sha in manifest['files'].items():
 if name in OWNED:continue
 assert hashlib.sha256((ROOT/name).read_bytes()).hexdigest()==sha,('initial file changed',name)
 preserved+=1
names=set(re.findall(r'^TestWANW01\w+$',(EVIDENCE/'final-discovery.log').read_text(),re.M))
for name in ('TestWANW01PinnedTransport','TestWANW01V3EnrollmentTransportAndMembership','TestWANW01MaximumGoldenAndOneOver','TestWANW01AttachmentExpiryIsNotTunnelExpiry','TestWANW01NetworkControlContracts'):
 assert name in names,('zero discovery',name)
# The actual final runner must not omit a discovered W01 acceptance test.
executed=set(re.findall(r'^=== RUN\s+(TestWANW01\w+)$',(EVIDENCE/'final-focused-race.log').read_text(),re.M))
assert names<=executed,(names-executed)
assert 'FAIL' not in (EVIDENCE/'final-focused-race.log').read_text()
# Existing runtime capabilities stay unchanged (hash preservation includes controller).
assert 'network_control_v1' not in (ROOT/'internal/control/terminal_lifecycle.go').read_text()
print(f'PASS: {links} local links/anchors; 18 ordered packets; {len(names)} W01 tests executed; {preserved} initial files preserved')
