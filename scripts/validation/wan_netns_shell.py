#!/usr/bin/env python3
"""W16 namespace shell for a host where the runner has no sudo (the laptop).

The owner starts this once, as their own user, inside the W16 namespace:

    sudo ip netns exec orbit-w16 sudo -u "$USER" python3 wan_netns_shell.py SOCKET

It listens on a private Unix socket (mode 0600 in a 0700 directory). Each
connection carries {"worker": <python source>, "request": <json>}; the shell
runs that worker with the request on stdin and returns "<exit code>\\n<stdout>".
Workers inherit the namespace, so daemons they launch use only its veth. The
shell runs as the connecting user's own account and grants no privilege beyond
that user's SSH login; it exits after --idle seconds without a request.
"""
import argparse
import json
import os
from pathlib import Path
import socket
import subprocess
import sys


def serve(path, idle):
    path = Path(path)
    if path.parent.stat().st_mode & 0o077 or path.parent.stat().st_uid != os.getuid():
        raise SystemExit('socket directory must be private and owned by this user')
    if path.exists() or path.is_symlink():
        path.unlink()
    server = socket.socket(socket.AF_UNIX)
    old = os.umask(0o177)
    try:
        server.bind(str(path))
    finally:
        os.umask(old)
    server.listen(4)
    server.settimeout(idle)
    try:
        while True:
            try:
                conn, _ = server.accept()
            except socket.timeout:
                return
            with conn:
                creds = conn.getsockopt(socket.SOL_SOCKET, socket.SO_PEERCRED, 12)
                if int.from_bytes(creds[4:8], sys.byteorder) != os.getuid():
                    continue
                conn.settimeout(30)
                data = b''.join(iter(lambda: conn.recv(1 << 16), b''))
                conn.settimeout(None)
                try:
                    message = json.loads(data)
                    result = subprocess.run([sys.executable, '-c', message['worker']],
                                            input=json.dumps(message['request']), capture_output=True,
                                            text=True, timeout=310, cwd=Path.home())
                    reply = f'{result.returncode}\n{result.stdout}'
                except Exception as error:
                    reply = '1\n' + json.dumps({'error': 'namespace shell: ' + type(error).__name__})
                conn.sendall(reply.encode())
    finally:
        server.close()
        path.unlink(missing_ok=True)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('socket')
    parser.add_argument('--idle', type=float, default=4 * 3600, help='exit after this many idle seconds')
    args = parser.parse_args()
    serve(args.socket, args.idle)
