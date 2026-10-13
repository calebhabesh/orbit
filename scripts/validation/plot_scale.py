#!/usr/bin/env python3
"""Charts for the scale/churn campaign and R05 network matrix.

  python3 scripts/validation/plot_scale.py --scale n5.json n10.json n20.json \
      --soak n10.json --round-cost round-cost.json \
      --r05 docs/evidence/final-r05-20261011/raw --out OUT_DIR

Writes PNG and SVG for each chart. Every number comes from the input JSON.
"""
import argparse
import json
from pathlib import Path
import statistics

import matplotlib

matplotlib.use('Agg')
import matplotlib.pyplot as plt  # noqa: E402
from matplotlib.patches import Patch  # noqa: E402

GREEN, SLATE, RUST, SAND, GREY = '#2f6b55', '#56688f', '#b0563a', '#c9a227', '#9aa0a6'
plt.rcParams.update({
    'font.size': 11, 'axes.spines.top': False, 'axes.spines.right': False, 'axes.grid': True,
    'grid.color': '#e6e6e6', 'axes.axisbelow': True, 'figure.dpi': 150, 'savefig.bbox': 'tight',
    'figure.facecolor': 'white', 'axes.facecolor': 'white',
})


def save(fig, out, name):
    for ext in ('png', 'svg'):
        fig.savefig(out / f'{name}.{ext}')
    plt.close(fig)


def pct(xs, f):
    xs = sorted(xs)
    return xs[min(len(xs) - 1, int(round(f * (len(xs) - 1))))]


def scale_chart(runs, out, note=None):
    runs = sorted(runs, key=lambda r: r['devices'])
    ns = [r['devices'] for r in runs]
    fig, (a, b) = plt.subplots(1, 2, figsize=(13, 4.8))
    for r in runs:
        xs = [v for rep in r['results']['fanout'] for v in [rep['all_seconds']]]
        a.scatter([r['devices']] * len(xs), xs, color=SLATE, alpha=0.35, s=18, zorder=2)
    p50 = [pct([x['all_seconds'] for x in r['results']['fanout']], 0.5) for r in runs]
    p95 = [pct([x['all_seconds'] for x in r['results']['fanout']], 0.95) for r in runs]
    a.plot(ns, p50, '-o', color=GREEN, label='p50', zorder=3)
    a.plot(ns, p95, '-o', color=RUST, label='p95', zorder=3)
    a.set_xticks(ns)
    a.set_xlabel('Devices in the Orbit')
    a.set_ylabel('Seconds until every device holds the edit')
    a.set_title('One 64 KiB edit → all devices (20 reps each)', fontsize=11)
    a.legend(frameon=False, loc='upper left')
    a.set_ylim(bottom=0)
    if note:
        a.text(0.03, 0.78, note, transform=a.transAxes, ha='left', va='top', fontsize=9, color=RUST,
               bbox={'facecolor': 'white', 'edgecolor': RUST, 'boxstyle': 'round,pad=0.4'})
    conc = [statistics.median(x['seconds'] for x in r['results']['concurrent']) for r in runs]
    copies = [r['results']['concurrent'][0]['copies'] for r in runs]
    bars = b.bar([str(n) for n in ns], conc, color=SLATE, width=0.55)
    for bar, c in zip(bars, copies):
        b.annotate(f'{c} copies', (bar.get_x() + bar.get_width() / 2, bar.get_height()), ha='center', va='bottom',
                   fontsize=9, xytext=(0, 3), textcoords='offset points')
    b.set_xlabel('Devices (each writes one file at the same moment)')
    b.set_ylabel('Seconds until all-to-all replication (median of 3)')
    b.set_title('Concurrent writers', fontsize=11)
    fig.suptitle('Orbit scale-out · one inviter (star replication), NAT homes, 20 ms WAN delay', fontweight='bold')
    save(fig, out, 'scale-fanout')


