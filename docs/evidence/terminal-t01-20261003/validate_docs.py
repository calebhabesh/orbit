#!/usr/bin/env python3
"""Reproducible local-link/fence validation of T01 owning documents."""
import pathlib
import re
import urllib.parse
root = pathlib.Path(__file__).resolve().parents[3]
documents = [root / p for p in (
    'schemas/terminal-control-v1.md', 'docs/terminal-design-gates.md',
    'docs/orbit-terminal-architecture.md',
    'docs/implementation/terminal-source-ownership.md',
    'docs/implementation/terminal-status.md',
    'docs/protocol.md', 'docs/persistence.md', 'docs/operations.md',
    'docs/verification.md',
    'docs/evidence/terminal-t01-20261003/summary.md',
    'docs/evidence/terminal-t01-20261003/commands.md',
)]
def anchors(path):
    found = set()
    counts = {}
    for line in path.read_text().splitlines():
        if not re.match(r'^#{1,6} ', line):
            continue
        title = re.sub(r'^#{1,6} ', '', line).strip().lower()
        slug = re.sub(r'[^\w\- ]', '', title).replace(' ', '-')
        n = counts.get(slug, 0)
        counts[slug] = n + 1
        found.add(slug if n == 0 else f'{slug}-{n}')
    return found
links = 0
for path in documents:
    text = path.read_text()
    assert text.count('```') % 2 == 0, f'unbalanced fences: {path}'
    assert not any(line.rstrip() != line for line in text.splitlines()), path
    for target in re.findall(r'\[[^\]]*\]\(([^)\s]+)\)', text):
        parsed = urllib.parse.urlsplit(target)
        if parsed.scheme:
            continue
        dest = (path.parent / urllib.parse.unquote(parsed.path)).resolve() if parsed.path else path
        assert dest.exists(), f'{path}: missing {target}'
        if parsed.fragment:
            assert parsed.fragment in anchors(dest), f'{path}: missing anchor {target}'
        links += 1
print(f'PASS: {len(documents)} documents, {links} local links/anchors; fences and whitespace')
