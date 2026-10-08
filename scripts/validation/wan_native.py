#!/usr/bin/env python3
"""W16 packaged CLI journey and read-only native route preflight.

SSH administers marked roots only, with forwarding and shared masters disabled.
Hosted execution requires closed WG6. Local self-host rehearsals never establish
physical WAN or ordinary bundled-profile acceptance.
"""
import argparse
import base64
import datetime
import hashlib
import json
from pathlib import Path
import re
import shlex
import subprocess
import time

from harness import AGENT, prepare_output
from terminal_native import TerminalNode, approve, package_binary, wait

ROOT = Path(__file__).resolve().parents[2]
ADDITIONS = Path(__file__).with_name('wan_native_agent.py').read_text()
# Allow the daemon's bounded graceful shutdown to finish. Keep CLI commands
# shorter than the supervising SSH deadline; no production timing is changed.
HOST_AGENT = AGENT.replace('for _ in range(100):', 'for _ in range(300):').replace(
    'communicate(timeout=1800)', 'communicate(timeout=120)')
# Reuse the existing root/path/PID guards without executing its stdin entrypoint.
WORKER = HOST_AGENT.split('\nif __name__ == "__main__":')[0] + '\n' + ADDITIONS + '''
request = json.load(sys.stdin)
try:
    print(json.dumps(wan_dispatch(request, dispatch)))
except Exception:
    # A failed CLI can contain invitation capabilities: never echo raw output.
    print(json.dumps({"error": "W16 worker failed; private run root retained"}))
    sys.exit(1)
'''


def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def check_wg6(text):
    # Fail closed against the authoritative gate section, not a user-supplied flag.
    section = text.split('### WG6 W13 outcome', 1)[-1].split('\n### ', 1)[0]
    if not re.search(r'^### WG6 W13 outcome — closed\b', text, re.M) or re.search(r'WG6 (?:stays|remains) open', section):
        raise RuntimeError('WG6 remains open: authority backup and received alert evidence required')


NETNS = re.compile(r'orbit-([a-z0-9-]{1,10})')
SSH_OPTIONS = ['-T', '-o', 'BatchMode=yes', '-o', 'ConnectTimeout=8', '-o', 'ClearAllForwardings=yes',
               '-o', 'ControlMaster=no', '-o', 'ControlPath=none']


def parse_isolation(rules, uplink_links):
    """Summarize the tagged wan_netns.sh rules (iptables -S -v) of one namespace.

    Requires the confining shape: accept only namespace -> uplink, reject any
    other egress interface, and NAT to that same physical uplink.
    """
    counters = {}
    uplinks = set()
    for line in rules:
        match = re.fullmatch(r'-A (\S+) (.*) -c (\d+) (\d+) -j (\S+).*', line.strip())
        if not match:
            continue
        chain, spec, packets, octets, target = match.groups()
        key = None
        if chain == 'FORWARD' and target == 'ACCEPT' and 'conntrack' not in spec:
            key, out = 'egress_accept', re.search(r'(?<!! )-o (\S+)', spec)
            uplinks.add(out and out.group(1))
        elif chain == 'FORWARD' and target == 'ACCEPT':
            key = 'ingress_established'
        elif chain == 'FORWARD' and target == 'REJECT' and re.search(r'! -o \S+', spec):
            key = 'egress_other_rejected'
            uplinks.add(re.search(r'! -o (\S+)', spec).group(1))
        elif chain == 'POSTROUTING' and target == 'MASQUERADE':
            key = 'nat'
            uplinks.add(re.search(r'-o (\S+)', spec).group(1))
        if key:
            counters[key] = {'packets': int(packets), 'bytes': int(octets)}
    if set(counters) != {'egress_accept', 'ingress_established', 'egress_other_rejected', 'nat'} or len(uplinks) != 1:
        raise RuntimeError('namespace isolation rules missing or inconsistent')
    uplink = uplinks.pop()
    link = next((v for v in uplink_links if v.get('ifname') == uplink), None)
    if not link or link.get('link_type') != 'ether' or link.get('linkinfo', {}).get('info_kind'):
        raise RuntimeError('namespace NAT must leave through a physical Ethernet/Wi-Fi uplink')
    return {'uplink': uplink, 'counters': counters}


ACCOUNT_CHAINS = {'out': ('OUTPUT', 'w16acct-out'), 'in': ('INPUT', 'w16acct-in')}
BLOCK_CHAIN = 'w16block'


def account_rules(service, direction):
    """Namespace-local counting rules (no verdicts): which path carried bytes.

    Relay/rendezvous is TCP to the service address; STUN is UDP to it; any
    other peer traffic is a direct path. DNS, LAN-discovery multicast/broadcast
    and the namespace's own loopback (worker <-> daemon control) are counted
    separately. Other ports at the
    service address belong to a replica sharing that public address (the VPS),
    so they are direct peer traffic too.
    """
    peer, port = ('-d', '--dport') if direction == 'out' else ('-s', '--sport')
    loop = '-o lo' if direction == 'out' else '-i lo'
    rules = [f'{loop} -m comment --comment {direction}-loopback',
             # Looped-back LAN discovery multicast is neither relay nor peer data.
             f'-m addrtype --dst-type MULTICAST,BROADCAST -m comment --comment {direction}-lan-discovery',
             f'-p tcp {peer} {service} {port} 8443 -m comment --comment {direction}-service-tcp',
             f'-p udp {peer} {service} {port} 3478 -m comment --comment {direction}-stun',
             f'-p udp {port} 53 -m comment --comment {direction}-dns',
             f'-p udp {peer} {service} -m comment --comment {direction}-colocated-udp',
             f'-p tcp {peer} {service} -m comment --comment {direction}-colocated-tcp',
             f'-p udp ! {peer} {service} -m comment --comment {direction}-direct-udp',
             f'-p tcp ! {peer} {service} -m comment --comment {direction}-direct-tcp']
    return rules


def parse_paths(text):
    totals = {}
    for line in text.splitlines():
        match = re.fullmatch(r'-A w16acct-(?:in|out) .*--comment (\S+) -c (\d+) (\d+)(?: -j RETURN)?', line.strip())
        if match:
            name, packets, octets = match.groups()
            entry = totals.setdefault(name, {'packets': 0, 'bytes': 0})
            entry['packets'] += int(packets)
            entry['bytes'] += int(octets)
    return totals


def path_delta(before, after):
    return {k: {f: v[f] - before.get(k, {}).get(f, 0) for f in ('packets', 'bytes')} for k, v in after.items()}


def counter_delta(before, after):
    return {k: {f: after['counters'][k][f] - before['counters'][k][f] for f in ('packets', 'bytes')}
            for k in after['counters']}


