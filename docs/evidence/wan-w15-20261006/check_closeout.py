"""Check final acceptance records, display whitespace and cleaned root provenance."""
import json,pathlib,subprocess
base=pathlib.Path(__file__).resolve().parent
repo=base.parents[2]
source=json.loads((base/'final-source.json').read_text())['tested_source_digest']
for name in ['aggregate-final','full-race-final','security-final','impaired-final','mtu-final','native-ice-final','local-only-startup-final','demo-packages-final','preservation-closeout','docs-links-closeout']:
 record=json.loads((base/(name+'.json')).read_text())
 assert record['exit_code']==0,(name,'not successful')
 assert record['source_before']==source and record['source_after']==source,(name,'source drift')
for name in ['pi-impaired-runtime-fixed','pi-recovery','pi-fairness','local-fairness','local-enrollment','fuzz-codec','fuzz-signed-profile','fuzz-envelope','fuzz-path','harness-runtime-final']:
 record=json.loads((base/(name+'.json')).read_text())
 assert record['exit_code']==0,(name,'not successful')
violations=[]
for path in base.rglob('*'):
 if not path.is_file() or path.suffix=='.gz' or '__pycache__' in path.parts:continue
 try:lines=path.read_text().splitlines()
 except UnicodeDecodeError:continue
 for number,line in enumerate(lines,1):
  if line.rstrip(' \t')!=line:violations.append((str(path.relative_to(base)),number))
assert not violations,violations[:20]
subprocess.run(['git','diff','--check'],cwd=repo,check=True)
roots=[json.loads(p.read_text()) for p in base.glob('*-invocation.json')]
assert json.loads((base/'cleanup.json').read_text())['exit_code']==0
for invocation in roots:
 if invocation.get('host')!='rpi':assert not pathlib.Path(invocation['root']).exists(),('uncleaned root',invocation['root'])
print(json.dumps({'acceptance':True,'source':source,'display_whitespace':True,'git_diff_check':True,'native_invocations':len(roots),'cleanup_record':'cleanup.json'},indent=2))
