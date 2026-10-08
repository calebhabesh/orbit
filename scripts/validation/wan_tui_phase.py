#!/usr/bin/env python3
"""W16 keyboard TUI phase on one native host, inside an existing marked W16 root.

The runner uploads this file and terminal_vt.py beside the packaged binary and
runs one phase per call: create, invite, join, approve or observe. Each phase
starts the packaged `orbit tui` on a real PTY, types keys, and exits; the
launcher-started daemon keeps running. Queries use the loopback owner control
only as an oracle. Private invitation capabilities never enter frames or
output; redacted frames are written to <root>/tui-frames/<phase>.txt.
"""
import argparse
import copy
import fcntl
import json
import os
from pathlib import Path
import pty
import re
import select
import signal
import struct
import subprocess
import sys
import termios
import time
import urllib.request

sys.path.insert(0, str(Path(__file__).resolve().parent))
from terminal_vt import Screen  # noqa: E402


class Host:
    def __init__(self, root, token):
        self.root = Path(root).resolve(strict=True)
        marker = self.root / '.orbit-disposable'
        if marker.is_symlink() or marker.read_text() != token or not self.root.name.startswith('orbit-validation-'):
            raise RuntimeError('W16 TUI phase requires the marked validation root')
        self.binary = self.root / 'orbit'
        self.state = self.root / 'state'

    def query(self, kind, **fields):
        endpoint = (self.state / 'control.addr').read_text().strip()
        if not endpoint.startswith('127.0.0.1:'):
            raise RuntimeError('owner control must be loopback')
        token = (self.state / 'control.token').read_text().strip()
        request = urllib.request.Request('http://' + endpoint + '/control/terminal/v1/query',
                                         data=json.dumps(dict(version='1', kind=kind, **fields)).encode(),
                                         headers={'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json'})
        with urllib.request.build_opener(urllib.request.ProxyHandler({})).open(request, timeout=8) as response:
            return json.load(response)


class UI:
    def __init__(self, host, name, size=(100, 36)):
        self.host, self.name = host, name
        self.master, self.slave = pty.openpty()
        fcntl.ioctl(self.slave, termios.TIOCSWINSZ, struct.pack('HHHH', size[1], size[0], 0, 0))
        self.before = copy.deepcopy(termios.tcgetattr(self.slave))
        self.screen, self.raw, self.frames = Screen(*size), bytearray(), []
        self.p = subprocess.Popen([str(host.binary), 'tui', '--state', str(host.state), '--no-color'],
                                  stdin=self.slave, stdout=self.slave, stderr=self.slave, cwd=host.root,
                                  env=dict(os.environ, TERM='xterm-256color', NO_COLOR='1'))

    def pump(self, timeout=.02):
        if select.select([self.master], [], [], timeout)[0]:
            data = os.read(self.master, 65536)
            self.raw.extend(data)
            self.screen.feed(data)
            if len(self.raw) > 8 << 20:
                raise RuntimeError('unbounded terminal output')

    def send(self, keys):
        os.write(self.master, keys)
        self.pump(.04)

    def text(self):
        return self.screen.text()

    def wait(self, text, timeout=15):
        until = time.monotonic() + timeout
        while time.monotonic() < until:
            self.pump(.05)
            if text in self.text():
                self.frames.append(self.text())
                return
            if self.p.poll() is not None:
                break
        self.frames.append(self.text())
        raise RuntimeError(f'{self.name}: visible {text!r} absent')

    def submit(self, text, timeout=60, presses=6):
        # Enter advances field by field and confirms on the last (E03).
        for _ in range(presses):
            self.send(b'\r')
            time.sleep(.2)
            self.pump(.1)
            if text in self.text():
                self.frames.append(self.text())
                return
        self.wait(text, timeout)

    def replace(self, text):
        # End, delete-before-cursor, then one bracketed paste.
        self.send(b'\x05\x15\x1b[200~' + text.encode() + b'\x1b[201~')

    def back(self):
        self.send(b'\x1b')
        time.sleep(.1)
        self.pump()

    def finish(self):
        if self.p.poll() is None:
            self.send(b'\x03')
        until = time.monotonic() + 12
        while self.p.poll() is None and time.monotonic() < until:
            self.pump(.05)
        if self.p.poll() is None:
            self.p.send_signal(signal.SIGTERM)
            self.p.wait(timeout=5)
        self.pump(.01)
        restored = termios.tcgetattr(self.slave) == self.before and b'\x1b[?1049l' in self.raw
        os.close(self.master)
        os.close(self.slave)
        return restored


def redact(text, host, secrets):
    text = re.sub(r'[ \t\r\n]*'.join(re.escape(c) for c in str(host.root)), '<root>', text)
    for secret in secrets:
        text = text.replace(secret, '<capability>')
    # Observed candidates and service addresses stay out of evidence.
    return re.sub(r'\b\d{1,3}(?:\.\d{1,3}){3}\b', '<ipv4>', text)


def root_review(ui, label, name, root):
    for value in (label, name, str(root)):
        ui.replace(value)
        ui.send(b'\t')
    if 'Peer listen:' in ui.text():
        raise RuntimeError('ordinary journey showed a manual address prompt')
    ui.submit('Confirm adoption')