def soak_chart(run, out):
    s = run['results']['soak']
    fig, ax = plt.subplots(figsize=(13, 4.8))
    events = s['events']
    end = max(e['t'] for e in events)
    # Fault windows: kill..restart per device, partition..heal per home.
    open_kill, open_part = {}, {}
    windows = []
    for e in events:
        if e['kind'] == 'kill':
            open_kill[e['device']] = e['t']
        elif e['kind'] == 'restart' and e['device'] in open_kill:
            windows.append(('kill', open_kill.pop(e['device']), e['t']))
        elif e['kind'] == 'partition':
            open_part[e['home']] = e['t']
        elif e['kind'] == 'heal' and e['home'] in open_part:
            windows.append(('partition', open_part.pop(e['home']), e['t']))
    for kind, t0, t1 in windows:
        ax.axvspan(t0 / 60, t1 / 60, color=RUST if kind == 'partition' else SAND, alpha=0.12, lw=0)
    w = [x for x in s['writes'] if x['all_seconds'] is not None]
    ax.scatter([x['t'] / 60 for x in w], [x['all_seconds'] for x in w], s=12, color=SLATE, alpha=0.7, zorder=3)
    heal = next(e['t'] for e in events if e['kind'] == 'heal_all')
    conv = heal + s['heal_to_converged_seconds']
    ax.axvspan(heal / 60, conv / 60, color=GREEN, alpha=0.18, lw=0)
    ax.axvline(heal / 60, color=GREEN, lw=1.5)
    ax.set_xlim(0, max(end, conv) / 60 * 1.02)
    ax.annotate(f"all faults healed → every device identical in {s['heal_to_converged_seconds']:.0f} s",
                (heal / 60, 1.0), xycoords=('data', 'axes fraction'), xytext=(-6, -14), textcoords='offset points', ha='right', color=GREEN)
    ax.set_xlabel('Soak time (minutes)')
    ax.set_ylabel('Seconds until a write reached every device')
    c = s['event_counts']
    ax.set_title(f"{run['devices']} devices · {c.get('write', 0)} writes, {c.get('edit', 0)} edits, {c.get('delete', 0)} deletes, "
                 f"{c.get('kill', 0)} SIGKILLs, {c.get('partition', 0)} home outages, {c.get('contested_edit', 0)} concurrent edits",
                 fontsize=10)
    ax.legend(handles=[Patch(color=SAND, alpha=0.35, label='device SIGKILLed'), Patch(color=RUST, alpha=0.3, label='home internet down'),
                       plt.Line2D([], [], marker='o', ls='', color=SLATE, label='write (time until on all devices)'),
                       Patch(color=GREEN, alpha=0.3, label='heal → exact convergence')],
              frameon=False, loc='upper center', bbox_to_anchor=(0.5, -0.13), ncol=4)
    fig.suptitle('Churn soak · random crashes and partitions, then exact convergence check', fontweight='bold')
    save(fig, out, 'soak-timeline')


def round_chart(rc, out, small=None):
    """Idle-round cost (unit fixture); with small=(before, after) R05-style
    runs, a second panel shows end-to-end 300-file batches as the folder grows."""
    if small:
        fig, (ax, bx) = plt.subplots(1, 2, figsize=(14, 4.8), gridspec_kw={'width_ratios': [1.2, 1]})
        for run, color, label, dx in ((small[0], RUST, 'v2.5.1', -0.2), (small[1], GREEN, 'batched receipts', 0.2)):
            secs = [x['seconds'] for x in run['results']['small']]
            bx.bar([i + dx for i in range(len(secs))], secs, 0.4, color=color, label=label)
            for i, v in enumerate(secs):
                bx.annotate(f'{v:.0f} s', (i + dx, v), ha='center', va='bottom', fontsize=9, xytext=(0, 2), textcoords='offset points')
        n = small[0]['results']['small'][0]['files']
        bx.set_xticks(range(3), [f'batch {i + 1}\n(folder {n * i}→{n * (i + 1)})' for i in range(3)])
        bx.set_ylabel(f'Seconds until {n} × 4 KiB files replicated')
        bx.set_title('End to end: NAT QUIC, 20 ms WAN (1 run each)', fontsize=11)
        bx.legend(frameon=False, loc='upper center', bbox_to_anchor=(0.5, -0.2), ncol=2)
    else:
        fig, ax = plt.subplots(figsize=(8, 4.6))
    v = [r['versions'] for r in rc['before']]
    x = range(len(v))
    ax.bar([i - 0.2 for i in x], [r['seconds'] for r in rc['before']], 0.4, color=RUST, label=f"before ({rc['before_label']})")
    ax.bar([i + 0.2 for i in x], [r['seconds'] for r in rc['after']], 0.4, color=GREEN, label=f"after ({rc['after_label']})")
    for i, (b, a) in enumerate(zip(rc['before'], rc['after'])):
        ax.annotate(f"{b['requests']} req", (i - 0.2, b['seconds']), ha='center', va='bottom', fontsize=9, xytext=(0, 2), textcoords='offset points')
        ax.annotate(f"{a['requests']} req\n{b['seconds'] / a['seconds']:.0f}× faster", (i + 0.2, a['seconds']), ha='center', va='bottom',
                    fontsize=9, xytext=(0, 2), textcoords='offset points')
    ax.set_xticks(list(x), [f'{n} versions' for n in v])
    ax.set_ylabel(f"Idle sync round (s), {rc['delay_ms']} ms per request")
    ax.set_title('Idle round, unit fixture', fontsize=11)
    ax.legend(frameon=False, loc='upper center', bbox_to_anchor=(0.5, -0.12), ncol=1)
    fig.suptitle('Batched durable receipts: sync cost no longer grows with folder size', fontweight='bold')
    save(fig, out, 'round-cost')


