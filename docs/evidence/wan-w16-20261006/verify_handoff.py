"""Check preserved inputs, local evidence oracles and new documentation links."""
import hashlib
import json
from pathlib import Path
import re
from urllib.parse import unquote

OUT = Path(__file__).resolve().parent
ROOT = OUT.parents[2]

def require(condition, message):
    if not condition:
        raise RuntimeError(message)

def read(name):
    return json.loads((OUT / name).read_text())

original = read('preservation-before.json')
for name, digest in original.items():
    path = ROOT / name
    require(path.is_file() and hashlib.sha256(path.read_bytes()).hexdigest() == digest, 'historical evidence changed: ' + name)
source = json.loads((ROOT / 'docs/evidence/wan-w15-20261006/final-source.json').read_text())['files']
existing_source = {n: h for n, h in source.items() if not n.startswith('docs/')}
for name, digest in existing_source.items():
    path = ROOT / name
    require(path.is_file() and hashlib.sha256(path.read_bytes()).hexdigest() == digest, 'inherited non-document source changed: ' + name)

record = read('runner-process-race.json')
require(record['exit_code'] == 0 and record['source_before'] == record['source_after'], 'final source/race execution invalid')
report = read('runner-process-race-report.json')
require(report['success'] and report['rehearsal'] and not report['cleanup_errors'] and not report['retained_roots'], 'rehearsal/cleanup invalid')
require(report['physical_wan_acceptance'] == report['hosted_default_acceptance'] == 'unexecuted', 'local fixture gained native credit')
a, b = [h['device'] for h in report['hosts']]
scenarios = report['scenarios']
for transfer, author, receiver in [(scenarios['two_way'][0], a, b), (scenarios['two_way'][1], b, a),
                                   (scenarios['large'], a, b), (scenarios['reconnect'], a, b),
                                   (scenarios['second_folder']['transfer'], b, a)]:
    require(transfer['head']['author'] == author, 'wrong author')
    require(transfer['receipt']['device'] == receiver and transfer['receipt']['stored'], 'receipt not from endpoint')
    require(transfer['receipt']['version'] == transfer['head'], 'receipt for wrong version')
    require(transfer['receiver_working_hash_verified'] and re.fullmatch('[a-f0-9]{64}', transfer['sha256']), 'working hash missing')
    require(int(transfer['receiver_readiness']['conflicts']) == 0 and int(transfer['receiver_readiness']['pending_publication']) == 0, 'unqualified readiness')
for host in report['hosts']:
    require(not Path(host['root']).exists(), 'successful private root still exists')
raw = (OUT / 'runner-process-race-report.json').read_text()
require('orbit-invitation:v3:' not in raw and 'PRIVATE KEY' not in raw, 'private transfer material in report')
gate = read('hosted-gate-refusal/wan-native.json')
require(not gate['success'] and not gate['hosts'] and not gate['retained_roots'], 'gate refusal performed host actions')

names = ['docs/orbit-wan-implementation-plan.md', 'docs/implementation/wan-status.md',
         'docs/implementation/wan-release.md', 'scripts/validation/WAN_NATIVE.md']
names += [str(p.relative_to(ROOT)) for p in OUT.glob('*.md')]
def prose(p):
    return re.sub(r'```.*?```', '', p.read_text(), flags=re.S)
def anchors(p):
    values, counts = set(), {}
    for heading in re.findall(r'^#{1,6}\s+(.+?)\s*#*$', prose(p), re.M):
        slug = re.sub(r'[^\w\- ]', '', heading.lower()).replace(' ', '-')
        count = counts.get(slug, 0)
        counts[slug] = count + 1
        values.add(slug + (f'-{count}' if count else ''))
    return values
links = 0
for name in names:
    path = ROOT / name
    for destination in re.findall(r'\[[^\]]+\]\(([^)]+)\)', prose(path)):
        if '://' in destination or destination.startswith('mailto:'):
            continue
        filename, _, anchor = unquote(destination).partition('#')
        target = path.parent / filename if filename else path
        require(target.exists(), 'missing documentation target: ' + name + ' ' + destination)
        if anchor:
            require(anchor in anchors(target), 'missing anchor: ' + name + ' ' + destination)
        links += 1
result = {'success': True, 'historical_evidence_files_preserved': len(original),
          'inherited_non_document_source_files_preserved': len(existing_source), 'relative_links_checked': links,
          'local_endpoint_oracles': 'pass', 'successful_roots_removed': 'pass', 'WG6_refusal_before_host_actions': 'pass'}
(OUT / 'handoff-verification.json').write_text(json.dumps(result, indent=2) + '\n')
print(json.dumps(result))
