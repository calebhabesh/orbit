import json, os, pathlib, subprocess, sys, time
repo = pathlib.Path(__file__).resolve().parents[3]
evidence = pathlib.Path(__file__).resolve().parent
name, scenario, suite = sys.argv[1:4]
setup = '''import tempfile,pathlib,json
roots=[]
for label in ['assets','run']:
 p=pathlib.Path(tempfile.mkdtemp(prefix='orbit-w15-pi-'+label+'-',dir='/tmp'))
 (p/'.filesync-disposable').write_text('orbit W15 disposable network/process campaign\\n')
 (p/'.filesync-disposable').chmod(0o600)
 roots.append(str(p))
(pathlib.Path(roots[0])/'validation').mkdir(mode=0o700)
print(json.dumps(roots))
'''
roots=json.loads(subprocess.check_output(['ssh','-o','BatchMode=yes','-o','ConnectTimeout=5','rpi','python3','-'],input=setup,text=True))
assets,root=roots
for source,dest in [('scripts/wan_failure_campaign.py','wan_failure_campaign.py'),('scripts/validation/wan_safety.py','validation/wan_safety.py'),('/tmp/orbit-w15-terminal-arm64.test','input.test')]:
 subprocess.run(['scp','-q',source,'rpi:'+assets+'/'+dest],check=True)
run = '''import os,pathlib,subprocess,sys
os.environ['PATH']='/usr/sbin:/usr/bin:/sbin:/bin'
argv=['unshare','--user','--map-root-user','--net','python3',sys.argv[1]+'/wan_failure_campaign.py','--root',sys.argv[2],'--parent-namespace',os.readlink('/proc/self/ns/net'),'--test-binary',sys.argv[1]+'/input.test','--scenario',sys.argv[3],'--suite',sys.argv[4]]
print(argv,flush=True)
sys.exit(subprocess.call(argv))
'''
(evidence/(name+'-invocation.json')).write_text(json.dumps({'host':'rpi','assets':assets,'root':root,'scenario':scenario,'suite':suite,'remote_python':run},indent=2)+'\n')
start=time.monotonic()
with (evidence/'logs'/(name+'.log')).open('w') as log:
 process=subprocess.Popen(['ssh','-o','BatchMode=yes','rpi','python3','-',assets,root,scenario,suite],stdin=subprocess.PIPE,stdout=log,stderr=subprocess.STDOUT,text=True)
 process.communicate(run,timeout=1000)
 code=process.returncode
metrics=evidence/'logs'/('metrics-'+name)
metrics.mkdir(exist_ok=True)
for pattern in ['network.json','result.json','daemon-metrics-*.jsonl']:
 result=subprocess.run(['scp','-q','rpi:'+root+'/'+pattern,str(metrics if pattern.startswith('daemon-') else evidence/'logs')],capture_output=True,text=True)
 if result.returncode and pattern!='daemon-metrics-*.jsonl': print(result.stderr)
# Prefix topology/results since multiple fixtures run separately.
for base in ['network.json','result.json']:
 p=evidence/'logs'/base
 if p.exists(): p.rename(evidence/(name+'-'+base))
(evidence/(name+'.json')).write_text(json.dumps({'exit_code':code,'seconds':time.monotonic()-start,'host':'rpi','roots':roots},indent=2)+'\n')
print('Pi',name,'exit',code)
print((evidence/'logs'/(name+'.log')).read_text()[-3500:])
sys.exit(code)