def check_routes(facts, isolation=None):
    """Conservative preflight: fail before setup when a tunnel can carry sync.

    Installed/active VPNs are not disabled by this runner. Isolate the test host
    before acceptance. Read-only inventory remains available on these hosts.
    With isolation (a wan_netns.sh namespace), the namespace's only link must be
    its veth, confined by verified host rules to one physical uplink.
    """
    forbidden = {'wireguard', 'tun', 'tap', 'ipip', 'sit', 'gre', 'gretap', 'vxlan', 'ip6tnl', 'vti', 'vti6'}
    links = {v['ifname']: v for v in facts['links']}
    if isolation is not None:
        others = [n for n, v in links.items() if v.get('link_type') != 'loopback']
        if len(others) != 1 or links[others[0]].get('kind') != 'veth':
            raise RuntimeError('isolated namespace must contain exactly one veth link')
        for target, routes in facts['routes'].items():
            if not ipaddress_global(target) or len(routes) != 1 or routes[0].get('dev') != others[0]:
                raise RuntimeError('namespace route must use its veth toward a public target')
        if not facts['routes']:
            raise RuntimeError('missing route targets')
        return
    for name, link in links.items():
        if 'UP' in link.get('flags', []) and (link.get('kind') in forbidden or
                name.startswith(('tailscale', 'wg', 'tun', 'tap')) or link.get('link_type') == 'none'):
            raise RuntimeError('active tunnel requires isolated native fixture: ' + name)
    if not facts['routes']:
        raise RuntimeError('missing route targets')
    for target, routes in facts['routes'].items():
        if not ipaddress_global(target):
            raise RuntimeError('WAN acceptance requires public route targets')
        if len(routes) != 1 or routes[0].get('dev') not in links:
            raise RuntimeError('missing/ambiguous native route')
        link = links[routes[0]['dev']]
        if link.get('link_type') != 'ether' or link.get('kind'):  # veth/bridge/tunnel are not physical
            raise RuntimeError('WAN route must use a physical Ethernet/Wi-Fi uplink')


def ipaddress_global(value):
    import ipaddress
    return ipaddress.ip_address(value).is_global


def validate_topology(topology):
    third = topology.get('third')
    if third is not None:
        if set(third) != {'host', 'role', 'physical_network', 'shell'} or not all(
                isinstance(v, str) and v.strip() for v in third.values()):
            raise RuntimeError('third host requires host/role/physical_network/shell')
        if not re.fullmatch(r'[A-Za-z0-9_.@-]+', third['host']) or not third['shell'].startswith('/'):
            raise RuntimeError('third host must be a safe SSH alias with an absolute shell socket')
    hosts = topology.get('hosts', [])
    required = {'host', 'role', 'physical_network'}
    if len(hosts) != 2 or any(not required <= set(v) <= required | {'netns'} for v in hosts):
        raise RuntimeError('topology requires exactly two host/role/physical_network records')
    if any(not all(isinstance(x, str) and x.strip() for x in v.values()) for v in hosts):
        raise RuntimeError('empty topology field')
    if len({v['role'] for v in hosts}) != 2 or len({v['physical_network'] for v in hosts}) != 2:
        raise RuntimeError('declare two distinct device roles and physical networks')
    for v in hosts:
        if not re.fullmatch(r'[A-Za-z0-9_.@-]+', v['host']) or v['host'].startswith('-'):
            raise RuntimeError('host must be a local label or safe SSH alias')
        if 'netns' in v and (not NETNS.fullmatch(v['netns']) or v['host'] == 'local'):
            raise RuntimeError('netns must name a remote wan_netns.sh namespace (orbit-NAME)')
    return hosts


# Client for wan_netns_shell.py: relays one worker request into the namespace.
SHELL_CLIENT = (
    'import socket,sys\n'
    's=socket.socket(socket.AF_UNIX);s.connect(sys.argv[1])\n'
    's.sendall(sys.stdin.buffer.read());s.shutdown(socket.SHUT_WR)\n'
    'data=b"".join(iter(lambda:s.recv(65536),b""))\n'
    'code,_,out=data.partition(b"\\n");sys.stdout.buffer.write(out);sys.exit(int(code))\n')


def decode_output(text, what, stderr='', rehearsal=False):
    """JSON from a worker or CLI; name the call when it printed none."""
    try:
        return json.loads(text)
    except ValueError:
        detail = f'{what}: no JSON output ({len(text)} bytes)'
        if rehearsal and stderr:
            detail += f'; stderr {stderr[-500:]!r}'
        raise RuntimeError(detail) from None


