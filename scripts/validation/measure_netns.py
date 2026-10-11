#!/usr/bin/env python3
"""R05 measurement campaign in rootless namespaces (RG4).

Each run builds one isolated user + network namespace (`unshare -rn`, no sudo,
no external route), an emulated internet in it, and one network *cell*. It
then runs production `orbit` and `orbit-net` binaries (made with `orbit-net
init`), pairs two devices once, and measures:

  reconnect  N repetitions: restart B's daemon, write a fresh 256 KiB file on
             A, time until B holds it, and record the route class B counted
  throughput one large random file and many small files, A -> B
  chunks     tail append vs. one-byte prefix insertion on a large file
             (bytes B received, from its route accounting)
  faults     receiver SIGKILL mid-file and resume; partition with concurrent
             edits, then heal (both heads on both devices); A's WAN address
             changes mid-transfer
  resources  B daemon RSS, CPU seconds and open descriptors, idle and after

Addresses: the emulated internet uses synthetic 11.77.0.0/16 (W10/W15
precedent); homes use 10.x behind nftables masquerade. Cells:

  lan          A and B on one home LAN behind one router
  eim          two homes, port-preserving masquerade (endpoint-independent
               mapping, endpoint-dependent filtering)
  symmetric    two homes, `masquerade random` (per-destination ports)
  udp_blocked  two eim homes; B's router drops UDP except to the service
  public_tcp   A on a public address; B behind eim NAT with UDP dropped

Usage:
  python3 scripts/validation/measure_netns.py --orbit BIN --orbit-net BIN \
     --cell eim --reps 10 --phases reconnect,throughput --root NEW_DIR --out result.json
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

sys.path.insert(0, str(Path(__file__).resolve().parent))
from selfhost_netns import Device, sh, wait  # noqa: E402

MARKER = '.orbit-measure-netns-disposable'
SVC = '11.77.0.2'


class Netns:
    all = []

    def __init__(self, name):
        self.name = name
        self.holder = subprocess.Popen(['unshare', '-n', 'sleep', 'infinity'])
        Netns.all.append(self)
        time.sleep(0.15)

    def run(self, *args, env=None, check=True, timeout=120, input=None):
        r = subprocess.run(['nsenter', '-t', str(self.holder.pid), '-n', *args], env=env, capture_output=True,
                           text=True, timeout=timeout, input=input)
        if check and r.returncode:
            raise RuntimeError(f'{self.name} {args[:4]} exit {r.returncode}: {r.stderr[-600:]}')
        return r

    def popen(self, args, env, log):
        return subprocess.Popen(['nsenter', '-t', str(self.holder.pid), '-n', *args], env=env, stdout=log, stderr=log)

    def nft(self, ruleset):
        return self.run('nft', '-f', '-', input=ruleset)


def link(a, a_if, a_addr, b, b_if, b_addr):
    """veth between namespace a (None = core) and b, with /24 addresses."""
    sh('ip', 'link', 'add', a_if, 'type', 'veth', 'peer', 'name', b_if)
    for ns, ifname, addr in ((a, a_if, a_addr), (b, b_if, b_addr)):
        if ns is not None:
            sh('ip', 'link', 'set', ifname, 'netns', str(ns.holder.pid))
        run = (ns.run if ns else lambda *x: sh(*x))
        run('ip', 'addr', 'add', addr + '/24', 'dev', ifname)
        run('ip', 'link', 'set', ifname, 'up')


def home(name, wan, lan_prefix, devices, nat, drop_udp=False):
    """A home router with a WAN on the core and a LAN bridge for devices."""
    router = Netns('router-' + name)
    router.run('ip', 'link', 'set', 'lo', 'up')
    link(None, 'c' + name, wan.rsplit('.', 1)[0] + '.1', router, 'w' + name, wan)
    router.run('ip', 'route', 'add', 'default', 'via', wan.rsplit('.', 1)[0] + '.1')
    router.run('sysctl', '-qw', 'net.ipv4.ip_forward=1')
    router.run('ip', 'link', 'add', 'br' + name, 'type', 'bridge')
    router.run('ip', 'addr', 'add', lan_prefix + '.1/24', 'dev', 'br' + name)
    router.run('ip', 'link', 'set', 'br' + name, 'up')
    out = {}
    for i, dev in enumerate(devices):
        ns = Netns(dev)
        ns.run('ip', 'link', 'set', 'lo', 'up')
        addr = f'{lan_prefix}.{i + 2}'
        sh('ip', 'link', 'add', 'h' + dev, 'type', 'veth', 'peer', 'name', 'p' + dev)
        sh('ip', 'link', 'set', 'h' + dev, 'netns', str(ns.holder.pid))
        sh('ip', 'link', 'set', 'p' + dev, 'netns', str(router.holder.pid))
        router.run('ip', 'link', 'set', 'p' + dev, 'master', 'br' + name)
        router.run('ip', 'link', 'set', 'p' + dev, 'up')
        ns.run('ip', 'addr', 'add', addr + '/24', 'dev', 'h' + dev)
        ns.run('ip', 'link', 'set', 'h' + dev, 'up')
        ns.run('ip', 'route', 'add', 'default', 'via', lan_prefix + '.1')
        out[dev] = ns
    masq = 'masquerade random' if nat == 'symmetric' else 'masquerade'
    udp = f'    ip daddr != {SVC} meta l4proto udp drop\n' if drop_udp else ''
    router.nft(f'''table inet home {{
  chain post {{
    type nat hook postrouting priority 100; policy accept;
    oifname "w{name}" {masq}
  }}
  chain fwd {{
    type filter hook forward priority 0; policy accept;
{udp}  }}
}}
'''.replace('chain fwd', 'chain forward_filter'))
    return router, out


def topology(cell, netem=''):
    sh('sysctl', '-qw', 'net.ipv4.ip_forward=1')
    svc = Netns('svc')
    svc.run('ip', 'link', 'set', 'lo', 'up')
    link(None, 'csvc', '11.77.0.1', svc, 'wsvc', SVC)
    svc.run('ip', 'route', 'add', 'default', 'via', '11.77.0.1')
    routers, devs = {}, {}
    if cell == 'lan':
        routers['a'], devs = home('a', '11.77.1.2', '10.1.0', ['a', 'b'], 'eim')
    elif cell in ('eim', 'symmetric', 'udp_blocked'):
        nat = 'symmetric' if cell == 'symmetric' else 'eim'
        routers['a'], d = home('a', '11.77.1.2', '10.1.0', ['a'], nat)
        devs.update(d)
        routers['b'], d = home('b', '11.77.2.2', '10.2.0', ['b'], nat, drop_udp=cell == 'udp_blocked')
        devs.update(d)
    elif cell == 'public_tcp':
        a = Netns('a')
        a.run('ip', 'link', 'set', 'lo', 'up')
        link(None, 'ca', '11.77.3.1', a, 'wa', '11.77.3.2')
        a.run('ip', 'route', 'add', 'default', 'via', '11.77.3.1')
        devs['a'] = a
        routers['b'], d = home('b', '11.77.2.2', '10.2.0', ['b'], 'eim', drop_udp=True)
        devs.update(d)
    else:
        raise SystemExit('unknown cell ' + cell)
    if netem:
        # WAN delay/loss on each home (or public device) uplink, core side.
        for ifname in ('ca', 'cb'):
            if sh('ip', 'link', 'show', ifname, check=False).returncode == 0:
                sh('tc', 'qdisc', 'add', 'dev', ifname, 'root', 'netem', *netem.split())
    return svc, routers, devs


def usage(d, peer):
    """B's counted bytes received from peer, per class (24 h window)."""
    out = {}
    for u in d.query('network_status')['network'].get('route_usage', []):
        if u['window'] == '24h' and u['device'] == peer and u['purpose'] == 'peer_data':
            for r in u['routes']:
                out[r['route']] = {'requests': int(r['requests']), 'received': int(r['bytes_received'])}
    return out


