#!/usr/bin/env python3
"""Repeat live smokes and record saturation evidence.

This is intentionally separate from additional_language_smoke.py. The smoke
script records one fresh run per language; this script proves the optimization
loop rule for rows where the benchmark cannot produce a graphify-equivalent
query score.
"""

from __future__ import annotations

import argparse
import json
import os
import subprocess
import sys
import time
from pathlib import Path
from typing import Any


def ratio(num: float, den: float) -> float | None:
    if not num or not den:
        return None
    return round(num / den, 2)


def coverage_ratio(coverage: dict[str, Any]) -> float | None:
    for key, value in sorted(coverage.items()):
        if key.startswith("atlas_vs_") and key.endswith("_definition_ratio"):
            try:
                return float(value)
            except (TypeError, ValueError):
                return None
    return None


def query_metrics(rows: list[dict[str, Any]]) -> dict[str, Any]:
    equivalent = [row for row in rows if not row.get("atlas_missing") and not row.get("graphify_missing")]
    atlas_tokens = sum(int(row.get("atlas_tokens", 0) or 0) for row in equivalent)
    graphify_tokens = sum(int(row.get("graphify_tokens", 0) or 0) for row in equivalent)
    atlas_ms = sum(float(row.get("atlas_ms", 0) or 0) for row in equivalent)
    graphify_ms = sum(float(row.get("graphify_ms", 0) or 0) for row in equivalent)
    return {
        "rows": len(rows),
        "equivalent_rows": len(equivalent),
        "atlas_missing": sum(1 for row in rows if row.get("atlas_missing")),
        "graphify_missing": sum(1 for row in rows if row.get("graphify_missing")),
        "atlas_tokens": atlas_tokens,
        "graphify_tokens": graphify_tokens,
        "atlas_ms": round(atlas_ms, 3),
        "graphify_ms": round(graphify_ms, 3),
        "token_ratio": ratio(graphify_tokens, atlas_tokens),
        "latency_ratio": ratio(graphify_ms, atlas_ms),
    }


def run_smoke(repo_root: Path, language: str, atlas: str, graphify: str, timeout: int) -> tuple[dict[str, Any], dict[str, Any]]:
    cmd = [
        sys.executable,
        "bench/additional_language_smoke.py",
        "--language",
        language,
        "--atlas",
        atlas,
        "--graphify",
        graphify,
    ]
    start = time.time()
    proc = subprocess.run(cmd, cwd=repo_root, capture_output=True, text=True, timeout=timeout)
    elapsed = round(time.time() - start, 3)
    if proc.returncode != 0:
        raise RuntimeError(
            f"{language} smoke failed with exit {proc.returncode}\nSTDOUT:\n{proc.stdout[-2000:]}\nSTDERR:\n{proc.stderr[-4000:]}"
        )
    artifact = repo_root / "bench" / f"LIVE_{language.upper()}_SMOKE.json"
    smoke = json.loads(artifact.read_text())
    metrics = query_metrics(smoke.get("queries") or [])
    return smoke, {
        "language": language,
        "seconds": elapsed,
        "artifact": f"bench/{artifact.name}",
        "commit": smoke.get("commit"),
        "coverage_ratio": coverage_ratio(smoke.get("coverage") or {}),
        "coverage": smoke.get("coverage") or {},
        "graphify_detector_only": smoke.get("graphify_detector_only", ""),
        "queries": metrics,
    }


def no_equivalent_saturation(iterations: list[dict[str, Any]]) -> bool:
    if not iterations:
        return False
    return all(
        item["queries"]["rows"] > 0
        and item["queries"]["equivalent_rows"] == 0
        and item["queries"]["graphify_missing"] == item["queries"]["rows"]
        and (item.get("coverage_ratio") or 0) >= 1.0
        for item in iterations
    )


def any_score_improvement(iterations: list[dict[str, Any]]) -> bool:
    best: tuple[float, float, float, float] | None = None
    for item in iterations:
        q = item["queries"]
        current = (
            float(q.get("equivalent_rows") or 0),
            float(q.get("token_ratio") or 0),
            float(q.get("latency_ratio") or 0),
            float(item.get("coverage_ratio") or 0),
        )
        if best is None:
            best = current
            continue
        if any(current[i] > best[i] for i in range(len(current))):
            return True
        best = tuple(max(best[i], current[i]) for i in range(len(current)))
    return False