def phase_create(host, ui, args, out):
    ui.wait('Join an existing Orbit [j]')
    ui.send(b'c')
    ui.wait('Review setup inputs')
    root_review(ui, args.label, args.name, host.root / args.relative)
    review = ui.text()
    out['review_mode'] = 'packaged profile' if 'packaged profile' in review else 'other'
    if args.hosted and out['review_mode'] != 'packaged profile':
        raise RuntimeError('hosted create review did not show the packaged profile operator')
    ui.send(b'\r')
    ui.wait('Locally ready', 60)
    folders = host.query('folders', limit='20')['items']
    out['folder'] = next(f['id'] for f in folders if f['root'] == str(host.root / args.relative))
    ui.back()
    ui.wait('[Overview]')


def overview(ui):
    # E08: a configured device with nothing needing attention opens on Files.
    ui.wait('Orbit', 30)
    ui.send(b'1')
    ui.wait('[Overview]', 30)


def phase_invite(host, ui, args, out):
    overview(ui)
    ui.send(b'a')
    ui.wait('Select folder')
    items = host.query('folders', limit='20')['items']
    ui.wait('> ' + items[0]['name'])
    index = next(i for i, it in enumerate(items) if it['id'] == args.folder)
    ui.send(b'j' * index + b'\r')
    ui.wait('Private invitation', 60)
    ui.send(b's')
    ui.wait('save_invitation')
    target = host.root / args.invitation
    ui.replace(str(target))
    ui.send(b'\r')
    ui.wait('Private invitation saved')
    invitation = json.loads(target.read_text())
    if target.stat().st_mode & 0o077:
        raise RuntimeError('saved invitation is not private')
    if invitation['capability'].encode() in ui.raw:
        raise RuntimeError('capability leaked into the masked UI')
    out['secrets'] = [invitation['capability']]
    out['invitation'] = {'version': invitation.get('version'), 'folder': invitation.get('folder'),
                         'peer_endpoint': bool(invitation.get('peer_endpoint')),
                         'enrollment_endpoint': bool(invitation.get('enrollment_endpoint')),
                         'routed': bool(invitation.get('route'))}
    ui.back()


def phase_join(host, ui, args, out):
    ui.wait('Join an existing Orbit [j]')
    ui.send(b'j')
    ui.wait('Join invitation')
    invitation = host.root / args.invitation
    out['secrets'] = [json.loads(invitation.read_text())['capability']]
    ui.replace(str(invitation))
    ui.send(b'\r')
    ui.wait('Review setup inputs', 30)
    root_review(ui, args.label, args.name, host.root / args.relative)
    ui.wait('Inviter operator:')
    if args.hosted and '(same operator and profile as this device)' not in ui.text():
        raise RuntimeError('joiner did not review the same packaged operator')
    ui.send(b'\r')
    ui.wait('Waiting for approval', 90)
    setup = next(it for it in host.query('setups', limit='20')['items'] if it['root'] == str(host.root / args.relative))
    operation = host.query('operation', id=setup['id'])
    out['operation'] = operation['operation']['id']
    out['attempt'] = operation['join']['attempt']
    out['request'] = operation['join']['request']
    out['verification_code'] = operation['requests'][0]['verification_code']
    out['folder'] = operation['join']['folder']


def phase_approve(host, ui, args, out):
    overview(ui)
    ui.send(b'w')
    ui.wait('Enrollment requests')
    requests = host.query('requests', limit='20')['requests']
    ui.wait('> ' + requests[0]['label'])
    index = next(i for i, r in enumerate(requests) if r['id'] == args.request)
    ui.send(b'j' * index + b'\r')
    ui.wait('Exact request approval')
    code = requests[index]['verification_code']
    if code not in ui.text():
        raise RuntimeError('approval review did not show the verification code')
    ui.send(b'a')
    ui.wait('Exact request approve completed', 60)
    out['verification_code'] = code
    ui.back()


def phase_observe(host, ui, args, out):
    overview(ui)
    ui.send(b'N')
    ui.wait('Connection details')
    deadline = time.monotonic() + 60
    while time.monotonic() < deadline:
        ui.pump(.1)
        for label in ('Direct connection', 'Connected via relay'):
            if label in ui.text():
                ui.frames.append(ui.text())
                out['route_label'] = label
                ui.back()
                return
    raise RuntimeError('no connected peer route shown')


PHASES = {'create': phase_create, 'invite': phase_invite, 'join': phase_join,
          'approve': phase_approve, 'observe': phase_observe}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', required=True)
    parser.add_argument('--token', required=True)
    parser.add_argument('--phase', required=True, choices=sorted(PHASES))
    parser.add_argument('--hosted', action='store_true')
    parser.add_argument('--relative', default='data')
    parser.add_argument('--label', default='Device')
    parser.add_argument('--name', default='W16 TUI')
    parser.add_argument('--folder')
    parser.add_argument('--request')
    parser.add_argument('--invitation', default='tui-invitation.json')
    args = parser.parse_args()
    host = Host(args.root, args.token)
    out = {'phase': args.phase}
    started = time.monotonic()
    ui = UI(host, args.phase)
    error = None
    try:
        PHASES[args.phase](host, ui, args, out)
    except Exception as failure:  # reported after frames are saved
        error = failure
    finally:
        out['terminal_restored'] = ui.finish()
        frames = redact('\n--- frame ---\n'.join(ui.frames), host, out.pop('secrets', []))
        target = host.root / 'tui-frames'
        target.mkdir(mode=0o700, exist_ok=True)
        (target / (args.phase + '-' + args.relative + '.txt')).write_text(frames)
    out['seconds'] = round(time.monotonic() - started, 1)
    if error is not None:
        out['error'] = type(error).__name__ + ': ' + redact(str(error), host, [])
    print(json.dumps(out))
    sys.exit(1 if error is not None or not out['terminal_restored'] else 0)


if __name__ == '__main__':
    main()
