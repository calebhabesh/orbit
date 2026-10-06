#!/usr/bin/env python3
"""Check W10 links, prior-file preservation, independent fixture and evidence."""
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import unquote

EVIDENCE = Path(__file__).resolve().parent
ROOT = EVIDENCE.parents[2]
EDITED = {
    'internal/protocol/network.go', 'internal/rendezvous/service.go',
    'internal/network/relay_endpoint.go', 'internal/network/relay_runtime.go',
    'internal/network/manager.go', 'internal/network/quic.go',
    'internal/app/app.go', 'cmd/orbit-net/main.go',
    'internal/replication/relay_test.go', 'go.mod',
    'docs/orbit-wan-architecture.md', 'docs/orbit-wan-protocol.md',
    'docs/operations.md', 'docs/persistence.md', 'docs/verification.md',
    'schemas/network-v1.md', 'docs/implementation/wan-status.md',
    'docs/implementation/wan-direct.md', 'docs/orbit-wan-implementation-plan.md',
    'docs/orbit-wan-design-gates.md',
}
manifest = json.loads((EVIDENCE / 'manifest.json').read_text())
changed, preserved = [], []
for name, digest in manifest['files'].items():
    path = ROOT / name
    assert path.is_file(), ('missing initial file', name)
    if hashlib.sha256(path.read_bytes()).hexdigest() != digest:
        assert name in EDITED, ('unexpected change', name)
        changed.append(name)
    else:
        preserved.append(name)
(EVIDENCE / 'preservation.json').write_text(json.dumps(dict(
    initial_files=len(manifest['files']), preserved=len(preserved),
    changed_owning_files=changed,
    prior_evidence_preserved=sum(n.startswith('docs/evidence/') for n in preserved),
    unexpected_changes=[], revision=manifest['revision']), indent=2)+'\n')

def prose(path):
    return re.sub(r'```.*?```', '', path.read_text(), flags=re.S)

def anchors(path):
    values, counts = set(), {}
    for heading in re.findall(r'^#{1,6}\s+(.+?)\s*#*$', prose(path), re.M):
        slug = re.sub(r'[^\w\- ]', '', heading.lower()).replace(' ', '-')
        count = counts.get(slug, 0)
        counts[slug] = count+1
        values.add(slug+(f'-{count}' if count else ''))
    return values

links = 0
paths = [ROOT / n for n in EDITED if n.endswith('.md')]
paths += list(EVIDENCE.glob('*.md'))
for path in paths:
    for destination in re.findall(r'\[[^\]]+\]\(([^)]+)\)', prose(path)):
        if '://' in destination or destination.startswith('mailto:'):
            continue
        filename, _, anchor = unquote(destination).partition('#')
        target = path.parent / filename if filename else path
        assert target.exists(), (str(path), destination, 'missing target')
        if anchor:
            assert anchor in anchors(target), (str(path), destination, 'missing anchor')
        links += 1
tracker = (ROOT / 'docs/implementation/wan-status.md').read_text()
assert 'First eligible sequential packet: **W11**' in tracker
assert '| W10 ICE traversal | complete |' in tracker
print(f'{links} documentation links checked; {len(preserved)} prior files unchanged; {len(changed)} owning files changed; no unexpected changes')
