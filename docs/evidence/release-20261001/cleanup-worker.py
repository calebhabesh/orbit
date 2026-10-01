from pathlib import Path
from datetime import datetime,timezone
import hashlib,json,os,re,shutil,stat,subprocess

# PLAN is supplied from the preceding read-only inventory by the caller.
home=Path.home()
preserve={'/home/owner/filesync-validation-7u9wrjxf','/home/owner/filesync-validation-gkgpluz5'}
deleted=[];skipped=[]
def active(root):
    for p in Path('/proc').iterdir():
        if not p.name.isdigit() or int(p.name)==os.getpid():continue
        try:
            if p.stat().st_uid!=os.getuid():continue
            args=(p/'cmdline').read_bytes().split(b'\0')
            if any(str(root).encode() in arg for arg in args):return int(p.name)
            paths=[]
            try:paths.append(os.readlink(p/'cwd'))
            except OSError:pass
            try:
                for fd in (p/'fd').iterdir():
                    try:paths.append(os.readlink(fd))
                    except OSError:pass
            except OSError:pass
            if any(v==str(root) or v.startswith(str(root)+'/') for v in paths):return int(p.name)
        except (OSError,ProcessLookupError):continue
    return None
for row in PLAN['roots']:
    root=Path(row['root'])
    if str(root) in preserve or row.get('active_pids') or row.get('registered_units'):
        skipped.append({'root':str(root),'reason':'active or explicitly preserved'});continue
    try:
        assert root.parent==home and root.name.startswith('filesync-validation-') and root.resolve()==root
        info=root.lstat()
        assert stat.S_ISDIR(info.st_mode) and info.st_uid==os.getuid()
        assert not (root/'.filesync-pilot').exists()
        marker=root/'.filesync-disposable';m=marker.lstat()
        assert stat.S_ISREG(m.st_mode) and m.st_nlink==1 and m.st_uid==os.getuid()
        assert re.fullmatch('[a-f0-9]{32}',marker.read_text().strip())
        marker_hash=hashlib.sha256(marker.read_bytes()).hexdigest()
        for directory,dirs,files in os.walk(root,followlinks=False):
            assert Path(directory).lstat().st_dev==info.st_dev,'nested filesystem'
            for name in dirs:
                child=Path(directory)/name
                assert not child.is_symlink() and child.lstat().st_dev==info.st_dev,'symlink or nested filesystem'
        pid=active(root)
        if pid:raise RuntimeError(f'active process {pid}')
        for unit in (home/'.config/systemd/user').glob('filesync-validation-*.service'):
            if unit.is_symlink() and (unit.resolve()==root or root in unit.resolve().parents):
                raise RuntimeError('registered service')
        size=int(subprocess.check_output(['du','-x','-B1','-s',str(root)],text=True).split()[0])
        assert hashlib.sha256(marker.read_bytes()).hexdigest()==marker_hash
        assert root.lstat().st_ino==info.st_ino and shutil.rmtree.avoids_symlink_attacks
        shutil.rmtree(root)
        deleted.append({'root':str(root),'allocated_bytes':size,'disposable_marker_validated':True,'no_active_process':True,'no_registered_service':True})
    except Exception as error:skipped.append({'root':str(root),'reason':str(error)})
disk=os.statvfs(home)
print(json.dumps({'utc':datetime.now(timezone.utc).isoformat(),'host':os.uname().nodename,'home':str(home),
    'deleted':deleted,'skipped':skipped,'allocated_bytes_removed':sum(x['allocated_bytes'] for x in deleted),
    'available_bytes_after':disk.f_bavail*disk.f_frsize,'pilot_folders_preserved':True,'release_reports_preserved':True},indent=2))
