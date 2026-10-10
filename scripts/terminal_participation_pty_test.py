#!/usr/bin/env python3
"""E13: real Leave, name-confirmed removal and removed-device PTYs on marked roots."""
import argparse
import base64
import json
import os
from pathlib import Path
import shutil
import signal
import tempfile
import time

from terminal_onboarding_pty_test import Peer, UI, invite, approve, wait_bytes
from terminal_everyday_pty_test import until, folder_ui

os.environ.setdefault('ORBIT_DISABLE_PACKAGED_PROFILE', '1')


def pair(binary, output, scenario):
    peers, active, daemons = [], [], []
    try:
        for _ in range(2):
            root = Path(tempfile.mkdtemp(prefix='orbit-e13-pty-')).resolve()
            root.chmod(0o700)
            peer = Peer(root, binary, output)
            peer.initialize()
            peers.append(peer)
            daemons.append(peer.start_daemon())
        a, b = peers
        (a.data / 'kept.txt').write_bytes(b'captured before participation ends')
        ua = UI(a, scenario + '-owner', size=(100, 30)); active.append(ua)
        ua.wait('Join an existing Orbit [j]'); ua.send(b'c'); ua.wait('Connection choices:')
        ua.form('PC', a.data); ua.send(b'\r'); ua.wait('is ready on', timeout=25)
        ua.back(); ua.wait('[Overview]')
        folder = a.query('folders', limit='20')['items'][0]['id']
        invitation = invite(ua, a, a.root / 'invite.json')
        code = 'orbit-invitation:v2:' + base64.urlsafe_b64encode(json.dumps(invitation).encode()).decode().rstrip('=')
        ub = UI(b, scenario + '-laptop', size=(80, 26)); active.append(ub)
        ub.wait('Join an existing Orbit [j]'); ub.send(b'j'); ub.wait('Join an Orbit')
        ub.invitation(code); ub.form('Laptop', b.data); ub.send(b'\r')
        ub.wait('Check that it shows this code', timeout=25)
        op = b.query('setups', limit='20')['items'][0]['id']
        request = b.query('operation', id=op)['join']['request']
        approve(ua, a, request)
        wait_bytes(a.data, b.data, 'kept.txt', b'captured before participation ends', active)
        ub.wait('is ready on', timeout=45); ub.back(); ub.wait('[Overview]')
        ua.back()
        if scenario == 'leave':
            folder_ui(ub); ub.send(b'L'); ub.wait('Leave Orbit'); ub.wait('Unsynced edits')
            ub.back(); assert b.query('folders', limit='20')['items'], 'Esc performed Leave'
            folder_ui(ub); ub.send(b'L'); ub.wait('Leave Orbit'); ub.send(b'\r')
            until(lambda: not b.query('folders', limit='20')['items'], active)
            # Later local edits stay local; the other side gets one actionable item.
            (a.data / 'after-leave.txt').write_bytes(b'not sent to left device')
            (b.data / 'local-after-leave.txt').write_bytes(b'not sent from left device')
            attention = until(lambda: [it for it in a.query('attention', limit='20')['attention'] if it['code'] == 'PEER_LEFT'], active)
            assert len(attention) == 1, attention
            time.sleep(2)
            assert not (b.data / 'after-leave.txt').exists()
            assert not (a.data / 'local-after-leave.txt').exists()
            # Remove the departed member directly from the attention item.
            ua.send(b'3'); ua.wait('PEER_LEFT'); ua.send(b'\r'); ua.wait('Remove Device')
            ua.wait('recorded changes received here.'); ua.wait('Type the device name')
        else:
            folder_ui(ua); ua.send(b'X'); ua.wait('Select Device to Remove')
            ua.send(b'\r'); ua.wait('Remove Device'); ua.wait('Type the device name')
        ua.replace('Wrong'); ua.send(b'\r'); ua.wait('exactly to confirm')
        assert any(m['id'] == b.device for m in a.query('folder_management', folder=folder)['folder_management']['members'])
        ua.replace('Laptop'); ua.send(b'\r'); ua.wait('Device removed from this Orbit.', timeout=25)
        assert not any(m['id'] == b.device for m in a.query('folder_management', folder=folder)['folder_management']['members'])
        if scenario == 'remove':
            until(lambda: b.query('folder_management', folder=folder)['folder_management'].get('removed_by') == 'PC', active)
            ub.back(); ub.send(b'2'); ub.wait('Notes'); ub.send(b'\r'); ub.wait('Device Removed'); ub.wait('removed by PC')
            ub.send(b'\r'); ub.wait('Leave Orbit'); ub.send(b'\r')
            until(lambda: not b.query('folders', limit='20')['items'], active)
        for peer in peers:
            assert (peer.data / 'kept.txt').read_bytes() == b'captured before participation ends'
            assert json.loads((peer.state / 'config.json').read_text())['device_id'] == peer.device
        for ui in list(active): ui.finish(); active.remove(ui)
        return dict(scenario=scenario, result='passed', assertions=['real PTY confirmation', 'name check', 'working bytes retained', 'identities retained', 'per-Orbit sync stop', 'terminal restored'])
    finally:
        for ui in active: ui.finish()
        for peer in peers:
            for proc, _ in peer.children.values():
                if proc.poll() is None: peer.send_signal(proc, signal.SIGTERM); proc.wait(timeout=12)
            peer.checked(); shutil.rmtree(peer.root)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--binary', default='bin/orbit'); parser.add_argument('--output')
    args = parser.parse_args()
    binary = Path(args.binary).resolve(strict=True)
    results = [pair(binary, args.output, scenario) for scenario in ('leave', 'remove')]
    if args.output:
        out = Path(args.output); out.mkdir(parents=True, exist_ok=True)
        (out / 'participation-results.json').write_text(json.dumps(results, indent=2) + '\n')
    print(json.dumps(results, indent=2))


if __name__ == '__main__': main()
