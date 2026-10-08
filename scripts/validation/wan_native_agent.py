"""W16 worker additions; no firewall, routing, forwarding or service mutations."""
import ipaddress
import hashlib
import json
import os
from pathlib import Path
import socket
import shutil
import re
import subprocess
import sys


def wan_inventory(targets):
    # Addresses are observations/probe targets, never Orbit connection settings.
    targets = sorted({str(ipaddress.ip_address(v)) for v in targets})
    if any(ipaddress.ip_address(v).is_unspecified or ipaddress.ip_address(v).is_loopback
           for v in targets):
        raise RuntimeError('WAN route probe requires nonloopback numeric targets')
    def ip(*args):
        return json.loads(subprocess.check_output(['ip', '-j', *args], text=True, timeout=10))
    links = ip('-d', 'link')
    # Store only relevant network facts, without MACs or unrelated container detail.
    links = [{k: v[k] for k in ('ifname', 'flags', 'operstate', 'link_type', 'mtu') if k in v}
             | {'kind': v.get('linkinfo', {}).get('info_kind', '')} for v in links]
    return {'hostname': socket.gethostname(), 'arch': os.uname().machine,
            'kernel': os.uname().release, 'links': links,
            'addresses': [{'ifname': v['ifname'], 'addresses': [
                {k: a[k] for k in ('family', 'local', 'prefixlen', 'scope') if k in a}
                for a in v.get('addr_info', [])]} for v in ip('address')], 'rules': ip('rule'),
            'routes': {v: ip('route', 'get', v) for v in targets}}


def daemon_resources(root):
    """Read-only /proc sample of this root's launcher-started daemon."""
    pidfile = root / 'state' / '.agent.pid'
    if not pidfile.exists():
        return {'running': False}
    pid = int(pidfile.read_text().strip())
    proc = Path('/proc') / str(pid)
    try:
        argv = (proc / 'cmdline').read_bytes().split(b'\0')
        if os.fsencode(root / 'orbit') not in argv:
            raise RuntimeError('daemon ownership mismatch')
        status = dict(line.split(':', 1) for line in (proc / 'status').read_text().splitlines() if ':' in line)
        stat = (proc / 'stat').read_text()
        fields = stat[stat.rfind(')') + 2:].split()
        ticks = os.sysconf('SC_CLK_TCK')
        uptime = float(Path('/proc/uptime').read_text().split()[0])
        fds = len(list((proc / 'fd').iterdir()))
    except FileNotFoundError:
        return {'running': False}
    kib = lambda key: int(status[key].split()[0])
    state_bytes = sum(p.lstat().st_size for p in (root / 'state').rglob('*') if p.is_file() and not p.is_symlink())
    return {'running': True, 'rss_kib': kib('VmRSS'), 'peak_rss_kib': kib('VmHWM'), 'threads': int(status['Threads']),
            'fds': fds, 'user_seconds': int(fields[11]) / ticks, 'system_seconds': int(fields[12]) / ticks,
            'age_seconds': round(uptime - int(fields[19]) / ticks, 1), 'state_bytes': state_bytes}


def wan_dispatch(req, base):
    if req['action'] == 'wan-inventory':
        return wan_inventory(req['targets'])
    if req['action'] == 'wan-netdev':
        # Namespace-wide interface totals (readable without privileges).
        rows = [line.split() for line in Path('/proc/net/dev').read_text().splitlines()[2:]]
        return {r[0].rstrip(':'): {'rx_bytes': int(r[1]), 'tx_bytes': int(r[9])} for r in rows if r[0] != 'lo:'}
    if req['action'] == 'wan-netns-snapshot':
        # Saved by the owner's one sudo command that started the namespace.
        return {'text': (Path.home() / 'orbit-w16' / 'netns-snapshot.txt').read_text()}
    # Child commands are hermetic unless explicitly running the hosted journey.
    # Never inherit a test shell's disable switch or an HTTP/SOCKS proxy.
    for key in list(os.environ):
        if key.lower() in ('http_proxy', 'https_proxy', 'all_proxy', 'no_proxy'):
            os.environ.pop(key)
    os.environ['ORBIT_DISABLE_PACKAGED_PROFILE'] = '1' if req.get('rehearsal') else '0'
    if req['action'] not in ('inventory', 'create'):
        root = base.__globals__['validated_root'](req)
        if root.parent != Path.home().resolve() or not root.name.startswith('orbit-validation-'):
            raise RuntimeError('W16 requires a freshly allocated validation root')
        if (root / '.orbit-pilot').exists():
            raise RuntimeError('personal pilot refused')
        if (root / '.orbit-disposable').lstat().st_mode & 0o077:
            raise RuntimeError('disposable marker must be private')
        if req['action'] == 'wan-identity':
            # Hash only the public SPKI. Never send private PEM outside the host
            # or into an OpenSSL subprocess; running daemons own the state lock.
            identity = base.__globals__['beneath'](root, 'state/identity/peer-identity.pem').read_bytes()
            cert = re.search(rb'-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----', identity, re.S)
            if cert is None:
                raise RuntimeError('public identity certificate missing')
            public = subprocess.run(['openssl', 'x509', '-pubkey', '-noout'], input=cert[0],
                                    capture_output=True, check=True, timeout=10).stdout
            spki = subprocess.run(['openssl', 'pkey', '-pubin', '-outform', 'DER'], input=public,
                                  capture_output=True, check=True, timeout=10).stdout
            return {'pin': hashlib.sha256(spki).hexdigest()}
        if req['action'] == 'wan-tui':
            # One keyboard phase on a real PTY; the script reports sanitized JSON.
            args = [sys.executable, str(root / 'wan_tui_phase.py'), '--root', str(root), '--token', req['token'],
                    '--phase', req['phase'], *req.get('options', [])]
            result = subprocess.run(args, capture_output=True, text=True, timeout=300, cwd=root)
            lines = result.stdout.strip().splitlines()
            report = json.loads(lines[-1]) if lines else {'error': 'no phase report'}
            report['returncode'] = result.returncode
            return report
        if req['action'] == 'wan-resources':
            return daemon_resources(root)
        if req['action'] == 'wan-clean':
            # Refuse removal while any current process references the private
            # executable/state. Never signal a PID discovered by a global scan.
            for entry in Path('/proc').iterdir():
                if not entry.name.isdigit():
                    continue
                try:
                    argv = (entry / 'cmdline').read_bytes().split(b'\0')
                except (FileNotFoundError, PermissionError, ProcessLookupError):
                    continue
                if os.fsencode(root / 'orbit') in argv or os.fsencode(root / 'state') in argv or b'--state=' + os.fsencode(root / 'state') in argv:
                    raise RuntimeError('owned process still references root; removal refused')
            shutil.rmtree(root)
            return {'removed': True}
    return base(req)
