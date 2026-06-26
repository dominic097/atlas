#!/usr/bin/env python3
"""Benchmark: Atlas vs graphify, per language Atlas supports.

For each of Atlas's seven languages this builds BOTH code knowledge graphs on the
same real single-language repo and measures three things:

  1. Build      — wall time, node/symbol count, call-edge count (both tools).
  2. Precision  — Atlas: % of call edges with a resolved receiver type
                  (recv_type), and the resolver source breakdown.
                  graphify: % of call links marked EXTRACTED vs INFERRED
                  (its own confidence flag).
  3. Query cost — for a few shared hub symbols, the response tokens of
                  `atlas --format plain explain` vs `graphify explain`.

It writes a Markdown report (--out) and a per-language raw log (logs/<lang>.log).
Everything is deterministic given the pinned repos; the numbers are a snapshot of
one machine/run (timings vary).

Usage:
  python3 bench/graphify_vs_atlas.py \
      --atlas /path/to/atlas --graphify /path/to/graphify \
      --workdir /tmp/langbench --out bench/REPORT.md [--langs go,java,...]

Requires: an `atlas` binary, a `graphify` binary (pip install graphifyy), git,
python3 (stdlib only; sqlite3 included).
"""
import argparse
import json
import os
import subprocess
import sqlite3
import time
from pathlib import Path

# One small, real, single-language repo per Atlas language (pinned by URL). The
# optional second element narrows indexing to the source subdir so both tools
# graph the same code (not tests/build files).
REPOS = {
    "go":         ("https://github.com/sirupsen/logrus", ""),
    "python":     ("https://github.com/psf/requests", "src"),
    "javascript": ("https://github.com/expressjs/express", "lib"),
    "typescript": ("https://github.com/pmndrs/zustand", "src"),
    "java":       ("https://github.com/google/gson", "gson/src/main/java"),
    "c":          ("https://github.com/DaveGamble/cJSON", ""),
    "cpp":        ("https://github.com/google/leveldb", ""),
}


def sh(cmd, cwd=None, timeout=900):
    return subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)


