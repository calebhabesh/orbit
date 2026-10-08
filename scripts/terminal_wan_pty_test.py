#!/usr/bin/env python3
"""W07 real keyboard TUI, private marked roots and signed local-development relay.

The Go acceptance fixture supplies independent TLS/profile trust. No hosted or
physical WAN claim; direct candidates are not advertised in this relay fixture. CLI queries supply byte/identity
oracles, while setup/invitation/join/approval mutations use the keyboard TUI.
"""
import argparse
import base64
import fcntl
import hashlib
import json
import os
import re
from pathlib import Path
import shutil
import signal
import struct
import tempfile
import time

from terminal_onboarding_pty_test import Peer, UI, invite, approve, wait_bytes

# Hermetic: never select the packaged hosted profile, so no daemon started here
# contacts the operated service (W14). Child processes inherit this.
os.environ.setdefault('ORBIT_DISABLE_PACKAGED_PROFILE', '1')


def code(inv):
    return 'orbit-invitation:v3:' + base64.urlsafe_b64encode(json.dumps(inv).encode()).decode().rstrip('=')


def resize(ui, width, height):
    ui.screen.resize(width, height)
    fcntl.ioctl(ui.slave, __import__('termios').TIOCSWINSZ, struct.pack('HHHH', height, width, 0, 0))
    ui.peer.send_signal(ui.p, signal.SIGWINCH)
    time.sleep(.15)
    ui.pump()


def reveal(ui):
    # Inspect deliberate transfer without saving frames or exposing it on failure.
    ui.send(b'v')
    deadline = time.monotonic() + 12
    while time.monotonic() < deadline:
        ui.pump(.05)
        if 'orbit-invitation:v3:' in ''.join(ui.screen.text().split()): return
    raise AssertionError('v3 reveal label absent; private transfer screen omitted')


def hide_transfer(ui):
    # E05: v shows the code outside the panel; Enter returns and clears it.
    ui.send(b'\r')
    deadline = time.monotonic() + 12
    while time.monotonic() < deadline:
        ui.pump(.05)
        if 'characters and stays hidden' in ui.screen.text():
            # Raw transfer bytes are deliberately discarded before any helper
            # can include its diagnostic tail in a later error.
            ui.raw = bytearray()
            return
    raise AssertionError('private transfer hide failed; private screen omitted')


def root_review(ui, label, name, root):
    for value in (label, name, str(root)):
        ui.replace(value)
        ui.send(b'\t')
    assert 'Peer listen:' not in ui.screen.text(), 'ordinary address prompt'
    # Enter advances field by field and confirms on the last (E03).
    ui.submit('Confirm adoption')


def oracle(peer, folder, name, value, author):
    versions = peer.query('history', folder=folder, path=name, limit='20')['versions']
    assert any(v['digest'] == hashlib.sha256(value).hexdigest() and v['version']['author'] == author for v in versions), 'head/hash/author oracle failed'