def matrix_chart(raw, out):
    cells = [('lan', 'Same LAN'), ('eim', 'Port-preserving NAT'), ('public_tcp', 'Public host,\nUDP dropped'),
             ('symmetric', 'Symmetric NAT'), ('udp_blocked', 'UDP blocked')]
    fig, (a, b) = plt.subplots(1, 2, figsize=(13, 4.8), gridspec_kw={'width_ratios': [1.4, 1]})
    for i, (cell, label) in enumerate(cells):
        reps = json.loads((raw / f'conn-{cell}.json').read_text())['results']['reconnect']
        direct = sum(1 for r in reps if r['upgraded_route'])
        a.bar(i, direct, color=GREEN, width=0.6)
        a.bar(i, len(reps) - direct, bottom=direct, color=GREY, width=0.6)
        route = {'quic': 'QUIC', 'direct': 'direct TCP'}.get(next((r['upgraded_route'] for r in reps if r['upgraded_route']), None), 'relay only')
        a.annotate(f'{direct}/{len(reps)}\n{route}', (i, len(reps)), ha='center', va='bottom', fontsize=9, xytext=(0, 3), textcoords='offset points')
    a.set_xticks(range(len(cells)), [c[1] for c in cells], fontsize=9)
    a.set_ylim(0, 12)
    a.set_ylabel('Repetitions')
    a.set_title('Direct path established (else relay fallback)', fontsize=11)
    a.legend(handles=[Patch(color=GREEN, label='upgraded to direct'), Patch(color=GREY, label='stayed on relay')], frameon=False,
             loc='upper center', bbox_to_anchor=(0.5, -0.16), ncol=2)
    for i, (cell, label) in enumerate([('lan', 'LAN QUIC'), ('eim', 'NAT QUIC\n(hole punch)'), ('relay', 'Relay\n(2 MiB/s budget)')]):
        large = json.loads((raw / f'bulk-{cell}.json').read_text())['results']['large']
        rates = [x['bytes'] / x['seconds'] / (1 << 20) for x in large]
        m = statistics.median(rates)
        b.bar(i, m, color=[GREEN, SLATE, RUST][i], width=0.55, yerr=[[m - min(rates)], [max(rates) - m]], capsize=4, ecolor='#555')
        b.annotate(f'{m:.1f} MiB/s', (i, max(rates)), ha='center', va='bottom', fontsize=9, xytext=(0, 3), textcoords='offset points')
        b.set_xticks([0, 1, 2], ['LAN QUIC', 'NAT QUIC\n(hole punch)', 'Relay\n(2 MiB/s budget)'], fontsize=9)
    b.set_ylabel('64 MiB transfer, MiB/s (median, min–max)')
    b.set_title('Throughput by route (3 reps)', fontsize=11)
    fig.suptitle('Orbit NAT traversal matrix · 50 namespace runs, 20 ms WAN delay', fontweight='bold')
    save(fig, out, 'network-matrix')


def main():
    p = argparse.ArgumentParser()
    p.add_argument('--scale', nargs='*', default=[])
    p.add_argument('--scale-note')
    p.add_argument('--soak')
    p.add_argument('--round-cost')
    p.add_argument('--small', nargs=2, metavar=('BEFORE', 'AFTER'))
    p.add_argument('--r05')
    p.add_argument('--out', required=True)
    a = p.parse_args()
    out = Path(a.out)
    out.mkdir(parents=True, exist_ok=True)
    if a.scale:
        scale_chart([json.loads(Path(f).read_text()) for f in a.scale], out, a.scale_note)
    if a.soak:
        soak_chart(json.loads(Path(a.soak).read_text()), out)
    if a.round_cost:
        small = [json.loads(Path(f).read_text()) for f in a.small] if a.small else None
        round_chart(json.loads(Path(a.round_cost).read_text()), out, small)
    if a.r05:
        matrix_chart(Path(a.r05), out)


if __name__ == '__main__':
    main()
