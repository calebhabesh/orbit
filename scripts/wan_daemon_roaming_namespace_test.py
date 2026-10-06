#!/usr/bin/env python3
"""W11 whole-daemon roaming campaign inside a new disposable network namespace only."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import subprocess


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--test-binary', required=True)
    parser.add_argument('--root', required=True)
    parser.add_argument('--parent-namespace', required=True)
    parser.add_argument('--default-timing', action='store_true')
    parser.add_argument('--latency-ms', type=int, default=0)
    parser.add_argument('--loss-percent', type=float, default=0)
    args = parser.parse_args()
    assert 0 <= args.latency_ms <= 100 and 0 <= args.loss_percent <= 5
    root = Path(args.root)
    assert root.is_absolute() and root.resolve(strict=True) == root
    assert not root.is_symlink() and (root / '.filesync-disposable').is_file() and not (root / '.filesync-disposable').is_symlink()
    assert root.stat().st_mode & 0o077 == 0, 'disposable root must be private'
    assert os.readlink('/proc/self/ns/net') != args.parent_namespace, 'refusing host network namespace'
    net_fd = os.open('/proc/self/ns/net', os.O_RDONLY)
    try:
        owner_fd = fcntl.ioctl(net_fd, 0xb701)  # Linux NS_GET_USERNS
        try:
            assert os.fstat(owner_fd).st_ino == os.stat('/proc/self/ns/user').st_ino, 'network namespace must belong to this disposable user namespace'
        finally:
            os.close(owner_fd)
    finally:
        os.close(net_fd)
    assert os.geteuid() == 0 and Path('/proc/self/uid_map').read_text().split()[:2] != ['0', '0'], 'requires disposable user namespace'
    commands = [
        ['ip', 'link', 'set', 'lo', 'up'],
        ['ip', 'link', 'add', 'orbit-w11', 'type', 'dummy'],
        ['ip', 'link', 'set', 'orbit-w11', 'up', 'multicast', 'on'],
        ['ip', 'addr', 'add', '11.23.45.1/24', 'dev', 'orbit-w11'],
        ['ip', 'addr', 'add', '10.23.45.1/24', 'dev', 'orbit-w11'],
        ['ip', '-6', 'addr', 'add', '2606:4700:4700::1111/64', 'dev', 'orbit-w11', 'nodad'],
    ]
    for command in commands:
        subprocess.run(command, check=True, timeout=5)
    if args.latency_ms or args.loss_percent:
        impairments = [
            ['tc', 'qdisc', 'add', 'dev', 'lo', 'root', 'handle', '1:', 'prio', 'bands', '3', 'priomap'] + ['2'] * 16,
            ['tc', 'qdisc', 'add', 'dev', 'lo', 'parent', '1:3', 'handle', '30:', 'netem', 'delay', str(args.latency_ms)+'ms', 'loss', str(args.loss_percent)+'%'],
            ['tc', 'filter', 'add', 'dev', 'lo', 'protocol', 'ip', 'parent', '1:', 'prio', '1', 'u32', 'match', 'ip', 'dst', '127.0.0.0/8', 'flowid', '1:1'],
            ['tc', 'filter', 'add', 'dev', 'lo', 'protocol', 'ipv6', 'parent', '1:', 'prio', '2', 'u32', 'match', 'ip6', 'dst', '::1/128', 'flowid', '1:1'],
        ]
        for command in impairments:
            subprocess.run(command, check=True, timeout=5)
            commands.append(command)
    (root / 'network.json').write_text(json.dumps(dict(
        environment='new isolated user/network namespace; no external route',
        commands=commands, public_addresses='simulated; no native internet claim'), indent=2)+'\n')
    env = dict(os.environ, ORBIT_W11_NAMESPACE='isolated-marked-namespace', ORBIT_W11_PARENT_NETNS=args.parent_namespace, TMPDIR=str(root))
    if args.default_timing:
        env['ORBIT_W11_DEFAULT_TIMING'] = '1'
    subprocess.run([str(Path(args.test_binary).resolve(strict=True)),
                    '-test.run=^TestWANW11WholeDaemonRoamingAndMixedProgress$', '-test.v', '-test.timeout=8m'],
                   env=env, check=True, timeout=500)


if __name__ == '__main__':
    main()