def delta(before, after):
    return {k: {f: after[k][f] - before.get(k, {}).get(f, 0) for f in ('requests', 'received')} for k in after
            if after[k]['requests'] - before.get(k, {}).get('requests', 0)}


def proc_sample(d):
    """RSS, peak RSS, CPU seconds and descriptors of B's daemon. The sampled
    process is the descendant of the supervised one with the largest RSS
    (nsenter execs; `orbit serve` may hand off to a child)."""
    ticks = os.sysconf('SC_CLK_TCK')

    def tree(pid):
        out = [pid]
        try:
            for c in Path(f'/proc/{pid}/task/{pid}/children').read_text().split():
                out += tree(int(c))
        except OSError:
            pass
        return out

    best = None
    for pid in tree(d.agent_pid() or d.proc.pid):
        try:
            status = dict(l.split(':', 1) for l in Path(f'/proc/{pid}/status').read_text().splitlines() if ':' in l)
            if 'VmRSS' not in status:
                continue
            stat = Path(f'/proc/{pid}/stat').read_text().rsplit(')', 1)[1].split()
            sample = {'pid': pid, 'name': status['Name'].strip(), 'rss_kib': int(status['VmRSS'].split()[0]),
                      'hwm_kib': int(status['VmHWM'].split()[0]), 'cpu_seconds': (int(stat[11]) + int(stat[12])) / ticks,
                      'fds': len(os.listdir(f'/proc/{pid}/fd')), 'threads': int(status['Threads'])}
        except (OSError, KeyError, ValueError):
            continue
        if best is None or sample['rss_kib'] > best['rss_kib']:
            best = sample
    return best or {'error': 'no live daemon process', 'pid': d.proc.pid}


