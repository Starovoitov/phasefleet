"""Read sweep CSV and write the comparison table plus two SVG figures."""

import csv
import math
import sys
from collections import defaultdict
from pathlib import Path


def percentile(values, p):
    if not values:
        return 0
    s = sorted(values)
    if len(s) == 1:
        return s[0]
    rank = math.ceil(p * len(s)) - 1
    rank = min(max(rank, 0), len(s) - 1)
    return s[rank]


def median(values):
    s = sorted(values)
    n = len(s)
    if n == 0:
        return 0
    mid = n // 2
    if n % 2:
        return s[mid]
    return (s[mid - 1] + s[mid]) / 2


def boundary(n, slots, jitter_median, ideal):
    load = n / slots
    if jitter_median <= ideal:
        return "jitter is enough: N·d is much smaller than T"
    if load <= 3:
        return "near capacity: jitter peak is several times the ideal"
    ratio = jitter_median / ideal if ideal else 0
    return f"large N: relative peak is {ratio:.2f}× ideal, the absolute gap remains"


def load_trials(path):
    with path.open(newline="") as f:
        return list(csv.DictReader(f))


def load_recovery(path):
    with path.open(newline="") as f:
        return list(csv.DictReader(f))


def summarize(rows):
    groups = defaultdict(lambda: defaultdict(list))
    singles = {}
    neg = None
    for row in rows:
        key = (int(row["N"]), int(row["slots"]))
        strategy = row["strategy"]
        peak = int(row["peak"])
        if strategy == "jitter":
            groups[key][strategy].append(peak)
            continue
        if strategy == "kuramoto" and float(row["K"]) < 0:
            neg = row
            continue
        singles[(key, strategy)] = row
    return groups, singles, neg


def write_table(path, groups, singles, neg, recovery_rows):
    keys = sorted(groups, key=lambda k: (k[1], k[0]))
    # Keep the grid order: smaller load first, then by N.
    keys = sorted(set(list(groups) + [k for (k, _) in singles]), key=lambda k: (k[0], k[1]))
    ntrials = 0
    for peaks in groups.values():
        ntrials = max(ntrials, len(peaks.get("jitter", [])))
    lines = [
        "# Jitter, hash, and DESYNC",
        "",
        "Task duration d = 1. Slots are T/d. Ideal is ceil(N·d/T).",
        f"Jitter median and p99 are {ntrials} seeds. Hash is one deterministic assignment from the node id.",
        "DESYNC is one seeded run. Aligned fires every node together, so its peak is N.",
        "",
        "| N | slots T/d | ideal | aligned | jitter median | jitter p99 | hash | DESYNC | where this row sits |",
        "| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |",
    ]
    for key in keys:
        n, slots = key
        peaks = groups.get(key, {}).get("jitter", [])
        if not peaks:
            continue
        ideal = int(singles[(key, "aligned")]["ideal"])
        aligned = int(singles[(key, "aligned")]["peak"])
        hashed = int(singles[(key, "hash")]["peak"])
        desync = singles[(key, "desync")]
        desync_peak = int(desync["peak"])
        if desync["settled"] == "true":
            mark = ""
        else:
            mark = f" after {desync['steps']} ticks, still above the ideal"
        lines.append(
            "| {n} | {slots} | {ideal} | {aligned} | {med:g} | {p99:g} | {hashed} | {desync}{mark} | {note} |".format(
                n=n,
                slots=slots,
                ideal=ideal,
                aligned=aligned,
                med=median(peaks),
                p99=percentile(peaks, 0.99),
                hashed=hashed,
                desync=desync_peak,
                mark=mark,
                note=boundary(n, slots, median(peaks), ideal),
            )
        )
    if any(singles[(key, "desync")]["settled"] != "true" for key in keys if (key, "desync") in singles):
        lines.append("")
        lines.append(
            "Where DESYNC is marked as still above the ideal, that is the peak after the step budget, not the equilibrium. "
            "The same stepper reached the ideal column on every smaller row. "
            "The last extra overlap disappears by neighbor-to-neighbor mixing, which takes on the order of N² ticks."
        )
        lines.append("")
    lines.append("## After killing 30% of 60 nodes")
    lines.append("")
    desync_pts = [r for r in recovery_rows if r["strategy"] == "desync"]
    jitter_pts = [r for r in recovery_rows if r["strategy"] == "jitter"]
    if desync_pts:
        last = desync_pts[-1]
        first = desync_pts[0]
        lines.append(
            f"DESYNC live-gap MAD starts at {float(first['gap_mad']):.4g} "
            f"and reaches {float(last['gap_mad']):.4g} after {last['tick']} coupling ticks "
            f"({last['alive']} nodes still alive)."
        )
    if jitter_pts:
        lines.append(
            f"Jitter on the same initial phases stays at MAD {float(jitter_pts[0]['gap_mad']):.4g}. "
            "The holes left by the dead nodes do not close."
        )
    lines.append("")
    if neg is not None:
        lines.append("## Negative Kuramoto coupling")
        lines.append("")
        lines.append(
            f"N={neg['N']}, slots={neg['slots']}, K=-1. "
            f"Order parameter r={float(neg['gap_mad']):.3f}, "
            f"peak={neg['peak']}, ideal={neg['ideal']}. "
            "r near 0 is not an even schedule."
        )
        lines.append("")
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")


