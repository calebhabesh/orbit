#!/usr/bin/env python3
"""R03/R05 rootless namespace fixture for a self-hosted Orbit service.

Runs entirely inside a user + network namespace created with `unshare -rn`
(no sudo), so nothing touches host networking, the owner's folders or any
deployed service. Topology, all private 10.77.0.0/16 addresses:

    router (the outer namespace, forwards)
      ├─ svc  10.77.0.2   orbit-net (made by `orbit-net init`)
      ├─ a    10.77.1.2   orbit daemon A
      └─ b    10.77.2.2   orbit daemon B

`--block-direct` drops A↔B forwarding (TCP and UDP) in the router, so the
devices can reach each other only through the service relay. `--netem` adds
delay/loss on the device links. The driver creates and verifies a fresh
marked root, runs `orbit-net init`, adds both devices to the allowlist,
creates an Orbit on A, invites B by code, approves, syncs both ways and
reads each device's route accounting. It writes results JSON and keeps the
root on failure.

    python3 scripts/validation/selfhost_netns.py --orbit bin/orbit \
        --orbit-net bin/orbit-net --root /new/private/dir --out result.json --block-direct
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import sys
import time
import urllib.request

MARKER = '.orbit-selfhost-netns-disposable'
NODES = {'svc': ('10.77.0.1', '10.77.0.2'), 'a': ('10.77.1.1', '10.77.1.2'), 'b': ('10.77.2.1', '10.77.2.2')}


def sh(*args, check=True, **kw):
    r = subprocess.run(list(args), capture_output=True, text=True, timeout=kw.pop('timeout', 120), **kw)
    if check and r.returncode:
        raise RuntimeError(f'{args[:4]} exit {r.returncode}: {r.stderr[-800:]}')
    return r


def wait(what, fn, seconds):
    deadline = time.monotonic() + seconds
    last = None
    while time.monotonic() < deadline:
        try:
            v = fn()
            if v:
                return v
        except Exception as e:  # noqa: BLE001 - diagnostic only
            last = e
        time.sleep(0.5)
    raise RuntimeError(f'timeout: {what} ({last})')


class NS:
    def __init__(self, name):
        self.name = name
        self.holder = subprocess.Popen(['unshare', '-n', 'sleep', 'infinity'])
        time.sleep(0.2)
        self.gw, self.ip = NODES[name]

    def run(self, *args, env=None, check=True, timeout=120):
        return sh('nsenter', '-t', str(self.holder.pid), '-n', *args, env=env, check=check, timeout=timeout)

    def popen(self, args, env, log):
        return subprocess.Popen(['nsenter', '-t', str(self.holder.pid), '-n', *args], env=env, stdout=log, stderr=log)


def topology(block_direct, netem):
    sh('sysctl', '-qw', 'net.ipv4.ip_forward=1')
    ns = {}
    for name in NODES:
        n = NS(name)
        ns[name] = n
        sh('ip', 'link', 'add', 'v' + name, 'type', 'veth', 'peer', 'name', 'r' + name)
        sh('ip', 'link', 'set', 'v' + name, 'netns', str(n.holder.pid))
        sh('ip', 'addr', 'add', n.gw + '/24', 'dev', 'r' + name)
        sh('ip', 'link', 'set', 'r' + name, 'up')
        n.run('ip', 'addr', 'add', n.ip + '/24', 'dev', 'v' + name)
        n.run('ip', 'link', 'set', 'v' + name, 'up')
        n.run('ip', 'link', 'set', 'lo', 'up')
        n.run('ip', 'route', 'add', 'default', 'via', n.gw)
        if netem and name != 'svc':
            sh('tc', 'qdisc', 'add', 'dev', 'r' + name, 'root', 'netem', *netem.split())
    rules = []
    if block_direct:
        a, b = NODES['a'][1], NODES['b'][1]
        subprocess.run(['nft', '-f', '-'], check=True, text=True, input=f'''table inet selfhost {{
  chain forward_drop {{
    type filter hook forward priority 0; policy accept;
    ip saddr {a} ip daddr {b} counter drop
    ip saddr {b} ip daddr {a} counter drop
  }}
}}
''')
        rules = sh('nft', 'list', 'table', 'inet', 'selfhost').stdout
    return ns, rules


class Device:
    def __init__(self, name, ns, root, orbit):
        self.name, self.ns, self.orbit_bin = name, ns, orbit
        self.dir = root / name
        self.home, self.state, self.data = self.dir / 'home', self.dir / 'state', self.dir / 'data'
        for d in (self.dir, self.home, self.state):
            d.mkdir(mode=0o700)
        self.env = {**os.environ, 'HOME': str(self.home), 'XDG_CONFIG_HOME': str(self.home / '.config'),
                    'XDG_STATE_HOME': str(self.home / '.local/state'), 'ORBIT_DISABLE_PACKAGED_PROFILE': '1'}
        self.log = open(self.dir / 'daemon.log', 'ab')
        self.proc = None

    def cli(self, *args, check=True, timeout=120):
        r = self.ns.run(self.orbit_bin, *args, '--state', str(self.state), env=self.env, check=False, timeout=timeout)
        if check and r.returncode:
            raise RuntimeError(f'{self.name} orbit {args[:3]} exit {r.returncode}: {r.stderr[-800:]} {r.stdout[-400:]}')
        return r

    def start(self):
        self.proc = self.ns.popen([self.orbit_bin, 'serve', '--state', str(self.state), '--control-listen', '127.0.0.1:0'], self.env, self.log)
        wait(self.name + ' control', lambda: self.query('capabilities'), 30)

    def stop(self):
        if self.proc and self.proc.poll() is None:
            self.proc.send_signal(signal.SIGTERM)
            try:
                self.proc.wait(20)
            except subprocess.TimeoutExpired:
                self.proc.kill()
                self.proc.wait()

    def query(self, kind, **fields):
        address = (self.state / 'control.addr').read_text().strip()
        token = (self.state / 'control.token').read_text().strip()
        body = json.dumps({'version': '1', 'kind': kind, **fields}).encode()
        # Owner control is loopback inside the device's namespace.
        r = self.ns.run('python3', '-c', 'import sys,urllib.request,json;'
                        'q=urllib.request.Request(sys.argv[1],data=sys.argv[2].encode(),headers={"Authorization":"Bearer "+sys.argv[3],"Content-Type":"application/json"});'
                        'print(urllib.request.build_opener(urllib.request.ProxyHandler({})).open(q,timeout=8).read().decode())',
                        'http://' + address + '/control/terminal/v1/query', body.decode(), token, timeout=20)
        return json.loads(r.stdout)

    def identity(self):
        device = json.loads((self.state / 'config.json').read_text())['device_id']
        pem = (self.state / 'identity' / 'peer-identity.pem')
        if not pem.exists():
            pem = self.state / 'peer-identity.pem'
        cert = sh('openssl', 'x509', '-in', str(pem), '-pubkey', '-noout').stdout
        spki = subprocess.run(['openssl', 'pkey', '-pubin', '-outform', 'DER'], input=cert.encode(), capture_output=True, check=True).stdout
        return device, hashlib.sha256(spki).hexdigest()

    def put(self, rel, data):
        p = self.data / rel
        p.write_bytes(data)

    def sha(self, rel):
        p = self.data / rel
        return hashlib.sha256(p.read_bytes()).hexdigest() if p.exists() else None


def inner(args):
    root = Path(args.root)
    out = {'packet': args.packet, 'started_at': time.time(), 'block_direct': args.block_direct, 'netem': args.netem,
           'success': False, 'steps': {}}
    procs = []
    ns, rules = topology(args.block_direct, args.netem)
    out['nft'] = rules
    svc_dir = root / 'svc'
    devices = {}
    try:
        t = time.monotonic()
        r = ns['svc'].run(args.orbit_net, 'init', '--dir', str(svc_dir), '--origin', 'https://10.77.0.2:8443', '--stun', '10.77.0.2:3478')
        out['steps']['init'] = {'seconds': time.monotonic() - t, 'stdout': r.stdout}
        modes = {str(p.relative_to(svc_dir)): oct(p.stat().st_mode & 0o777) for p in sorted(svc_dir.rglob('*'))}
        out['steps']['init']['modes'] = modes
        if any(m not in ('0o600', '0o700') for m in modes.values()):
            raise RuntimeError('init wrote a non-owner-only file')
        check = ns['svc'].run(args.orbit_net, 'serve', '--config', str(svc_dir / 'serve.json'), '--check', check=False)
        out['steps']['check_before_devices'] = {'returncode': check.returncode, 'stderr': check.stderr[-300:]}
        if check.returncode == 0:
            raise RuntimeError('--check passed with no allowed device')
        for name in ('a', 'b'):
            d = Device(name, ns[name], root, args.orbit)
            devices[name] = d
            # The launcher initializes fresh state and starts a detached daemon.
            d.cli('network', 'preview', '--mode', 'self_hosted', '--profile-file', str(svc_dir / 'profile.json'),
                  '--service-roots', str(svc_dir / 'service-ca.pem'), '--review-file', str(d.dir / 'network-review.json'), '--json')
            d.cli('network', 'apply', '--review-file', str(d.dir / 'network-review.json'), '--json', timeout=60)
            # apply restarts the daemon detached in this namespace; stop it and
            # run our own supervised daemon once the service is up.
            d.cli('stop', check=False)
            d.stop()
            device, pin = d.identity()
            r = ns['svc'].run(args.orbit_net, 'devices', 'add', '--config', str(svc_dir / 'serve.json'), '--label', name, f'device={device} pin={pin}')
            out['steps'].setdefault('devices_add', []).append(r.stdout.strip())
        check = ns['svc'].run(args.orbit_net, 'serve', '--config', str(svc_dir / 'serve.json'), '--check')
        out['steps']['check'] = check.stderr.strip()
        slog = open(svc_dir / 'serve.log', 'ab')
        service = ns['svc'].popen([args.orbit_net, 'serve', '--config', str(svc_dir / 'serve.json')], os.environ, slog)
        procs.append(service)
        a, b = devices['a'], devices['b']
        for d in (a, b):
            d.start()
        t = time.monotonic()
        for d in (a, b):
            wait(d.name + ' service ready', lambda d=d: d.query('network_status')['network']['ready'], 90)
        out['steps']['ready_seconds'] = time.monotonic() - t
        # Create on A, invite B by code, approve on A.
        a.data.mkdir(mode=0o700)
        a.put('from-a.txt', b'A bytes through a self-hosted service\n')
        a.cli('setup', '--root', str(a.data), '--label', 'A', '--name', 'Selfhost', '--preview', '--review-file', str(a.dir / 'setup.json'), '--json')
        created = json.loads(a.cli('setup', '--request-file', str(a.dir / 'setup.json'), '--timeout', '0', '--json').stdout)
        folder = created['join']['folder']
        code = a.cli('devices', 'invite', '--folder', folder, '--code', '--long').stdout.strip()
        (b.dir / 'invitation.txt').write_text(code)
        os.chmod(b.dir / 'invitation.txt', 0o600)
        t = time.monotonic()
        b.cli('join', '--root', str(b.data), '--label', 'B', '--name', 'Selfhost', '--invitation-file', str(b.dir / 'invitation.txt'),
              '--preview', '--review-file', str(b.dir / 'join.json'), '--json')
        submitted = b.cli('join', '--request-file', str(b.dir / 'join.json'), '--timeout', '0', '--json', check=False)
        pending = json.loads(submitted.stdout)
        operation = pending['operation']['id']
        request = wait('B request', lambda: b.query('operation', id=operation)['join']['request'], 120)
        item = wait('A sees request', lambda: next((x for x in a.query('requests', limit='20')['requests'] if x['id'] == request), None), 120)
        review = {'request': request, 'folder': item['folder'], 'requester': item['requester'], 'key_pin': item['key_pin'],
                  'transcript_digest': item['transcript_digest'], 'expected_membership': item['expected_membership'], 'decision': 'approve'}
        (a.dir / 'approval.json').write_text(json.dumps(review))
        os.chmod(a.dir / 'approval.json', 0o600)
        a.cli('requests', 'approve', '--request', request, '--review-file', str(a.dir / 'approval.json'), '--json')
        wait('B join completed', lambda: b.query('operation', id=operation)['state'] == 'completed', 240)
        out['steps']['join_seconds'] = time.monotonic() - t
        want = hashlib.sha256(b'A bytes through a self-hosted service\n').hexdigest()
        t = time.monotonic()
        wait('A file on B', lambda: b.sha('from-a.txt') == want, 180)
        out['steps']['a_to_b_seconds'] = time.monotonic() - t
        data = os.urandom(8 << 20)
        b.put('from-b.bin', data)
        t = time.monotonic()
        wait('B file on A', lambda: a.sha('from-b.bin') == hashlib.sha256(data).hexdigest(), 240)
        out['steps']['b_to_a_8mib_seconds'] = time.monotonic() - t
        routes = {}
        for d in (a, b):
            status = d.query('network_status')['network']
            routes[d.name] = {'code': status['code'], 'observations': [o['route'] for o in status['observations']], 'route_usage': status.get('route_usage', [])}
        out['routes'] = routes
        if args.block_direct:
            for name, r in routes.items():
                for u in r['route_usage']:
                    for t2 in u['routes']:
                        if t2['route'] != 'relay' and int(t2['requests']) > 0:
                            raise RuntimeError(f'{name} used {t2["route"]} although direct paths are blocked')
        if args.block_direct:
            out['nft_after'] = sh('nft', 'list', 'table', 'inet', 'selfhost').stdout
        metrics = ns['svc'].run('python3', '-c', 'import urllib.request;print(urllib.request.urlopen("http://127.0.0.1:9464/metrics",timeout=5).read().decode())', check=False)
        out['service_metrics'] = [l for l in metrics.stdout.splitlines() if l and not l.startswith('#')]
        out['success'] = True
    except Exception as e:  # noqa: BLE001 - recorded, root retained
        out['error'] = str(e)
    finally:
        for d in devices.values():
            d.stop()
            d.cli('stop', check=False)
        for p in procs:
            p.send_signal(signal.SIGTERM)
            try:
                p.wait(10)
            except subprocess.TimeoutExpired:
                p.kill()
        for n in ns.values():
            n.holder.kill()
        out['finished_at'] = time.time()
        Path(args.out).write_text(json.dumps(out, indent=2))
    return 0 if out['success'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--orbit', required=True)
    parser.add_argument('--orbit-net', required=True)
    parser.add_argument('--root', required=True, help='new private directory (created, marked, removed on success)')
    parser.add_argument('--out', required=True)
    parser.add_argument('--block-direct', action='store_true')
    parser.add_argument('--netem', default='', help='tc netem arguments on device links, e.g. "delay 40ms loss 1%%"')
    parser.add_argument('--packet', default='R03')
    parser.add_argument('--keep', action='store_true')
    parser.add_argument('--inner', action='store_true', help=argparse.SUPPRESS)
    args = parser.parse_args()
    args.orbit, args.orbit_net = str(Path(args.orbit).resolve()), str(Path(args.orbit_net).resolve())
    root = Path(args.root)
    if args.inner:
        if not (root / MARKER).is_file():
            raise SystemExit('refusing: root is not the marked disposable root')
        return inner(args)
    if root.exists():
        raise SystemExit('refusing: --root must not exist')
    root.mkdir(mode=0o700, parents=False)
    (root / MARKER).write_text('R03 self-host namespace fixture; disposable\n')
    r = subprocess.run(['unshare', '-rn', sys.executable, __file__, '--inner', *sys.argv[1:]])
    if r.returncode == 0 and not args.keep and (root / MARKER).is_file():
        shutil.rmtree(root)
    return r.returncode


if __name__ == '__main__':
    sys.exit(main())
