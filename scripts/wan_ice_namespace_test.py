#!/usr/bin/env python3
"""W10 actual ICE fixture inside a new disposable network namespace only."""
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
    args = parser.parse_args()
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
        ['ip', 'link', 'add', 'orbit-w10', 'type', 'dummy'],
        ['ip', 'link', 'set', 'orbit-w10', 'up', 'multicast', 'on'],
        ['ip', 'addr', 'add', '11.23.45.1/24', 'dev', 'orbit-w10'],
        ['ip', 'addr', 'add', '10.23.45.1/24', 'dev', 'orbit-w10'],
        ['ip', '-6', 'addr', 'add', '2606:4700:4700::1111/64', 'dev', 'orbit-w10', 'nodad'],
    ]
    for command in commands:
        subprocess.run(command, check=True, timeout=5)
    (root / 'network.json').write_text(json.dumps(dict(
        environment='new isolated user/network namespace; no external route',
        commands=commands, public_addresses='simulated; no native internet claim'), indent=2)+'\n')
    env = dict(os.environ, ORBIT_W10_PUBLIC_FIXTURE='isolated-marked-namespace', TMPDIR=str(root))
    subprocess.run([str(Path(args.test_binary).resolve(strict=True)),
                    '-test.run=^TestWANW10Isolated(ICEPeerSyncAndFallback|STUNIPv6)$', '-test.v'],
                   env=env, check=True, timeout=180)


if __name__ == '__main__':
    main()
