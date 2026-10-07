import json, os, pathlib, subprocess, sys, tempfile
repo = pathlib.Path(__file__).resolve().parents[3]
evidence = pathlib.Path(__file__).resolve().parent
sys.path.insert(0, str(repo/'scripts/validation'))
from wan_safety import TOKEN, MARKER
name = sys.argv[1]
scenario = sys.argv[2] if len(sys.argv)>2 else name
suite = sys.argv[3] if len(sys.argv)>3 else 'crash'
binary = sys.argv[4] if len(sys.argv)>4 else '/tmp/orbit-w15-terminal.test'
root = pathlib.Path(tempfile.mkdtemp(prefix='orbit-w15-', dir='/tmp'))
(root/MARKER).write_text(TOKEN)
(root/MARKER).chmod(0o600)
argv = ['unshare', '--user', '--map-root-user', '--net', 'python3', str(repo/'scripts/wan_failure_campaign.py'), '--root', str(root), '--parent-namespace', os.readlink('/proc/self/ns/net'), '--test-binary', binary, '--scenario', scenario, '--suite', suite]
(evidence/(name+'-invocation.json')).write_text(json.dumps({'argv':argv,'root':str(root)},indent=2)+'\n')
code = subprocess.call(['python3',str(evidence/'record_command.py'),name,*argv],cwd=repo)
metrics=evidence/'logs'/('metrics-'+name)
metrics.mkdir(exist_ok=True)
for path in root.glob('daemon-metrics-*.jsonl'):
    (metrics/path.name).write_bytes(path.read_bytes())
for name in ['network.json','result.json']:
    if (root/name).exists(): (evidence/(sys.argv[1]+'-'+name)).write_bytes((root/name).read_bytes())
sys.exit(code)
