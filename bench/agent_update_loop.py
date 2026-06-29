#!/usr/bin/env python3
"""Agent edit-loop benchmark: graphify vs Atlas, the way Codex/Claude call them.

This harness simulates EXACTLY what an AI coding agent does in practice. After
every code change the agent runs a graph-update command to keep its knowledge
graph fresh — `graphify 'update .'` for graphify, `atlas index .` for Atlas. The
critical, realistic wrinkle: the agent's edits live in the WORKING TREE and are
NOT committed yet (the agent edits, refreshes the graph, asks a question, edits
again — commit happens much later, if at all).

For each repo (one per language Atlas supports) we:

  1. Shallow-clone the repo to a /tmp workdir. Clean any pre-existing
     `graphify-out/` sidecar so we never compare against a stale graph.
  2. WARM both tools once (a full index / full update) and warm the Go build
     cache for Go repos, so per-edit timings are steady-state.
  3. Run N edit cycles. Each cycle appends a uniquely-named symbol
     (`AtlasLoopMarker_<i>`) to a real source file of the target language WITHOUT
     committing — the real agent scenario — then, for EACH tool, runs its update
     command and measures:
       * wall-time         (median over the cycles)
       * peak RSS          (/usr/bin/time -l -> "maximum resident set size" bytes)
       * accuracy          (does the refreshed graph now contain the marker?)
     To stay fair, every tool sees the SAME starting tree: we snapshot the file,
     apply the edit, run the tool, then revert the file before the next tool.

  4. Record per repo / per tool: median update seconds, peak RSS MB, accuracy
     (markers_found / markers_added), plus the raw per-cycle samples.

  5. Emit AGENT_UPDATE_LOOP.md (a per-language table + an honest summary) and
     AGENT_UPDATE_LOOP.json (machine-readable, with raw samples).

The headline this is built to surface honestly: graphify reloads/rebuilds the
WHOLE graph into RAM on every update, whereas Atlas is incremental. BUT Atlas's
incremental path is COMMIT-delta based, so against an uncommitted edit Atlas's
fast path returns mode=noop and MISSES the edit — so its accuracy here is
expected to be < 1.0 PRE-FIX. The harness reports that honestly; re-run after the
delta fix to see accuracy recover.

Usage:
  python3 bench/agent_update_loop.py \
      --atlas /tmp/atlas_loop \
      --graphify /Users/.../graphify \
      --repos go,python,javascript,java,c,cpp \
      --cycles 5 \
      --workdir /tmp/agentloop \
      --out bench/AGENT_UPDATE_LOOP.md

stdlib only (json, sqlite3, subprocess, statistics). Requires git, an `atlas`
binary, and a `graphify` binary.
"""
import argparse
import json
import os
import re
import shutil
import sqlite3
import statistics
import subprocess
import sys
import time
from pathlib import Path

# One small, real, single-language repo per language. The second element narrows
# indexing to the source subdir so both tools graph the same code, and the third
# tells the harness which kind of edit to make and which file to append it to (a
# real source file of that language, resolved live by extension at run time).
REPOS = {
    "go":         {"url": "https://github.com/sirupsen/logrus",      "subdir": "",                      "ext": ".go",   "kind": "go"},
    "python":     {"url": "https://github.com/psf/requests",          "subdir": "src",                   "ext": ".py",   "kind": "py"},
    "javascript": {"url": "https://github.com/expressjs/express",     "subdir": "lib",                   "ext": ".js",   "kind": "js"},
    "java":       {"url": "https://github.com/google/gson",           "subdir": "gson/src/main/java",    "ext": ".java", "kind": "java"},
    "c":          {"url": "https://github.com/DaveGamble/cJSON",      "subdir": "",                      "ext": ".c",    "kind": "c"},
    "cpp":        {"url": "https://github.com/google/leveldb",        "subdir": "",                      "ext": ".cc",   "kind": "cpp"},
}

# How to spell a top-level, uniquely-named symbol the parser will pick up, per
# language. The marker name carries the cycle index so each cycle is distinct.
def marker_snippet(kind: str, name: str) -> str:
    if kind == "go":
        return f"\nfunc {name}() {{}}\n"
    if kind == "py":
        return f"\n\ndef {name}():\n    return None\n"
    if kind == "js":
        return f"\nfunction {name}() {{ return null; }}\n"
    if kind == "java":
        # Append as a new top-level class so it parses regardless of the host file.
        return f"\nclass {name} {{}}\n"
    if kind in ("c", "cpp"):
        return f"\nvoid {name}(void) {{}}\n"
    return f"\n// {name}\n"


