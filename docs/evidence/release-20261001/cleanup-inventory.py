from pathlib import Path
import json,os,stat,subprocess

home=Path.home()
roots=sorted(home.glob('filesync-validation-*'))
processes=[]
for proc in Path('/proc').iterdir():
    if not proc.name.isdigit() or int(proc.name)==os.getpid(): continue
    try:
        if proc.stat().st_uid!=os.getuid(): continue
        args=(proc/'cmdline').read_bytes().split(b'\0')
        paths=[]
        try: paths.append(os.readlink(proc/'cwd'))
        except OSError: pass
        try:
            for fd in (proc/'fd').iterdir():
                try: paths.append(os.readlink(fd))
                except OSError: pass
        except OSError: pass
        processes.append((int(proc.name),args,paths))
    except (OSError,ProcessLookupError): continue
units=list((home/'.config/systemd/user').glob('filesync-validation-*.service'))
items=[]
for root in roots:
    entry={'root':str(root),'disposable_marker':False,'pilot_marker':(root/'.filesync-pilot').exists()}
    marker=root/'.filesync-disposable'
    try:
        info=marker.lstat()
        entry['disposable_marker']=stat.S_ISREG(info.st_mode) and info.st_nlink==1 and info.st_uid==os.getuid() and bool(marker.read_text().strip())
        entry['root_is_real_directory']=root.resolve()==root and stat.S_ISDIR(root.lstat().st_mode) and root.stat().st_uid==os.getuid()
        entry['allocated_bytes']=int(subprocess.check_output(['du','-x','-B1','-s',str(root)],text=True).split()[0])
        entry['active_pids']=[pid for pid,args,paths in processes if any(str(root).encode() in arg for arg in args) or any(p==str(root) or p.startswith(str(root)+'/') for p in paths)]
        entry['registered_units']=[u.name for u in units if u.is_symlink() and (u.resolve()==root or root in u.resolve().parents)]
    except OSError as error:entry['error']=str(error)
    items.append(entry)
disk=os.statvfs(home)
print(json.dumps({'host':os.uname().nodename,'home':str(home),'available_bytes':disk.f_bavail*disk.f_frsize,'roots':items},indent=2))
