#!/usr/bin/python3
"""Build the runlike progress page.

Usage: /usr/bin/python3 dashboard/build.py [coverage.json] [status.json]
Defaults: spec/v29.9.3/coverage.json and dashboard/status.json.
Writes dashboard/index.html. Stdlib only.
"""
import html
import json
import sys
from datetime import datetime
from pathlib import Path

HERE = Path(__file__).resolve().parent
COVERAGE = Path(sys.argv[1]) if len(sys.argv) > 1 else HERE.parent / "spec/v29.9.3/coverage.json"
STATUS = Path(sys.argv[2]) if len(sys.argv) > 2 else HERE / "status.json"
OUT = HERE / "index.html"

AREAS = {
    "networking": "Networking", "storage": "Storage and volumes",
    "resources": "CPU, memory and limits", "security": "Security",
    "process": "Process and user", "logging": "Logging",
    "health": "Health checks", "metadata": "Names and labels",
    "runtime": "Runtime", "client": "Command-line only",
}
GO_STATE = {"not-started": "Not started", "in-progress": "In progress", "done": "Done"}
PY_STATE = {"supported": "Yes", "partial": "Partly", "missing": "No"}

e = html.escape


def when(s):
    try:
        d = datetime.fromisoformat(str(s).replace("Z", "+00:00"))
    except ValueError:
        return e(str(s))
    out = f"{d.day} {d:%b %Y}"
    return out + (f", {d:%H:%M}" if "T" in str(s) else "")


def seg_bar(parts, total, cls="bar", scale=1.0):
    """parts: [(css_class, count, label)]. Thin segmented bar; title gives hover text."""
    if not total:
        return f'<div class="{cls}" style="width:{scale*100:.1f}%"></div>'
    segs = "".join(
        f'<span class="seg {c}" style="flex-grow:{n}" title="{e(lbl)}: {n} of {total}"></span>'
        for c, n, lbl in parts if n
    )
    return f'<div class="{cls}" style="width:{scale*100:.1f}%" role="img" aria-label="' + e(
        ", ".join(f"{lbl} {n}" for _, n, lbl in parts)) + f'">{segs}</div>'


def coverage_section(cov):
    opts = cov.get("options", [])
    total = len(opts)
    done = sum(o.get("go") == "done" for o in opts)
    prog = sum(o.get("go") == "in-progress" for o in opts)
    py_full = sum(o.get("python_1_6_3") == "supported" for o in opts)
    py_part = sum(o.get("python_1_6_3") == "partial" for o in opts)
    pct = round(100 * done / total) if total else 0
    ver = e(cov.get("docker_cli_version", ""))

    legend = ('<div class="legend"><span><i class="sw done"></i>Done</span>'
              '<span><i class="sw prog"></i>In progress</span>'
              '<span><i class="sw rest"></i>Not started</span></div>')

    by_area = {}
    for o in opts:
        by_area.setdefault(o.get("area", "runtime"), []).append(o)
    biggest = max((len(v) for v in by_area.values()), default=1)
    rows = []
    for area in sorted(by_area, key=lambda a: -len(by_area[a])):
        g = by_area[area]
        d = sum(o.get("go") == "done" for o in g)
        p = sum(o.get("go") == "in-progress" for o in g)
        rows.append(
            f'<div class="arow"><div class="aname">{e(AREAS.get(area, area.title()))}</div>'
            f'<div class="atrack">{seg_bar([("done", d, "Done"), ("prog", p, "In progress"), ("rest", len(g)-d-p, "Not started")], len(g), "bar", len(g)/biggest)}</div>'
            f'<div class="anum">{d} of {len(g)}</div></div>')

    def state(o):
        tags = []
        if o.get("kind") == "client-only":
            tags.append("Cannot be recovered")
        elif o.get("kind") == "saved-data-only":
            tags.append("Tested from saved data")
        if o.get("platform") == "windows":
            tags.append("Windows only")
        elif o.get("platform") == "linux":
            tags.append("Linux only")
        return tags

    trs = []
    for o in sorted(opts, key=lambda o: (AREAS.get(o.get("area"), "~"), o.get("flag", ""))):
        flag = e(o.get("flag", ""))
        if o.get("short"):
            flag += f', {e(o["short"])}'
        g = o.get("go", "not-started")
        tags = "".join(f'<span class="tag">{e(t)}</span>' for t in state(o))
        trs.append(
            f'<tr><td class="flag">{flag}</td><td>{e(o.get("description", ""))}</td>'
            f'<td>{e(AREAS.get(o.get("area"), o.get("area", "")))}</td>'
            f'<td><i class="sw {"done" if g=="done" else "prog" if g=="in-progress" else "rest"}"></i>{GO_STATE.get(g, g)}{tags}</td>'
            f'<td>{PY_STATE.get(o.get("python_1_6_3"), "")}</td></tr>')

    return f"""
<section class="card wide">
  <h2>Coverage of <span class="nowrap">docker run</span> options (Docker {ver})</h2>
  <div class="hero-row">
    <div>
      <div class="hero">{done}<span class="of"> of {total}</span></div>
      <div class="sub">options the new version handles ({pct}%){f", {prog} in progress" if prog else ""}</div>
    </div>
    <div class="tile">
      <div class="tlabel">Old version (Python 1.6.3), for comparison</div>
      <div class="tvalue">{py_full} fully, {py_part} partly</div>
      <div class="sub">{total - py_full - py_part} not at all</div>
    </div>
  </div>
  {seg_bar([("done", done, "Done"), ("prog", prog, "In progress"), ("rest", total-done-prog, "Not started")], total, "bar big")}
  {legend}
  <p class="note">Options that Docker does not keep after a container starts count as handled once we document them: runlike cannot print them.</p>
  <h3>By area</h3>
  <div class="areas">{"".join(rows)}</div>
  <details><summary>Every option ({total})</summary>
  <div class="tablewrap"><table><thead><tr><th>Option</th><th>What it does</th><th>Area</th><th>New version</th><th>Old version</th></tr></thead>
  <tbody>{"".join(trs)}</tbody></table></div></details>
</section>"""