def summarize_language(language: str, iterations: list[dict[str, Any]]) -> dict[str, Any]:
    saturated_no_equivalent = no_equivalent_saturation(iterations)
    improved = any_score_improvement(iterations)
    if saturated_no_equivalent:
        status = "saturated_no_equivalent_graphify_rows"
        note = (
            "Five repeated live benchmark attempts produced zero graphify-equivalent query rows "
            "while Atlas maintained native coverage >= 1.0; no honest 5x latency/token ratio can be computed."
        )
    elif improved:
        status = "improved"
        note = "At least one repeated benchmark attempt improved equivalent-row count, token ratio, latency ratio, or coverage ratio."
    else:
        status = "saturated_no_score_increase"
        note = "Repeated benchmark attempts did not improve equivalent-row count, token ratio, latency ratio, or coverage ratio."
    return {
        "language": language,
        "status": status,
        "iterations_run": len(iterations),
        "non_improving_iterations": len(iterations) if not improved else 0,
        "saturated": status.startswith("saturated"),
        "note": note,
        "iterations": iterations,
    }


def render_markdown(report: dict[str, Any]) -> str:
    lines: list[str] = []
    w = lines.append
    w("# Atlas benchmark saturation report")
    w("")
    w("This report records repeated live smokes for languages where graphify did not expose equivalent query rows.")
    w("")
    w(f"- Iterations requested per language: {report['iterations_requested']}")
    w(f"- Atlas binary: `{report['atlas']}`")
    w(f"- graphify binary: `{report['graphify']}`")
    w("")
    w("| language | status | iterations | equivalent rows | graphify missing rows | coverage ratio | note |")
    w("|---|---|---:|---|---|---|---|")
    for item in report["languages"]:
        eq = ", ".join(f"{it['queries']['equivalent_rows']}/{it['queries']['rows']}" for it in item["iterations"])
        missing = ", ".join(str(it["queries"]["graphify_missing"]) for it in item["iterations"])
        coverage = ", ".join(str(it.get("coverage_ratio")) for it in item["iterations"])
        w(
            f"| {item['language']} | {item['status']} | {item['iterations_run']} | "
            f"{eq} | {missing} | {coverage} | {item['note']} |"
        )
    w("")
    w("Raw JSON sits next to this report at `bench/SATURATION_REPORT.json`.")
    w("")
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--languages", default="byond,ets,r")
    parser.add_argument("--iterations", type=int, default=5)
    parser.add_argument("--atlas", default=os.environ.get("ATLAS_BIN", "bin/atlas"))
    parser.add_argument("--graphify", default=os.environ.get("GRAPHIFY_BIN", "graphify"))
    parser.add_argument("--timeout", type=int, default=900)
    parser.add_argument("--out", default="bench/SATURATION_REPORT.json")
    args = parser.parse_args()

    if args.iterations < 1:
        raise SystemExit("--iterations must be >= 1")

    repo_root = Path(__file__).resolve().parents[1]
    languages = [part.strip().lower() for part in args.languages.split(",") if part.strip()]
    report = {
        "generated_at_unix": int(time.time()),
        "iterations_requested": args.iterations,
        "atlas": args.atlas,
        "graphify": args.graphify,
        "languages": [],
    }
    for language in languages:
        iterations: list[dict[str, Any]] = []
        for index in range(1, args.iterations + 1):
            print(f"[saturation] {language} iteration {index}/{args.iterations}", flush=True)
            _smoke, metrics = run_smoke(repo_root, language, args.atlas, args.graphify, args.timeout)
            metrics["iteration"] = index
            iterations.append(metrics)
        report["languages"].append(summarize_language(language, iterations))

    out = repo_root / args.out
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(report, indent=2) + "\n")
    md = out.with_suffix(".md")
    md.write_text(render_markdown(report))
    print(json.dumps({"wrote": str(out), "markdown": str(md), "languages": languages}, indent=2))


if __name__ == "__main__":
    main()
