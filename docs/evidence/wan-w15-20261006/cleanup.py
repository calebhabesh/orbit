"""Remove only invocation-recorded W15 roots after copying metrics/transcripts."""
import json,pathlib,subprocess,sys
repo=pathlib.Path(__file__).resolve().parents[3]
evidence=pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(repo/'scripts/validation'))
from wan_safety import validate_root

# The same guards run remotely; no signal and no existing workload is modified.
body='''import os,pathlib,shutil,sys
sys.path.insert(0,sys.argv[1])
from wan_safety import validate_root
roots=[validate_root(value) for value in sys.argv[2:]]
for root in roots:
 if not (root/'network.json').exists() and not root.name.startswith('orbit-w15-pi-assets-'):
  raise RuntimeError('allocated root has no topology record')
 for proc in pathlib.Path('/proc').iterdir():
  if not proc.name.isdecimal():continue
  try: argv=(proc/'cmdline').read_bytes().split(b'\\0')
  except (FileNotFoundError,PermissionError,ProcessLookupError):continue
  # Ignore this cleanup's own argv; all other matching processes block deletion.
  if int(proc.name)==os.getpid():continue
  if any(arg.startswith(os.fsencode(root)+b'/') for arg in argv):
   raise RuntimeError('fixture process still uses allocated root: '+proc.name)
for root in roots:
 validate_root(root)
 shutil.rmtree(root)
 print('removed private owned marked allocated root',root)
'''
local=[];remote=[]
for file in sorted(evidence.glob('*-invocation.json')):
 inv=json.loads(file.read_text())
 if inv.get('host')=='rpi':remote.extend([inv['assets'],inv['root']])
 else:local.append(inv['root'])
local=sorted(set(local));remote=sorted(set(remote))
if local:
 subprocess.run(['python3','-',str(repo/'scripts/validation'),*local],input=body,text=True,check=True)
if remote:
 # Safety module is already captured in the allocated asset directory.
 assets=next(json.loads(p.read_text())['assets'] for p in evidence.glob('pi-*-invocation.json'))
 subprocess.run(['ssh','-o','BatchMode=yes','rpi','python3','-',assets+'/validation',*remote],input=body,text=True,check=True)
