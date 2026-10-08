#!/usr/bin/env python3
"""T12 extracted package/standalone install checks in fresh marked roots.
No real user service changes: installation scripts see a fixture systemctl.
Optional container transactions use only newly created --rm containers and
read-only package mounts. No existing containers or services are inspected.
"""
import argparse
import gzip
import hashlib
import io
import json
import os
import platform
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import uuid

# Hermetic: never select the packaged hosted profile, so no daemon started here
# contacts the operated service (W14). Child processes inherit this.
os.environ.setdefault('ORBIT_DISABLE_PACKAGED_PROFILE', '1')



def run(argv, **kwargs):
    result = subprocess.run(list(map(str, argv)), capture_output=True, timeout=180, **kwargs)
    assert result.returncode == 0, (argv, result.returncode, (result.stdout+result.stderr).decode(errors='replace')[-4000:])
    return result.stdout


def deb_members(data):
    assert data[:8] == b'!<arch>\n'
    pos = 8
    while pos < len(data):
        header = data[pos:pos+60]
        size = int(header[48:58]); name = header[:16].decode().strip().rstrip('/')
        yield name, data[pos+60:pos+60+size]
        pos += 60 + size + size % 2


def rpm_payload(data):
    assert data[:4] == bytes.fromhex('edabeedb')
    pos = 96
    for number in range(2):
        assert data[pos:pos+3] == bytes.fromhex('8eade8')
        count = int.from_bytes(data[pos+8:pos+12], 'big')
        size = int.from_bytes(data[pos+12:pos+16], 'big')
        pos += 16 + count*16 + size
        if number == 0:
            pos = (pos+7)//8*8
    return gzip.decompress(data[pos:])


def unpack_cpio(data, target):
    pos = 0
    while True:
        header = data[pos:pos+110]; assert header[:6] == b'070701'
        fields = [int(header[6+i*8:14+i*8], 16) for i in range(13)]
        mode, size, namesize = fields[1], fields[6], fields[11]
        name = data[pos+110:pos+110+namesize-1].decode()
        pos = (pos+110+namesize+3)//4*4
        content = data[pos:pos+size]; pos = (pos+size+3)//4*4
        if name == 'TRAILER!!!': return
        path = target / name
        assert path.resolve().is_relative_to(target.resolve())
        path.parent.mkdir(parents=True, exist_ok=True)
        if mode & 0o170000 == 0o120000:
            assert '/' not in content.decode()
            path.symlink_to(content.decode())
        else:
            path.write_bytes(content); path.chmod(mode & 0o777)


