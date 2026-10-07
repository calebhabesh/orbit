import datetime, hashlib, json, os, pathlib, platform, subprocess, sys, time
ROOT = pathlib.Path(__file__).resolve().parents[3]
EVIDENCE = pathlib.Path(__file__).resolve().parent
EVIDENCE.mkdir(exist_ok=True)
(EVIDENCE / 'logs').mkdir(exist_ok=True)
def now():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()
def snapshot():
    names = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard'], cwd=ROOT).decode().split('\0')
    files = {}
    for name in sorted(set(names)):
        if not name or name.startswith('docs/evidence/'):
            continue
        path = ROOT / name
        if path.is_file():
            files[name] = hashlib.sha256(path.read_bytes()).hexdigest()
    tested = {n: h for n, h in files.items() if not n.startswith('docs/')}
    digest = hashlib.sha256(json.dumps(tested, sort_keys=True).encode()).hexdigest()
    return {'captured_at': now(), 'files': files, 'tested_source_digest': digest}
if sys.argv[1] == 'snapshot':
    data = snapshot()
    (EVIDENCE / (sys.argv[2]+'.json')).write_text(json.dumps(data, indent=2)+'\n')
    print(data['tested_source_digest'], len(data['files']), 'files')
else:
    name, argv = sys.argv[1], sys.argv[2:]
    before = snapshot()
    start = time.monotonic()
    record = {'argv': argv, 'cwd': str(ROOT), 'started_at': now(), 'source_before': before['tested_source_digest'], 'environment': {'ORBIT_DISABLE_PACKAGED_PROFILE': '1', 'GOFLAGS': os.environ.get('GOFLAGS', '')}}
    (EVIDENCE / (name+'.json')).write_text(json.dumps(record, indent=2)+'\n')
    env = dict(os.environ, ORBIT_DISABLE_PACKAGED_PROFILE='1')
    with (EVIDENCE / 'logs' / (name+'.log')).open('w') as log:
        result = subprocess.run(argv, cwd=ROOT, env=env, stdout=log, stderr=subprocess.STDOUT)
    record.update(exit_code=result.returncode, duration_seconds=round(time.monotonic()-start, 3), finished_at=now(), source_after=snapshot()['tested_source_digest'])
    (EVIDENCE / (name+'.json')).write_text(json.dumps(record, indent=2)+'\n')
    print(json.dumps(record))
    print((EVIDENCE / 'logs' / (name+'.log')).read_text()[-4000:])
    sys.exit(result.returncode)
