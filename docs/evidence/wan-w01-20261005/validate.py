import subprocess,pathlib,json,os,time
root=pathlib.Path('docs/evidence/wan-w01-20261005')
commands=[
 ('discovery',['go','test','-list','^TestWANW01','./internal/...','./model/...','./tests/...','./cmd/filesync/...']),
 ('focused',['go','test','-count=1','-timeout','60s','-run','^TestWANW01','-v','./internal/network','./internal/protocol','./internal/replication','./internal/control/terminalcontract','./model']),
 ('focused-race',['go','test','-race','-count=1','-timeout','90s','-run','^TestWANW01','-v','./internal/network','./internal/protocol','./internal/replication','./internal/control/terminalcontract','./model']),
 ('compatibility',['go','test','-count=1','./cmd/filesync/...','./internal/control/...','./internal/replication/...','./internal/protocol/...','./tests/terminal','-run','TestTerminalT0[1345]|TestWAN|TestTerminalT01|TestEnrollment|TestReplication|TestClient|TestIdentity|TestControl']),
 ('all-unit',['go','test','-count=1','./internal/...','./model/...','./cmd/filesync/...']),
 ('vet',['go','vet','./...']),
 ('compile-amd64',['go','build','./...']),
 ('compile-arm64',['go','build','./...']),
]
results=[]
with (root/'commands.md').open('w') as f:
 f.write('# W01 validation commands\n\nAll runs in repository root. Local disposable test roots/listeners only.\nInitial reads, dependency/API/license audit and fixture generator are summarized\nin dependency.md and summary.md. Earlier failing relay reproduction retained\nin transport-first-failure.log.\n\n')
 for name,cmd in commands:
  env=os.environ.copy()
  if name.startswith('compile-'):env.update(CGO_ENABLED='0',GOOS='linux',GOARCH=name.split('-')[1])
  shown=('CGO_ENABLED=0 GOOS=linux GOARCH='+env['GOARCH']+' ' if name.startswith('compile-') else '')+' '.join(cmd)
  print('RUN '+shown,flush=True);start=time.monotonic()
  with (root/(name+'.log')).open('w') as log:p=subprocess.run(cmd,stdout=log,stderr=subprocess.STDOUT,env=env)
  result={'name':name,'command':shown,'exit_code':p.returncode,'seconds':round(time.monotonic()-start,3),'log':name+'.log'}
  results.append(result);f.write('```sh\n'+shown+'\n```\n\nExit '+str(p.returncode)+'; ['+name+'.log]('+name+'.log).\n\n');f.flush()
  (root/'results.json').write_text(json.dumps(results,indent=2)+'\n');print('EXIT '+str(p.returncode),flush=True)
  if p.returncode:break