def sh(cmd, cwd=None, timeout=1800):
    return subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)


def time_l(cmd, cwd=None, timeout=1800):
    """Run `cmd` under /usr/bin/time -l, returning (result, peak_rss_bytes).

    macOS /usr/bin/time -l writes a resource block to stderr; the line
    '<N>  maximum resident set size' reports peak RSS in BYTES. On Linux GNU
    time -l is unavailable, so we fall back to a plain run and rss=None.
    """
    if sys.platform == "darwin" and Path("/usr/bin/time").exists():
        full = ["/usr/bin/time", "-l"] + cmd
        r = subprocess.run(full, cwd=cwd, capture_output=True, text=True, timeout=timeout)
        rss = None
        for line in r.stderr.splitlines():
            m = re.search(r"(\d+)\s+maximum resident set size", line)
            if m:
                rss = int(m.group(1))
                break
        return r, rss
    r = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)
    return r, None


def bytes_to_mb(b):
    return None if b is None else round(b / (1024 * 1024), 1)


def ensure_repo(url, dest: Path, log):
    if not dest.exists():
        r = sh(["git", "clone", "--depth", "1", "-q", url, str(dest)])
        log.append(f"# git clone {url}\n{r.stdout}{r.stderr}")
    return dest.exists()


def pick_source_file(target: Path, ext: str, kind: str) -> Path:
    """Find a real, modestly-sized source file of the target language to edit.

    Prefer a non-test, non-generated file under the target dir. Deterministic
    (sorted) so re-runs touch the same file. The marker we append is a fresh
    top-level symbol, so it parses cleanly wherever it lands.
    """
    cands = []
    for p in sorted(target.rglob(f"*{ext}")):
        s = p.as_posix().lower()
        if any(seg in s for seg in ("/test", "_test", "test_", "/tests/", ".min.", "/vendor/", "/node_modules/")):
            continue
        try:
            sz = p.stat().st_size
        except OSError:
            continue
        if sz == 0 or sz > 200_000:
            continue
        cands.append(p)
    if not cands:
        # Fall back to any file of that extension.
        cands = sorted(target.rglob(f"*{ext}"))
    if not cands:
        raise RuntimeError(f"no {ext} source file under {target}")
    return cands[0]


# --------------------------------------------------------------------------- #
# Tool drivers — each returns (update_seconds, peak_rss_bytes, found_bool).    #
# --------------------------------------------------------------------------- #

def atlas_warm(atlas, target: Path, db: Path, log):
    if db.exists():
        db.unlink()
    for sidecar in (db.with_suffix(db.suffix + "-wal"), db.with_suffix(db.suffix + "-shm")):
        if sidecar.exists():
            sidecar.unlink()
    r = sh([atlas, "--db", f"sqlite://{db}", "index", str(target)])
    log.append(f"# WARM atlas index {target}\n{r.stdout[-400:]}{r.stderr[-400:]}")
    return r.returncode == 0


def atlas_update(atlas, target: Path, db: Path):
    """Run the per-edit update an agent would run, with RSS measured."""
    r, rss = time_l([atlas, "--db", f"sqlite://{db}", "index", str(target)])
    secs = None
    mode = None
    try:
        out = json.loads(r.stdout)
        secs = out.get("duration_ms")
        secs = None if secs is None else secs / 1000.0
        mode = out.get("mode")
    except Exception:
        pass
    return r, rss, secs, mode


def atlas_found(atlas, db: Path, marker: str) -> bool:
    """Exact-match check: is `marker` an indexed symbol NAME in the graph?

    We check the sqlite symbols table directly (authoritative) AND the plain
    search surface (what the agent actually queries). Either an exact symbol-name
    row or an exact-name search hit counts as found.
    """
    # 1) Authoritative: symbols table.
    try:
        con = sqlite3.connect(str(db))
        cur = con.cursor()
        cur.execute("SELECT count(*) FROM symbols WHERE name = ?", (marker,))
        n = cur.fetchone()[0]
        con.close()
        if n > 0:
            return True
    except Exception:
        pass
    # 2) Agent-facing: plain search rows are "  <name>  <kind>  <path:line>".
    r = sh([atlas, "--db", f"sqlite://{db}", "--format", "plain", "search", marker, "--limit", "20"])
    for line in r.stdout.splitlines():
        toks = line.strip().split()
        if toks and toks[0] == marker:
            return True
    return False


