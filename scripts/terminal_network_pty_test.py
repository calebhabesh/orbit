#!/usr/bin/env python3
"""Explicit W12 doctor over an already reviewed, marked disposable daemon."""
import argparse
import json
import os
import shutil
from pathlib import Path
from terminal_onboarding_pty_test import Peer, UI

# Hermetic: never select the packaged hosted profile, so no daemon started here
# contacts the operated service (W14). Child processes inherit this.
os.environ.setdefault('ORBIT_DISABLE_PACKAGED_PROFILE', '1')

p = argparse.ArgumentParser()
p.add_argument('--binary', required=True)
p.add_argument('--root', required=True)
p.add_argument('--state', required=True)
a = p.parse_args()
root, state = Path(a.root).resolve(strict=True), Path(a.state).resolve(strict=True)
assert (root / '.orbit-disposable').is_file() and not (root / '.orbit-disposable').is_symlink()
assert state.is_relative_to(root) and state != root
peer = Peer.__new__(Peer)
peer.root, peer.state = root, state
peer.binary = root / "orbit"
if Path(a.binary).resolve(strict=True) != peer.binary.resolve():
    shutil.copyfile(Path(a.binary).resolve(strict=True), peer.binary)
peer.binary.chmod(0o700)
peer.token = (root / ".orbit-disposable").read_text()
peer.children, peer.frames, peer.results, peer.raw = {}, [], [], {}
peer.output = None
peer.data = root
ui = UI(peer, 'W12-connection-doctor', size=(100, 45))
try:
    ui.overview()
    ui.send(b'N')
    ui.wait('Connection details')
    ui.wait('Connected via relay')
    ui.wait('Freshness: recent')
    ui.send(b'd')
    ui.wait('Probe service_tls: VERIFIED', timeout=25)
    ui.wait('Probe directory: VERIFIED', timeout=25)
    # Cached refresh must discard explicit probe output without rerunning probes.
    ui.send(b'r')
    ui.wait('refresh cached observations')
    for _ in range(10): ui.pump(.1)
    assert 'Probe service_tls:' not in ui.screen.text()
    ui.send(b'd\x1b')
    ui.wait('[Overview]', timeout=25)
    ui.finish()
    print(json.dumps({'scenario': 'W12-explicit-doctor-keyboard-cancel', 'status': 'passed'}))
finally:
    if ui.p.poll() is None:
        peer.send_signal(ui.p, __import__('signal').SIGTERM)
        ui.p.wait(timeout=15)