def inner(args):
    root = Path(args.root)
    phases = set(args.phases.split(','))
    out = {'packet': 'R05', 'cell': args.cell, 'netem': args.netem, 'phases': sorted(phases), 'reps': args.reps, 'started_at': time.time(),
           'success': False, 'results': {}}
    svc, routers, devs = topology(args.cell, args.netem)
    devices, procs = {}, []
    try:
        sd = root / 'svc'
        svc.run(args.orbit_net, 'init', '--dir', str(sd), '--origin', f'https://{SVC}:8443', '--stun', f'{SVC}:3478')
        for name in ('a', 'b'):
            d = Device(name, devs[name], root, args.orbit)
            devices[name] = d
            # LAN advertising on, as a fresh setup proposes.
            d.cli('network', 'preview', '--mode', 'self_hosted', '--lan-advertising', '--profile-file', str(sd / 'profile.json'),
                  '--service-roots', str(sd / 'service-ca.pem'), '--review-file', str(d.dir / 'nr.json'), '--json')
            d.cli('network', 'apply', '--review-file', str(d.dir / 'nr.json'), '--json', timeout=60)
            d.cli('stop', check=False)
            device, pin = d.identity()
            svc.run(args.orbit_net, 'devices', 'add', '--config', str(sd / 'serve.json'), f'device={device} pin={pin}')
            d.device = device
        procs.append(svc.popen([args.orbit_net, 'serve', '--config', str(sd / 'serve.json')], os.environ, open(sd / 'serve.log', 'ab')))
        a, b = devices['a'], devices['b']
        for d in (a, b):
            d.start()
            wait(d.name + ' ready', lambda d=d: d.query('network_status')['network']['ready'], 90)
        a.data.mkdir(mode=0o700)
        a.put('seed.txt', b'seed\n')
        a.cli('setup', '--root', str(a.data), '--label', 'A', '--name', 'Measure', '--preview', '--review-file', str(a.dir / 's.json'), '--json')
        folder = json.loads(a.cli('setup', '--request-file', str(a.dir / 's.json'), '--timeout', '0', '--json').stdout)['join']['folder']
        code = a.cli('devices', 'invite', '--folder', folder, '--code', '--long').stdout.strip()
        (b.dir / 'inv.txt').write_text(code)
        os.chmod(b.dir / 'inv.txt', 0o600)
        t = time.monotonic()
        b.cli('join', '--root', str(b.data), '--label', 'B', '--name', 'Measure', '--invitation-file', str(b.dir / 'inv.txt'),
              '--preview', '--review-file', str(b.dir / 'j.json'), '--json')
        op = json.loads(b.cli('join', '--request-file', str(b.dir / 'j.json'), '--timeout', '0', '--json', check=False).stdout)['operation']['id']
        req = wait('request', lambda: b.query('operation', id=op)['join']['request'], 120)
        item = wait('A sees request', lambda: next((x for x in a.query('requests', limit='20')['requests'] if x['id'] == req), None), 120)
        (a.dir / 'ap.json').write_text(json.dumps({'request': req, 'folder': item['folder'], 'requester': item['requester'], 'key_pin': item['key_pin'],
                                                  'transcript_digest': item['transcript_digest'], 'expected_membership': item['expected_membership'], 'decision': 'approve'}))
        os.chmod(a.dir / 'ap.json', 0o600)
        a.cli('requests', 'approve', '--request', req, '--review-file', str(a.dir / 'ap.json'), '--json')
        wait('join completed', lambda: b.query('operation', id=op)['state'] == 'completed', 300)
        wait('seed on B', lambda: b.sha('seed.txt') == hashlib.sha256(b'seed\n').hexdigest(), 180)
        out['results']['join_seconds'] = time.monotonic() - t
        res = out['results']

        def transfer(name, data, seconds=600):
            before = usage(b, a.device)
            t0 = time.monotonic()
            (a.data / name).parent.mkdir(parents=True, exist_ok=True)
            a.put(name, data)
            want = hashlib.sha256(data).hexdigest()
            wait('B has ' + name, lambda: b.sha(name) == want, seconds)
            return {'seconds': time.monotonic() - t0, 'bytes': len(data), 'routes': delta(before, usage(b, a.device))}

        if 'resources' in phases:
            time.sleep(5)
            res['resources_idle_b'] = proc_sample(b)
        res['b_network_after_join'] = {k: v for k, v in b.query('network_status')['network'].items() if k in ('code', 'observations', 'probes')}
        if 'doctor' in phases:
            r = b.cli('network', 'doctor', '--device', a.device, '--json', check=False, timeout=60)
            try:
                n = json.loads(r.stdout)['network']
                res['doctor_b'] = {'probes': n.get('probes'), 'observations': n.get('observations')}
            except (ValueError, KeyError):
                res['doctor_b'] = {'returncode': r.returncode, 'stderr': r.stderr[-500:]}
            a_status = a.query('network_status')['network']
            res['a_candidates'] = [(o['lan_candidates'], o['public_candidates'], o['udp_code']) for o in a_status['observations']]
        if 'reconnect' in phases:
            reps = []
            for i in range(args.reps):
                b.stop()
                b.start()
                rep_start = usage(b, a.device)
                t0 = time.monotonic()
                r = transfer(f'r{i}.bin', os.urandom(256 << 10), 300)
                r['first_sync_since_restart_seconds'] = time.monotonic() - t0

                # Keep ordinary traffic flowing: ICE probes ride on live
                # requests and are deferred when a request ends first.
                r['upgraded_route'] = None
                deadline = time.monotonic() + args.settle
                k = 0
                while time.monotonic() < deadline:
                    transfer(f'k{i}-{k}.bin', os.urandom(args.keepalive_mib << 20), 300)
                    k += 1
                    obs = b.query('network_status')['network']['observations']
                    if obs and obs[0]['route'] in ('direct', 'quic'):
                        r['upgraded_route'] = obs[0]['route']
                        r['upgrade_since_restart_seconds'] = time.monotonic() - t0
                        break
                    time.sleep(3)
                r['keepalive_transfers'] = k
                r['settled'] = transfer(f's{i}.bin', os.urandom(256 << 10), 300)
                obs = b.query('network_status')['network']['observations']
                r['settled_observation'] = obs[0]['route'] if obs else None
                r['udp_code'] = obs[0]['udp_code'] if obs else None
                whole = delta(rep_start, usage(b, a.device))
                total = sum(v['received'] for v in whole.values()) or 1
                r['rep_routes'] = whole
                r['rep_direct_share'] = sum(v['received'] for k, v in whole.items() if k != 'relay') / total
                reps.append(r)
            res['reconnect'] = reps
        def settle_direct(tag):
            # Keep ordinary traffic flowing until a direct route is observed.
            t0 = time.monotonic()
            k = 0
            while time.monotonic() - t0 < args.settle:
                transfer(f'{tag}-{k}.bin', os.urandom(args.keepalive_mib << 20), 300)
                k += 1
                obs = b.query('network_status')['network']['observations']
                if obs and obs[0]['route'] in ('direct', 'quic'):
                    return {'route': obs[0]['route'], 'seconds': time.monotonic() - t0}
                time.sleep(3)
            return {'route': None, 'seconds': time.monotonic() - t0}

        if 'throughput' in phases or 'chunks' in phases:
            res['settle_before_bulk'] = settle_direct('settle')
        if 'throughput' in phases:
            res['large'] = [transfer(f'large{i}.bin', os.urandom(args.large_mib << 20), 900) for i in range(args.throughput_reps)]
            small = []
            for i in range(args.throughput_reps):
                before = usage(b, a.device)
                t0 = time.monotonic()
                d = a.data / f'small{i}'
                d.mkdir()
                files = {f'f{j:04d}': os.urandom(4096) for j in range(args.small_files)}
                for n, v in files.items():
                    (d / n).write_bytes(v)
                last = hashlib.sha256(files[f'f{args.small_files - 1:04d}']).hexdigest()
                wait('small files', lambda: all(b.sha(f'small{i}/{n}') == hashlib.sha256(v).hexdigest() for n, v in files.items()), 900)
                small.append({'seconds': time.monotonic() - t0, 'files': args.small_files, 'bytes': 4096 * args.small_files, 'routes': delta(before, usage(b, a.device))})
            res['small'] = small
        if 'resources' in phases:
            res['resources_after_b'] = proc_sample(b)
        if 'chunks' in phases:
            base = os.urandom(args.chunk_mib << 20)
            res['chunk_base'] = transfer('chunked.bin', base, 900)
            res['chunk_tail_append'] = transfer('chunked.bin', base + os.urandom(1 << 20), 900)
            res['chunk_prefix_insert'] = transfer('chunked.bin', b'X' + base + os.urandom(1 << 20), 900)
        if 'faults' in phases:
            # Receiver SIGKILL mid-file, then resume.
            data = os.urandom(args.large_mib << 20)
            before = usage(b, a.device)
            a.put('killed.bin', data)
            target = len(data) * 3 // 10

            def received():
                return sum(v['received'] for v in delta(before, usage(b, a.device)).values())
            wait('B receiving', lambda: received() >= target, 600)
            got_before_kill = received()
            b.proc.send_signal(signal.SIGKILL)
            b.proc.wait()
            b.start()
            # SIGKILL loses unsaved in-memory counts: take a fresh baseline.
            restarted = usage(b, a.device)
            t0 = time.monotonic()
            wait('B resumed', lambda: b.sha('killed.bin') == hashlib.sha256(data).hexdigest(), 900)
            after = sum(v['received'] for v in delta(restarted, usage(b, a.device)).values())
            res['kill_resume'] = {'file_bytes': len(data), 'received_before_kill': got_before_kill,
                                  'received_after_restart': after, 'remaining_at_kill': len(data) - got_before_kill,
                                  'resume_seconds': time.monotonic() - t0}
            # Partition: A's home router loses the internet; concurrent edits; heal.
            res['partition'] = {}
            base_v = b'shared base\n'
            transfer('shared.txt', base_v)
            r_a = routers['a']
            wan_if = 'wa'
            r_a.run('ip', 'link', 'set', wan_if, 'down')
            a.put('shared.txt', b'edit on A while partitioned\n')
            b.put('shared.txt', b'edit on B while partitioned\n')
            time.sleep(10)
            t0 = time.monotonic()
            r_a.run('ip', 'link', 'set', wan_if, 'up')
            r_a.run('ip', 'route', 'replace', 'default', 'via', '11.77.1.1')

            def converged():
                heads = {}
                for d in (a, b):
                    att = d.query('conflicts', folder=folder, limit='20').get('attention') or []
                    if not any(v['path'] == 'shared.txt' for v in att):
                        return False
                    review = d.query('content_review', folder=folder, path='shared.txt')['content_review']
                    heads[d.name] = sorted(json.dumps(h, sort_keys=True) for h in review['heads'])
                return heads if heads['a'] == heads['b'] and len(heads['a']) == 2 else False
            heads = wait('both heads on both devices', converged, 300)
            res['partition'] = {'heal_to_converged_seconds': time.monotonic() - t0, 'heads_each': 2, 'identical': True}
            # Roaming: A's router WAN address changes mid-transfer.
            data = os.urandom(32 << 20)
            before = usage(b, a.device)
            a.put('roam.bin', data)
            wait('roam start', lambda: sum(v['received'] for v in delta(before, usage(b, a.device)).values()) > 0, 300)
            r_a.run('ip', 'addr', 'add', '11.77.1.3/24', 'dev', wan_if)
            r_a.run('sysctl', '-qw', f'net.ipv4.conf.{wan_if}.promote_secondaries=1')
            r_a.run('ip', 'addr', 'del', '11.77.1.2/24', 'dev', wan_if)
            r_a.run('ip', 'route', 'replace', 'default', 'via', '11.77.1.1')
            r_a.run('conntrack', '-F', check=False)
            t0 = time.monotonic()
            wait('roam complete', lambda: b.sha('roam.bin') == hashlib.sha256(data).hexdigest(), 900)
            res['roaming'] = {'bytes': len(data), 'after_change_seconds': time.monotonic() - t0, 'routes': delta(before, usage(b, a.device))}
        out['success'] = True
    except Exception as e:  # noqa: BLE001 - recorded; root retained
        out['error'] = f'{type(e).__name__}: {e}'
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
        for n in Netns.all:
            n.holder.kill()
            n.holder.wait()
        out['finished_at'] = time.time()
        Path(args.out).write_text(json.dumps(out, indent=2))
    return 0 if out['success'] else 1


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument('--orbit', required=True)
    p.add_argument('--orbit-net', required=True)
    p.add_argument('--cell', required=True)
    p.add_argument('--phases', default='reconnect')
    p.add_argument('--reps', type=int, default=10)
    p.add_argument('--settle', type=int, default=150, help='seconds to wait for a direct upgrade after the first sync')
    p.add_argument('--throughput-reps', type=int, default=3)
    p.add_argument('--large-mib', type=int, default=128)
    p.add_argument('--small-files', type=int, default=500)
    p.add_argument('--chunk-mib', type=int, default=64)
    p.add_argument('--root', required=True)
    p.add_argument('--out', required=True)
    p.add_argument('--keep', action='store_true', help='keep the root on success')
    p.add_argument('--netem', default='', help='tc netem on home uplinks, e.g. "delay 20ms 2ms loss 0.1%%"')
    p.add_argument('--keepalive-mib', type=int, default=1, help='file size written during the settle window')
    p.add_argument('--inner', action='store_true', help=argparse.SUPPRESS)
    args = p.parse_args()
    args.orbit, args.orbit_net = str(Path(args.orbit).resolve()), str(Path(args.orbit_net).resolve())
    root = Path(args.root)
    if args.inner:
        if not (root / MARKER).is_file():
            raise SystemExit('refusing: unmarked root')
        return inner(args)
    if root.exists():
        raise SystemExit('refusing: --root must not exist')
    root.mkdir(mode=0o700)
    (root / MARKER).write_text('R05 measurement namespace fixture; disposable\n')
    r = subprocess.run(['unshare', '-rn', sys.executable, __file__, '--inner', *sys.argv[1:]])
    if r.returncode == 0 and not args.keep and (root / MARKER).is_file():
        shutil.rmtree(root)
    return r.returncode


if __name__ == '__main__':
    sys.exit(main())