class WANNode(TerminalNode):
    def __init__(self, host, role, rehearsal=False, netns=None, shell=None):
        super().__init__(host, role)
        self.rehearsal = rehearsal
        self.netns = netns
        # A namespace started by the owner (no runner sudo): worker calls go
        # through its user-owned socket; host rules come from the saved snapshot.
        self.shell = shell
        self.accounting = False

    def call(self, action, **fields):
        command = ['python3', '-c', WORKER]
        req = {'action': action, 'root': self.root, 'token': self.token,
               'purpose': 'validation', 'rehearsal': self.rehearsal, **fields}
        if self.shell:
            payload = json.dumps({'worker': WORKER, 'request': req})
            command = ['python3', '-c', SHELL_CLIENT, self.shell]
            if self.host != 'local':
                command = ['ssh', *SSH_OPTIONS, self.host, shlex.join(command)]
            result = subprocess.run(command, input=payload,
                                    capture_output=True, text=True, timeout=320)
            if result.returncode:
                raise RuntimeError(self.role + ' ' + action + ' failed; private root retained')
            return decode_output(result.stdout, self.role + ' ' + action + ' (shell)', result.stderr, self.rehearsal)
        if self.host != 'local':
            remote = shlex.join(command)
            if self.netns:
                # Root only enters the namespace; the worker runs as the SSH user
                # with that user's home, so root guards stay unchanged.
                remote = 'sudo -n ip netns exec ' + shlex.quote(self.netns) + ' sudo -n -H -u "$(id -un)" ' + remote
            command = ['ssh', *SSH_OPTIONS, self.host, remote]
        result = subprocess.run(command, input=json.dumps(req), capture_output=True, text=True,
                                timeout=320 if action == 'wan-tui' else 140)
        if result.returncode:
            raise RuntimeError(self.role + ' ' + action + ' failed; private root retained')
        return decode_output(result.stdout, self.role + ' ' + action, result.stderr, self.rehearsal)

    def daemon_logs(self):
        """Logged daemon output with IPv4 addresses redacted (diagnostics only)."""
        logs = {}
        for name in self.workers:
            try:
                text = self.call('poll', name=name).get('log') or ''
            except RuntimeError:
                continue
            logs[name] = re.sub(r'\b\d{1,3}(?:\.\d{1,3}){3}\b', '<ipv4>', text)[-400000:]
        return logs

    def restart(self):
        """Restart with the product's default reconciliation interval.

        The inherited helper forces --sync-interval 1s; ordinary W16 journeys
        must observe the shipped default instead.
        """
        self.call('terminal-stop')
        self.workers.clear()
        self.start('serve', ['serve', '--control-listen', '127.0.0.1:0'])
        wait('live owner control', lambda: self.live(), 15)

    def isolation(self):
        """Read the host-side confinement rules/counters outside the namespace."""
        if self.shell:
            # Snapshot the owner saved while starting the namespace (rules,
            # then @@LINKS and the host's links); counters are setup-time only.
            text = self.call('wan-netns-snapshot')['text']
            rules, links = text.split('@@LINKS', 1)
            tagged = [line for line in rules.splitlines() if '--comment orbit-w16-' in line]
            summary = parse_isolation(tagged, json.loads(links))
            summary['rules'] = [re.sub(r' -c \d+ \d+', '', line) for line in tagged]
            summary['source'] = 'owner-started namespace snapshot'
            return summary
        if not self.netns:
            return None
        tag = 'orbit-w16-' + NETNS.fullmatch(self.netns).group(1)
        remote = ('sudo -n iptables -w -S -v; sudo -n iptables -w -t nat -S -v; echo @@LINKS; ip -j -d link')
        result = subprocess.run(['ssh', *SSH_OPTIONS, self.host, remote], capture_output=True, text=True, timeout=30)
        if result.returncode:
            raise RuntimeError(self.role + ' isolation inspection failed')
        rules, links = result.stdout.split('@@LINKS', 1)
        tagged = [line for line in rules.splitlines() if '--comment ' + tag + ' ' in line + ' ']
        summary = parse_isolation(tagged, json.loads(links))
        summary['rules'] = [re.sub(r' -c \d+ \d+', '', line) for line in tagged]
        return summary

    def netns_run(self, *commands, check=True):
        """Run fixed iptables/ip commands inside this node's namespace only."""
        remote = '; '.join('sudo -n ip netns exec ' + shlex.quote(self.netns) + ' ' + shlex.join(c) for c in commands)
        result = subprocess.run(['ssh', *SSH_OPTIONS, self.host, remote], capture_output=True, text=True, timeout=60)
        if check and result.returncode:
            raise RuntimeError(self.role + ' namespace command failed')
        return result.stdout

    def account(self, service):
        ipt = ['iptables', '-w']
        existing = self.netns_run(ipt + ['-S'])
        commands = []
        for direction, (parent, chain) in ACCOUNT_CHAINS.items():
            while '-A ' + parent + ' -j ' + chain in self.netns_run(ipt + ['-S', parent]):
                self.netns_run(ipt + ['-D', parent, '-j', chain])
            if '-N ' + chain + '\n' not in existing + '\n':
                commands.append(ipt + ['-N', chain])
            commands += [ipt + ['-F', chain], ipt + ['-I', parent, '1', '-j', chain]]
            for rule in account_rules(service, direction):
                # First match wins via RETURN, so each packet counts once.
                commands.append(ipt + ['-A', chain, *shlex.split(rule), '-j', 'RETURN'])
        self.netns_run(*commands)

    def paths(self):
        return parse_paths(''.join(self.netns_run(['iptables', '-w', '-S', chain, '-v'])
                                   for _, chain in ACCOUNT_CHAINS.values()))

    def block_udp(self, enabled):
        """Forced-relay drill: drop non-DNS UDP inside this namespace only."""
        ipt = ['iptables', '-w']
        for command in (['-D', 'OUTPUT', '-j', BLOCK_CHAIN], ['-D', 'INPUT', '-j', BLOCK_CHAIN],
                        ['-F', BLOCK_CHAIN], ['-X', BLOCK_CHAIN]):
            self.netns_run(ipt + command, check=False)
        if enabled:
            self.netns_run(ipt + ['-N', BLOCK_CHAIN],
                           ipt + ['-A', BLOCK_CHAIN, '-p', 'udp', '--dport', '53', '-j', 'RETURN'],
                           ipt + ['-A', BLOCK_CHAIN, '-p', 'udp', '--sport', '53', '-j', 'RETURN'],
                           ipt + ['-A', BLOCK_CHAIN, '-p', 'udp', '-j', 'DROP'],
                           # Ahead of accounting: dropped packets are never counted as carried.
                           ipt + ['-I', 'OUTPUT', '1', '-j', BLOCK_CHAIN], ipt + ['-I', 'INPUT', '1', '-j', BLOCK_CHAIN])

    def change_address(self):
        """Roaming drill: move the namespace from .2 to .3 (same /29)."""
        out = json.loads(self.netns_run(['ip', '-j', 'addr', 'show', 'scope', 'global']))
        link = next(v for v in out if v.get('addr_info'))
        old = next(a for a in link['addr_info'] if a.get('family') == 'inet')
        new = old['local'].rsplit('.', 1)[0] + '.' + ('3' if old['local'].endswith('.2') else '2')
        gateway = old['local'].rsplit('.', 1)[0] + '.1'
        # Promotion keeps the new (secondary) address when the old primary goes.
        self.netns_run(['sysctl', '-qw', 'net.ipv4.conf.' + link['ifname'] + '.promote_secondaries=1'],
                       ['ip', 'addr', 'add', new + '/29', 'dev', link['ifname']],
                       ['ip', 'addr', 'del', old['local'] + '/29', 'dev', link['ifname']],
                       ['ip', 'route', 'replace', 'default', 'via', gateway, 'src', new])
        now = json.loads(self.netns_run(['ip', '-j', 'addr', 'show', 'scope', 'global']))
        if [a['local'] for v in now for a in v.get('addr_info', []) if a.get('family') == 'inet'] != [new]:
            raise RuntimeError(self.role + ' address change did not leave exactly the new address')
        return {'from': old['local'], 'to': new, 'interface': link['ifname']}

    def cli(self, *args, check=True):
        result = self.call('run', args=[*args, '--state', self.root + '/state'])
        self.last_resources = result.get('resources')
        if check and result['returncode']:
            self.put('last-cli-failure.json', json.dumps(result).encode())
            raise RuntimeError(self.role + ' CLI failed; private output retained on host')
        return result['stdout'] if check else result

    def prepare(self, dist, profile=None, roots=None):
        self.inventory = self.call('inventory')
        arch = {'x86_64': 'amd64', 'aarch64': 'arm64'}.get(self.inventory['arch'])
        if arch is None:
            raise RuntimeError('unsupported host architecture')
        binary, self.provenance = package_binary(dist, arch)
        self.root = self.call('create')['root']
        self.put('host_agent.py', HOST_AGENT.encode())
        for name in ('wan_tui_phase.py', '../terminal_vt.py'):
            self.put(Path(name).name, (Path(__file__).parent / name).read_bytes())
        self.put('orbit', binary, mode=0o700)
        version = self.call('run', args=['orbit', 'version', '--json'])
        if version['returncode']:
            raise RuntimeError('packaged version command failed')
        self.version = json.loads(version['stdout'])
        if profile:
            self.put('rehearsal-profile.json', profile.read_bytes())
            self.put('rehearsal-ca.pem', roots.read_bytes())
            self.orbit('network', 'preview', '--mode', 'self_hosted',
                       '--profile-file', self.root + '/rehearsal-profile.json',
                       '--service-roots', self.root + '/rehearsal-ca.pem',
                       '--review-file', self.root + '/network-review.json', '--json')
            self.orbit('network', 'apply', '--review-file', self.root + '/network-review.json', '--json')
        # No init/register, address, listener, runtime settings or endpoint overrides.

    def tui(self, phase, **options):
        """One real-PTY keyboard phase (wan_tui_phase.py) on this host."""
        flags = [] if self.rehearsal else ['--hosted']
        for key, value in options.items():
            flags += ['--' + key, value]
        result = self.call('wan-tui', phase=phase, options=flags)
        frames = self.call('read', path='tui-frames/' + phase + '-' + options.get('relative', 'data') + '.txt')
        result['frames'] = base64.b64decode(frames['data']).decode()
        if result.get('returncode') or result.get('error'):
            raise RuntimeError(self.role + ' TUI ' + phase + ' failed: ' + str(result.get('error')))
        return result

    def resources(self):
        sample = self.call('wan-resources')
        if self.shell:
            sample['interface_bytes'] = self.call('wan-netdev')
        return sample

    def identify(self):
        self.device = json.loads(base64.b64decode(self.call('read', path='state/config.json')['data']))['device_id']
        self.pin = self.call('wan-identity')['pin']

    def setup(self, relative='data', invitation=None):
        args = ['join' if invitation else 'setup', '--root', self.root + '/' + relative,
                '--label', self.role, '--name', 'W16 ' + relative, '--preview',
                '--review-file', self.root + '/' + relative + '-review.json', '--json']
        if invitation:
            self.put('invitation.txt', invitation.encode())
            args += ['--invitation-file', self.root + '/invitation.txt']
        preview = decode_output(self.orbit(*args), self.role + ' ' + args[0] + ' preview')
        if not preview['preview']['complete']:
            raise RuntimeError('root review incomplete')
        submitted = self.orbit(args[0], '--request-file',
                               self.root + '/' + relative + '-review.json', '--timeout', '0', '--json', check=False)
        try:
            result = json.loads(submitted['stdout'])
        except ValueError:
            # Transport errors exit nonzero with text on stderr only.
            result = {}
        if not result or (submitted['returncode'] and not (result.get('error', {}).get('retryable') and result.get('operation'))):
            self.put('last-cli-failure.json', json.dumps(submitted).encode())
            detail = ''
            if self.rehearsal:
                detail = f": exit {submitted['returncode']}: {submitted.get('stderr', '')[-500:]!r}"
            raise RuntimeError(self.role + ' setup failed; private output retained on host' + detail)
        self.identify()
        return result


