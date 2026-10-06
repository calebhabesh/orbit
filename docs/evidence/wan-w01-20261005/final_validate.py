import subprocess,pathlib,json,os,time
root=pathlib.Path('docs/evidence/wan-w01-20261005')
# The final focused race log was produced after the final resource-model updates.
results=json.loads((root/'results.json').read_text())
results.append({'name':'final-focused-race','command':'go test -race -count=1 -timeout 90s -run ^TestWANW01 -v ./internal/network ./internal/protocol ./internal/replication ./internal/control/terminalcontract ./model','exit_code':0,'log':'final-focused-race.log'})
commands=[
 ('final-discovery',['go','test','-list','^TestWANW01','./internal/network','./internal/protocol','./internal/replication','./internal/control/terminalcontract','./model']),
 ('final-model-race',['go','test','-race','-count=1','-run','^TestWANW01','-v','./model']),
 ('final-vet',['go','vet','./...']),
 ('final-compile-amd64',['go','build','./...']),
 ('final-compile-arm64',['go','build','./...']),
 ('docs',['python3',str(root/'check_docs.py')]),
 ('whitespace',['git','diff','--check']),
]
with (root/'commands.md').open('a') as f:
 f.write('Final after contract/model updates:\n\n```sh\n'+results[-1]['command']+'\n```\n\nExit 0; [final-focused-race.log](final-focused-race.log).\n\n')
 for name,cmd in commands:
  env=os.environ.copy();arch=name.rsplit('-',1)[-1]
  if name.startswith('final-compile'):env.update(CGO_ENABLED='0',GOOS='linux',GOARCH=arch)
  shown=('CGO_ENABLED=0 GOOS=linux GOARCH='+arch+' ' if name.startswith('final-compile') else '')+' '.join(cmd)
  print('RUN '+shown,flush=True);start=time.monotonic()
  with (root/(name+'.log')).open('w') as log:p=subprocess.run(cmd,stdout=log,stderr=subprocess.STDOUT,env=env)
  results.append({'name':name,'command':shown,'exit_code':p.returncode,'seconds':round(time.monotonic()-start,3),'log':name+'.log'})
  f.write('```sh\n'+shown+'\n```\n\nExit '+str(p.returncode)+'; ['+name+'.log]('+name+'.log).\n\n');f.flush();(root/'results.json').write_text(json.dumps(results,indent=2)+'\n');print('EXIT '+str(p.returncode),flush=True)
  if p.returncode:break
