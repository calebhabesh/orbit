from pathlib import Path
import re
import urllib.parse

files = [Path(p) for p in (
    'docs/protocol.md', 'docs/persistence.md', 'docs/operations.md',
    'docs/verification.md', 'docs/orbit-terminal-architecture.md',
    'docs/terminal-design-gates.md',
    'docs/implementation/terminal-source-ownership.md',
    'docs/implementation/terminal-status.md',
    'docs/runbooks/membership-fork.md', 'schemas/peer-v1.md',
    'schemas/terminal-control-v1.md',
    'docs/evidence/terminal-t05-20261004/summary.md',
    'docs/evidence/terminal-t05-20261004/commands.md',
)]
links = 0
for document in files:
    contents = document.read_text()
    assert all(line == line.rstrip() for line in contents.splitlines()), document
    assert sum(line.startswith('```') for line in contents.splitlines()) % 2 == 0, document
    for target in re.findall(r'\]\(([^)]+)\)', contents):
        if ':' in target or target.startswith('#'):
            continue
        raw, _, anchor = target.partition('#')
        path = (document.parent / urllib.parse.unquote(raw)).resolve()
        assert path.exists(), (document, target)
        if anchor:
            headings = re.findall(r'^#{1,6}\s+(.+)$', path.read_text(), re.M)
            slugs = {re.sub(r'[^\w\- ]', '', h.replace('`', '').lower()).replace(' ', '-')
                     for h in headings}
            assert anchor in slugs, (document, target)
        links += 1
print(f'{len(files)} documents, {links} local links/anchors passed; balanced fences and no trailing whitespace')