def network_check(status, digest, rehearsal):
    expected = 'self_hosted' if rehearsal else 'automatic'
    if status['policy']['mode'] != expected or not status['ready'] or status['policy']['profile'] != digest:
        raise RuntimeError('reviewed profile/policy not ready')
    if not rehearsal and (status['service_trust'] != 'system' or status['builtin']['digest'] != digest):
        raise RuntimeError('hosted journey must use bundled profile and system trust')
    return status


def request_record(node, submitted):
    current = node.query('operation', id=submitted['operation']['id'])
    if current['operation']['id'] != submitted['operation']['id'] or current['join']['attempt'] != submitted['join']['attempt']:
        raise RuntimeError('retry replaced the reviewed operation/attempt')
    return current if current['join']['request'] else False


class TransferTimeout(RuntimeError):
    def __init__(self, message, diagnostics):
        super().__init__(message)
        self.diagnostics = diagnostics


def transfer(nodes, folder, sender, receiver, relative, data, write=True, root='data', timeline_enabled=False):
    before = {n.role: n.isolation() for n in nodes if n.netns}
    paths_before = {n.role: n.paths() for n in nodes if n.netns and n.accounting is True}
    started = time.monotonic()
    if write:
        sender.put(root + '/' + relative, data)
    digest = hashlib.sha256(data).hexdigest()
    last = {'reason': 'not yet evaluated'}
    timeline, sampled = [], {'at': 0.0}
    def unmet(reason):
        last['reason'] = reason
        return False
    def oracle():
        if timeline_enabled and time.monotonic() - sampled['at'] >= 15:
            sampled['at'] = time.monotonic()
            entry = {'t': round(time.monotonic() - started, 1)}
            for n in nodes:
                try:
                    entry[str(n.role)] = network_snapshot(n)
                except Exception as error:
                    entry[str(n.role)] = 'unavailable: ' + type(error).__name__
            timeline.append(entry)
        try:
            histories = [n.query('history', folder=folder, path=relative, limit='20')['versions'] for n in nodes]
            # Each fixture path is authored exactly once. Histories are then heads.
            if any(len(v) != 1 or v[0]['digest'] != digest or v[0]['version']['author'] != sender.device for v in histories):
                return unmet('history: ' + ', '.join(f'{n.role}={len(v)} version(s)' for n, v in zip(nodes, histories)))
            head = histories[0][0]['version']
            if any(v[0]['version'] != head for v in histories):
                return unmet('heads differ')
            def working_hash(n):
                if root == 'data':
                    return n.call('hash', path=relative)['sha256']
                return hashlib.sha256(base64.b64decode(n.call('read', path=root + '/' + relative)['data'])).hexdigest()
            if any(working_hash(n) != digest for n in nodes):
                return unmet('working bytes not yet identical')
            status = sender.query('status', folder=folder, limit='20')
            receipt = next((v for v in status['observations'] if v['device'] == receiver.device
                            and v['version'] == head and v['stored']), None)
            if not receipt:
                return unmet('sender has no stored receipt from the receiver')
            receiver_status = receiver.query('status', folder=folder, limit='20')
            readiness = receiver_status['readiness']
            if not readiness['approved'] or not readiness['membership_current'] or not readiness['scan_complete'] or any(
                    int(readiness[k]) for k in ('missing_content', 'pending_publication', 'conflicts', 'uncaptured')):
                return unmet('receiver readiness: ' + json.dumps(readiness, sort_keys=True))
            # A durable receipt means stored; remote applied may still be unknown.
            # Preserve that value and separately record actual receiver working bytes.
            return {'head': head, 'sha256': digest, 'bytes': len(data), 'receipt': receipt,
                    'receiver_readiness': readiness, 'receiver_working_hash_verified': True}
        except RuntimeError as error:
            return unmet('query failed: ' + str(error))
    try:
        result = wait('endpoint hash/head/stored/applied receipt ' + relative, oracle, 180)
    except RuntimeError as error:
        diagnostics = {'last_unmet': last['reason'], 'seconds': time.monotonic() - started, 'timeline': timeline}
        for n in nodes:
            role = str(n.role)
            try:
                diagnostics[role + '_routes'] = peer_routes(n)
                if n.netns and n.accounting is True:
                    diagnostics[role + '_paths_since_start'] = path_delta(paths_before[n.role], n.paths())
            except Exception as inner:
                diagnostics[role + '_routes'] = 'unavailable: ' + type(inner).__name__
        raise TransferTimeout(str(error), diagnostics) from None
    result['seconds'] = time.monotonic() - started
    if timeline_enabled:
        result['timeline'] = timeline
    result['completion_bytes_per_second'] = len(data) / result['seconds']
    if before:
        # Host NAT counters around the transfer: the namespace's only exit path.
        result['route_counters'] = {n.role: counter_delta(before[n.role], n.isolation())
                                    for n in nodes if n.netns}
    if paths_before:
        result['paths'] = {role: path_delta(paths_before[role], n.paths())
                           for role, n in ((n.role, n) for n in nodes) if role in paths_before}
        result['path_class'] = classify_paths(result['paths'])
    return result


