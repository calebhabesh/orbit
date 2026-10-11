#!/usr/bin/env python3
"""Summarize R05 measure_netns.py results into Markdown tables (median, range, n).

    python3 scripts/validation/summarize_r05.py RESULT.json... > summary-tables.md
"""
import json
import statistics
import sys


def stats(values):
    values = [v for v in values if v is not None]
    if not values:
        return 'n=0'
    return f'{statistics.median(values):.2f} [{min(values):.2f}–{max(values):.2f}] n={len(values)}'


def mib_s(t):
    return t['bytes'] / (1 << 20) / t['seconds'] if t.get('seconds') else None


def main(paths):
    runs = [json.load(open(p)) for p in paths]
    print('## Connectivity (reconnect repetitions)\n')
    print('| Cell | Netem | Runs ok | First sync after restart, s | Direct upgrade within settle | Upgrade after restart, s | Direct share of bytes per rep | Route of settled 256 KiB |')
    print('| --- | --- | --- | --- | --- | --- | --- | --- |')
    for r in runs:
        reps = r.get('results', {}).get('reconnect') or []
        if not reps:
            continue
        upgraded = [x for x in reps if x.get('upgraded_route')]
        routes = {}
        for x in reps:
            for k in x['settled']['routes']:
                routes[k] = routes.get(k, 0) + 1
        print(f"| {r['cell']} | {r.get('netem') or '—'} | {'yes' if r['success'] else 'no: ' + r.get('error', '')[:60]} | "
              f"{stats([x['first_sync_since_restart_seconds'] for x in reps])} | {len(upgraded)}/{len(reps)} "
              f"({', '.join(sorted({x['upgraded_route'] for x in upgraded})) or 'none'}) | "
              f"{stats([x.get('upgrade_since_restart_seconds') for x in upgraded])} | "
              f"{stats([x.get('rep_direct_share') for x in reps])} | {', '.join(f'{k}×{v}' for k, v in sorted(routes.items()))} |")
    print('\n## Join, resources and doctor\n')
    print('| Cell | Join, s | B idle RSS KiB / fds | B after reps RSS KiB / CPU s / fds | Doctor UDP / direct |')
    print('| --- | --- | --- | --- | --- |')
    for r in runs:
        res = r.get('results', {})
        idle, after = res.get('resources_idle_b') or {}, res.get('resources_after_b') or {}
        probes = {p['kind']: p['code'] for p in (res.get('doctor_b') or {}).get('probes') or []}
        print(f"| {r['cell']} | {res.get('join_seconds', 0):.1f} | {idle.get('rss_kib')} / {idle.get('fds')} | "
              f"{after.get('rss_kib')} / {after.get('cpu_seconds')} / {after.get('fds')} | {probes.get('udp_stun')} / {probes.get('direct_tls')} |")
    if any(r.get('results', {}).get('large') for r in runs):
        print('\n## Throughput (A → B)\n')
        print('| Cell | Large file MiB/s | Large route | Small files/s | Small route |')
        print('| --- | --- | --- | --- | --- |')
        for r in runs:
            res = r.get('results', {})
            if not res.get('large'):
                continue
            lr = sorted({k for t in res['large'] for k in t['routes']})
            sr = sorted({k for t in res.get('small', []) for k in t['routes']})
            print(f"| {r['cell']} | {stats([mib_s(t) for t in res['large']])} | {', '.join(lr)} | "
                  f"{stats([t['files'] / t['seconds'] for t in res.get('small', [])])} | {', '.join(sr)} |")
    for r in runs:
        res = r.get('results', {})
        if res.get('chunk_base'):
            print(f"\n## Chunk reuse ({r['cell']})\n")
            print('| Change | File bytes | Bytes B received | Ratio |')
            print('| --- | --- | --- | --- |')
            for name in ('chunk_base', 'chunk_tail_append', 'chunk_prefix_insert'):
                t = res[name]
                got = sum(v['received'] for v in t['routes'].values())
                print(f"| {name.replace('chunk_', '')} | {t['bytes']} | {got} | {got / t['bytes']:.3f} |")
        if res.get('kill_resume') or res.get('partition') or res.get('roaming'):
            print(f"\n## Faults ({r['cell']})\n")
            k = res.get('kill_resume') or {}
            if k:
                print(f"- Receiver SIGKILL at {k['received_before_kill']} of {k['file_bytes']} bytes: resumed and verified in "
                      f"{k['resume_seconds']:.1f} s; received after restart {k['received_after_restart']} bytes "
                      f"(remaining at kill {k['remaining_at_kill']}).")
            p = res.get('partition') or {}
            if p:
                print(f"- Partition of A with concurrent edits on both sides: heal to identical two-head conflict on both devices in {p['heal_to_converged_seconds']:.1f} s.")
            m = res.get('roaming') or {}
            if m:
                print(f"- A's WAN address changed mid-transfer ({m['bytes']} bytes): completed {m['after_change_seconds']:.1f} s after the change; routes {sorted(m['routes'])}.")
        if not r['success']:
            print(f"\n- **{r['cell']} run failed:** {r.get('error')}")


if __name__ == '__main__':
    main(sys.argv[1:])