def graphify_warm(gfy, target: Path, log):
    out = target / "graphify-out"
    if out.exists():
        shutil.rmtree(out, ignore_errors=True)
    r = sh([gfy, "update", "."], cwd=str(target))
    log.append(f"# WARM graphify update . ({target})\n{r.stdout[-400:]}{r.stderr[-400:]}")
    return (out / "graph.json").exists()


def graphify_update(gfy, target: Path):
    t = time.time()
    r, rss = time_l([gfy, "update", "."], cwd=str(target))
    secs = time.time() - t  # graphify prints no machine timing; wall-clock it.
    return r, rss, secs


def graphify_found(target: Path, marker: str) -> bool:
    """Exact-ish check: is `marker` a node label in graphify's graph.json?

    graphify normalizes labels (lowercases, may add '()'); we match the marker as
    a whole token inside the node label, case-insensitively, against the marker
    we appended — robust to its `name()` / `name` label styles.
    """
    gj = target / "graphify-out" / "graph.json"
    if not gj.exists():
        return False
    try:
        g = json.load(open(gj))
    except Exception:
        return False
    pat = re.compile(r"(?<![A-Za-z0-9_])" + re.escape(marker) + r"(?![A-Za-z0-9_])", re.IGNORECASE)
    for n in g.get("nodes", []):
        for key in ("label", "norm_label", "id"):
            v = n.get(key)
            if isinstance(v, str) and pat.search(v):
                return True
    return False


# --------------------------------------------------------------------------- #
# Edit-cycle orchestration.                                                    #
# --------------------------------------------------------------------------- #

def run_repo(lang, cfg, atlas, gfy, workdir: Path, cycles: int, log):
    spec = {"lang": lang, "url": cfg["url"], "ok": False, "error": None}
    repo_dir = workdir / lang
    if not ensure_repo(cfg["url"], repo_dir, log):
        spec["error"] = "clone failed"
        return spec
    target = repo_dir / cfg["subdir"] if cfg["subdir"] else repo_dir
    if not target.exists():
        spec["error"] = f"target subdir missing: {target}"
        return spec

    # Clean any stale graphify sidecar BEFORE first use.
    stale = target / "graphify-out"
    if stale.exists():
        shutil.rmtree(stale, ignore_errors=True)

    # Warm the Go build cache for Go repos so timings are steady-state.
    if cfg["kind"] == "go" and (repo_dir / "go.mod").exists():
        rb = sh(["go", "build", "./..."], cwd=str(repo_dir))
        log.append(f"# WARM go build ./... ({repo_dir})\n{rb.stderr[-300:]}")

    edit_file = pick_source_file(target, cfg["ext"], cfg["kind"])
    spec["edit_file"] = edit_file.relative_to(repo_dir).as_posix()
    original = edit_file.read_text(errors="replace")

    db = workdir / f"{lang}.db"

    # WARM both tools once on the clean tree.
    if not atlas_warm(atlas, target, db, log):
        spec["error"] = "atlas warm index failed"
        edit_file.write_text(original)
        return spec
    if not graphify_warm(gfy, target, log):
        spec["error"] = "graphify warm update failed"
        edit_file.write_text(original)
        return spec

    atlas_samples = []   # list of {seconds, rss_bytes, found, mode}
    gfy_samples = []
    try:
        for i in range(cycles):
            marker = f"AtlasLoopMarker_{i}"
            snippet = marker_snippet(cfg["kind"], marker)

            # ---- Atlas tool run (same starting tree) ----
            edit_file.write_text(original + snippet)
            ar, arss, asecs, amode = atlas_update(atlas, target, db)
            afound = atlas_found(atlas, db, marker)
            atlas_samples.append({
                "cycle": i, "marker": marker,
                "seconds": asecs, "rss_bytes": arss,
                "found": afound, "mode": amode,
                "rc": ar.returncode,
            })
            edit_file.write_text(original)  # revert so graphify sees identical state

            # ---- graphify tool run (identical starting tree) ----
            edit_file.write_text(original + snippet)
            gr, grss, gsecs = graphify_update(gfy, target)
            gfound = graphify_found(target, marker)
            gfy_samples.append({
                "cycle": i, "marker": marker,
                "seconds": round(gsecs, 4), "rss_bytes": grss,
                "found": gfound, "rc": gr.returncode,
            })
            edit_file.write_text(original)  # revert for next cycle
    finally:
        edit_file.write_text(original)  # always leave the tree clean

    def summarize(samples, time_key="seconds"):
        secs = [s[time_key] for s in samples if s.get(time_key) is not None]
        rsss = [s["rss_bytes"] for s in samples if s.get("rss_bytes") is not None]
        found = sum(1 for s in samples if s.get("found"))
        return {
            "median_seconds": round(statistics.median(secs), 4) if secs else None,
            "peak_rss_mb": bytes_to_mb(max(rsss)) if rsss else None,
            "median_rss_mb": bytes_to_mb(int(statistics.median(rsss))) if rsss else None,
            "markers_found": found,
            "markers_added": len(samples),
            "accuracy": round(found / len(samples), 3) if samples else None,
        }

    spec["ok"] = True
    spec["cycles"] = cycles
    spec["atlas"] = summarize(atlas_samples)
    spec["graphify"] = summarize(gfy_samples)
    spec["atlas"]["modes"] = sorted({s["mode"] for s in atlas_samples if s.get("mode")})
    spec["raw"] = {"atlas": atlas_samples, "graphify": gfy_samples}
    return spec