def classify_paths(paths):
    """Name the path that delivered most peer bytes across all namespaces.

    Only received bytes count: an unanswered outbound attempt toward a blocked
    peer is recorded as an attempt, never as a direct path.
    """
    def total(*keys):
        return sum(v.get(k, {}).get('bytes', 0) for v in paths.values() for k in keys)
    direct = total('in-direct-udp', 'in-direct-tcp', 'in-colocated-udp', 'in-colocated-tcp')
    relay = total('in-service-tcp')
    return {'direct_bytes': direct, 'service_tcp_bytes': relay,
            'direct_attempt_bytes': total('out-direct-udp', 'out-direct-tcp', 'out-colocated-udp', 'out-colocated-tcp'),
            'dominant': 'direct' if direct > relay else 'relay_or_control'}


def network_snapshot(n):
    """Sanitized per-peer observations (no pins/addresses) for timelines."""
    status = json.loads(n.orbit('network', 'status', '--json'))['network']
    keep = ('purpose', 'route', 'code', 'udp_code', 'freshness', 'lan_candidates', 'public_candidates', 'generation')
    return {'ready': status.get('ready'), 'code': status.get('code'),
            'observations': [{k: o.get(k) for k in keep} for o in status.get('observations', [])]}


def service_metrics(host):
    """Read-only loopback metrics of the operated service (counters only)."""
    if not host:
        return None
    result = subprocess.run(['ssh', *SSH_OPTIONS, host, 'curl -s --max-time 5 http://127.0.0.1:9464/metrics'],
                            capture_output=True, text=True, timeout=30)
    values = {}
    for line in result.stdout.splitlines():
        if line.startswith('orbit_net_') and ('refusals' in line or 'relay_' in line or 'sessions' in line or 'stun' in line):
            name, _, value = line.rpartition(' ')
            values[name] = float(value)
    return values


def service_process(host):
    """Read-only /proc sample of the operated service's main process."""
    if not host:
        return None
    remote = ('pid=$(systemctl show -p MainPID --value orbit-net); echo "pid $pid"; '
              'grep -E "^(VmRSS|VmHWM|Threads):" /proc/$pid/status; cat /proc/$pid/stat; getconf CLK_TCK; '
              'systemctl show -p ActiveEnterTimestamp --value orbit-net')
    result = subprocess.run(['ssh', *SSH_OPTIONS, host, remote], capture_output=True, text=True, timeout=30)
    lines = result.stdout.splitlines()
    try:
        status = {k: int(v.split()[0]) for k, _, v in (l.partition(':') for l in lines[1:4])}
        stat = lines[4][lines[4].rfind(')') + 2:].split()
        ticks = int(lines[5])
        return {'pid': int(lines[0].split()[1]), 'rss_kib': status['VmRSS'], 'peak_rss_kib': status['VmHWM'],
                'threads': status['Threads'], 'user_seconds': int(stat[11]) / ticks,
                'system_seconds': int(stat[12]) / ticks, 'active_since': lines[6]}
    except (IndexError, ValueError, KeyError):
        return {'unavailable': True}


def sample_resources(report, label, nodes, metrics_host=None):
    entry = {'at': now()}
    for n in nodes:
        try:
            entry[str(n.role)] = n.resources()
        except Exception as error:
            entry[str(n.role)] = 'unavailable: ' + type(error).__name__
    entry['service'] = service_process(metrics_host)
    report.setdefault('resources', {})[label] = entry


def service_restart(host, nodes, folder, a, b, out, metrics_host):
    """Owner-approved restart of the operated service during forced relay.

    One `systemctl restart orbit-net` (binary/config unchanged), then require
    relay recovery and a transfer in each direction.
    """
    before = service_process(metrics_host or host)
    started = time.monotonic()
    result = subprocess.run(['ssh', *SSH_OPTIONS, host, 'sudo -n systemctl restart orbit-net && systemctl is-active orbit-net'],
                            capture_output=True, text=True, timeout=120)
    out['restart_command_seconds'] = time.monotonic() - started
    out['restart_active'] = result.stdout.strip()
    if result.returncode or result.stdout.strip() != 'active':
        raise RuntimeError('service restart did not return to active')
    after = service_process(metrics_host or host)
    out['service_pid_changed'] = bool(before and after and before.get('pid') != after.get('pid'))
    out['service_before'], out['service_after'] = before, after
    def relay_ready():
        statuses = [json.loads(n.orbit('network', 'status', '--json'))['network'] for n in nodes]
        routes = sorted(set(peer_routes(a)) | set(peer_routes(b)))
        return {'routes': routes} if all(s['ready'] for s in statuses) and routes == ['relay'] else False
    out['recovered'] = wait('service ready and relay routes after restart', relay_ready, 300)
    out['recovery_seconds'] = time.monotonic() - started
    out['transfer'] = transfer(nodes, folder, a, b, 'after-restart', b'bytes after service restart\n', timeline_enabled=True)
    out['reverse'] = transfer(nodes, folder, b, a, 'after-restart-back', b'reverse after service restart\n', timeline_enabled=True)
    for t in (out['transfer'], out['reverse']):
        if t['path_class']['direct_bytes']:
            raise RuntimeError('direct bytes observed while UDP was blocked')
    out['service_after_transfers'] = service_metrics(metrics_host or host)


def peer_routes(n):
    status = json.loads(n.orbit('network', 'status', '--json'))['network']
    return sorted({o.get('route') for o in status.get('observations', [])
                   if o.get('purpose') == 'peer_data' and o.get('code') == 'CONNECTED'} - {None})


def impairments(nodes, folder, a, b, ready, out, metrics_host=None, restart_host=None):
    """Forced relay (UDP blocked in b's namespace) and an address change on a.

    Results are written into out as they happen, so a failure keeps them.
    """
    if not all(n.netns for n in nodes):
        raise RuntimeError('impairment drills require isolated namespaces on both hosts')
    out['service_before'] = service_metrics(metrics_host)
    b.block_udp(True)
    try:
        started = time.monotonic()
        out['udp_blocked_routes'] = wait('relay-only peer routes', lambda: (lambda r: r if r == ['relay'] else False)(
            sorted(set(peer_routes(a)) | set(peer_routes(b)))), 300)
        out['udp_blocked_route_seconds'] = time.monotonic() - started
        out['udp_blocked_transfer'] = transfer(nodes, folder, a, b, 'relay-only', bytes(range(251)) * 4096, timeline_enabled=True)
        out['service_after_forward'] = service_metrics(metrics_host)
        try:
            out['udp_blocked_reverse'] = transfer(nodes, folder, b, a, 'relay-only-back', b'relay reverse bytes\n', timeline_enabled=True)
        finally:
            out['service_after_reverse'] = service_metrics(metrics_host)
        for t in (out['udp_blocked_transfer'], out['udp_blocked_reverse']):
            if t['path_class']['direct_bytes']:
                raise RuntimeError('direct bytes observed while UDP was blocked')
        if restart_host:
            out['service_restart'] = {}
            service_restart(restart_host, nodes, folder, a, b, out['service_restart'], metrics_host)
    finally:
        b.block_udp(False)
    out['address_change'] = a.change_address()
    started = time.monotonic()
    out['after_change_transfer'] = transfer(nodes, folder, a, b, 'after-roam', b'bytes after address change\n', timeline_enabled=True)
    out['after_change_reverse'] = transfer(nodes, folder, b, a, 'after-roam-back', b'reverse after address change\n', timeline_enabled=True)
    out['service_after_change'] = service_metrics(metrics_host)
    out['after_change_seconds'] = time.monotonic() - started
    out['after_change_routes'] = {n.role: peer_routes(n) for n in nodes}