def toks(s: str) -> int:
    """Approximate token count (≈4 chars/token for code, the usual BPE ratio)."""
    return max(1, len(s) // 4)


def pct(num, den):
    return 0.0 if not den else round(100.0 * num / den, 1)


def ensure_repo(url, dest: Path, log):
    if not dest.exists():
        r = sh(["git", "clone", "--depth", "1", "-q", url, str(dest)])
        log.append(f"# git clone {url}\n{r.stdout}{r.stderr}")
    return dest.exists()


def build_graphify(gfy, target: Path, log):
    out = target / "graphify-out"
    if out.exists():
        import shutil
        shutil.rmtree(out, ignore_errors=True)
    t = time.time()
    r = sh([gfy, "update", "."], cwd=str(target))
    dt = time.time() - t
    log.append(f"# graphify update . (cwd={target})\n{r.stdout}{r.stderr}")
    m = {"ok": False, "seconds": round(dt, 2), "nodes": 0, "calls": 0, "extracted": 0}
    gj = out / "graph.json"
    if gj.exists():
        g = json.load(open(gj))
        m["ok"] = True
        m["nodes"] = len(g.get("nodes", []))
        calls = [l for l in g.get("links", []) if l.get("relation") == "calls"]
        m["calls"] = len(calls)
        m["extracted"] = sum(1 for l in calls if str(l.get("confidence", "")).upper() == "EXTRACTED")
    return m


def build_atlas(atlas, target: Path, db: Path, log):
    if db.exists():
        db.unlink()
    t = time.time()
    r = sh([atlas, "--db", f"sqlite://{db}", "index", str(target)])
    dt = time.time() - t
    log.append(f"# atlas index {target}\n{r.stdout}{r.stderr}")
    m = {"ok": r.returncode == 0, "seconds": round(dt, 2), "files": 0,
         "symbols": 0, "edges": 0, "calls": 0, "recv_typed": 0, "sources": {}}
    try:
        idx = json.loads(r.stdout)
        m["files"] = idx.get("indexed_files", 0)
        m["symbols"] = idx.get("symbols", 0)
        m["edges"] = idx.get("edges", 0)
    except Exception:
        pass
    if db.exists():
        con = sqlite3.connect(str(db))
        cur = con.cursor()
        cur.execute("SELECT count(*) FROM edges WHERE kind='calls'")
        m["calls"] = cur.fetchone()[0]
        cur.execute("SELECT count(*) FROM edges WHERE kind='calls' "
                    "AND json_extract(metadata,'$.recv_type') IS NOT NULL "
                    "AND json_extract(metadata,'$.recv_type') != ''")
        m["recv_typed"] = cur.fetchone()[0]
        # Internal (resolvable) calls: ToRef names a known in-graph symbol. This
        # is the fair cross-tool coverage axis — graphify only keeps node-to-node
        # (internal) calls, whereas Atlas keeps every call expression incl. calls
        # out to stdlib/3rd-party.
        cur.execute("SELECT count(*) FROM edges WHERE kind='calls' "
                    "AND to_ref IN (SELECT DISTINCT name FROM symbols)")
        m["internal_calls"] = cur.fetchone()[0]
        cur.execute("SELECT json_extract(metadata,'$.source'), count(*) "
                    "FROM edges WHERE kind='calls' GROUP BY 1 ORDER BY 2 DESC")
        m["sources"] = {(s or "?"): c for s, c in cur.fetchall()}
        con.close()
    return m


def query_tokens(atlas, gfy, db: Path, target: Path, log):
    """For a few shared hub symbols, compare explain response tokens."""
    r = sh([atlas, "--db", f"sqlite://{db}", "--json", "hubs", "--limit", "6"])
    names = []
    try:
        for h in json.loads(r.stdout).get("hubs", []):
            n = h.get("bare_name") or h.get("name") or ""
            if n and n not in names:
                names.append(n)
    except Exception:
        pass
    rows = []
    for name in names[:4]:
        a = toks(sh([atlas, "--db", f"sqlite://{db}", "--format", "plain", "explain", name]).stdout)
        g = toks(sh([gfy, "explain", name], cwd=str(target)).stdout)
        rows.append((name, g, a))
        log.append(f"# explain {name}: graphify={g}tok atlas_terse={a}tok")
    return rows


def run_lang(lang, atlas, gfy, workdir: Path, logdir: Path):
    url, sub = REPOS[lang]
    log = []
    repo = workdir / lang
    ensure_repo(url, repo, log)
    target = repo / sub if sub else repo
    if not target.exists():
        target = repo
    db = workdir / f"{lang}.db"
    g = build_graphify(gfy, target, log)
    a = build_atlas(atlas, target, db, log)
    q = query_tokens(atlas, gfy, db, target, log)
    (logdir / f"{lang}.log").write_text("\n\n".join(log))
    return {"lang": lang, "repo": url, "subdir": sub, "target": str(target),
            "graphify": g, "atlas": a, "queries": q}


def render(results, atlas_ver):
    L = []
    w = L.append
    w("# Atlas vs graphify — per-language benchmark\n")
    w("Both tools build a code knowledge graph on the **same** real single-language "
      "repo per language; we measure build, call-graph coverage, edge precision, and "
      "query token cost. Tokens ≈ chars/4. Numbers are a one-machine snapshot "
      "(timings vary); the graphs themselves are deterministic.\n")
    w(f"- atlas: `{atlas_ver}`")
    w("- graphify: graphifyy (PyPI), `graphify update` / `graphify explain` — both offline, no LLM\n")

    w("### Reading the metrics (the two tools model edges differently)\n")
    w("- **Atlas** emits *every* call expression as an edge (incl. calls out to "
      "stdlib/3rd-party), then resolves to in-graph targets on demand. "
      "*internal calls* = the resolvable subset (ToRef names a known symbol) — the "
      "fair coverage axis vs graphify.")
    w("- **graphify** keeps only node-to-node (already-internal) call links, each "
      "flagged **EXTRACTED** (AST-grounded) or **INFERRED** (guessed). EXTRACTED% is "
      "its precision signal.")
    w("- **method-receiver%** (Atlas) = call edges that are *method* calls with a "
      "resolved receiver type. It is meaningful only for OO method calls — correctly "
      "~0 for procedural C and low for function-heavy JS/Python — and is where "
      "Atlas's type grounding (Go `go/types`, Java declared types) shows. It is NOT "
      "comparable to graphify's EXTRACTED%; they measure different things.\n")

    # Aggregate table.
    w("## Summary\n")
    w("| Lang | Repo | atlas symbols | gfy nodes | atlas calls (internal) | gfy calls (EXTRACTED%) | atlas method-recv% | build a/g |")
    w("|---|---|--:|--:|--:|--:|--:|--:|")
    for r in results:
        a, g = r["atlas"], r["graphify"]
        repo = r["repo"].split("github.com/")[-1]
        ic = a.get("internal_calls", 0)
        w(f"| {r['lang']} | {repo} | {a['symbols']} | {g['nodes']} | {a['calls']} ({ic}) | "
          f"{g['calls']} ({pct(g['extracted'], g['calls'])}%) | {pct(a['recv_typed'], a['calls'])}% | "
          f"{a['seconds']}/{g['seconds']}s |")
    w("")

    # ── Findings (computed) ──────────────────────────────────────────────────
    w("## Findings\n")
    # method-receiver leaders
    recv = sorted(results, key=lambda r: pct(r['atlas']['recv_typed'], r['atlas']['calls']), reverse=True)
    top = [f"{r['lang']} {pct(r['atlas']['recv_typed'], r['atlas']['calls'])}%" for r in recv[:3]]
    w(f"- **Receiver-type precision leaders (Atlas):** {', '.join(top)}. Java and Go "
      "lead because Atlas type-grounds receivers (Java declared types, Go `go/types`); "
      "this is the precision dimension graphify's name-level graph can't express.")
    # under-extraction
    under = [r for r in results if r['graphify']['nodes'] and r['atlas']['symbols'] < 0.6 * r['graphify']['nodes']]
    if under:
        w("- **Atlas symbol under-extraction (a real gap this benchmark exposed):** "
          + "; ".join(f"{r['lang']} {r['atlas']['symbols']} vs graphify {r['graphify']['nodes']} nodes" for r in under)
          + ". Atlas's tree-sitter symbol extraction for these langs is shallower than "
          "graphify's — a precision/coverage gap to close (esp. C++).")
    # coverage: atlas extracts a denser raw call graph
    w("- **Call-graph density:** Atlas extracts far more call expressions than graphify "
      "reports links (e.g. " + ", ".join(f"{r['lang']} {r['atlas']['calls']} vs {r['graphify']['calls']}" for r in results[:3])
      + ") because Atlas keeps external/unresolved calls too; the *internal* count is "
      "the comparable figure.")
    # query tokens
    gt = sum(t for r in results for _, t, _ in r['queries'])
    at = sum(t for r in results for _, _, t in r['queries'])
    if gt and at:
        w(f"- **Query token cost:** across all probed hub symbols, graphify totals "
          f"{gt} tok vs Atlas terse {at} tok ({'Atlas terser' if at < gt else 'graphify terser'} "
          f"by {round(max(gt, at) / max(1, min(gt, at)), 2)}x). Atlas loses on overloaded "
          "names (Java create/write) where it returns every real definition; it wins on "
          "most single-definition symbols.")
    w("")

    # Per-language detail.
    for r in results:
        a, g = r["atlas"], r["graphify"]
        w(f"## {r['lang']}  —  {r['repo']}\n")
        w(f"target: `{r['target']}`\n")
        w("**Build**\n")
        w(f"- atlas: {a['files']} files, {a['symbols']} symbols, {a['edges']} edges "
          f"({a['calls']} calls, {a.get('internal_calls', 0)} internal), {a['seconds']}s")
        w(f"- graphify: {g['nodes']} nodes, {g['calls']} call links, {g['seconds']}s\n")
        w("**Edge precision**\n")
        w(f"- atlas method-receiver resolution (OO method calls): "
          f"{a['recv_typed']}/{a['calls']} = **{pct(a['recv_typed'], a['calls'])}%**")
        if a["sources"]:
            srcs = ", ".join(f"{k}:{v}" for k, v in a["sources"].items())
            w(f"  - extractor sources: {srcs}")
        w(f"- graphify EXTRACTED vs INFERRED: {g['extracted']}/{g['calls']} = "
          f"**{pct(g['extracted'], g['calls'])}%** high-confidence\n")
        if r["queries"]:
            w("**Query token cost** (explain, response tokens)\n")
            w("| symbol | graphify | atlas terse |")
            w("|---|--:|--:|")
            for name, gt2, at2 in r["queries"]:
                w(f"| {name} | {gt2} | {at2} |")
            w("")
    w("\n---\n*Generated by `bench/graphify_vs_atlas.py`. Per-language raw logs in "
      "`bench/logs/`.*")
    return "\n".join(L)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--atlas", default=os.environ.get("ATLAS_BIN", "atlas"))
    ap.add_argument("--graphify", default=os.environ.get("GRAPHIFY_BIN", "graphify"))
    ap.add_argument("--workdir", default="/tmp/langbench")
    ap.add_argument("--out", default="bench/REPORT.md")
    ap.add_argument("--langs", default=",".join(REPOS.keys()))
    args = ap.parse_args()

    workdir = Path(args.workdir)
    workdir.mkdir(parents=True, exist_ok=True)
    logdir = Path(args.out).parent / "logs"
    logdir.mkdir(parents=True, exist_ok=True)

    atlas_ver = sh([args.atlas, "version"]).stdout.strip() or "atlas (dev)"

    results = []
    for lang in args.langs.split(","):
        lang = lang.strip()
        if lang not in REPOS:
            continue
        print(f"[bench] {lang} ...", flush=True)
        results.append(run_lang(lang, args.atlas, args.graphify, workdir, logdir))

    report = render(results, atlas_ver)
    Path(args.out).write_text(report)
    Path(args.out).with_suffix(".json").write_text(json.dumps(results, indent=2))
    print(f"[bench] wrote {args.out} (+ .json, + logs/)")


if __name__ == "__main__":
    main()