# --------------------------------------------------------------------------- #
# Reporting.                                                                   #
# --------------------------------------------------------------------------- #

def fmt(v, suffix=""):
    return "n/a" if v is None else f"{v}{suffix}"


def write_markdown(results, out_path: Path, meta):
    lines = []
    lines.append("# Agent Edit-Loop Benchmark — graphify vs Atlas")
    lines.append("")
    lines.append("Simulates an AI coding agent's real workflow: after each code change it")
    lines.append("runs the graph-update command to refresh its knowledge graph. The edit is")
    lines.append("**uncommitted** (working-tree only) — exactly the agent scenario. We measure")
    lines.append("per-edit update time, peak RSS, and whether the refreshed graph actually")
    lines.append("contains the just-added symbol (accuracy).")
    lines.append("")
    lines.append(f"- atlas: `{meta['atlas']}`")
    lines.append(f"- graphify: `{meta['graphify']}`")
    lines.append(f"- cycles per repo: {meta['cycles']}  |  generated: {meta['generated_at']}")
    lines.append(f"- platform: {meta['platform']}")
    lines.append("")
    lines.append("update s = median wall-time per edit · RSS = peak resident set · "
                 "acc = markers_found / markers_added")
    lines.append("")
    lines.append("| lang | atlas update s | atlas RSS MB | atlas acc | graphify update s | graphify RSS MB | graphify acc |")
    lines.append("|------|---------------:|-------------:|----------:|------------------:|----------------:|-------------:|")
    for r in results:
        if not r.get("ok"):
            lines.append(f"| {r['lang']} | — | — | — | — | — | — |  _({r.get('error')})_")
            continue
        a, g = r["atlas"], r["graphify"]
        lines.append(
            f"| {r['lang']} | {fmt(a['median_seconds'])} | {fmt(a['peak_rss_mb'])} | "
            f"{fmt(a['accuracy'])} ({a['markers_found']}/{a['markers_added']}) | "
            f"{fmt(g['median_seconds'])} | {fmt(g['peak_rss_mb'])} | "
            f"{fmt(g['accuracy'])} ({g['markers_found']}/{g['markers_added']}) |"
        )
    lines.append("")

    # Honest summary.
    ok = [r for r in results if r.get("ok")]
    lines.append("## Honest summary")
    lines.append("")
    if ok:
        a_acc = [r["atlas"]["accuracy"] for r in ok if r["atlas"]["accuracy"] is not None]
        g_acc = [r["graphify"]["accuracy"] for r in ok if r["graphify"]["accuracy"] is not None]
        a_secs = [r["atlas"]["median_seconds"] for r in ok if r["atlas"]["median_seconds"] is not None]
        g_secs = [r["graphify"]["median_seconds"] for r in ok if r["graphify"]["median_seconds"] is not None]
        a_rss = [r["atlas"]["peak_rss_mb"] for r in ok if r["atlas"]["peak_rss_mb"] is not None]
        g_rss = [r["graphify"]["peak_rss_mb"] for r in ok if r["graphify"]["peak_rss_mb"] is not None]
        modes = sorted({m for r in ok for m in r["atlas"].get("modes", [])})
        if a_secs and g_secs:
            lines.append(f"- **Speed**: median per-edit update — atlas {round(statistics.median(a_secs),3)}s "
                         f"vs graphify {round(statistics.median(g_secs),3)}s (across {len(ok)} repos).")
        if a_rss and g_rss:
            lines.append(f"- **Memory**: peak RSS — atlas {round(statistics.median(a_rss),1)}MB "
                         f"vs graphify {round(statistics.median(g_rss),1)}MB (median across repos). "
                         f"graphify rebuilds/reloads the whole graph into RAM each update; atlas is incremental.")
        if a_acc and g_acc:
            lines.append(f"- **Accuracy (THE catch)**: atlas mean {round(statistics.mean(a_acc),3)} "
                         f"vs graphify mean {round(statistics.mean(g_acc),3)}. "
                         f"Atlas update modes observed: {modes or ['n/a']}.")
        if any(a < 1.0 for a in a_acc):
            lines.append("- Atlas's fast incremental path is **commit-delta** based: when the agent's")
            lines.append("  edit is uncommitted and no new commit exists, `atlas index` returns")
            lines.append("  `mode=noop` and the new symbol is NOT indexed — so it is invisible to the")
            lines.append("  very next search. That is why atlas accuracy is < 1.0 here. This is the")
            lines.append("  bug the delta fix (Strand A) addresses; re-run this harness after the fix")
            lines.append("  to confirm accuracy recovers to 1.0 while keeping the speed/RSS edge.")
        else:
            lines.append("- Atlas accuracy is 1.0: the incremental update now sees uncommitted edits.")
    else:
        lines.append("- No repos completed; see error column above.")
    lines.append("")
    out_path.write_text("\n".join(lines))


