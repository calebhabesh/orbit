"""Preserve raw new logs before trimming display whitespace, never old evidence."""
import gzip,json,pathlib
base=pathlib.Path(__file__).resolve().parent
changed=[]
for p in sorted((base/'logs').glob('*.log')):
 raw=p.read_bytes()
 normalized=b'\n'.join(line.rstrip(b' \t\r') for line in raw.split(b'\n'))
 if normalized==raw:continue
 archive=p.with_suffix('.raw.gz')
 if archive.exists():raise RuntimeError('raw archive already exists: '+str(archive))
 archive.write_bytes(gzip.compress(raw,mtime=0))
 p.write_bytes(normalized)
 changed.append({'display_log':str(p.relative_to(base)),'raw_archive':str(archive.relative_to(base))})
(base/'normalized-logs.json').write_text(json.dumps(changed,indent=2)+'\n')
print('preserved raw bytes and normalized display whitespace in',len(changed),'new logs')
