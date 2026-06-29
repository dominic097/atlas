#!/usr/bin/env python3
"""Real-LLM code-Q&A benchmark: Atlas vs graphify vs raw-file context.

This script PREPARES a question set whose ground truth is `gopls` (the official
Go LSP, go/types-based — neutral to both Atlas and graphify). For each target
symbol it builds:

  - the question + the precise ground-truth answer (from gopls),
  - three candidate *contexts* an LLM could be given: Atlas's op output,
    graphify's explain output, and the raw definition file,
  - the real token cost of each context.

The actual LLM answering + grading is done by the companion workflow
(`atlas-llm-qa`), which feeds each context to a real Claude subagent, parses its
answer, and scores it against the gopls ground truth. So "accuracy" is measured
by whether a real LLM, given each tool's output, answers the code question
correctly — not by a proxy.

Output: bench/LLM_QA_SET.json  (consumed by the workflow).

Usage:
  python3 bench/llm_qa.py --atlas /tmp/atlas --graphify <graphify> \
      --repo /tmp/.../logrus --db sqlite:///tmp/llmqa.db \
      --symbols New,SetLevel,WithError,Errorf,ParseLevel,NewEntry --out bench/LLM_QA_SET.json
"""
import argparse
import json
import os
import re
import subprocess
from pathlib import Path


def sh(cmd, cwd=None, timeout=180):
    return subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)


def toks(s: str) -> int:
    return max(1, len(s) // 4)


def find_func_pos(repo: Path, name: str):
    """Locate the (relfile, line, col) of `func name(` or `func (recv) name(` —
    the position gopls needs. Returns the first match (1-based line/col)."""
    pat = re.compile(r"\bfunc\b.*?\b(" + re.escape(name) + r")\s*(?:\[|\()")
    for p in repo.rglob("*.go"):
        try:
            text = p.read_text(errors="ignore")
        except Exception:
            continue
        for i, line in enumerate(text.splitlines(), start=1):
            m = pat.search(line)
            if m:
                col = m.start(1) + 1  # 1-based
                return str(p.relative_to(repo)), i, col
    return None, 0, 0


def gopls_callers(repo: Path, relfile: str, line: int, col: int):
    """Precise caller function names via gopls call_hierarchy."""
    r = sh(["gopls", "call_hierarchy", f"{relfile}:{line}:{col}"], cwd=str(repo))
    names = []
    for ln in r.stdout.splitlines():
        # caller[N]: ranges .. from/to function NAME in FILE:line:col-col
        m = re.search(r"from/to function (\w+) in ", ln)
        if ln.startswith("caller[") and m:
            names.append(m.group(1))
    return sorted(set(names))


def atlas_ctx(atlas, db, sym, op):
    r = sh([atlas, "--db", db, "--format", "plain", op, sym])
    return r.stdout.strip()


def graphify_ctx(graphify, repo: Path, sym):
    r = sh([graphify, "explain", sym], cwd=str(repo))
    return r.stdout.strip()


def baseline_ctx(repo: Path, relfile: str, cap_tokens=6000):
    """Raw definition file (what a tool-less agent would open), capped."""
    try:
        text = (repo / relfile).read_text(errors="ignore")
    except Exception:
        return ""
    if toks(text) > cap_tokens:
        text = text[: cap_tokens * 4] + "\n/* ...truncated... */"
    return text


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--atlas", required=True)
    ap.add_argument("--graphify", required=True)
    ap.add_argument("--repo", required=True)
    ap.add_argument("--db", required=True)
    ap.add_argument("--symbols", required=True, help="comma-separated symbol names")
    ap.add_argument("--out", default="bench/LLM_QA_SET.json")
    args = ap.parse_args()

    repo = Path(args.repo)
    qset = []
    for sym in [s.strip() for s in args.symbols.split(",") if s.strip()]:
        relfile, line, col = find_func_pos(repo, sym)
        if not relfile:
            print(f"[skip] {sym}: no func def found")
            continue
        callers = gopls_callers(repo, relfile, line, col)
        if len(callers) < 2:
            print(f"[skip] {sym}: too few callers ({len(callers)}) for a meaningful question")
            continue

        ctx_atlas = atlas_ctx(args.atlas, args.db, sym, "callers")
        ctx_gfy = graphify_ctx(args.graphify, repo, sym)
        ctx_base = baseline_ctx(repo, relfile)

        q = {
            "id": f"callers::{sym}",
            "symbol": sym,
            "qtype": "callers",
            "question": f"List the names of the functions that directly call `{sym}`. "
                        f"Answer with ONLY a comma-separated list of function names, nothing else.",
            "truth": callers,
            "truth_source": f"gopls call_hierarchy {relfile}:{line}:{col}",
            "contexts": {"atlas": ctx_atlas, "graphify": ctx_gfy, "baseline": ctx_base},
            "ctx_tokens": {"atlas": toks(ctx_atlas), "graphify": toks(ctx_gfy), "baseline": toks(ctx_base)},
        }
        qset.append(q)
        print(f"[ok] {sym}: {len(callers)} gopls callers | ctx tok atlas={q['ctx_tokens']['atlas']} "
              f"gfy={q['ctx_tokens']['graphify']} base={q['ctx_tokens']['baseline']}")

    Path(args.out).write_text(json.dumps(qset, indent=2))
    print(f"\n[wrote] {args.out}  ({len(qset)} questions)")


if __name__ == "__main__":
    main()