def run(binary, profile, output, outage_marker):
    peers, active, daemons = [], [], []
    try:
        for _ in range(2):
            root = Path(tempfile.mkdtemp(prefix='orbit-w07-pty-')).resolve()
            root.chmod(0o700)
            peer = Peer(root, binary, None)
            peers.append(peer)
            # Configure development services independently of the invitation.
            # No init/serve or manual addresses: the CLI network review prepares
            # private state; the actual TUI starts each daemon.
            review = root / 'network-review.json'
            peer.run('orbit', 'network', 'preview', '--state', str(peer.state), '--mode', 'self_hosted', '--profile-file', str(profile), '--review-file', str(review), '--json')
            peer.run('orbit', 'network', 'apply', '--state', str(peer.state), '--review-file', str(review), '--json')
            peer.device = json.loads((peer.state / 'config.json').read_text())['device_id']
        a, b = peers
        (a.data / 'owner.txt').write_bytes(b'verified owner relay bytes')
        (b.data / 'local.txt').write_bytes(b'verified joining relay bytes')
        ua = UI(a, 'relay-create-back-edit', size=(100, 36)); active.append(ua)
        ua.wait('Join an existing Orbit [j]'); ua.send(b'c'); ua.wait('Review setup inputs')
        root_review(ua, 'Laptop', 'Notes', a.data)
        ua.wait('files=1'); ua.wait('Self-hosted automatic')
        ua.wait('contents stay encrypted in transit.')
        ua.back(); ua.wait('Review setup inputs'); ua.send(b'\r'); ua.wait('Confirm adoption')
        ua.send(b'\r'); ua.wait('Locally ready', timeout=30)
        folder = a.query('folders', limit='20')['items'][0]['id']
        settings = a.query('settings')['settings']
        assert not settings['advertised_peer'] and not settings['advertised_enrollment'], 'direct endpoints configured'
        ua.back(); ua.wait('[Overview]')
        inv = invite(ua, a, a.root / 'invite.json')
        assert inv['version'] == '3' and not inv['peer_endpoint'] and not inv['enrollment_endpoint']
        ub = UI(b, 'relay-join-expiry-pin-root-review', size=(100, 36)); active.append(ub)
        ub.wait('Join an existing Orbit [j]'); ub.send(b'j'); ub.wait('Join invitation')
        expired = dict(inv, expires_at='2000-01-01T00:00:00Z')
        ub.replace(code(expired)); ub.send(b'\r'); ub.wait('INVITATION_EXPIRED')
        wrong = dict(inv, key_pin='ff' * 32, route=dict(inv['route'], pin='ff' * 32))
        ub.replace(code(wrong)); ub.send(b'\r'); ub.wait('IDENTITY_MISMATCH')
        wrapped = '\n'.join(code(inv)[i:i+50] for i in range(0, len(code(inv)), 50))
        ub.invitation(wrapped)
        # Root-review error retains names and location before explicit correction.
        blocked = b.root / 'blocked'; blocked.mkdir(mode=0o700)
        (blocked / 'unsupported').symlink_to(b.data / 'local.txt')
        root_review_values = ('Pi', 'Notes', str(blocked))
        for value in root_review_values:
            ub.replace(value); ub.send(b'\t')
        ub.submit('ROOT_REVIEW_INCOMPLETE')
        # The rejected root is focused (E03); correct it and confirm.
        ub.replace(str(b.data)); ub.submit('Confirm adoption')
        ub.wait('Inviter operator:'); ub.send(b'\r'); ub.wait('Waiting for approval', timeout=35)
        operation = b.query('setups', limit='20')['items'][0]['id']
        before = b.query('operation', id=operation); request = before['join']['request']
        assert not before['readiness']['approved']
        assert inv['capability'].encode() not in ub.raw
        ub.finish(); active.remove(ub)
        # Actual daemon stop/relaunch; reuse exact reviewed attempt and operation.
        b.checked(); b.run('stop', '--state', str(b.state))
        ub = UI(b, 'relay-resume-resize-colorless', size=(40, 16)); active.append(ub)
        ub.wait('Waiting for approval', timeout=30)
        resize(ub, 100, 36); ub.wait('Waiting for approval')
        after = b.query('operation', id=operation)
        assert after['join']['attempt'] == before['join']['attempt'] and after['join']['request'] == request
        assert after['operation']['id'] == operation and after['join']['root'] == str(b.data)
        verification = approve(ua, a, request)
        assert verification == before['requests'][0]['verification_code']
        wait_bytes(a.data, b.data, 'owner.txt', b'verified owner relay bytes', active)
        wait_bytes(a.data, b.data, 'local.txt', b'verified joining relay bytes', active)
        ub.wait('Locally ready', timeout=55)
        oracle(b, folder, 'owner.txt', b'verified owner relay bytes', a.device)
        oracle(a, folder, 'local.txt', b'verified joining relay bytes', b.device)
        saved = json.loads((b.state / 'peer-routes.json').read_text())
        assert a.device in json.dumps(saved) and inv['key_pin'] in json.dumps(saved) and inv['certificate_der'] in json.dumps(saved)
        for name in ('owner.txt', 'local.txt'):
            left = a.query('history', folder=folder, path=name, limit='20')['versions']
            right = b.query('history', folder=folder, path=name, limit='20')['versions']
            assert len(left) == len(right) == 1 and left[0]['version'] == right[0]['version'], 'exact head identity mismatch'
        edited = b'joining device edited the existing owner file'
        (b.data / 'owner.txt').write_bytes(edited)
        wait_bytes(a.data, b.data, 'owner.txt', edited, active)
        oracle(a, folder, 'owner.txt', edited, b.device)
        left = a.query('history', folder=folder, path='owner.txt', limit='20')['versions']
        right = b.query('history', folder=folder, path='owner.txt', limit='20')['versions']
        assert {json.dumps(v['version'], sort_keys=True) for v in left} == {json.dumps(v['version'], sort_keys=True) for v in right}
        ub.send(b'N'); ub.wait('Connection details'); ub.wait('Connected via relay', timeout=30)
        ub.wait('observed='); assert 'Observed connections are separate' in ub.screen.text()
        ub.back(); ub.wait('[Overview]'); ua.back(); ua.wait('[Overview]')
        # Let the existing finite enrollment admission bucket refill after the
        # deliberately repeated proof/restart journey. Keep both actual daemons
        # and their verified routes running; no limit or authorization is changed.
        cooldown = time.monotonic() + 65
        while time.monotonic() < cooldown:
            for ui in active: ui.pump(.05)
            time.sleep(.1)
        # Second-folder invitation and distinct root/attempt, same device identities.
        second_a, second_b = a.root / 'second', b.root / 'second'
        second_a.mkdir(mode=0o700); second_b.mkdir(mode=0o700)
        (second_a / 'second.txt').write_bytes(b'separate scoped folder bytes')
        ua.send(b'c'); ua.wait('Review setup inputs'); root_review(ua, 'Laptop', 'Second', second_a)
        ua.send(b'\r'); ua.wait('Locally ready', timeout=30); ua.back(); ua.wait('[Overview]')
        ua.send(b'a'); ua.wait('Select folder')
        items = a.query('folders', limit='20')['items']; ua.wait('> ' + items[0]['name'])
        idx = next(i for i, it in enumerate(items) if it['root'] == str(second_a))
        second = items[idx]['id']; ua.send(b'j' * idx + b'\r'); ua.wait('Reviewed membership revision:')
        ua.send(b'\r'); ua.wait('Private invitation'); ua.send(b's'); ua.wait('save_invitation')
        transfer = a.root / 'second-invite.json'; ua.replace(str(transfer)); ua.send(b'\r'); ua.wait('Private invitation saved'); ua.back()
        inv2 = json.loads(transfer.read_text()); assert inv2['folder'] == second
        ub.send(b'J'); ub.wait('Join invitation'); ub.invitation(str(transfer))
        root_review(ub, 'Pi', 'Second', second_b); ub.send(b'\r')
        time.sleep(1); ub.pump()
        prepared = next(it for it in b.query('setups', limit='20')['items'] if it['root'] == str(second_b))
        diagnostic = b.query('operation', id=prepared['id'])
        if diagnostic.get('error', {}).get('code') == 'SETUP_BLOCKED':
            message = re.sub(r'https?://\S+|[A-Za-z0-9_+/=-]{40,}', '<REDACTED>', diagnostic['error']['message'])
            raise AssertionError('second-folder category: ' + message)
        ub.wait('Waiting for approval', timeout=60)
        pending = b.query('setups', limit='20')['items']
        op2 = next(it['id'] for it in pending if it['root'] == str(second_b))
        second_join = b.query('operation', id=op2)
        assert second_join['join']['attempt'] != before['join']['attempt']
        approve(ua, a, second_join['join']['request'])
        wait_bytes(second_a, second_b, 'second.txt', b'separate scoped folder bytes', active)
        ub.wait('Locally ready', timeout=55)
        # Verify invitation reveal labeling and revoke through existing controller.
        ua.back(); ua.wait('[Overview]')
        ua.send(b'a'); ua.wait('Select folder'); ua.wait('> ' + a.query('folders', limit='20')['items'][0]['name']); ua.send(b'\r'); ua.wait('Reviewed membership revision:')
        ua.send(b'\r'); ua.wait('Private invitation')
        ua.send(b's'); ua.wait('save_invitation')
        revoked_file = a.root / 'revoked-transfer.json'; ua.replace(str(revoked_file)); ua.send(b'\r'); ua.wait('Private invitation saved')
        revoked_inv = json.loads(revoked_file.read_text())
        assert revoked_inv['capability'].encode() not in ua.raw
        reveal(ua)  # deliberate transfer excluded from evidence
        hide_transfer(ua); ua.send(b'x'); ua.wait('Revoke invitation'); ua.send(b'\r'); ua.wait('Invitation revoked')
        invitations = json.loads(a.run('orbit', 'invite', 'list', '--state', str(a.state), '--folder', revoked_inv['folder'], '--json'))['invitations']
        verifier = hashlib.sha256(bytes.fromhex(revoked_inv['capability'])).digest()
        assert any(inv['Digest'] == verifier.hex() and inv['Revoked'] for inv in invitations), 'revocation not durable'
        for ui in list(active): ui.finish(); active.remove(ui)
        # Daemons keep capturing and pulling after every terminal client exits.
        (a.data / 'after-ui.txt').write_bytes(b'background relay after terminal exit')
        wait_bytes(a.data, b.data, 'after-ui.txt', b'background relay after terminal exit', [])
        oracle(b, folder, 'after-ui.txt', b'background relay after terminal exit', a.device)
        for peer in peers:
            assert json.loads((peer.state / 'config.json').read_text())['device_id'] == peer.device
        # Shut down only this marked in-process development service. Daemon local
        # capture and entered new-folder drafts remain usable through the outage.
        outage_path = Path(outage_marker)
        assert outage_path.is_absolute() and outage_path.parent.resolve(strict=True) == outage_path.parent
        marker = outage_path.parent / '.orbit-disposable'
        assert marker.is_file() and not marker.is_symlink(), 'disposable service marker required'
        with outage_path.open('x') as trigger: trigger.write('stop disposable service only')
        ua = UI(a, 'service-outage-local-capture-draft', size=(100, 36)); active.append(ua)
        ua.wait('[Overview]'); ua.send(b'N'); ua.wait('Connection details'); ua.wait('SERVICE_UNAVAILABLE', timeout=25)
        ua.back(); ua.wait('[Overview]'); ua.send(b'c'); ua.wait('Review setup inputs')
        root_review(ua, 'Laptop', 'OfflineDraft', a.root / 'offline-draft')
        ua.back(); ua.wait('Review setup inputs'); ua.wait('OfflineDraft')
        ua.finish(); active.remove(ua)
        value = b'capture during development service outage'
        (a.data / 'outage.txt').write_bytes(value)
        deadline = time.monotonic() + 25
        while time.monotonic() < deadline:
            versions = a.query('history', folder=folder, path='outage.txt', limit='20')['versions']
            if versions: break
            time.sleep(.2)
        assert versions and versions[0]['digest'] == hashlib.sha256(value).hexdigest()
        # Fresh TUI default consent and local-only choice before first mutation.
        for local_only in (False, True):
            root = Path(tempfile.mkdtemp(prefix='orbit-w07-fresh-')).resolve(); root.chmod(0o700)
            peer = Peer(root, binary, None); peers.append(peer)
            ui = UI(peer, 'fresh-local-only' if local_only else 'fresh-automatic-missing-profile', size=(100, 36)); active.append(ui)
            ui.wait('Join an existing Orbit [j]'); ui.send(b'c'); ui.wait('Review setup inputs')
            ui.wait('automatic')
            assert not (peer.state / 'network.json').exists(), 'policy persisted before review'
            if local_only: ui.send(b'\x0e')
            root_review(ui, 'Fresh', 'Local', peer.data)
            ui.wait('Local network only' if local_only else 'Connection: Automatic')
            ui.send(b'\r'); ui.wait('Locally ready', timeout=30)
            ui.send(b'N'); ui.wait('Connection details')
            ui.wait('LOCAL_ONLY' if local_only else 'PROFILE_MISSING_OR_EXPIRED')
            policy = peer.query('network_status')['network']['policy']
            assert policy['mode'] == ('local_only' if local_only else 'automatic')
            peer.device = json.loads((peer.state / 'config.json').read_text())['device_id']
            ui.finish(); active.remove(ui)
            (peer.data / 'offline.txt').write_bytes(b'local capture with unavailable WAN profile')
            deadline = time.monotonic() + 25
            while time.monotonic() < deadline:
                versions = peer.query('history', folder=peer.query('folders', limit='20')['items'][0]['id'], path='offline.txt', limit='20')['versions']
                if versions: break
                time.sleep(.2)
            assert versions and versions[0]['digest'] == hashlib.sha256(b'local capture with unavailable WAN profile').hexdigest()
        results = [{'scenario': 'W07-keyboard-relay-onboarding', 'result': 'passed', 'assertions': [
            'production binary real PTY; no manual addresses/listeners/init/serve; relay fixture preserves LAN advertising=false',
            'reviewed existing contents, Back/Edit, wrong pin, expired v3 paste, unsupported-root correction',
            'private transfer, independently selected development operator and TLS trust',
            'exact cross-device verification/approval, delayed exit and actual daemon restart',
            'exact operation/attempt/request/root/identity retained; byte/hash/version-author oracles',
            'second folder separate consent/attempt/root, relay observations separate from readiness',
            'narrow/colorless/resize, masked paste, v3 reveal, deliberate revocation',
            'termios and alt/bracketed-paste restoration; daemon capture/transfer after client exit',
            'actual marked service outage retains new-folder drafts and daemon local capture',
            'fresh Automatic/missing-profile and preannouncement Local-only review; local capture after quit']}]
        if output:
            out = Path(output); out.mkdir(mode=0o700, parents=True, exist_ok=True)
            assert not any(out.iterdir()), 'evidence output must be empty'
            for i, peer in enumerate(peers):
                for name, frames in peer.raw.items():
                    (out / f'{i}-{name}.txt').write_text(frames)
            (out / 'results.json').write_text(json.dumps(results, indent=2) + '\n')
        print(json.dumps(results, indent=2))
    finally:
        for ui in active: ui.finish()
        for peer in peers:
            peer.checked()
            if (peer.state / 'config.json').exists():
                peer.run('stop', '--state', str(peer.state))
            for p, _ in peer.children.values():
                if p.poll() is None: peer.send_signal(p, signal.SIGTERM); p.wait(timeout=12)
            peer.checked(); shutil.rmtree(peer.root)


if __name__ == '__main__':
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--outage-marker', required=True); p.add_argument('--binary', required=True); p.add_argument('--profile', required=True); p.add_argument('--output')
    args = p.parse_args()
    run(Path(args.binary).resolve(strict=True), Path(args.profile).resolve(strict=True), args.output, args.outage_marker)
