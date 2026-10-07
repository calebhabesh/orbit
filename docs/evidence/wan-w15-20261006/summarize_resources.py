import json,pathlib,re
base=pathlib.Path(__file__).resolve().parent
result={'sampling':'daemon every 250 ms; combined three-peer fairness every 25 ms; short peaks may be missed','hosts':{}}
for name in ['impaired-final','mtu-final','pi-recovery','pi-impaired-runtime-fixed']:
 path=base/(name+'-invocation.json')
 if not path.exists():continue
 inv=json.loads(path.read_text())
 log=(base/'logs'/(name+'.log'))
 if not log.exists():continue
 text=log.read_text()
 starts=[int(n) for n in re.findall(r'idle interval start_ns=(\d+)',text)]
 ends=[int(n) for n in re.findall(r'idle interval end_ns=(\d+)',text)]
 # Copied metric PIDs are unique for the local/Pi campaigns in this record.
 files=list((base/'logs'/('metrics-'+name)).glob('daemon-metrics-*.jsonl'))
 samples=[]
 for file in files:
  rows=[json.loads(x) for x in file.read_text().splitlines() if x]
  if not rows:continue
  # Match sample lifetime to the idle intervals belonging to this campaign.
  intervals=[(a,b) for a,b in zip(starts,ends) if rows[0]['time_ns']<=a<=rows[-1]['time_ns']]
  def cpu(row):return row['cpu_user_us']+row['cpu_system_us']
  idle=[]
  for a,b in intervals:
   points=[row for row in rows if a<=row['time_ns']<=b]
   if len(points)>1: idle.append({'seconds':(points[-1]['time_ns']-points[0]['time_ns'])/1e9,'cpu_seconds':(cpu(points[-1])-cpu(points[0]))/1e6})
  samples.append({'pid':rows[0]['pid'],'sample_count':len(rows),'peak':{k:max(x[k] for x in rows) for k in ['fds','goroutines','heap_bytes','rss_bytes']},'sampled_lifetime_seconds':(rows[-1]['time_ns']-rows[0]['time_ns'])/1e9,'cpu_seconds':(cpu(rows[-1])-cpu(rows[0]))/1e6,'idle':idle})
 result['hosts'][name]={'daemons':samples,'relay_forwarded_ciphertext_bytes':[int(n) for n in re.findall(r'service relayed encrypted inner stream bytes=(\d+)',text)],'route_waits':re.findall(r'whole daemon route (\w+) after (\S+)',text),'limitations':'callback receipt uncertainty; advanced 500 ms poll / 2 s quiet / 10 s reprobe; 1 MiB/s daemon payload cap; native isolated synthetic addresses'}
for name in ['local-fairness','pi-fairness']:
 log=base/'logs'/(name+'.log')
 if log.exists(): result['hosts'][name]={'measurements':re.findall(r'actual (?:relay|direct|quic):.*',log.read_text()),'scope':'combined fixture process: three peers and local service, 16 MiB + continuous small; shared 2 MiB/s payload cap; warm/cold/initial burst as transcript; local race vs Pi non-race'}
(base/'resources.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps({k:len(v.get('daemons',v.get('measurements',[]))) for k,v in result['hosts'].items()}))