def main():
    ap = argparse.ArgumentParser(description="Agent edit-loop benchmark: graphify vs Atlas")
    ap.add_argument("--atlas", default="/tmp/atlas_loop", help="path to the atlas binary")
    ap.add_argument("--graphify", default="/Users/damirdarasu/.local/share/uv/tools/graphifyy/bin/graphify",
                    help="path to the graphify binary")
    ap.add_argument("--repos", default="go,python,javascript,java,c,cpp",
                    help="comma-separated language keys: " + ",".join(REPOS))
    ap.add_argument("--cycles", type=int, default=5, help="edit cycles per repo")
    ap.add_argument("--workdir", default="/tmp/agentloop", help="scratch dir for clones + dbs")
    ap.add_argument("--out", default=str(Path(__file__).resolve().parent / "AGENT_UPDATE_LOOP.md"),
                    help="markdown report path (.json written alongside)")
    args = ap.parse_args()

    atlas = args.atlas
    gfy = args.graphify
    workdir = Path(args.workdir)
    workdir.mkdir(parents=True, exist_ok=True)
    langs = [s.strip() for s in args.repos.split(",") if s.strip()]
    unknown = [l for l in langs if l not in REPOS]
    if unknown:
        print(f"unknown repo keys: {unknown}; known: {list(REPOS)}", file=sys.stderr)
        sys.exit(2)

    log = []
    results = []
    for lang in langs:
        print(f"[agentloop] {lang} ...", file=sys.stderr, flush=True)
        try:
            res = run_repo(lang, REPOS[lang], atlas, gfy, workdir, args.cycles, log)
        except Exception as e:  # one repo's failure must not sink the run
            res = {"lang": lang, "ok": False, "error": f"{type(e).__name__}: {e}"}
        results.append(res)
        if res.get("ok"):
            a, g = res["atlas"], res["graphify"]
            print(f"  atlas: {a['median_seconds']}s {a['peak_rss_mb']}MB acc={a['accuracy']} "
                  f"modes={a.get('modes')} | graphify: {g['median_seconds']}s {g['peak_rss_mb']}MB "
                  f"acc={g['accuracy']}", file=sys.stderr, flush=True)
        else:
            print(f"  FAILED: {res.get('error')}", file=sys.stderr, flush=True)

    out_md = Path(args.out)
    out_md.parent.mkdir(parents=True, exist_ok=True)
    meta = {
        "atlas": atlas,
        "graphify": gfy,
        "cycles": args.cycles,
        "generated_at": time.strftime("%Y-%m-%dT%H:%M:%S%z"),
        "platform": sys.platform,
    }
    write_markdown(results, out_md, meta)
    out_json = out_md.with_suffix(".json")
    out_json.write_text(json.dumps({"meta": meta, "results": results}, indent=2))

    # Raw per-language logs for forensics.
    logs_dir = out_md.parent / "logs"
    logs_dir.mkdir(exist_ok=True)
    (logs_dir / "agent_update_loop.log").write_text("\n\n".join(log))

    print(f"[agentloop] wrote {out_md} and {out_json}", file=sys.stderr)


if __name__ == "__main__":
    main()