def checked_root(root, token):
    assert root.is_absolute() and root.is_dir() and not root.is_symlink()
    assert (root/'.orbit-disposable').read_text() == token


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dist', default='dist')
    parser.add_argument('--output')
    parser.add_argument('--pty-output', help='new empty directory for sanitized package-extracted PTY frames')
    parser.add_argument('--containers', action='store_true')
    parser.add_argument('--emulate-arm64', action='store_true', help='use local multiarch QEMU image; never claim native arm64')
    args = parser.parse_args(); dist = Path(args.dist).resolve(strict=True)
    root = Path(tempfile.mkdtemp(prefix='orbit-t12-packages-')).resolve(); root.chmod(0o700)
    token = uuid.uuid4().hex; (root/'.orbit-disposable').write_text(token)
    results=[]
    try:
        for line in (dist/'SHA256SUMS').read_text().splitlines():
            digest, name = line.split('  ', 1)
            assert Path(name).name == name
            assert hashlib.sha256((dist/name).read_bytes()).hexdigest() == digest
        results.append({'scenario':'all-package-checksums', 'result':'passed'})
        qemu = None
        if args.emulate_arm64:
            qemu = root/'qemu-aarch64-static'
            cid = run(['docker','create','--label','orbit.disposable='+token,'multiarch/qemu-user-static:x86_64-aarch64','/usr/bin/qemu-aarch64-static','--version']).decode().strip()
            try:
                run(['docker','cp',cid+':/usr/bin/qemu-aarch64-static',qemu])
            finally:
                assert run(['docker','inspect','--format','{{index .Config.Labels "orbit.disposable"}}',cid]).decode().strip() == token
                run(['docker','rm',cid])
            qemu.chmod(0o700)
            print(run([qemu,'--version']).decode())
        # Execute the host architecture natively; the other runs under QEMU when
        # requested and is otherwise checked structurally.
        host = {'x86_64': 'amd64', 'aarch64': 'arm64'}.get(platform.machine(), platform.machine())
        for arch,rpmarch in [('amd64','x86_64'),('arm64','aarch64')]:
            for kind in ['tar','deb','rpm']:
                target=root/f'{arch}-{kind}';target.mkdir()
                if kind=='tar':
                    with tarfile.open(dist/f'orbit-v2.2.0-linux-{arch}.tar.gz') as archive:
                        archive.extractall(target, filter='data')
                    binary=target/'orbit'; share=target/'share';desktop=target/'desktop/orbit.desktop';unit=target/'systemd/orbit.service'
                elif kind=='deb':
                    members=dict(deb_members((dist/f'orbit_2.2.0_{arch}.deb').read_bytes()))
                    with tarfile.open(fileobj=io.BytesIO(members['data.tar.gz'])) as archive:
                        archive.extractall(target,filter='data')
                    binary=target/'usr/bin/orbit';share=target/'usr/share';desktop=share/'applications/orbit.desktop';unit=target/'usr/lib/systemd/user/orbit.service'
                else:
                    unpack_cpio(rpm_payload((dist/f'orbit-2.2.0-1.{rpmarch}.rpm').read_bytes()),target)
                    binary=target/'usr/bin/orbit';share=target/'usr/share';desktop=share/'applications/orbit.desktop';unit=target/'usr/lib/systemd/user/orbit.service'
                assert unit.is_file() and not unit.is_symlink(), 'single regular service unit'
                assert 'Terminal=true' in desktop.read_text() and 'Exec=orbit\n' in desktop.read_text()
                for name in ['bash-completion/completions/orbit','zsh/site-functions/_orbit','fish/vendor_completions.d/orbit.fish','doc/orbit/runbooks/terminal-operator.md']:
                    assert (share/name).stat().st_size > 0
                if arch==host or (qemu and arch=='arm64'):
                    prefix = [] if arch==host else [qemu]
                    state=root/f'state-{arch}-{kind}'
                    run([*prefix,binary,'init','--state',state])
                    original=(state/'config.json').read_bytes()
                    status=json.loads(run([*prefix,binary,'--state',state,'--json']))
                    assert status['state_directory']==str(state) and not status['service']['running']
                    run([*prefix,binary,'doctor','--state',state])
                    run([*prefix,binary,'completion','bash'])
                    assert (state/'config.json').read_bytes()==original
                results.append({'scenario':f'{arch}-{kind}-payload-and-entry','result':'passed','execution':f'native {arch}' if arch==host else ('QEMU emulation; not native' if qemu and arch=='arm64' else f'structure only; {arch} execution separate')})
        # Real standalone install, repeated upgrade and uninstall with a private HOME.
        target=root/f'{host}-tar';home=root/'home';home.mkdir(mode=0o700)
        fake=root/'fake-bin';fake.mkdir()
        (fake/'systemctl').write_text('#!/bin/sh\nexit 1\n');(fake/'systemctl').chmod(0o700)
        (fake/'update-desktop-database').write_text('#!/bin/sh\nexit 0\n');(fake/'update-desktop-database').chmod(0o700)
        env=dict(os.environ,HOME=str(home),XDG_STATE_HOME=str(home/'.local/state'),PATH=str(fake)+':'+os.environ['PATH'])
        run(['bash',target/'install.sh','user'],env=env)
        unit=home/'.config/systemd/user/orbit.service'
        custom=unit.read_bytes()+b'\n# operator-customized unit retained\n';unit.write_bytes(custom)
        binary=home/'.local/bin/orbit';state=home/'.local/state/orbit'
        run([binary,'init','--state',state],env=env)
        original=(state/'config.json').read_bytes()
        for attempt in range(2):
            run(['bash',target/'install.sh','user'],env=env)
            assert unit.read_bytes()==custom
            status=json.loads(run([binary,'--json'],env=env));assert status['state_directory']==str(state)
            assert (state/'config.json').read_bytes()==original
        working=home/'Notes';working.mkdir();(working/'keep').write_bytes(b'ordinary protected file')
        checked_root(root,token)
        run(['bash',target/'uninstall.sh','user'],env=env)
        assert (state/'config.json').read_bytes()==original and (working/'keep').read_bytes()==b'ordinary protected file'
        assert not binary.exists() and not binary.is_symlink()
        results.append({'scenario':'standalone-install-repeat-upgrade-uninstall','result':'passed','assertions':['custom unit preserved','identity/state preserved','ordinary bytes preserved','no user service changes']})
        # Run the production real-PTY lifetime oracle against package-extracted bytes.
        pty_args = ['python3',Path(__file__).with_name('terminal_pty_test.py'),'--binary',target/'orbit','--bare']
        if args.pty_output:
            pty_args += ['--output', args.pty_output]
        run(pty_args)
        results.append({'scenario':'package-extracted-bare-PTY','result':'passed'})
        if args.containers:
            for image,package,install,remove in [
                ('debian:bookworm-slim','orbit_2.2.0_amd64.deb','dpkg -i','dpkg -r orbit'),
                ('fedora:43','orbit-2.2.0-1.x86_64.rpm','rpm -i --nosignature','rpm -e orbit')]:
                script='''set -eu
same_bytes() { test "$(sha256sum "$1" | cut -d ' ' -f 1)" = "$(sha256sum "$2" | cut -d ' ' -f 1)"; }
mkdir -m 700 /tmp/orbit-t12
printf 'disposable container\\n' > /tmp/orbit-t12/.orbit-disposable
export HOME=/tmp/orbit-t12
export XDG_STATE_HOME=$HOME/.local/state
export ORBIT_DISABLE_PACKAGED_PROFILE=1
INSTALL /packages/PACKAGE
orbit init --state "$HOME/.local/state/orbit"
cp "$HOME/.local/state/orbit/config.json" "$HOME/identity-before"
mkdir -m 700 "$HOME/Notes"
printf 'protected package journey\n' > "$HOME/Notes/notes.txt"
orbit setup --root "$HOME/Notes" --name Notes --label Packaged --preview --review-file "$HOME/setup.json"
orbit setup --request-file "$HOME/setup.json" --timeout 10
cd "$HOME/Notes"
orbit history notes.txt --state "$HOME/.local/state/orbit" --folder Notes --json > "$HOME/history-before"
grep -q 'digest' "$HOME/history-before"
orbit stop --state "$HOME/.local/state/orbit"
orbit --json
orbit doctor --state "$HOME/.local/state/orbit"
INSTALL /packages/PACKAGE
same_bytes "$HOME/identity-before" "$HOME/.local/state/orbit/config.json"
orbit history notes.txt --state "$HOME/.local/state/orbit" --folder Notes --json > "$HOME/history-after"
same_bytes "$HOME/history-before" "$HOME/history-after"
test -f /tmp/orbit-t12/.orbit-disposable
REMOVE
same_bytes "$HOME/identity-before" "$HOME/.local/state/orbit/config.json"
test ! -e /usr/bin/orbit
grep -q 'protected package journey' "$HOME/Notes/notes.txt"
test -f "$HOME/.local/state/orbit/metadata.sqlite"
'''.replace('INSTALL',install).replace('PACKAGE',package).replace('REMOVE',remove)
                if image.startswith('fedora'):
                    script=script.replace('rpm -i --nosignature /packages/', 'rpm -U --replacepkgs --nosignature /packages/')
                output=run(['docker','run','--rm','--label','orbit.disposable='+token,'--mount',f'type=bind,source={dist},target=/packages,readonly',image,'sh','-c',script])
                print(output.decode(errors='replace'))
                results.append({'scenario':image+'-install-reinstall-remove','result':'passed','limitations':'no boot/logout/systemd user manager'})
        if args.output:
            Path(args.output).write_text(json.dumps(results,indent=2)+'\n')
        print(json.dumps(results,indent=2))
    finally:
        checked_root(root,token);shutil.rmtree(root)


if __name__=='__main__':main()