def tui_onboarding(a, b, report, ready, digest, rehearsal):
    """Create/invite/join/approve by keyboard on both hosts' real PTYs.

    The saved private invitation moves between hosts like the CLI journey's
    code does (owner-administered copy); nothing else is configured.
    """
    created = a.tui('create', label=str(a.role), name='W16 TUI')
    folder = created['folder']
    a.identify()
    network_check(wait('inviter service ready', lambda: ready(a), 60), digest, rehearsal)
    invited = a.tui('invite', folder=folder)
    if invited['invitation'] != {'version': '3', 'folder': folder, 'peer_endpoint': False,
                                 'enrollment_endpoint': False, 'routed': True}:
        raise RuntimeError('TUI invitation is not a routed v3 invitation without endpoints')
    b.put('tui-invitation.json', base64.b64decode(a.call('read', path='tui-invitation.json')['data']))
    started = time.monotonic()
    joined = b.tui('join', label=str(b.role), name='W16 TUI')
    b.identify()
    if joined['folder'] != folder:
        raise RuntimeError('joined folder differs from the invitation')
    wait('exact pending approval', lambda: any(v['id'] == joined['request']
        for v in a.query('requests', limit='20')['requests']), 90)
    approved = a.tui('approve', request=joined['request'])
    if approved['verification_code'] != joined['verification_code']:
        raise RuntimeError('verification code differs between devices')
    def joined_ready():
        current = b.query('operation', id=joined['operation'])
        if current['operation']['id'] != joined['operation'] or current['join']['attempt'] != joined['attempt']:
            raise RuntimeError('approval replaced the reviewed operation/attempt')
        return current if current['state'] == 'completed' else False
    completed = wait('joiner setup completed', joined_ready, 180)
    report['scenarios']['tui'] = {
        'create': {k: created[k] for k in ('review_mode', 'seconds', 'terminal_restored')},
        'invite': {'invitation': invited['invitation'], 'seconds': invited['seconds']},
        'join': {k: joined[k] for k in ('seconds', 'terminal_restored')},
        'approve': {'verification_code_matched': True, 'seconds': approved['seconds']},
        'join_to_completed_seconds': time.monotonic() - started,
        'joiner_readiness': completed.get('readiness'),
        'frames': {p: r['frames'] for p, r in (('create', created), ('invite', invited), ('join', joined), ('approve', approved))}}
    report['scenarios']['approval'] = {'request': joined['request'], 'operation': joined['operation'],
                                       'join_to_approved_seconds': time.monotonic() - started}
    return folder


def head_ids(n, folder, path):
    return {json.dumps(v['version'], sort_keys=True) for v in n.query('history', folder=folder, path=path, limit='20')['versions']}


def three_host(nodes, c, folder, out):
    """Laptop joins as a third device; forwarding, conflict and restore.

    a = owner (Pi), b = second device (VPS), c = laptop.
    """
    a, b = nodes
    everyone = [a, b, c]
    code = a.orbit('devices', 'invite', '--folder', folder, '--code').strip()
    started = time.monotonic()
    pending = c.setup(invitation=code)
    pending = wait('third durable routed request', lambda: request_record(c, pending), 180)
    wait('third exact approval', lambda: any(v['id'] == pending['join']['request']
         for v in a.query('requests', limit='20')['requests']), 90)
    out['approval'] = approve(a, c, pending)
    out['approval']['join_to_approved_seconds'] = time.monotonic() - started
    out['laptop_to_all'] = transfer(everyone, folder, c, a, 'from-laptop', b'laptop bytes over WAN\n')
    wait('VPS has laptop bytes', lambda: b.call('hash', path='from-laptop')['sha256']
         == hashlib.sha256(b'laptop bytes over WAN\n').hexdigest(), 120)
    # Forwarding: the VPS is offline while the laptop authors; the laptop is
    # offline while the VPS reconnects. Only the Pi can carry the version.
    value = b'authored on the laptop, forwarded by the Pi\n'
    digest = hashlib.sha256(value).hexdigest()
    b.call('terminal-stop')
    c.put('data/forwarded', value)
    def on(n):
        versions = n.query('history', folder=folder, path='forwarded', limit='20')['versions']
        return versions if len(versions) == 1 and versions[0]['digest'] == digest and versions[0]['version']['author'] == c.device else False
    wait('Pi has the laptop version', lambda: on(a), 180)
    c.call('terminal-stop')
    started = time.monotonic()
    b.restart()
    forwarded = wait('VPS receives the laptop version via the Pi', lambda: on(b), 180)
    wait('VPS working bytes', lambda: b.call('hash', path='forwarded')['sha256'] == digest, 60)
    out['forwarding'] = {'author': forwarded[0]['version']['author'], 'author_is_laptop': True,
                         'laptop_offline_during_delivery': True, 'seconds': time.monotonic() - started,
                         'vps_routes': peer_routes(b)}
    c.restart()
    # Conflict: a shared base, then the VPS edits while stopped and the laptop
    # edits online. The VPS captures its edit on restart, concurrent to the laptop's.
    base = b'shared base\n'
    out['conflict_base'] = transfer(everyone, folder, a, b, 'conflict', base)
    wait('laptop has base', lambda: c.call('hash', path='conflict')['sha256'] == hashlib.sha256(base).hexdigest(), 120)
    base_head = a.query('history', folder=folder, path='conflict', limit='20')['versions'][0]['version']
    b.call('terminal-stop')
    b.put('data/conflict', b'VPS offline edit\n')
    c.put('data/conflict', b'laptop online edit\n')
    wait('Pi has the laptop edit', lambda: len(head_ids(a, folder, 'conflict')) == 2, 120)
    started = time.monotonic()
    b.restart()
    def conflicted():
        reviews = {}
        for n in everyone:
            attention = n.query('conflicts', folder=folder, limit='20').get('attention') or []
            if not any(v['path'] == 'conflict' for v in attention):
                return False
            review = n.query('content_review', folder=folder, path='conflict')['content_review']
            reviews[n.role] = sorted(json.dumps(h, sort_keys=True) for h in review['heads'])
        return reviews if len({json.dumps(v) for v in reviews.values()}) == 1 and len(next(iter(reviews.values()))) == 2 else False
    heads = wait('same two conflict heads on all three', conflicted, 240)
    out['conflict'] = {'heads': heads, 'seconds_to_converge': time.monotonic() - started}
    vps_head = next(json.loads(h) for h in heads[str(a.role)] if json.loads(h)['author'] == b.device)
    review = c.root + '/conflict-review.json'
    c.orbit('conflicts', 'show', c.root + '/data/conflict', '--folder', folder, '--out', review, '--json')
    selected = vps_head['author'] + ':' + str(vps_head['counter'])
    c.orbit('conflicts', 'select', c.root + '/data/conflict', '--folder', folder, '--selected', selected, '--review-file', review, '--json')
    resolved = hashlib.sha256(b'VPS offline edit\n').hexdigest()
    def cleared():
        for n in everyone:
            if any(v['path'] == 'conflict' for v in n.query('conflicts', folder=folder, limit='20').get('attention') or []):
                return False
            if n.call('hash', path='conflict')['sha256'] != resolved:
                return False
        return True
    started = time.monotonic()
    wait('resolution reaches all three', cleared, 180)
    out['resolution'] = {'resolved_on': str(c.role), 'selected_author_is_vps': True, 'seconds': time.monotonic() - started}
    # Restore the shared base on the VPS as a new version.
    review = b.root + '/restore-review.json'
    source = base_head['author'] + ':' + str(base_head['counter'])
    b.orbit('restore', b.root + '/data/conflict', '--folder', folder, '--version', source, '--out', review, '--json')
    b.orbit('restore', b.root + '/data/conflict', '--folder', folder, '--version', source, '--review-file', review, '--json')
    base_digest = hashlib.sha256(base).hexdigest()
    last = {}
    def restored():
        # History order is not newest-first: find the restored version itself.
        found = set()
        for n in everyone:
            if n.call('hash', path='conflict')['sha256'] != base_digest:
                last['unmet'] = str(n.role) + ': working bytes'
                return False
            if any(v['path'] == 'conflict' for v in n.query('conflicts', folder=folder, limit='20').get('attention') or []):
                last['unmet'] = str(n.role) + ': conflict attention'
                return False
            versions = n.query('history', folder=folder, path='conflict', limit='20')['versions']
            new = [v for v in versions if v['version']['author'] == b.device and v['digest'] == base_digest
                   and v['version'] != base_head]
            if len(new) != 1:
                last['unmet'] = f'{n.role}: {len(new)} restored versions among {len(versions)}'
                return False
            # Device names are each viewer's local labels; compare identity and digest.
            found.add(json.dumps({'version': new[0]['version'], 'digest': new[0]['digest']}, sort_keys=True))
        if len(found) != 1:
            last['unmet'] = 'restored version summaries differ: ' + ' | '.join(sorted(found))
            return False
        return json.loads(found.pop())
    started = time.monotonic()
    try:
        head = wait('restored base on all three', restored, 180)
    except RuntimeError:
        out['restore_unmet'] = last.get('unmet')
        raise
    out['restore'] = {'restored_version': head['version'], 'base_version': base_head,
                      'new_identity_authored_by_vps': True, 'seconds': time.monotonic() - started}
    out['final_heads_agree'] = True


