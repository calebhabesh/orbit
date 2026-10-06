#!/usr/bin/env python3
"""Validate W13 local links/anchors and record final changed-file hashes."""
import hashlib, json, re, subprocess
from pathlib import Path
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[3]
EVIDENCE = Path(__file__).resolve().parent
DOCS = [
    'docs/orbit-net-operator.md', 'docs/implementation/wan-status.md',
    'docs/implementation/wan-release.md', 'docs/orbit-wan-architecture.md',
    'docs/orbit-wan-protocol.md', 'docs/orbit-wan-design-gates.md',
    'docs/orbit-wan-implementation-plan.md', 'docs/operations.md',
    'docs/persistence.md', 'schemas/terminal-control-v1.md',
]
SOURCES = [
    'cmd/orbit-net/main.go', 'cmd/orbit-net/serve.go', 'cmd/orbit-net/metrics.go',
    'cmd/orbit-net/profile.go', 'cmd/orbit-net/main_test.go',
    'internal/rendezvous/service.go', 'internal/rendezvous/control.go',
    'internal/rendezvous/relay.go', 'internal/rendezvous/metrics.go',
    'internal/rendezvous/rotation_test.go', 'internal/rendezvous/relay_test.go',
    'internal/network/stun_server.go', 'internal/network/service_client.go',
    'internal/config/service_roots.go', 'internal/config/service_roots_test.go',
    'internal/config/peer_routes.go', 'internal/config/peer_routes_test.go',
    'internal/control/terminal_network.go', 'internal/control/terminalcontract/network.go',
    'internal/control/terminalcontract/validate.go', 'internal/app/app.go',
    'cmd/filesync/terminal_network.go', 'cmd/filesync/terminal_enrollment.go',
    'tests/terminal/wan_w13_test.go', 'scripts/build_orbit_net.go', 'Makefile',
    'packaging/systemd/orbit-net.service', 'packaging/sysusers/orbit-net.conf',
    'packaging/orbit-net/serve.example.json', 'packaging/orbit-net/profile-template.example.json',
]

def prose(path):
    return re.sub(r'```.*?```', '', path.read_text(), flags=re.S)

def anchors(path):
    values, counts = set(), {}
    for heading in re.findall(r'^#{1,6}\s+(.+?)\s*#*$', prose(path), re.M):
        slug = re.sub(r'[^\w\- ]', '', heading.lower()).replace(' ', '-')
        n = counts.get(slug, 0)
        counts[slug] = n + 1
        values.add(slug + (f'-{n}' if n else ''))
    return values

hashes = {name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest() for name in SOURCES}
revision = subprocess.run(['git', 'rev-parse', 'HEAD'], cwd=ROOT, capture_output=True, text=True, check=True).stdout.strip()
(EVIDENCE / 'final-source.json').write_text(json.dumps({'head': revision, 'files': hashes}, indent=2) + '\n')
links = 0
for name in DOCS + [str(p.relative_to(ROOT)) for p in EVIDENCE.glob('*.md')]:
    path = ROOT / name
    for dst in re.findall(r'\[[^\]]+\]\(([^)]+)\)', prose(path)):
        if '://' in dst or dst.startswith('mailto:'):
            continue
        filename, _, anchor = unquote(dst).partition('#')
        target = path.parent / filename if filename else path
        assert target.exists(), (name, dst, 'missing target')
        if anchor and target.suffix == '.md':
            assert anchor in anchors(target), (name, dst, 'missing anchor')
        links += 1
print(f'W13 docs: {links} local links/anchors valid; {len(hashes)} final source hashes recorded')
