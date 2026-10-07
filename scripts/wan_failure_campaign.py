#!/usr/bin/env python3
"""W15 native sockets/process faults in a new, marked user/network namespace.

Run only using the documented unshare command. Refusals never touch networking.
"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time

sys.path.insert(0, str(Path(__file__).resolve().parent / 'validation'))
from wan_safety import validate_child, validate_namespace, validate_root, validate_transcript


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--root', required=True)
    parser.add_argument('--parent-namespace', required=True)
    parser.add_argument('--test-binary', required=True)
    parser.add_argument('--suite', choices=['crash', 'fairness', 'enrollment', 'ice', 'public'], default='crash')
    parser.add_argument('--scenario', choices=['recovery', 'impaired', 'mtu'], default='recovery')
    args = parser.parse_args()
    root = validate_root(args.root)
    validate_namespace(args.parent_namespace)
    source = Path(args.test_binary)
    if not source.is_absolute() or source.resolve(strict=True) != source or not source.is_file():
        raise RuntimeError('requires canonical test executable')
    target = root / 'terminal.test'
    # Never replace prior executable/transcripts, and never accept an arbitrary PID.
    if any(p.name != '.filesync-disposable' for p in root.iterdir()):
        raise RuntimeError('requires fresh disposable root; prior evidence preserved')
    for tool in ('ip', 'tc', 'iptables', 'ip6tables'):
        if not shutil.which(tool):
            raise RuntimeError('missing privilege prerequisite tool: ' + tool)
    with target.open('xb') as out, source.open('rb') as inp:
        shutil.copyfileobj(inp, out)
    target.chmod(0o700)
    interface = {'ice': 'orbit-w10', 'public': 'orbit-w08'}.get(args.suite, 'orbit-w11')
    commands = [
        ['ip', 'link', 'set', 'lo', 'up'],
        ['ip', 'link', 'add', interface, 'type', 'dummy'],
        ['ip', 'link', 'set', interface, 'up', 'multicast', 'on'],
        ['ip', 'addr', 'add', '11.23.45.1/24', 'dev', interface],
        ['ip', 'addr', 'add', '10.23.45.1/24', 'dev', interface],
        ['ip', '-6', 'addr', 'add', '2606:4700:4700::1111/64', 'dev', interface, 'nodad'],
    ]
    if args.scenario == 'mtu':
        commands += [['ip', 'link', 'set', 'lo', 'mtu', '1280'],
                     ['ip', 'link', 'set', interface, 'mtu', '1280']]
    if args.scenario == 'impaired':
        commands += [
            ['tc', 'qdisc', 'add', 'dev', 'lo', 'root', 'handle', '1:', 'prio', 'bands', '3', 'priomap'] + ['2'] * 16,
            ['tc', 'qdisc', 'add', 'dev', 'lo', 'parent', '1:3', 'handle', '30:', 'netem', 'delay', '25ms', '5ms', 'loss', '1%', 'reorder', '5%', '50%', 'rate', '20mbit', 'limit', '1000'],
            ['tc', 'filter', 'add', 'dev', 'lo', 'protocol', 'ip', 'parent', '1:', 'prio', '1', 'u32', 'match', 'ip', 'dst', '127.0.0.0/8', 'flowid', '1:1'],
            ['tc', 'filter', 'add', 'dev', 'lo', 'protocol', 'ipv6', 'parent', '1:', 'prio', '2', 'u32', 'match', 'ip6', 'dst', '::1/128', 'flowid', '1:1'],
        ]
    record = dict(scenario=args.scenario, suite=args.suite, environment='new isolated user/network namespace; no external route',
                  parent=args.parent_namespace, namespace=os.readlink('/proc/self/ns/net'), commands=commands,
                  oracle='exact immutable heads/authors/manifests/bytes, persistent identity/pin, durable readiness and receipts',
                  topology='simulated addresses on dummy interface; native sockets; no physical WAN claim')
    (root / 'network.json').write_text(json.dumps(record, indent=2) + '\n')
    for command in commands:
        validate_root(root)
        validate_namespace(args.parent_namespace)
        subprocess.run(command, check=True, timeout=5)
    env = dict(os.environ, ORBIT_W15_NAMESPACE='isolated-marked-namespace',
               ORBIT_W11_NAMESPACE='isolated-marked-namespace', ORBIT_W11_PARENT_NETNS=args.parent_namespace,
               ORBIT_DISABLE_PACKAGED_PROFILE='1', ORBIT_W10_PUBLIC_FIXTURE='isolated-marked-namespace',
               ORBIT_W08_PUBLIC_FIXTURE='isolated-marked-namespace', TMPDIR=str(root))
    pattern = {'crash': '^TestWANW15WholeDaemonCrashRouteAndReceiptRecovery$',
               'fairness': '^TestWANW11ActualLargeSmallPeerBandwidthAcrossRoutes$',
               'enrollment': '^TestWANW05DaemonSIGKILLRoutedEnrollmentBoundaries$',
               'ice': '^TestWANW10Isolated(ICEPeerSyncAndFallback|STUNIPv6)$',
               'public': '^TestWANW08IsolatedPublicTCPAndIPv6$'}[args.suite]
    start = time.monotonic()
    with (root / 'campaign.log').open('x') as log:
        process = subprocess.Popen([str(target), '-test.run='+pattern,
                                    '-test.v', '-test.timeout=12m'], env=env, stdout=log, stderr=subprocess.STDOUT)
        try:
            code = process.wait(timeout=750)
        except subprocess.TimeoutExpired:
            validate_child(process, root)
            process.terminate()
            try:
                process.wait(timeout=20)
            except subprocess.TimeoutExpired:
                validate_child(process, root)
                process.kill()
                process.wait(timeout=10)
            code = 124
    if code == 0:
        try:
            validate_transcript(args.suite, (root / 'campaign.log').read_text())
        except RuntimeError as error:
            record['unexecuted'] = str(error)
            code = 2
    record.update(exit_code=code, seconds=time.monotonic()-start)
    (root / 'result.json').write_text(json.dumps(record, indent=2)+'\n')
    print((root / 'campaign.log').read_text())
    print(json.dumps(record))
    return code


if __name__ == '__main__':
    sys.exit(main())