def run(args):
    prepare_output(args.output)
    hosts = validate_topology(json.loads(args.topology.read_text()))
    rehearsal = args.rehearsal_profile is not None
    if rehearsal and (not args.rehearsal_roots or any(v['host'] != 'local' for v in hosts)):
        raise RuntimeError('self-host rehearsal requires local hosts and explicit private CA')
    report = {'packet': 'W16', 'started_at': now(), 'success': False, 'rehearsal': rehearsal,
              'hosted_default_acceptance': 'unexecuted', 'physical_wan_acceptance': 'unexecuted',
              'hosts': [], 'scenarios': {}, 'cleanup_errors': []}
    nodes = [WANNode(v['host'], v['role'], rehearsal, v.get('netns')) for v in hosts]
    topology = json.loads(args.topology.read_text())
    third_record = topology.get('third') if getattr(args, 'three_host', False) else None
    third = WANNode(third_record['host'], third_record['role'], rehearsal, shell=third_record['shell']) if third_record else None
    if getattr(args, 'three_host', False) and third is None:
        raise RuntimeError('--three-host requires a third topology record')
    metrics_host = getattr(args, 'service_metrics_host', None)
    try:
        if not args.inventory_only and not rehearsal:
            check_wg6((ROOT / 'docs/orbit-wan-design-gates.md').read_text())
        for record, n in zip(hosts, nodes):
            facts = n.call('wan-inventory', targets=args.route_targets)
            record = {'declaration': record, 'network': facts}
            try:
                isolation = n.isolation()
                record['isolation'] = isolation
                check_routes(facts, isolation)
                record['route_preflight'] = 'eligible; physical separation still requires deployment evidence'
            except RuntimeError as error:
                record['route_preflight'] = str(error)
            report['hosts'].append(record)
            if not args.inventory_only and not rehearsal:
                check_routes(facts, n.isolation())
        if third:
            facts = third.call('wan-inventory', targets=args.route_targets)
            record = {'declaration': third_record, 'network': facts}
            if rehearsal:
                record['route_preflight'] = 'rehearsal: shell without a namespace'
            else:
                record['isolation'] = third.isolation()
                check_routes(facts, record['isolation'])
                record['route_preflight'] = 'eligible; owner-started namespace'
            report['third_host'] = record
        if args.inventory_only:
            report['success'] = True
            return
        for n in nodes:
            if n.netns:
                n.block_udp(False)
                n.account(args.route_targets[0])
                n.accounting = True
        manifest = json.loads((args.dist / 'release-manifest.json').read_text())
        report['package_manifest'] = manifest
        digest = manifest['packaged_profile']['digest'] if not rehearsal else None
        for n, record in zip(nodes, report['hosts']):
            n.prepare(args.dist, args.rehearsal_profile, args.rehearsal_roots)
            record.update(root=n.root, package=n.provenance, version=n.version)
            if not rehearsal and n.version['packaged_profile']['digest'] != digest:
                raise RuntimeError('package manifest and binary profile disagree')
        if third:
            third.prepare(args.dist, args.rehearsal_profile, args.rehearsal_roots)
            report['third_host'].update(root=third.root, package=third.provenance, version=third.version)
            if not rehearsal and third.version['packaged_profile']['digest'] != digest:
                raise RuntimeError('package manifest and binary profile disagree')
        a, b = nodes
        def ready(n):
            status = json.loads(n.orbit('network', 'status', '--json'))['network']
            return status if status['ready'] else False
        report['journey'] = getattr(args, 'journey', 'cli')
        if report['journey'] == 'tui':
            if rehearsal:
                digest = json.loads(a.orbit('network', 'status', '--json'))['network']['policy']['profile']
            folder = tui_onboarding(a, b, report, ready, digest, rehearsal)
            pending = {'join': {'request': report['scenarios']['approval']['request']}}
        else:
            created = a.setup()
            folder = created['join']['folder']
            # The operation runs asynchronously; do not invite until initial adoption completes.
            wait('local create', lambda: a.query('operation', id=created['operation']['id'])['state'] == 'completed', 60)
            first = wait('inviter service ready', lambda: ready(a), 60)
            if rehearsal:
                digest = first['policy']['profile']
            network_check(first, digest, rehearsal)
            code = a.orbit('devices', 'invite', '--code').strip()
            if not code.startswith('orbit-invitation:v3:'):
                raise RuntimeError('ordinary journey must use routed v3 invitation')
            started = time.monotonic()
            pending = b.setup(invitation=code)
            pending = wait('durable routed request', lambda: request_record(b, pending), 180)
            wait('exact pending approval', lambda: any(v['id'] == pending['join']['request']
                for v in a.query('requests', limit='20')['requests']), 90)
            report['scenarios']['approval'] = approve(a, b, pending)
            report['scenarios']['approval']['join_to_approved_seconds'] = time.monotonic() - started
        sample_resources(report, 'after_join', nodes, metrics_host)
        for n in nodes:
            network_check(wait('joiner service ready', lambda: ready(n), 60), digest, rehearsal)
        report['scenarios']['two_way'] = [transfer(nodes, folder, a, b, 'from-a', b'W16 owner bytes\n'),
                                           transfer(nodes, folder, b, a, 'from-b', b'W16 joining bytes\n')]
        if report['journey'] == 'tui':
            observed = b.tui('observe')
            report['scenarios']['tui']['observe'] = {'route_label': observed['route_label'], 'seconds': observed['seconds']}
            report['scenarios']['tui']['frames']['observe'] = observed['frames']
        report['scenarios']['large'] = transfer(nodes, folder, a, b, 'large', bytes(range(256)) * 16384)
        sample_resources(report, 'after_large', nodes, metrics_host)
        b.call('terminal-stop')
        a.put('data/offline', b'captured while receiver offline\n')
        wait('offline capture', lambda: a.query('history', folder=folder, path='offline', limit='20')['versions'], 60)
        b.restart()
        # No rewrite after restart: require the exact once-authored head and receipt.
        report['scenarios']['reconnect'] = transfer(nodes, folder, a, b, 'offline',
                                                   b'captured while receiver offline\n', write=False)
        identities = [(n.device, n.pin) for n in nodes]
        second = a.setup('second')
        second_folder = second['join']['folder']
        wait('second local create', lambda: a.query('operation', id=second['operation']['id'])['state'] == 'completed', 60)
        wait('second inviter service ready', lambda: ready(a), 60)
        second_code = a.orbit('devices', 'invite', '--folder', second_folder, '--code').strip()
        second_pending = b.setup('second', second_code)
        second_pending = wait('second durable routed request', lambda: request_record(b, second_pending), 180)
        wait('second exact approval', lambda: any(v['id'] == second_pending['join']['request']
            for v in a.query('requests', limit='20')['requests']), 90)
        if second_pending['join']['request'] == pending['join']['request'] or second_folder == folder:
            raise RuntimeError('second folder enrollment identity collision')
        report['scenarios']['second_folder'] = {'approval': approve(a, b, second_pending),
            'transfer': transfer(nodes, second_folder, b, a, 'second-file', b'explicit second folder bytes\n', root='second')}
        if identities != [(n.device, n.pin) for n in nodes]:
            raise RuntimeError('second folder changed persistent identity')
        sample_resources(report, 'after_second_folder', nodes, metrics_host)
        if third:
            report['scenarios']['three_host'] = {}
            three_host(nodes, third, folder, report['scenarios']['three_host'])
            sample_resources(report, 'after_three_host', nodes + [third], metrics_host)
        if getattr(args, 'impairments', False):
            if getattr(args, 'collect_logs', False):
                # Diagnostics: run the owner daemon under the logged worker too.
                a.restart()
            report['scenarios']['impairments'] = {}
            impairments(nodes, folder, a, b, ready, report['scenarios']['impairments'], metrics_host,
                        getattr(args, 'service_restart', None))
            sample_resources(report, 'after_impairments', nodes, metrics_host)
        for n, record in zip(nodes, report['hosts']):
            cfg = json.loads(base64.b64decode(n.call('read', path='state/config.json')['data']))
            if cfg['device_id'] != n.device:
                raise RuntimeError('persistent identity changed')
            if n.call('wan-identity')['pin'] != n.pin:
                raise RuntimeError('persistent key pin changed')
            record['device'] = n.device
            record['pin'] = n.pin
            record['final_status'] = n.query('status', folder=folder, limit='20')
            record['final_network'] = n.call('wan-inventory', targets=args.route_targets)
            if not rehearsal:
                record['final_isolation'] = n.isolation()
                check_routes(record['final_network'], record['final_isolation'])
        report['success'] = True
        # Observations remain observations: this slice does not certify all W16 cases.
    except Exception as error:
        report['failure'] = str(error)
        if isinstance(error, TransferTimeout):
            report['failure_diagnostics'] = error.diagnostics
        raise
    finally:
        if getattr(args, 'collect_logs', False):
            for n in nodes + ([third] if third else []):
                for name, text in n.daemon_logs().items():
                    (args.output / f'{n.role}-{name}.log').write_text(text)
        everyone = nodes + ([third] if third else [])
        for n in everyone:
            try:
                n.cleanup()
            except Exception:
                report['cleanup_errors'].append(n.role + ': owned daemon stop failed')
        # Include partially prepared roots even when a setup/CLI fails midway.
        report['retained_roots'] = {n.role: n.root for n in everyone if n.root}
        if report['cleanup_errors']:
            report['success'] = False
        if report['success'] and not report['cleanup_errors'] and not args.inventory_only:
            for n in everyone:
                try:
                    n.call('wan-clean')
                    report['retained_roots'].pop(n.role)
                except Exception:
                    report['cleanup_errors'].append(n.role + ': disposable root removal refused')
            if report['cleanup_errors']:
                report['success'] = False
        report['ended_at'] = now()
        (args.output / 'wan-native.json').write_text(json.dumps(report, indent=2) + '\n')
        if report['cleanup_errors']:
            raise RuntimeError('owned daemon cleanup failed; roots retained')


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dist', type=Path, default=ROOT / 'dist')
    parser.add_argument('--topology', type=Path, required=True)
    parser.add_argument('--route-targets', nargs='+', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--inventory-only', action='store_true')
    parser.add_argument('--service-metrics-host', help='SSH alias of the operated service host; reads its loopback metrics (read-only)')
    parser.add_argument('--collect-logs', action='store_true',
                        help='diagnostics: also run the owner daemon under the logged worker and save redacted daemon logs')
    parser.add_argument('--impairments', action='store_true',
                        help='also run forced-relay (namespace UDP block) and address-change drills')
    parser.add_argument('--journey', choices=('cli', 'tui'), default='cli',
                        help='onboard with CLI commands or the keyboard TUI on real PTYs')
    parser.add_argument('--three-host', action='store_true',
                        help="add the topology's third host (laptop) for forwarding/conflict/restore")
    parser.add_argument('--service-restart', metavar='SSH_ALIAS',
                        help='owner-approved: restart orbit-net on this host once during forced relay')
    parser.add_argument('--rehearsal-profile', type=Path)
    parser.add_argument('--rehearsal-roots', type=Path)
    try:
        run(parser.parse_args())
    except Exception as error:
        parser.exit(1, str(error) + '\n')
