#!/usr/bin/env python3
"""Validate W00 documentation, test discovery and original-file preservation."""
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
DOCS += [str(p.relative_to(ROOT)) for p in EVIDENCE.glob('*.md')]


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

plan = (ROOT / 'docs/orbit-wan-implementation-plan.md').read_text()
tracker = (ROOT / 'docs/implementation/wan-status.md').read_text()
rows = re.findall(r'^\| (W\d\d) \| [^|]+ \| ([^|]+) \|$', plan, re.M)
assert len(rows) == 18, 'missing authoritative dependency rows'
for packet, dependencies in rows:
    assert re.search(r'^\| ' + packet + r' [^|]+ \|', tracker, re.M), packet
    for dependency in re.findall(r'W\d\d', dependencies):
        assert int(dependency[1:]) < int(packet[1:]), (packet, dependency)
assert '| W00 Baseline | complete |' in tracker
assert 'First eligible packet: **W01**' in tracker

manifest = json.loads((EVIDENCE / 'manifest.json').read_text())
assert manifest['initial_status_porcelain'] == ''
checked = 0
for entry in manifest['initial_tracked_files']:
    if entry['path'] in EDITED:
        continue
    path = ROOT / entry['path']
    assert hashlib.sha256(path.read_bytes()).hexdigest() == entry['sha256'], entry['path']
    checked += 1

names = set(re.findall(r'^Test\w+$', (EVIDENCE / 'logs/discovery.log').read_text(), re.M))
for selected in (
    'TestWANW00ManualInvitationRequiresReachableAddress',
    'TestTerminalT04TwoDeviceCLIInterruptedJoinAndEdits',
    'TestTerminalT05ThreeProcessSharingOfflineRolloutAndEndpointRefresh',
):
    assert selected in names, ('zero-match discovery', selected)
print(f'PASS: {links} local links/anchors; 18 ordered packets; {len(names)} discovered tests; {checked} original files preserved')
