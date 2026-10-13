#!/usr/bin/env python3
"""Record a narrated terminal demo of Orbit on three namespaced devices.

Everything runs in one rootless user, network and mount namespace (`unshare -rnm`, no
sudo), like scripts/validation/scale_netns.py: an `orbit-net init` service,
a laptop alone behind one NAT home router, and a desktop and Pi sharing a
second home. Production `orbit` daemons and the real terminal UI run in each
device namespace; tmux shows them side by side above a narrator shell whose
helpers (`on`, `offline`, `online`, `crash`, `revive`) are typed on screen.
Xvfb + xterm + ffmpeg record the screen.

  python3 scripts/demo/record_demo.py --orbit bin/orbit --orbit-net bin/orbit-net \
      --root NEW_DIR --out demo.mp4
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shlex
import shutil
import signal
import subprocess
import sys
import tempfile
import time

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'validation'))
from selfhost_netns import Device, sh, wait  # noqa: E402
from measure_netns import SVC, Netns, home, link  # noqa: E402

MARKER = '.orbit-demo-netns-disposable'
W, H = 1920, 1080
COLS, ROWS = 240, 54  # IBM Plex Mono 10.5 pt cells fill 1920x1080


class Tmux:
    def __init__(self, sock):
        self.sock = str(sock)

    def __call__(self, *args, check=True):
        # -f /dev/null: no personal tmux configuration (status bars show hostnames).
        return subprocess.run(['tmux', '-f', '/dev/null', '-S', self.sock, *args], capture_output=True, text=True, check=check)

    def text(self, pane):
        return self('capture-pane', '-p', '-t', pane).stdout

    def keys(self, pane, *keys):
        self('send-keys', '-t', pane, *keys)

    def type(self, pane, text, cps=28):
        for ch in text:
            # A bare ';' ends a tmux command.
            self('send-keys', '-t', pane, '-l', '\\;' if ch == ';' else ch)
            time.sleep(1 / cps)
        time.sleep(0.4)
        self('send-keys', '-t', pane, 'Enter')
        time.sleep(0.8)

    def wait(self, pane, needle, seconds=60):
        return wait(f'{pane} shows {needle!r}', lambda: needle in self.text(pane), seconds)


def inner(args):
    root = Path(args.root)
    # Private mount namespace: a tmpfs over /home so on-screen paths read
    # /home/<device>/Trip instead of the operator's real paths.
    for b in ('orbit', 'orbit_net'):
        dst = root / 'bin' / Path(getattr(args, b)).name
        dst.parent.mkdir(exist_ok=True)
        shutil.copy2(getattr(args, b), dst)
        setattr(args, b, str(dst))
    sh('mount', '-t', 'tmpfs', '-o', 'mode=0755', 'demo-home', '/home')
    sh('sysctl', '-qw', 'net.ipv4.ip_forward=1')
    svc = Netns('svc')
    svc.run('ip', 'link', 'set', 'lo', 'up')
    link(None, 'csvc', '11.77.0.1', svc, 'wsvc', SVC)
    svc.run('ip', 'route', 'add', 'default', 'via', '11.77.0.1')
    routers, nss = {}, {}
    routers['h0'], d = home('h0', '11.77.1.2', '10.1.0', ['laptop'], 'eim')
    nss.update(d)
    routers['h1'], d = home('h1', '11.77.2.2', '10.2.0', ['desktop', 'pi'], 'eim')
    nss.update(d)
    for hn in routers:
        sh('tc', 'qdisc', 'add', 'dev', 'c' + hn, 'root', 'netem', 'delay', '20ms')
    devices, procs, recorder = {}, [], None
    # Unix socket paths are limited to 108 bytes; disposable roots are often deeper.
    sock_dir = Path(tempfile.mkdtemp(prefix='odemo-'))
    tm = Tmux(sock_dir / 's')
    try:
        sd = root / 'svc'
        svc.run(args.orbit_net, 'init', '--dir', str(sd), '--origin', f'https://{SVC}:8443', '--stun', f'{SVC}:3478')
        for name in ('laptop', 'desktop', 'pi'):
            dv = Device(name, nss[name], Path('/home'), args.orbit)
            dv.data = dv.dir / 'Trip'
            devices[name] = dv
            dv.cli('network', 'preview', '--mode', 'self_hosted', '--lan-advertising', '--profile-file', str(sd / 'profile.json'),
                   '--service-roots', str(sd / 'service-ca.pem'), '--review-file', str(dv.dir / 'nr.json'), '--json')
            dv.cli('network', 'apply', '--review-file', str(dv.dir / 'nr.json'), '--json', timeout=60)
            dv.cli('stop', check=False)
            dv.device, pin = dv.identity()
            svc.run(args.orbit_net, 'devices', 'add', '--config', str(sd / 'serve.json'), '--label', name, f'device={dv.device} pin={pin}')
        procs.append(svc.popen([args.orbit_net, 'serve', '--config', str(sd / 'serve.json')], os.environ, open(sd / 'serve.log', 'ab')))
        for dv in devices.values():
            dv.start()
            wait(dv.name + ' ready', lambda dv=dv: dv.query('network_status')['network']['ready'], 120)
        a = devices['laptop']
        a.data.mkdir(mode=0o700)
        (a.data / 'README.txt').write_text('Shared trip folder\n')
        a.cli('setup', '--root', str(a.data), '--label', 'laptop', '--name', 'Trip', '--preview', '--review-file', str(a.dir / 's.json'), '--json')
        folder = json.loads(a.cli('setup', '--request-file', str(a.dir / 's.json'), '--timeout', '0', '--json').stdout)['join']['folder']
        for name in ('desktop', 'pi'):
            b = devices[name]
            code = a.cli('devices', 'invite', '--folder', folder, '--code', '--long').stdout.strip()
            (b.dir / 'inv.txt').write_text(code)
            os.chmod(b.dir / 'inv.txt', 0o600)
            b.cli('join', '--root', str(b.data), '--label', name, '--name', 'Trip', '--invitation-file', str(b.dir / 'inv.txt'),
                  '--preview', '--review-file', str(b.dir / 'j.json'), '--json')
            op = json.loads(b.cli('join', '--request-file', str(b.dir / 'j.json'), '--timeout', '0', '--json', check=False).stdout)['operation']['id']
            req = wait('request', lambda: b.query('operation', id=op)['join']['request'], 300)
            item = wait('request seen', lambda: next((x for x in a.query('requests', limit='20')['requests'] if x['id'] == req), None), 300)
            (a.dir / 'ap.json').write_text(json.dumps({'request': req, 'folder': item['folder'], 'requester': item['requester'], 'key_pin': item['key_pin'],
                                                      'transcript_digest': item['transcript_digest'], 'expected_membership': item['expected_membership'],
                                                      'decision': 'approve'}))
            os.chmod(a.dir / 'ap.json', 0o600)
            a.cli('requests', 'approve', '--request', req, '--review-file', str(a.dir / 'ap.json'), '--json')
            wait(name + ' joined', lambda: b.query('operation', id=op)['state'] == 'completed', 600)
            wait(name + ' readme', lambda: (b.data / 'README.txt').exists(), 300)
            print('joined', name, flush=True)
        # Keep traffic flowing so direct paths can establish (ICE rides on
        # live requests), then remove the warm-up file everywhere.
        t = time.monotonic()
        while time.monotonic() - t < args.settle:
            for dv in devices.values():
                (dv.data / f'warmup-{dv.name}.bin').write_bytes(os.urandom(256 << 10))
            time.sleep(4)
        for dv in devices.values():
            (dv.data / f'warmup-{dv.name}.bin').unlink()
        wait('warm-up removed', lambda: not any(p.name.startswith('warmup-') for dv in devices.values() for p in dv.data.iterdir()), 300)
        routes = {n: [o['route'] for o in dv.query('network_status')['network']['observations']] for n, dv in devices.items()}
        print('routes after settle', routes, flush=True)

        # Narrator helpers, typed on screen.
        helpers = root / 'helpers.sh'
        lines = ['unset PROMPT_COMMAND', 'PS1="\\[\\e[1;32m\\]demo\\[\\e[0m\\]$ "', 'cd /home']
        for n, dv in devices.items():
            lines.append(f'_d_{n}={shlex.quote(str(dv.data))}')
        lines.append('on() { local d="_d_$1"; shift; (cd "${!d}" && eval "$*"); }')
        lines.append(f'offline() {{ nsenter -t {routers["h0"].holder.pid} -n ip link set wh0 down; echo "laptop home router: internet down"; }}')
        lines.append(f'online() {{ nsenter -t {routers["h0"].holder.pid} -n ip link set wh0 up; '
                     f'nsenter -t {routers["h0"].holder.pid} -n ip route replace default via 11.77.1.1; echo "laptop home router: internet back"; }}')
        lines.append(f'crash() {{ kill -9 $(cat {devices["pi"].state}/.agent.pid) && echo "pi daemon: SIGKILL"; }}')
        lines.append(f'revive() {{ touch {root}/revive-pi; echo "pi daemon: restarted"; }}')
        lines.append('clear')
        helpers.write_text('\n'.join(lines) + '\n')

        def tui(dv):
            env = ' '.join(f'{k}={shlex.quote(dv.env[k])}' for k in ('HOME', 'XDG_CONFIG_HOME', 'XDG_STATE_HOME'))
            return f'nsenter -t {dv.ns.holder.pid} -n env {env} TERM=xterm-256color {shlex.quote(args.orbit)} --state {shlex.quote(str(dv.state))}'

        tm('new-session', '-d', '-s', 'demo', '-x', str(COLS), '-y', str(ROWS), tui(devices['laptop']))
        tm('set', '-g', 'allow-rename', 'off')
        tm('set', '-g', 'allow-set-title', 'off')
        tm('set', '-g', 'pane-border-status', 'top')
        tm('set', '-g', 'pane-border-format', ' #[bold]#{pane_title} ')
        tm('set', '-g', 'status', 'off')
        tm('set', '-g', 'pane-active-border-style', 'fg=colour245')
        # Three equal device columns above a full-width narrator shell.
        tm('split-window', '-v', '-l', '13', '-t', 'demo:0.0', f'bash --noprofile --rcfile {helpers}')
        tm('split-window', '-h', '-l', '66%', '-t', 'demo:0.0', tui(devices['desktop']))
        tm('split-window', '-h', '-l', '50%', '-t', 'demo:0.1', tui(devices['pi']))
        panes = {}
        for line in tm('list-panes', '-t', 'demo', '-F', '#{pane_index} #{pane_start_command}').stdout.splitlines():
            idx, cmd = line.split(' ', 1)
            for n, dv in devices.items():
                if str(dv.state) in cmd:
                    panes[n] = f'demo:0.{idx}'
            if 'bash' in cmd:
                panes['shell'] = f'demo:0.{idx}'
        titles = {'laptop': 'laptop · home A', 'desktop': 'desktop · home B', 'pi': 'raspberry pi · home B', 'shell': 'narrator shell'}
        for n, p in panes.items():
            tm('select-pane', '-t', p, '-T', titles[n])
        for n in ('laptop', 'desktop', 'pi'):
            tm.wait(panes[n], 'README.txt', 60)

        # xterm cannot allocate a usable pty inside the user namespace, so the
        # outer process attaches to this tmux server and records the screen.
        (root / 'tmux-ready').write_text(tm.sock)
        wait('recording started', lambda: (root / 'x-recording').exists(), 60)
        recorder = True
        marks = []
        t0 = time.monotonic()

        def mark(label):
            marks.append({'t': round(time.monotonic() - t0, 2), 'label': label})
            print(f'{marks[-1]["t"]:6.1f}s {label}', flush=True)

        sh_ = panes['shell']
        say = lambda text: tm.type(sh_, f'# {text}', cps=40)  # noqa: E731
        time.sleep(2)
        mark('intro')
        say('3 devices, 2 NAT homes, 1 self-hosted relay. Production binaries in Linux network namespaces.')
        time.sleep(2.5)

        mark('write')
        say('Write a file on the laptop:')
        tm.type(sh_, "on laptop 'printf \"passport\\ncharger\\n\" > packing.txt'")
        t = time.monotonic()
        for n in ('desktop', 'pi'):
            tm.wait(panes[n], 'packing.txt', 90)
        replicated = time.monotonic() - t
        mark(f'replicated in {replicated:.1f}s')
        say(f'Replicated to desktop and pi in {replicated:.1f} s.')
        time.sleep(2.5)

        mark('partition')
        say('The laptop loses internet. Both sides edit the same file:')
        tm.type(sh_, 'offline')
        time.sleep(3)
        tm.type(sh_, "on laptop 'echo sunscreen >> packing.txt'")
        tm.type(sh_, "on desktop 'echo snacks >> packing.txt'")
        time.sleep(4)

        mark('heal')
        say('Back online. Orbit flags the concurrent edits as a conflict on every device:')
        tm.type(sh_, 'online')
        t = time.monotonic()
        heads_seen = {}

        def all_conflicted():
            for n, dv in devices.items():
                if n in heads_seen:
                    continue
                r = dv.query('content_review', folder=folder, path='packing.txt')['content_review']['heads']
                if len(r) == 2:
                    heads_seen[n] = time.monotonic() - t
            return len(heads_seen) == 3
        wait('conflict everywhere', all_conflicted, 240)
        for n in ('laptop', 'desktop', 'pi'):
            tm.keys(panes[n], '1')
        time.sleep(4)
        mark(f'conflict on all devices after {max(heads_seen.values()):.1f}s')

        mark('review')
        say('Review both versions on the desktop and pick one; the other stays in history:')
        p = panes['desktop']
        tm.keys(p, '2')
        time.sleep(1.2)
        tm.keys(p, 'Enter')
        tm.wait(p, 'Orbit Details', 20)
        time.sleep(1.2)
        tm.keys(p, 'C')
        tm.wait(p, 'packing.txt', 20)
        time.sleep(1.5)
        tm.keys(p, 'Enter')
        tm.wait(p, 'Review Versions', 20)
        time.sleep(4)
        tm.keys(p, 'Enter')
        tm.wait(p, 'Confirm Select', 20)
        time.sleep(2)
        tm.keys(p, 'Enter')
        tm.wait(p, 'completed', 60)
        time.sleep(2)
        t = time.monotonic()
        want = devices['desktop'].sha('packing.txt')
        wait('resolution everywhere', lambda: all(dv.sha('packing.txt') == want and
                                                  len(dv.query('content_review', folder=folder, path='packing.txt')['content_review']['heads']) == 1
                                                  for dv in devices.values()), 240)
        mark(f'resolved everywhere after {time.monotonic() - t:.1f}s')
        for _ in range(3):
            tm.keys(p, 'Escape')
            time.sleep(0.6)
        for n in ('laptop', 'desktop', 'pi'):
            tm.keys(panes[n], '5')
        tm.wait(p, 'Files: Trip', 20)
        tm.type(sh_, "on pi 'cat packing.txt'")
        time.sleep(3)

        mark('crash')
        say('Crash the Pi daemon (SIGKILL), write while it is down, restart it:')
        tm.type(sh_, 'crash')
        time.sleep(1)
        devices['pi'].proc.wait()
        tm.type(sh_, "on laptop 'head -c 8M /dev/urandom > photos.bin && sha256sum photos.bin | cut -c1-16'")
        time.sleep(3)
        tm.type(sh_, 'revive')
        wait('revive marker', lambda: (root / 'revive-pi').exists(), 10)
        devices['pi'].start()
        tm('respawn-pane', '-k', '-t', panes['pi'], tui(devices['pi']))
        tm('select-pane', '-t', panes['pi'], '-T', titles['pi'])
        t = time.monotonic()
        want = devices['laptop'].sha('photos.bin')
        wait('pi caught up', lambda: devices['pi'].sha('photos.bin') == want, 300)
        caught = time.monotonic() - t
        tm.keys(panes['pi'], '5')
        tm.type(sh_, "on pi 'sha256sum photos.bin | cut -c1-16'")
        mark(f'pi caught up {caught:.1f}s after restart')
        say(f'Pi caught up {caught:.1f} s after restart; hashes match.')
        time.sleep(4)
        mark('end')
        stop_recording(root)
        recorder = None
        (root / 'marks.json').write_text(json.dumps(marks, indent=2))
        shutil.copy(root / 'raw.mp4', args.out)
        Path(args.out).with_suffix('.json').write_text(json.dumps({'marks': marks, 'orbit': args.orbit_version}, indent=2))
        return 0
    finally:
        if recorder:
            stop_recording(root)
        tm('kill-server', check=False)
        shutil.rmtree(sock_dir, ignore_errors=True)
        for dv in devices.values():
            dv.stop()
            dv.cli('stop', check=False)
        for p in procs:
            p.send_signal(signal.SIGTERM)
            try:
                p.wait(10)
            except subprocess.TimeoutExpired:
                p.kill()
        for n in Netns.all:
            n.holder.kill()
            n.holder.wait()


def stop_recording(root):
    (root / 'x-stop').touch()
    wait('recording stopped', lambda: (root / 'x-stopped').exists(), 60)


def record(root, inner_proc):
    """Outer side: attach xterm to the namespaced tmux server and record."""
    while not (root / 'tmux-ready').exists():
        if inner_proc.poll() is not None:
            return
        time.sleep(0.5)
    sock = (root / 'tmux-ready').read_text()
    env = {**os.environ, 'DISPLAY': ':77'}
    procs = [subprocess.Popen(['Xvfb', ':77', '-screen', '0', f'{W}x{H}x24', '-nolisten', 'tcp'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)]
    time.sleep(1)
    procs.append(subprocess.Popen(['xterm', '-geometry', f'{COLS}x{ROWS}+0+0', '-fa', 'IBM Plex Mono', '-fs', '10.5', '-bg', '#111820', '-fg', '#e8e8e8',
                                   '-b', '0', '+sb', '-e', 'tmux', '-f', '/dev/null', '-S', sock, 'attach', '-t', 'demo'], env=env))
    time.sleep(2)
    ffmpeg = subprocess.Popen(['ffmpeg', '-y', '-loglevel', 'error', '-f', 'x11grab', '-video_size', f'{W}x{H}', '-framerate', '30',
                               '-i', ':77', '-c:v', 'libx264', '-preset', 'veryfast', '-crf', '20', '-pix_fmt', 'yuv420p',
                               str(root / 'raw.mp4')], env=env, stdin=subprocess.PIPE)
    subprocess.run(['xdotool', 'mousemove', str(W - 1), str(H - 1)], env=env, check=False)
    (root / 'x-recording').touch()
    while not (root / 'x-stop').exists() and inner_proc.poll() is None:
        time.sleep(0.2)
    ffmpeg.stdin.write(b'q')
    ffmpeg.stdin.flush()
    ffmpeg.wait(60)
    for p in reversed(procs):
        p.terminate()
        p.wait(10)
    (root / 'x-stopped').touch()


def main():
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument('--orbit', required=True)
    p.add_argument('--orbit-net', required=True)
    p.add_argument('--root', required=True)
    p.add_argument('--out', required=True)
    p.add_argument('--settle', type=int, default=120, help='seconds to idle after joins, before recording')
    p.add_argument('--keep', action='store_true')
    p.add_argument('--inner', action='store_true', help=argparse.SUPPRESS)
    args = p.parse_args()
    args.orbit, args.orbit_net = str(Path(args.orbit).resolve()), str(Path(args.orbit_net).resolve())
    args.out = str(Path(args.out).resolve())
    args.orbit_version = subprocess.run([args.orbit, 'version'], capture_output=True, text=True).stdout.splitlines()[:2]
    root = Path(args.root)
    if args.inner:
        if not (root / MARKER).is_file():
            raise SystemExit('refusing: unmarked root')
        return inner(args)
    if root.exists():
        raise SystemExit('refusing: --root must not exist')
    root.mkdir(mode=0o700)
    (root / MARKER).write_text('Demo recording namespace fixture; disposable\n')
    r = subprocess.Popen(['unshare', '-rnm', sys.executable, __file__, '--inner', *sys.argv[1:]])
    record(root, r)
    r.wait()
    if r.returncode == 0 and not args.keep and (root / MARKER).is_file():
        shutil.rmtree(root)
    return r.returncode


if __name__ == '__main__':
    sys.exit(main())
