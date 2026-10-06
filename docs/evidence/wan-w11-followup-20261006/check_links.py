#!/usr/bin/env python3
"""Validate current W11 owning-document and follow-up evidence links."""
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import unquote

ROOT = Path(__file__).resolve().parents[3]
EVIDENCE = Path(__file__).resolve().parent
EDITED = {
    'docs/orbit-wan-implementation-plan.md',
    'docs/implementation/wan-foundations.md',
    'docs/implementation/wan-status.md',
}
DOCS = sorted(EDITED | {'docs/implementation/wan-baseline.md'})
DOCS = ['docs/operations.md','docs/orbit-wan-architecture.md','docs/orbit-wan-design-gates.md','docs/orbit-wan-implementation-plan.md','docs/implementation/wan-status.md','docs/implementation/wan-direct.md','schemas/terminal-control-v1.md','docs/verification.md','tests/terminal/README.md'] + [str(p.relative_to(ROOT)) for p in EVIDENCE.glob('*.md')]


def prose(path):
    return re.sub(r'```.*?```', '', path.read_text(), flags=re.S)


def anchors(path):
    values, counts = set(), {}
    for heading in re.findall(r'^#{1,6}\s+(.+?)\s*#*$', prose(path), re.M):
        slug = re.sub(r'[^\w\- ]', '', heading.lower()).replace(' ', '-')
        count = counts.get(slug, 0)
        counts[slug] = count + 1
        values.add(slug + (f'-{count}' if count else ''))
    return values


links = 0
for name in DOCS:
    path = ROOT / name
    for destination in re.findall(r'\[[^\]]+\]\(([^)]+)\)', prose(path)):
        if '://' in destination or destination.startswith('mailto:'):
            continue
        filename, _, anchor = unquote(destination).partition('#')
        target = path.parent / filename if filename else path
        assert target.exists(), (name, destination, 'missing target')
        if anchor:
            assert anchor in anchors(target), (name, destination, 'missing anchor')
        links += 1

print(f"validated {links} relative documentation links and anchors")
