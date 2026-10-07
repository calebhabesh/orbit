#!/usr/bin/env python3
"""Compare final runtime source and historical evidence with the takeover hashes."""
import hashlib
import json
from pathlib import Path

root = Path(__file__).resolve().parents[3]
evidence = Path(__file__).resolve().parent
initial = json.loads((evidence / 'initial-source.json').read_text())
final = json.loads((evidence / 'final-source.json').read_text())
assert initial['tested_source_digest'] == final['tested_source_digest'], 'runtime/test/build source changed'
old = json.loads((evidence / 'historical-evidence.json').read_text())
for name, digest in old.items():
    path = root / name
    assert path.is_file() and not path.is_symlink(), (name, 'missing or replaced')
    assert hashlib.sha256(path.read_bytes()).hexdigest() == digest, (name, 'changed historical evidence')
inherited = json.loads((evidence / 'inherited-evidence.json').read_text())
for name, digest in inherited.items():
    if name.endswith('/summary.md'):
        continue  # W14 summary is corrected to link this closeout.
    assert hashlib.sha256((root / name).read_bytes()).hexdigest() == digest, (name, 'changed inherited W14 raw evidence')
print(f"Preserved runtime/test/build source, {len(old)} historical P/O/T/W evidence files and inherited W14 raw evidence")