def items(lst, empty, show_when=False):
    if not lst:
        return f'<p class="empty">{e(empty)}</p>'
    out = []
    for i in lst:
        w = f'<span class="when">{when(i["when"])}</span>' if show_when and i.get("when") else ""
        d = f'<div class="detail">{e(i["detail"])}</div>' if i.get("detail") else ""
        out.append(f'<li><div class="t">{e(i.get("title", ""))}</div>{d}{w}</li>')
    return "<ul>" + "".join(out) + "</ul>"


def main():
    st = json.loads(STATUS.read_text())
    cov_html = ('<section class="card wide"><h2>Coverage of docker run options</h2>'
                '<p class="empty">The option list is being built.</p></section>')
    if COVERAGE.exists():
        cov_html = coverage_section(json.loads(COVERAGE.read_text()))

    ms = st.get("milestones", [])
    mark = {"done": "✓", "active": "●", "todo": ""}
    word = {"done": "Done", "active": "Now", "todo": ""}
    ms_html = "".join(
        f'<li class="m {e(m.get("state", "todo"))}"><span class="dot">{mark.get(m.get("state"), "")}</span>'
        f'<span>{e(m["title"])}</span><span class="mstate">{word.get(m.get("state"), "")}</span></li>' for m in ms)
    ms_done = sum(m.get("state") == "done" for m in ms)
    waiting = st.get("waiting_on_you", [])
    done = sorted(st.get("done", []), key=lambda i: str(i.get("when", "")), reverse=True)[:8]

    page = f"""<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="60">
<title>runlike rewrite: progress</title>
<style>
:root {{ color-scheme: light dark;
  --page:#f9f9f7; --surface:#fcfcfb; --ink:#0b0b0b; --ink2:#52514e; --muted:#6f6d68;
  --grid:#e1e0d9; --ring:rgba(11,11,11,.10); --done:#2a78d6; --prog:#86b6ef; --rest:#e1e0d9;
  --accent-bg:#fff6e0; --accent-ring:#eda100; }}
@media (prefers-color-scheme: dark) {{ :root {{
  --page:#0d0d0d; --surface:#1a1a19; --ink:#f0efec; --ink2:#c3c2b7; --muted:#a3a19a;
  --grid:#2c2c2a; --ring:rgba(255,255,255,.10); --done:#3987e5; --prog:#184f95; --rest:#383835;
  --accent-bg:#2a2410; --accent-ring:#c98500; }} }}
* {{ box-sizing:border-box }}
body {{ margin:0; background:var(--page); color:var(--ink);
  font:15px/1.45 system-ui,-apple-system,"Segoe UI",sans-serif; }}
main {{ max-width:1100px; margin:0 auto; padding:20px 16px 40px }}
header h1 {{ font-size:24px; margin:0 0 2px }}
header .sub {{ margin:0 0 16px }}
.grid {{ display:grid; gap:14px; grid-template-columns:repeat(auto-fit,minmax(min(300px,100%),1fr)) }}
.card {{ background:var(--surface); border-radius:12px; box-shadow:0 0 0 1px var(--ring); padding:16px 18px }}
.wide {{ grid-column:1/-1 }}
.card.ask {{ background:var(--accent-bg); box-shadow:0 0 0 2px var(--accent-ring) }}
h2 {{ font-size:17px; margin:0 0 10px }} h3 {{ font-size:15px; margin:18px 0 8px; color:var(--ink2) }}
.sub, .note, .detail, .when {{ color:var(--ink2) }} .note {{ font-size:13px }}
.empty {{ color:var(--ink2); margin:4px 0 }}
ul {{ list-style:none; margin:0; padding:0 }} li {{ padding:7px 0; border-top:1px solid var(--grid) }}
li:first-child {{ border-top:0 }} .detail, .when {{ font-size:13px }} .when {{ display:block }}
.hero-row {{ display:flex; flex-wrap:wrap; gap:16px 40px; align-items:flex-end; margin-bottom:14px }}
.hero {{ font-size:56px; font-weight:600; line-height:1 }} .hero .of {{ font-size:24px; font-weight:500; color:var(--ink2) }}
.tile .tlabel {{ font-size:13px; color:var(--ink2) }} .tile .tvalue {{ font-size:22px; font-weight:600 }}
.bar {{ display:flex; gap:2px; height:8px; border-radius:4px; overflow:hidden; background:var(--rest) }}
.bar.big {{ height:14px; border-radius:7px }}
.seg {{ display:block; min-width:2px }} .seg.done, .sw.done {{ background:var(--done) }}
.seg.prog, .sw.prog {{ background:var(--prog) }} .seg.rest, .sw.rest {{ background:var(--rest) }}
.legend {{ display:flex; gap:16px; margin:8px 0; font-size:13px; color:var(--ink2) }}
.sw {{ display:inline-block; width:10px; height:10px; border-radius:3px; margin-right:6px; vertical-align:-1px }}
.areas {{ display:grid; gap:8px }}
.arow {{ display:grid; grid-template-columns:minmax(120px,190px) 1fr 70px; gap:10px; align-items:center }}
.aname {{ font-size:14px }} .anum {{ font-size:13px; color:var(--ink2); text-align:right; font-variant-numeric:tabular-nums }}
details {{ margin-top:16px }} summary {{ cursor:pointer; font-weight:600 }}
.tablewrap {{ overflow-x:auto; margin-top:8px }}
table {{ border-collapse:collapse; width:100%; font-size:13px }}
th, td {{ text-align:left; padding:6px 8px; border-top:1px solid var(--grid); vertical-align:top }}
th {{ color:var(--ink2); font-weight:600 }} td.flag {{ white-space:nowrap; font-weight:600 }}
.tag {{ display:inline-block; margin-left:6px; padding:0 6px; border-radius:8px; box-shadow:0 0 0 1px var(--ring); color:var(--ink2); font-size:12px }}
.nowrap {{ white-space:nowrap }}
ol.ms {{ list-style:none; margin:0; padding:0 }}
.m {{ display:grid; grid-template-columns:24px 1fr auto; gap:8px; align-items:center; border-top:1px solid var(--grid) }}
.m .dot {{ width:18px; height:18px; border-radius:50%; box-shadow:inset 0 0 0 2px var(--rest); font-size:12px; display:grid; place-items:center; color:#fff }}
.m.done .dot {{ background:var(--done); box-shadow:none }} .m.active .dot {{ box-shadow:inset 0 0 0 2px var(--done); color:var(--done) }}
.m.active {{ font-weight:600 }} .m.todo {{ color:var(--ink2) }} .mstate {{ font-size:13px; color:var(--ink2) }}
@media (max-width:560px) {{ .arow {{ grid-template-columns:1fr 64px }} .arow .atrack {{ grid-column:1/-1; grid-row:2 }} .hero {{ font-size:48px }} }}
</style></head><body><main>
<header><h1>runlike 2.0 rewrite</h1>
<p class="sub">Stage: <strong>{e(st.get("phase", ""))}</strong> · updated {when(st.get("updated", ""))} · page refreshes every minute</p></header>
<div class="grid">
<section class="card{' ask' if waiting else ''}"><h2>Waiting on you</h2>{items(waiting, "Nothing right now.")}</section>
<section class="card"><h2>Working on now</h2>{items(st.get("now", []), "Nothing in progress.")}</section>
<section class="card"><h2>Next up</h2>{items(st.get("next", []), "Nothing planned.")}</section>
{cov_html}
<section class="card"><h2>Finished recently</h2>{items(done, "Nothing yet.", True)}</section>
<section class="card"><h2>Milestones <span class="sub" style="font-weight:400">({ms_done} of {len(ms)} done)</span></h2><ol class="ms">{ms_html}</ol></section>
<section class="card"><h2>Risks</h2>{items(st.get("risks", []), "None known.")}</section>
</div></main></body></html>
"""
    OUT.write_text(page)
    print(f"wrote {OUT}")


if __name__ == "__main__":
    main()
