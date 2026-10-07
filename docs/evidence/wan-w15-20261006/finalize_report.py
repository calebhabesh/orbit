"""Index actual command records; unfinished and failed commands stay explicit."""
import json,pathlib,shlex
base=pathlib.Path(__file__).resolve().parent
commands=[]
for path in sorted(base.glob('*.json')):
 record=json.loads(path.read_text())
 if 'argv' not in record or 'started_at' not in record:
  if not isinstance(record,dict) or record.get('host') != 'rpi' or 'roots' not in record:continue
  invocation=json.loads((base/(path.stem+'-invocation.json')).read_text())
  record=dict(record,argv=['ssh','-o','BatchMode=yes','rpi','python3','-',invocation['assets'],invocation['root'],invocation['scenario'],invocation['suite']],duration_seconds=record['seconds'],outcome='exact worker/native result and binary hash retained; recorder UTC start/source digest not captured')
 log=base/'logs'/(path.stem+'.log')
 commands.append({'record':path.name,'outcome':record.get('outcome'),'command':shlex.join(record['argv']),'exit_code':record.get('exit_code'),'duration_seconds':record.get('duration_seconds'),'source_before':record.get('source_before'),'source_after':record.get('source_after'),'log':str(log.relative_to(base)) if log.exists() else None})
lines=['# W15 command records','','Rows link actual commands and transcripts. Local records include UTC timestamps\nand source digests; Pi invocation/native/binary records supply provenance, with\nuncaptured recorder UTC start/source digests stated explicitly.','Failed experiments are retained and receive no acceptance credit.','An absent exit code is still running/unexecuted, never a pass.','','| Record | Exit | Seconds | Transcript |','| --- | --- | --- | --- |']
for c in commands:
 lines.append(f"| [{c['record']}]({c['record']}) | {c['exit_code'] if c['exit_code'] is not None else ('interrupted' if c.get('outcome') else 'pending')} | {c['duration_seconds']} | [log]({c['log']}) |")
(base/'commands.md').write_text('\n'.join(lines)+'\n')
(base/'results.json').write_text(json.dumps({'packet':'W15','status':'complete for recorded local/native/emulator/Pi acceptance' if all((base/(n+'.json')).exists() and json.loads((base/(n+'.json')).read_text()).get('exit_code')==0 for n in ['aggregate-final','full-race-final','closeout']) else 'in progress','commands':commands,'limitations':['WG6 open: authority backup and working alert delivery deferred; no wider distribution/W16/W17 acceptance','Physical WAN/N10 is W16, unexecuted here','Inherited T13 technical lifecycle and P17 owner use/explanation remain incomplete','Native expired immutable bundled profile unexecuted','Process SIGKILL does not prove power-loss durability','Small simulated NAT matrix does not imply universal reachability','Resource samples may miss peaks; laptop race and Pi non-race results differ']},indent=2)+'\n')
print(len(commands),'actual command records indexed')