def polyline(points, x0, y0, x1, y1, xmin, xmax, ymin, ymax):
    if not points:
        return ""
    coords = []
    for x, y in points:
        px = x0 + (x - xmin) / (xmax - xmin or 1) * (x1 - x0)
        py = y1 - (y - ymin) / (ymax - ymin or 1) * (y1 - y0)
        coords.append(f"{px:.1f},{py:.1f}")
    return " ".join(coords)


def write_peak_svg(path, groups, singles):
    keys = sorted(set(groups), key=lambda k: (k[0],))
    series = []
    for key in keys:
        n, slots = key
        ideal = int(singles[(key, "aligned")]["ideal"]) or 1
        jitter = groups[key]["jitter"]
        series.append(
            {
                "label": f"{n}/{slots}",
                "jitter": median(jitter) / ideal,
                "p99": percentile(jitter, 0.99) / ideal,
                "hash": int(singles[(key, "hash")]["peak"]) / ideal,
                "desync": int(singles[(key, "desync")]["peak"]) / ideal,
            }
        )
    width, height = 840, 460
    left, right, top, bottom = 70, 790, 40, 400
    ymax = max(max(p["jitter"], p["p99"], p["hash"], p["desync"]) for p in series)
    ymax = max(ymax * 1.15, 1.2)
    colors = {
        "jitter": "#c45c26",
        "p99": "#e0a060",
        "hash": "#2c6e8a",
        "desync": "#2f6b4f",
    }
    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}">',
        '<rect width="100%" height="100%" fill="#fafaf7"/>',
        '<text x="70" y="28" font-family="sans-serif" font-size="16">Peak / ideal</text>',
    ]
    for frac in (0, 0.25, 0.5, 0.75, 1):
        y = bottom - frac * (bottom - top)
        val = frac * ymax
        parts.append(f'<line x1="{left}" y1="{y:.1f}" x2="{right}" y2="{y:.1f}" stroke="#ddd"/>')
        parts.append(
            f'<text x="{left - 8}" y="{y + 4:.1f}" text-anchor="end" font-family="sans-serif" font-size="11">{val:.1f}</text>'
        )
    ideal_y = bottom - (1 / ymax) * (bottom - top)
    parts.append(
        f'<line x1="{left}" y1="{ideal_y:.1f}" x2="{right}" y2="{ideal_y:.1f}" stroke="#888" stroke-dasharray="4 4"/>'
    )
    names = ["jitter", "p99", "hash", "desync"]
    n = len(series)
    group_w = (right - left) / max(n, 1)
    bar_w = group_w / (len(names) + 1)
    for i, point in enumerate(series):
        gx = left + i * group_w
        parts.append(
            f'<text x="{gx + group_w / 2:.1f}" y="{bottom + 22}" text-anchor="middle" font-family="sans-serif" font-size="12">{point["label"]}</text>'
        )
        for j, name in enumerate(names):
            h = point[name] / ymax * (bottom - top)
            x = gx + (j + 0.5) * bar_w
            y = bottom - h
            parts.append(
                f'<rect x="{x:.1f}" y="{y:.1f}" width="{bar_w * 0.8:.1f}" height="{h:.1f}" fill="{colors[name]}"/>'
            )
    legend = [("jitter median", colors["jitter"]), ("jitter p99", colors["p99"]), ("hash", colors["hash"]), ("DESYNC", colors["desync"])]
    for i, (label, color) in enumerate(legend):
        x = left + i * 160
        parts.append(f'<rect x="{x}" y="428" width="14" height="14" fill="{color}"/>')
        parts.append(
            f'<text x="{x + 20}" y="440" font-family="sans-serif" font-size="12">{label}</text>'
        )
    parts.append("</svg>")
    path.write_text("\n".join(parts), encoding="utf-8")


