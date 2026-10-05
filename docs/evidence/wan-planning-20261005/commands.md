# WAN planning validation — 2026-10-05

This records documentation planning checks, not implementation or WAN runtime evidence.

Commands:

```sh
python3 /tmp/orbit-wan-plan-check.py
git diff --check
```

The initial local-link inspection found a preexisting T13 anchor referring to
`release-campaign-and-owner-use`; it was corrected to the actual
`release-campaign-and-portfolio-delivery` heading. No terminal acceptance state
was changed. Final results are recorded below after execution.

The temporary validator is reproduced here so the command is reviewable and
reproducible. Its checks cover local Markdown links/anchors/whitespace, W packet
inventory, dependency agreement/cycles, pending states, scope/invariant/gate coverage
and retention of the independent causal model heading.

```python
from pathlib import Path
import re

root = Path('<repo>')
new = sorted(root.glob('docs/orbit-wan-*.md')) + sorted(root.glob('docs/implementation/wan-*.md')) + [root / 'docs/research/orbit-wan-transport-options-2026-10-05.md']
changed = [root / p for p in ['AGENTS.md', 'CONTEXT.md', 'README.md', 'docs/implementation-plan.md', 'docs/implementation/status.md', 'docs/implementation/terminal-status.md', 'docs/orbit-terminal-implementation-plan.md', 'docs/orbit-terminal-architecture.md', 'docs/orbit-terminal-ux.md', 'docs/portfolio-scope.md', 'docs/protocol.md', 'docs/persistence.md', 'docs/operations.md', 'docs/verification.md', 'docs/runbooks/private-network.md']]

def outside_code(text):
    return re.sub(r'```.*?```', '', text, flags=re.S)

def anchors(path):
    values, counts = [], {}
    for heading in re.findall(r'^#{1,6}\s+(.+?)\s*#*$', outside_code(path.read_text()), re.M):
        slug = re.sub(r'[^\w\- ]', '', heading.lower()).replace(' ', '-')
        count = counts.get(slug, 0)
        counts[slug] = count + 1
        values.append(slug + (f'-{count}' if count else ''))
    return values

errors, links = [], 0
for path in new + changed:
    text = path.read_text()
    if not text.endswith('\n'):
        errors.append(f'{path.relative_to(root)}: missing final newline')
    for number, line in enumerate(text.splitlines(), 1):
        if line.rstrip() != line:
            errors.append(f'{path.relative_to(root)}:{number}: trailing whitespace')
    for target in re.findall(r'\[[^\]\n]*\]\(([^)]+)\)', outside_code(text)):
        if re.match(r'^[a-zA-Z][\w+.-]*:', target):
            continue
        target = target.strip('<>')
        file_part, _, anchor = target.partition('#')
        destination = (path.parent / file_part).resolve() if file_part else path
        links += 1
        if not destination.exists():
            errors.append(f'{path.relative_to(root)}: missing link {target}')
        elif anchor and destination.suffix == '.md' and anchor not in anchors(destination):
            errors.append(f'{path.relative_to(root)}: missing anchor {target}')
print(f'Checked {len(new) + len(changed)} Markdown files, {links} local links; {len(errors)} errors')
for error in errors:
    print(error)
if errors:
    raise SystemExit(1)

def packet_rows(text):
    result = {}
    for line in text.splitlines():
        cells = [cell.strip() for cell in line.strip('|').split('|')]
        if len(cells) >= 3 and re.match(r'^W\d\d(?:\s|$)', cells[0]):
            result[cells[0][:3]] = cells
    return result

def dependencies(cell):
    for start, end in re.findall(r'W(\d\d)[–-]W(\d\d)', cell):
        replacement = ' '.join(f'W{i:02d}' for i in range(int(start), int(end) + 1))
        cell = re.sub(f'W{start}[–-]W{end}', replacement, cell)
    return set(re.findall(r'W\d\d', cell))

plan = packet_rows((root / 'docs/orbit-wan-implementation-plan.md').read_text())
status = packet_rows((root / 'docs/implementation/wan-status.md').read_text())
expected = {f'W{i:02d}' for i in range(18)}
assert set(plan) == set(status) == expected, 'Packet inventory mismatch'
graph = {packet: dependencies(cells[2]) for packet, cells in plan.items()}
assert all(graph[p] == dependencies(status[p][2]) for p in expected), 'Dependency mismatch'
assert all(status[p][1] == 'pending' for p in expected), 'Planning completed an implementation packet'
complete = set()
while len(complete) < len(expected):
    ready = {p for p, deps in graph.items() if p not in complete and deps <= complete}
    assert ready, 'Dependency cycle'
    complete.update(ready)
first = sorted(p for p, deps in graph.items() if not deps)
assert first == ['W00'], first
print('Plan/status agree on 18 pending packets; dependency graph is acyclic; first eligible W00.')

scope = (root / 'docs/portfolio-scope.md').read_text()
coverage = (root / 'docs/implementation/wan-status.md').read_text()
verification = (root / 'docs/verification.md').read_text()
gates = (root / 'docs/orbit-wan-design-gates.md').read_text()
assert all(f'| S{i:02d} |' in scope and f'S{i:02d}' in coverage for i in range(23, 29))
assert all(f'| N{i:02d} |' in verification for i in range(1, 11))
assert all(f'## WG{i} ' in gates for i in range(1, 7))
assert verification.count('## Independent causal model\n') == 1
print('Scope S23–S28, invariants N01–N10 and six design gates are linked; inherited causal model retained.')
```

## Results

```text
Checked 26 Markdown files, 431 local links; 0 errors
Plan/status agree on 18 pending packets; dependency graph is acyclic; first eligible W00.
Scope S23–S28, invariants N01–N10 and six design gates are linked; inherited causal model retained.
```

`git diff --check`: exit 0, no output.

No production code, dependencies, running services or external infrastructure were
changed. Runtime, NAT traversal, service deployment and native WAN checks are
unexecuted; all W packets and design gates remain pending.

