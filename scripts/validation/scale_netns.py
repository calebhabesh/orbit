#!/usr/bin/env python3
"""Scale-out and churn soak in rootless namespaces.

Extends the R05 fixture (measure_netns.py, selfhost_netns.py) from two
devices to N. One isolated user + network namespace (`unshare -rn`, no sudo)
holds an emulated internet (synthetic 11.77.0.0/16), an `orbit-net init`
service, and `--per-home` devices behind each nftables masquerade home router,
with `tc netem` on every home uplink. Device d00 creates the Orbit and invites
and approves every other device, one at a time. Phases:

  join        seconds per sequential join (invite -> approve -> seed present)
  fanout      R repetitions: a random device writes a 64 KiB file; arrival
              time at every other device (polled on disk)
  concurrent  every device writes its own 64 KiB file at once; time until all
              N x N copies exist
  soak        for --soak seconds, a random event every 1-4 s: new file,
              edit of the writer's own file, delete, SIGKILL + delayed restart
              of a device, home internet outage (router WAN down), concurrent
              edits of a shared "contested" file on two devices. Then heal
              everything and measure time until every device holds exactly the
              expected tree and identical heads for every contested path
              (docs/protocol.md section 9, conditional convergence)
  resources   RSS, CPU seconds, descriptors of every daemon and orbit-net

Results JSON includes every event and every (file, device) arrival time.

  python3 scripts/validation/scale_netns.py --orbit bin/orbit --orbit-net bin/orbit-net \
      --devices 10 --soak 1800 --root NEW_DIR --out result.json
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import random
import shutil
import signal
import subprocess
import sys
import threading
import time

sys.path.insert(0, str(Path(__file__).resolve().parent))
from selfhost_netns import Device, sh, wait  # noqa: E402
from measure_netns import SVC, Netns, home, link, proc_sample  # noqa: E402

MARKER = '.orbit-scale-netns-disposable'
SMALL = 64 << 10


class Tracker:
    """Polls device folders for expected files; records first-seen times."""

    def __init__(self, devices):
        self.devices = devices
        self.pending = {}  # (rel, device) -> sha256 hex
        self.written = {}  # rel -> (writer, t, sha)
        self.arrivals = {}  # rel -> {device: seconds after write}
        self.lock = threading.Lock()
        self.stop = threading.Event()
        self.thread = threading.Thread(target=self.loop, daemon=True)
        self.thread.start()

    def expect(self, rel, writer, data, t):
        sha = hashlib.sha256(data).hexdigest()
        with self.lock:
            self.written[rel] = (writer, t, sha)
            self.arrivals[rel] = {writer: 0.0}
            for name in self.devices:
                if name != writer:
                    self.pending[(rel, name)] = sha

    def forget(self, rel):
        with self.lock:
            for k in [k for k in self.pending if k[0] == rel]:
                del self.pending[k]

    def loop(self):
        while not self.stop.is_set():
            with self.lock:
                items = list(self.pending.items())
            now = time.monotonic()
            for (rel, name), sha in items:
                p = self.devices[name].data / rel
                try:
                    if p.stat().st_size and hashlib.sha256(p.read_bytes()).hexdigest() == sha:
                        with self.lock:
                            if self.pending.pop((rel, name), None) is not None:
                                self.arrivals[rel][name] = now - self.written[rel][1]
                except OSError:
                    pass
            time.sleep(0.25)

    def done(self, rels):
        with self.lock:
            return not any(k[0] in rels for k in self.pending)


def put(d, rel, data):
    """Atomic write: the scanner never sees a partial file."""
    p = d.data / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    tmp = d.dir / ('tmp-' + os.urandom(6).hex())
    tmp.write_bytes(data)
    os.replace(tmp, p)
    return time.monotonic()


def tree(d, skip):
    out = {}
    for p in sorted(d.data.rglob('*')):
        rel = str(p.relative_to(d.data))
        if p.is_file() and not rel.startswith('.') and not any(rel.startswith(s) for s in skip):
            out[rel] = hashlib.sha256(p.read_bytes()).hexdigest()
    return out


def stats(xs):
    xs = sorted(xs)
    if not xs:
        return None
    q = lambda f: xs[min(len(xs) - 1, int(round(f * (len(xs) - 1))))]  # noqa: E731
    return {'n': len(xs), 'min': xs[0], 'p50': q(0.5), 'p95': q(0.95), 'max': xs[-1]}


def inner(args):
    root = Path(args.root)
    rng = random.Random(args.seed)
    names = [f'd{i:02d}' for i in range(args.devices)]
    homes = [names[i:i + args.per_home] for i in range(0, len(names), args.per_home)]
    out = {'packet': 'scale', 'devices': args.devices, 'per_home': args.per_home, 'homes': len(homes), 'netem': args.netem,
           'seed': args.seed, 'started_at': time.time(), 'success': False, 'results': {}}
    res = out['results']
    sh('sysctl', '-qw', 'net.ipv4.ip_forward=1')
    svc = Netns('svc')
    svc.run('ip', 'link', 'set', 'lo', 'up')
    link(None, 'csvc', '11.77.0.1', svc, 'wsvc', SVC)
    svc.run('ip', 'route', 'add', 'default', 'via', '11.77.0.1')
    routers, nss, home_of = {}, {}, {}
    for h, members in enumerate(homes):
        hn = f'h{h}'
        routers[hn], d = home(hn, f'11.77.{h + 1}.2', f'10.{h + 1}.0', members, 'eim')
        nss.update(d)
        for m in members:
            home_of[m] = hn
        if args.netem:
            sh('tc', 'qdisc', 'add', 'dev', 'c' + hn, 'root', 'netem', *args.netem.split())
    devices, procs = {}, []
    tracker = None
    try:
        sd = root / 'svc'
        svc.run(args.orbit_net, 'init', '--dir', str(sd), '--origin', f'https://{SVC}:8443', '--stun', f'{SVC}:3478')
        for name in names:
            d = Device(name, nss[name], root, args.orbit)
            devices[name] = d
            d.cli('network', 'preview', '--mode', 'self_hosted', '--lan-advertising', '--profile-file', str(sd / 'profile.json'),
                  '--service-roots', str(sd / 'service-ca.pem'), '--review-file', str(d.dir / 'nr.json'), '--json')
            d.cli('network', 'apply', '--review-file', str(d.dir / 'nr.json'), '--json', timeout=60)
            d.cli('stop', check=False)
            d.device, pin = d.identity()
            svc.run(args.orbit_net, 'devices', 'add', '--config', str(sd / 'serve.json'), '--label', name, f'device={d.device} pin={pin}')
        procs.append(svc.popen([args.orbit_net, 'serve', '--config', str(sd / 'serve.json')], os.environ, open(sd / 'serve.log', 'ab')))
        for d in devices.values():
            d.start()
        for d in devices.values():
            wait(d.name + ' ready', lambda d=d: d.query('network_status')['network']['ready'], 120)

        # Join: d00 creates; everyone else joins sequentially.
        a = devices[names[0]]
        a.data.mkdir(mode=0o700)
        put(a, 'seed.txt', b'seed\n')
        put(a, 'contested/c.txt', b'contested base\n')
        a.cli('setup', '--root', str(a.data), '--label', a.name, '--name', 'Scale', '--preview', '--review-file', str(a.dir / 's.json'), '--json')
        folder = json.loads(a.cli('setup', '--request-file', str(a.dir / 's.json'), '--timeout', '0', '--json').stdout)['join']['folder']
        seed = hashlib.sha256(b'seed\n').hexdigest()
        joins = []
        for name in names[1:]:
            b = devices[name]
            t = time.monotonic()
            code = a.cli('devices', 'invite', '--folder', folder, '--code', '--long').stdout.strip()
            (b.dir / 'inv.txt').write_text(code)
            os.chmod(b.dir / 'inv.txt', 0o600)
            b.cli('join', '--root', str(b.data), '--label', name, '--name', 'Scale', '--invitation-file', str(b.dir / 'inv.txt'),
                  '--preview', '--review-file', str(b.dir / 'j.json'), '--json')
            op = json.loads(b.cli('join', '--request-file', str(b.dir / 'j.json'), '--timeout', '0', '--json', check=False).stdout)['operation']['id']
            req = wait('request', lambda: b.query('operation', id=op)['join']['request'], 180)
            steps = {'request': time.monotonic() - t}
            item = wait('d00 sees request', lambda: next((x for x in a.query('requests', limit='50')['requests'] if x['id'] == req), None), 180)
            (a.dir / 'ap.json').write_text(json.dumps({'request': req, 'folder': item['folder'], 'requester': item['requester'], 'key_pin': item['key_pin'],
                                                      'transcript_digest': item['transcript_digest'], 'expected_membership': item['expected_membership'],
                                                      'decision': 'approve'}))
            os.chmod(a.dir / 'ap.json', 0o600)
            steps['seen_by_d00'] = time.monotonic() - t
            a.cli('requests', 'approve', '--request', req, '--review-file', str(a.dir / 'ap.json'), '--json')
            steps['approved'] = time.monotonic() - t
            phases = []

            def completed():
                o = b.query('operation', id=op)
                key = (o.get('state'), o.get('phase'))
                if not phases or phases[-1][1:] != list(key):
                    phases.append([round(time.monotonic() - t, 2), *key])
                return o['state'] == 'completed'
            wait(name + ' join completed', completed, 600)
            steps['completed'] = time.monotonic() - t
            steps['phases'] = phases
            wait(name + ' seed', lambda: b.sha('seed.txt') == seed, 600)
            joins.append({'device': name, 'members_before': len(joins) + 1, 'seconds': time.monotonic() - t, 'steps': steps})
            print(f'joined {name} in {joins[-1]["seconds"]:.1f}s {steps}', flush=True)
        res['join'] = joins
        res['service_metrics'] = {'after_join': svc_metrics(svc)}
        res['join_total_seconds'] = sum(j['seconds'] for j in joins)
        # Every device holds the contested base before measuring.
        base = hashlib.sha256(b'contested base\n').hexdigest()
        wait('contested base everywhere', lambda: all(d.sha('contested/c.txt') == base for d in devices.values()), 600)

        tracker = Tracker(devices)
        if 'fanout' in args.phases:
            reps = []
            for i in range(args.fanout_reps):
                w = devices[rng.choice(names)]
                rel = f'fanout/{i:03d}.bin'
                data = os.urandom(SMALL)
                tracker.expect(rel, w.name, data, put(w, rel, data))
                wait(rel + ' everywhere', lambda: tracker.done({rel}), 600)
                arr = {k: v for k, v in tracker.arrivals[rel].items() if k != w.name}
                reps.append({'writer': w.name, 'arrivals': arr, 'all_seconds': max(arr.values()),
                             'same_home_max': max([v for k, v in arr.items() if home_of[k] == home_of[w.name]], default=None),
                             'other_home_max': max([v for k, v in arr.items() if home_of[k] != home_of[w.name]], default=None)})
                time.sleep(1)
            res['fanout'] = reps
            res['fanout_all_seconds'] = stats([r['all_seconds'] for r in reps])
            res['fanout_per_device_seconds'] = stats([v for r in reps for v in r['arrivals'].values()])
            res['service_metrics']['after_fanout'] = svc_metrics(svc)
            print('fanout', res['fanout_all_seconds'], flush=True)
        if 'concurrent' in args.phases:
            reps = []
            for i in range(args.concurrent_reps):
                rels = set()
                t0 = time.monotonic()
                for d in devices.values():
                    rel = f'concurrent/{i}/{d.name}.bin'
                    data = os.urandom(SMALL)
                    tracker.expect(rel, d.name, data, put(d, rel, data))
                    rels.add(rel)
                wait('concurrent everywhere', lambda: tracker.done(rels), 900)
                reps.append({'copies': len(rels) * (len(names) - 1), 'seconds': time.monotonic() - t0})
                time.sleep(2)
            res['concurrent'] = reps
            res['service_metrics']['after_concurrent'] = svc_metrics(svc)
            print('concurrent', reps, flush=True)
        if 'resources' in args.phases:
            res['resources_before_soak'] = {n: proc_sample(d) for n, d in devices.items()}
        if args.soak:
            res['soak'] = soak(args, rng, devices, routers, home_of, tracker, folder, svc)
            res['service_metrics']['after_soak'] = svc_metrics(svc)
            print('soak', {k: v for k, v in res['soak'].items() if k not in ('events', 'writes')}, flush=True)
        if 'resources' in args.phases:
            res['resources_after'] = {n: proc_sample(d) for n, d in devices.items()}
            res['resources_orbit_net'] = svc_sample(procs[0])
        out['success'] = res.get('soak', {}).get('converged', True)
    except Exception as e:  # noqa: BLE001 - recorded; root retained
        out['error'] = f'{type(e).__name__}: {e}'
        print(out['error'], flush=True)
    finally:
        if tracker:
            tracker.stop.set()
        for d in devices.values():
            d.stop()
            d.cli('stop', check=False)
        for p in procs:
            p.send_signal(signal.SIGTERM)
            try:
                p.wait(10)
            except subprocess.TimeoutExpired:
                p.kill()
        for n in Netns.all:
            n.holder.kill()
            n.holder.wait()
        out['finished_at'] = time.time()
        Path(args.out).write_text(json.dumps(out, indent=2))
    return 0 if out['success'] else 1


def svc_metrics(svc):
    """orbit-net counters (refusals by reason, relay bytes/sessions)."""
    r = svc.run('python3', '-c', 'import urllib.request;print(urllib.request.urlopen("http://127.0.0.1:9464/metrics",timeout=5).read().decode())',
                check=False, timeout=20)
    out = {}
    for line in r.stdout.splitlines():
        if line.startswith('orbit_net_') and ' ' in line:
            k, v = line.rsplit(' ', 1)
            try:
                out[k] = float(v)
            except ValueError:
                pass
    return out


def svc_sample(p):
    try:
        s = dict(l.split(':', 1) for l in Path(f'/proc/{p.pid}/status').read_text().splitlines() if ':' in l)
        return {'rss_kib': int(s['VmRSS'].split()[0]), 'hwm_kib': int(s['VmHWM'].split()[0])}
    except (OSError, KeyError):
        return None


def soak(args, rng, devices, routers, home_of, tracker, folder, svc):
    names = list(devices)
    t_start = time.monotonic()
    events, owned, deleted = [], {n: [] for n in names}, set()
    down = {}  # device -> restart at
    cut = {}  # home -> heal at
    seq = 0

    def log(kind, **kw):
        events.append({'t': time.monotonic() - t_start, 'kind': kind, **kw})

    def impaired():
        return set(down) | {n for n in names if home_of[n] in cut}

    def heal_router(hn):
        r = routers[hn]
        r.run('ip', 'link', 'set', 'w' + hn, 'up')
        r.run('ip', 'route', 'replace', 'default', 'via', f'11.77.{int(hn[1:]) + 1}.1')

    limit = max(1, len(names) // 3)
    deadline = t_start + args.soak
    while time.monotonic() < deadline:
        now = time.monotonic()
        for n, at in list(down.items()):
            if now >= at:
                devices[n].start()
                del down[n]
                log('restart', device=n)
        for hn, at in list(cut.items()):
            if now >= at:
                heal_router(hn)
                del cut[hn]
                log('heal', home=hn)
        up = [n for n in names if n not in down]
        roll = rng.random()
        if roll < 0.55:
            w = rng.choice(up)
            rel = f'soak/{w}/{seq:05d}.bin'
            seq += 1
            data = os.urandom(rng.choice((4 << 10, SMALL, 1 << 20)))
            tracker.expect(rel, w, data, put(devices[w], rel, data))
            owned[w].append(rel)
            log('write', device=w, path=rel, bytes=len(data))
        elif roll < 0.70:
            w = rng.choice(up)
            if owned[w]:
                rel = rng.choice(owned[w])
                tracker.forget(rel)
                data = os.urandom(SMALL)
                tracker.expect(rel, w, data, put(devices[w], rel, data))
                log('edit', device=w, path=rel)
        elif roll < 0.76:
            w = rng.choice(up)
            if owned[w]:
                rel = owned[w].pop(rng.randrange(len(owned[w])))
                tracker.forget(rel)
                (devices[w].data / rel).unlink()
                deleted.add(rel)
                log('delete', device=w, path=rel)
        elif roll < 0.86:
            cands = [n for n in up if n != names[0] and n not in impaired()]
            if cands and len(impaired()) < limit:
                n = rng.choice(cands)
                devices[n].proc.send_signal(signal.SIGKILL)
                devices[n].proc.wait()
                down[n] = time.monotonic() + rng.uniform(5, 30)
                log('kill', device=n)
        elif roll < 0.93:
            hs = sorted({home_of[n] for n in names if n != names[0]} - set(cut))
            if hs and len(impaired()) < limit:
                hn = rng.choice(hs)
                routers[hn].run('ip', 'link', 'set', 'w' + hn, 'down')
                cut[hn] = time.monotonic() + rng.uniform(10, 45)
                log('partition', home=hn)
        else:
            pair = rng.sample(up, 2) if len(up) >= 2 else []
            if pair:
                for n in pair:
                    put(devices[n], 'contested/c.txt', f'edit by {n} at {seq}\n'.encode())
                seq += 1
                log('contested_edit', devices=pair)
        time.sleep(rng.uniform(1, 4))

    # Heal everything, then wait for convergence.
    for hn in list(cut):
        heal_router(hn)
        log('heal', home=hn)
    for n in list(down):
        devices[n].start()
        log('restart', device=n)
    t_heal = time.monotonic()
    log('heal_all')
    expected = {}
    for rel, (w, _, sha) in tracker.written.items():
        if rel.startswith('soak/') and rel not in deleted:
            expected[rel] = sha

    def contested_heads():
        heads = {}
        for d in devices.values():
            r = d.query('content_review', folder=folder, path='contested/c.txt')
            heads[d.name] = sorted(json.dumps(h, sort_keys=True) for h in r['content_review']['heads'])
        return heads

    last = {}

    def converged():
        for d in devices.values():
            t = {k: v for k, v in tree(d, ('contested/', 'fanout/', 'concurrent/', 'seed.txt')).items()}
            if t != expected:
                missing = len(set(expected) - set(t))
                extra = len(set(t) - set(expected))
                wrong = sum(1 for k in expected if k in t and t[k] != expected[k])
                last['tree'] = {'device': d.name, 'missing': missing, 'extra': extra, 'wrong': wrong}
                return False
        heads = contested_heads()
        vals = list(heads.values())
        if any(v != vals[0] for v in vals):
            last['heads'] = {k: len(v) for k, v in heads.items()}
            return False
        last['heads'] = {k: len(v) for k, v in heads.items()}
        return True

    result = {'seconds': args.soak, 'events': events, 'converged': False}
    try:
        wait('soak convergence', converged, args.converge_timeout)
        result['converged'] = True
    except RuntimeError as e:
        result['error'] = str(e)
        result['last_difference'] = last
    result['heal_to_converged_seconds'] = time.monotonic() - t_heal
    result['contested_heads'] = last.get('heads')
    result['expected_files'] = len(expected)
    result['deleted_files'] = len(deleted)
    counts = {}
    for e in events:
        counts[e['kind']] = counts.get(e['kind'], 0) + 1
    result['event_counts'] = counts
    # Per-write replication: time until every device held it (None if a later
    # edit or delete superseded it first).
    writes = []
    for e in events:
        if e['kind'] in ('write', 'edit'):
            arr = tracker.arrivals.get(e['path'], {})
            complete = len(arr) == len(names)
            writes.append({'t': e['t'], 'path': e['path'], 'device': e['device'],
                           'all_seconds': max(arr.values()) if complete else None})
    result['writes'] = writes
    result['write_all_seconds'] = stats([w['all_seconds'] for w in writes if w['all_seconds'] is not None])
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument('--orbit', required=True)
    p.add_argument('--orbit-net', required=True)
    p.add_argument('--devices', type=int, default=5)
    p.add_argument('--per-home', type=int, default=2)
    p.add_argument('--phases', default='fanout,concurrent,resources')
    p.add_argument('--fanout-reps', type=int, default=20)
    p.add_argument('--concurrent-reps', type=int, default=3)
    p.add_argument('--soak', type=int, default=0, help='churn soak seconds (0 = skip)')
    p.add_argument('--converge-timeout', type=int, default=900)
    p.add_argument('--seed', type=int, default=1)
    p.add_argument('--netem', default='delay 20ms', help='tc netem on every home uplink')
    p.add_argument('--root', required=True)
    p.add_argument('--out', required=True)
    p.add_argument('--keep', action='store_true')
    p.add_argument('--inner', action='store_true', help=argparse.SUPPRESS)
    args = p.parse_args()
    args.phases = set(args.phases.split(','))
    args.orbit, args.orbit_net = str(Path(args.orbit).resolve()), str(Path(args.orbit_net).resolve())
    root = Path(args.root)
    if args.inner:
        if not (root / MARKER).is_file():
            raise SystemExit('refusing: unmarked root')
        return inner(args)
    if root.exists():
        raise SystemExit('refusing: --root must not exist')
    root.mkdir(mode=0o700)
    (root / MARKER).write_text('Scale/churn namespace fixture; disposable\n')
    r = subprocess.run(['unshare', '-rn', sys.executable, __file__, '--inner', *sys.argv[1:]])
    if r.returncode == 0 and not args.keep and (root / MARKER).is_file():
        shutil.rmtree(root)
    return r.returncode


if __name__ == '__main__':
    sys.exit(main())