def write_recovery_svg(path, rows):
    series = defaultdict(list)
    for row in rows:
        series[row["strategy"]].append((int(row["tick"]), float(row["gap_mad"])))
    width, height = 840, 420
    left, right, top, bottom = 70, 800, 30, 360
    xmax = max(p[0] for pts in series.values() for p in pts)
    ymax = max(p[1] for pts in series.values() for p in pts)
    xmax = max(xmax, 1)
    ymax = max(ymax * 1.1, 1e-6)
    colors = {"desync": "#2f6b4f", "jitter": "#c45c26"}
    parts = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" viewBox="0 0 {width} {height}">',
        '<rect width="100%" height="100%" fill="#fafaf7"/>',
        '<text x="70" y="22" font-family="sans-serif" font-size="16">Gap MAD after killing 30%</text>',
    ]
    for frac in (0, 0.5, 1):
        y = bottom - frac * (bottom - top)
        parts.append(f'<line x1="{left}" y1="{y:.1f}" x2="{right}" y2="{y:.1f}" stroke="#ddd"/>')
        parts.append(
            f'<text x="{left - 8}" y="{y + 4:.1f}" text-anchor="end" font-family="sans-serif" font-size="11">{frac * ymax:.3g}</text>'
        )
    for name, pts in series.items():
        step = max(1, len(pts) // 800)
        slim = pts[::step]
        if slim[-1] != pts[-1]:
            slim.append(pts[-1])
        coords = polyline(slim, left, top, right, bottom, 0, xmax, 0, ymax)
        parts.append(
            f'<polyline fill="none" stroke="{colors.get(name, "#333")}" stroke-width="2" points="{coords}"/>'
        )
    parts.append(
        f'<text x="{left}" y="{bottom + 28}" font-family="sans-serif" font-size="12">0</text>'
    )
    parts.append(
        f'<text x="{right}" y="{bottom + 28}" text-anchor="end" font-family="sans-serif" font-size="12">{xmax} ticks</text>'
    )
    parts.append('<rect x="70" y="390" width="14" height="14" fill="#2f6b4f"/>')
    parts.append('<text x="90" y="402" font-family="sans-serif" font-size="12">DESYNC</text>')
    parts.append('<rect x="180" y="390" width="14" height="14" fill="#c45c26"/>')
    parts.append('<text x="200" y="402" font-family="sans-serif" font-size="12">jitter</text>')
    parts.append("</svg>")
    path.write_text("\n".join(parts), encoding="utf-8")


def main():
    out = Path(sys.argv[1] if len(sys.argv) > 1 else "experiments/out")
    rows = load_trials(out / "trials.csv")
    recovery = load_recovery(out / "recovery.csv")
    groups, singles, neg = summarize(rows)
    write_table(out / "table.md", groups, singles, neg, recovery)
    write_peak_svg(out / "peak.svg", groups, singles)
    write_recovery_svg(out / "recovery.svg", recovery)
    print(out / "table.md")


if __name__ == "__main__":
    main()
