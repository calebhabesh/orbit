"""W15 namespace safety, kept separate so refusal tests never mutate networking."""
import fcntl
import os
from pathlib import Path
import stat

MARKER = '.orbit-disposable'
TOKEN = 'orbit W15 disposable network/process campaign\n'


def validate_root(value):
    root = Path(value)
    if not root.is_absolute() or '..' in root.parts or root.resolve(strict=True) != root:
        raise RuntimeError('requires canonical absolute disposable root')
    if root.parent != Path('/tmp') or not root.name.startswith('orbit-w15-'):
        raise RuntimeError('requires newly allocated /tmp/orbit-w15-* root; personal roots refused')
    info = root.lstat()
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.getuid() or info.st_mode & 0o077:
        raise RuntimeError('requires private owned directory')
    marker = root / MARKER
    info = marker.lstat()
    if not stat.S_ISREG(info.st_mode) or info.st_uid != os.getuid() or info.st_nlink != 1 or info.st_mode & 0o077:
        raise RuntimeError('requires private owned single-link marker')
    if marker.read_text() != TOKEN or (root / '.orbit-pilot').exists():
        raise RuntimeError('disposable token mismatch or personal pilot')
    return root


def validate_namespace(parent):
    current = os.readlink('/proc/self/ns/net')
    if not parent or current == parent:
        raise RuntimeError('refusing parent network namespace')
    fields = Path('/proc/self/uid_map').read_text().split()
    if os.geteuid() != 0 or len(fields) != 3 or fields[0] != '0' or fields[1] == '0' or fields[2] != '1':
        raise RuntimeError('requires a single remapped uid in disposable user namespace')
    with open('/proc/self/ns/net', 'rb') as net:
        owner = fcntl.ioctl(net.fileno(), 0xb701)  # NS_GET_USERNS
        try:
            if os.fstat(owner).st_ino != os.stat('/proc/self/ns/user').st_ino:
                raise RuntimeError('network namespace has foreign owner')
        finally:
            os.close(owner)


def validate_child(process, root):
    """No arbitrary PID API: accept only a still-running Popen child in this run."""
    validate_root(root)
    if process.poll() is not None:
        raise RuntimeError('child already exited')
    pid = process.pid
    fields = Path(f'/proc/{pid}/stat').read_text().rsplit(')', 1)[1].split()
    argv = Path(f'/proc/{pid}/cmdline').read_bytes().split(b'\0')
    if int(fields[1]) != os.getpid() or argv[0] != os.fsencode(root / 'terminal.test'):
        raise RuntimeError('refusing unrelated process')
    if os.readlink(f'/proc/{pid}/ns/net') != os.readlink('/proc/self/ns/net'):
        raise RuntimeError('child has foreign network namespace')


def validate_transcript(suite, text):
    import re
    required = {
        'crash': ['TestWANW15WholeDaemonCrashRouteAndReceiptRecovery'],
        'fairness': ['TestWANW11ActualLargeSmallPeerBandwidthAcrossRoutes'],
        'enrollment': ['TestWANW05DaemonSIGKILLRoutedEnrollmentBoundaries'],
        'ice': ['TestWANW10IsolatedICEPeerSyncAndFallback', 'TestWANW10IsolatedSTUNIPv6'],
        'public': ['TestWANW08IsolatedPublicTCPAndIPv6'],
    }[suite]
    for name in required:
        if not re.search(r'^--- PASS: ' + re.escape(name) + r' \(', text, re.M):
            raise RuntimeError('required suite missing, skipped or failed: ' + name)
