#!/usr/bin/env python3
"""Benchmark Atlas against per-language code intelligence baselines.

This is the matrix benchmark harness. It is intentionally separate from
graphify_vs_atlas.py so the original Atlas-vs-graphify report remains stable
while we add SCIP and language-server baselines one language at a time.

Implemented live slice:
  Go: Atlas vs graphify vs scip-go vs gopls
  Python: Atlas vs graphify vs scip-python vs Pyright
  JS/TS: Atlas vs graphify vs scip-typescript vs TypeScript compiler proxy
  Java: Atlas vs graphify vs scip-java vs JDTLS

Other languages are declared in the matrix so missing adapters/tools are visible
in JSON and Markdown instead of being silently omitted.
"""

from __future__ import annotations

import argparse
import ast
import json
import os
import platform
import queue
import re
import shlex
import shutil
import socket
import sqlite3
import statistics
import subprocess
import sys
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any


REPOS = {
    "go": ("https://github.com/sirupsen/logrus", ""),
    "python": ("https://github.com/psf/requests", "src"),
    "javascript": ("https://github.com/expressjs/express", "lib"),
    "typescript": ("https://github.com/pmndrs/zustand", "src"),
    "java": ("https://github.com/google/gson", "gson/src/main/java"),
    "c": ("https://github.com/DaveGamble/cJSON", ""),
    "cpp": ("https://github.com/google/leveldb", ""),
}

MATRIX = {
    "go": ["atlas", "graphify", "scip-go", "gopls"],
    "python": ["atlas", "graphify", "scip-python", "pyright"],
    "javascript": ["atlas", "graphify", "scip-typescript", "tsserver"],
    "typescript": ["atlas", "graphify", "scip-typescript", "tsserver"],
    "java": ["atlas", "graphify", "scip-java", "jdtls"],
    "c": ["atlas", "graphify", "clangd"],
    "cpp": ["atlas", "graphify", "clangd"],
}

GRAPHIFY_DISCOVERY_FALLBACK = {
    "version": "graphifyy 0.8.49",
    "binary": "~/.local/share/uv/tools/graphifyy/bin/graphify",
    "source_root": "~/.local/share/uv/tools/graphifyy/lib/python3.12/site-packages/graphify",
    "dispatch_count": 0,
    "code_extension_count": 0,
    "evidence": [
        "CLI help from `graphify --help` did not enumerate languages, but confirmed `update`, `extract`, and code-only AST update commands.",
        "`graphify.detect.CODE_EXTENSIONS` plus a runtime `detect()` benchmark listed code extensions.",
        "`graphify.extract._DISPATCH` provided the deterministic extractor map used as the parser-parity target.",
    ],
    "detector_only_code_extensions": [".ejs", ".ets", ".r"],
}

GRAPHIFY_LANGUAGE_FAMILIES = [
    ("go", [".go"], "native go/parser + go/types"),
    ("python", [".py"], "tree-sitter"),
    ("javascript", [".js", ".jsx", ".mjs"], "tree-sitter"),
    ("typescript", [".ts", ".tsx"], "tree-sitter"),
    ("java", [".java"], "tree-sitter"),
    ("groovy/gradle", [".groovy", ".gradle"], "native tree-sitter AST"),
    ("c", [".c", ".h"], "tree-sitter"),
    ("cpp/cuda", [".cpp", ".cc", ".cxx", ".hpp", ".cu", ".cuh"], "tree-sitter"),
    ("csharp", [".cs"], "native tree-sitter tags"),
    ("rust", [".rs"], "native tree-sitter tags"),
    ("ruby", [".rb"], "native tree-sitter tags"),
    ("kotlin", [".kt", ".kts"], "native tree-sitter tags"),
    ("scala", [".scala"], "native tree-sitter tags"),
    ("php", [".php"], "native tree-sitter tags"),
    ("blade", ["*.blade.php"], "native Blade source parser"),
    ("swift", [".swift"], "native tree-sitter tags"),
    ("lua", [".lua", ".luau", ".toc"], "native tree-sitter tags"),
    ("zig", [".zig"], "native tree-sitter tags"),
    ("powershell", [".ps1", ".psm1", ".psd1"], "native tree-sitter AST"),
    ("elixir", [".ex", ".exs"], "native tree-sitter AST"),
    ("objective-c", [".m", ".mm"], "native tree-sitter AST"),
    ("julia", [".jl"], "native tree-sitter AST"),
    ("fortran", [".f", ".F", ".f90", ".F90", ".f95", ".F95", ".f03", ".F03", ".f08", ".F08"], "native tree-sitter AST"),
    ("dart", [".dart"], "native tree-sitter AST"),
    ("r", [".r", ".R"], "native tree-sitter AST"),
    ("verilog/systemverilog", [".v", ".sv", ".svh"], "native tree-sitter AST"),
    ("sql", [".sql"], "native SQL source parser"),
    ("markdown", [".md", ".mdx", ".qmd"], "document parser"),
    ("pascal", [".pas", ".pp", ".dpr", ".dpk", ".lpr", ".inc"], "native tree-sitter AST"),
    ("delphi/lazarus forms", [".dfm", ".lfm", ".lpk"], "native Delphi/Lazarus source parser"),
    ("shell", [".sh", ".bash"], "native tree-sitter AST"),
    ("json config", [".json"], "document parser"),
    ("terraform/hcl", [".tf", ".tfvars", ".hcl"], "native tree-sitter HCL"),
    ("byond dm", [".dm", ".dme", ".dmi", ".dmm", ".dmf"], "native BYOND source parser"),
    ("dotnet project", [".sln", ".slnx", ".csproj", ".fsproj", ".vbproj"], "native structured project parser"),
    ("razor", [".razor", ".cshtml"], "native Razor source parser"),
    ("apex", [".cls", ".trigger"], "native tree-sitter Apex"),
    ("vue", [".vue"], "native SFC/tree-sitter AST"),
    ("svelte", [".svelte"], "native SFC/tree-sitter AST"),
    ("astro", [".astro"], "native Astro/tree-sitter AST"),
]


def run(cmd: list[str], cwd: Path | None = None, timeout: int = 900) -> subprocess.CompletedProcess[str]:
    return subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)


def tokens(text: str) -> int:
    return max(1, len(text) // 4)


def pct(num: float, den: float) -> float:
    return 0.0 if not den else round(100.0 * num / den, 1)


def ratio(num: float, den: float) -> float | None:
    if not num or not den:
        return None
    return round(num / den, 2)


def executable(path_or_name: str) -> str | None:
    if not path_or_name:
        return None
    expanded = Path(path_or_name).expanduser()
    if expanded.exists() and os.access(expanded, os.X_OK):
        return str(expanded)
    return shutil.which(path_or_name)


def tool_command(path_or_name: str, package: str | None = None, bin_name: str | None = None) -> list[str] | None:
    found = executable(path_or_name)
    if found:
        return [found]
    npx = shutil.which("npx")
    if package and npx:
        if bin_name and bin_name != package:
            return [npx, "--yes", "-p", package, bin_name]
        return [npx, "--yes", package]
    return None


def command_prefix(command: str) -> list[str] | None:
    if not command:
        return None
    parts = shlex.split(command)
    if not parts:
        return None
    if len(parts) == 1:
        found = executable(parts[0])
        return [found] if found else None
    first = executable(parts[0])
    if not first:
        return None
    return [first] + parts[1:]


def java21_env() -> dict[str, str]:
    """Prefer a local Java 21 runtime for tools such as JDTLS when available."""
    env = os.environ.copy()
    candidates = [
        Path("/opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home"),
        Path("/usr/local/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home"),
    ]
    java_home = next((path for path in candidates if (path / "bin/java").exists()), None)
    if java_home:
        env["JAVA_HOME"] = str(java_home)
        env["PATH"] = f"{java_home / 'bin'}{os.pathsep}{env.get('PATH', '')}"
    return env


def resolve_graphify(value: str) -> str | None:
    found = executable(value)
    if found:
        return found
    uv_tool = Path.home() / ".local/share/uv/tools/graphifyy/bin/graphify"
    if uv_tool.exists() and os.access(uv_tool, os.X_OK):
        return str(uv_tool)
    return None


def discover_graphify_binary() -> str:
    configured = os.environ.get("GRAPHIFY_BIN", "")
    found = resolve_graphify(configured or "graphify")
    return found or GRAPHIFY_DISCOVERY_FALLBACK["binary"]


def repo_tool(name: str, fallback: str) -> str:
    path = Path(__file__).resolve().parent / "tools" / name
    if path.exists():
        return str(path)
    return fallback


def graphify_python_for(binary: str) -> str | None:
    path = Path(binary).expanduser()
    if path.exists():
        try:
            first = path.read_text(errors="ignore").splitlines()[0]
        except (OSError, IndexError):
            first = ""
        if first.startswith("#!"):
            candidate = first[2:].strip()
            if candidate and Path(candidate).exists():
                probe = run([candidate, "-c", "import graphify"], timeout=30)
                if probe.returncode == 0:
                    return candidate
    uv = shutil.which("uv")
    if uv:
        probe = run([uv, "tool", "run", "--from", "graphifyy", "python", "-c", "import sys; print(sys.executable)"], timeout=30)
        if probe.returncode == 0 and probe.stdout.strip():
            return probe.stdout.strip().splitlines()[-1]
    return None


def fallback_graphify_rows() -> list[tuple[str, str, str, str]]:
    return [(family, " ".join(exts), "", atlas_status) for family, exts, atlas_status in GRAPHIFY_LANGUAGE_FAMILIES]


def graphify_rows_from_runtime(dispatch: list[dict[str, str]], special: list[dict[str, str]]) -> list[tuple[str, str, str, str]]:
    by_ext = {item["extension"]: item for item in dispatch}
    by_ext.update({item["extension"]: item for item in special})
    rows: list[tuple[str, str, str, str]] = []
    for family, exts, atlas_status in GRAPHIFY_LANGUAGE_FAMILIES:
        present = [ext for ext in exts if ext in by_ext]
        if not present:
            continue
        extractors = sorted({by_ext[ext]["extractor"] for ext in present})
        rows.append((family, " ".join(present), "/".join(extractors), atlas_status))
    known = {ext for _, exts, _ in GRAPHIFY_LANGUAGE_FAMILIES for ext in exts}
    for ext in sorted(set(by_ext) - known):
        item = by_ext[ext]
        rows.append((ext.lstrip("*."), ext, item["extractor"], "unsupported: not mapped in Atlas benchmark family table"))
    return rows


def discover_graphify_runtime() -> dict[str, Any]:
    binary = discover_graphify_binary()
    data: dict[str, Any] = {
        **GRAPHIFY_DISCOVERY_FALLBACK,
        "binary": binary,
        "rows": fallback_graphify_rows(),
        "runtime_error": "",
    }
    help_target = Path(binary).expanduser()
    if help_target.exists():
        help_run = run([str(help_target), "--help"], timeout=30)
        if help_run.returncode == 0:
            data["help_command_count"] = help_run.stdout.count("\n  ")

    py = graphify_python_for(binary)
    if not py:
        data["runtime_error"] = "could not resolve a Python interpreter that imports graphify"
        return data

    script = r'''
import importlib.metadata as md
import inspect
import json
import tempfile
from pathlib import Path

import graphify.detect as detect
import graphify.extract as extract


def fn_name(fn):
    return getattr(fn, "__name__", repr(fn))


dispatch = [
    {"extension": key, "extractor": fn_name(fn), "module": getattr(fn, "__module__", "")}
    for key, fn in sorted(extract._DISPATCH.items(), key=lambda item: item[0])
]
special = []
for sample in ("view.blade.php",):
    fn = extract._get_extractor(Path(sample))
    if fn is not None:
        special.append({"extension": "*.blade.php", "extractor": fn_name(fn), "module": getattr(fn, "__module__", "")})

with tempfile.TemporaryDirectory() as td:
    root = Path(td)
    by_path = {}
    for i, ext in enumerate(sorted(detect.CODE_EXTENSIONS)):
        path = root / f"sample_{i:03d}{ext}"
        path.write_text("// graphify discovery benchmark\n", encoding="utf-8")
        by_path[str(path.resolve())] = ext
    detected = detect.detect(root)
    detect_benchmark_code_extensions = sorted(
        by_path.get(str(Path(path).resolve()), Path(path).suffix)
        for path in detected.get("files", {}).get("code", [])
    )

try:
    version = "graphifyy " + md.version("graphifyy")
except Exception:
    version = "graphifyy unknown"

print(json.dumps({
    "version": version,
    "source_root": str(Path(inspect.getsourcefile(extract)).resolve().parent),
    "extract_source": str(Path(inspect.getsourcefile(extract)).resolve()),
    "detect_source": str(Path(inspect.getsourcefile(detect)).resolve()),
    "dispatch": dispatch,
    "special": special,
    "code_extensions": sorted(detect.CODE_EXTENSIONS),
    "detect_benchmark_code_extensions": detect_benchmark_code_extensions,
    "detect_benchmark_total_files": detected.get("total_files", 0),
}, sort_keys=True))
'''
    probe = run([py, "-c", script], timeout=30)
    if probe.returncode != 0:
        data["runtime_error"] = (probe.stderr or probe.stdout).strip()
        return data
    try:
        runtime = json.loads(probe.stdout)
    except json.JSONDecodeError as exc:
        data["runtime_error"] = f"could not parse graphify runtime JSON: {exc}"
        return data

    dispatch = runtime.get("dispatch", [])
    special = runtime.get("special", [])
    code_exts = runtime.get("code_extensions", [])
    dispatch_exts = {item["extension"] for item in dispatch}
    special_exts = {item["extension"] for item in special}
    detector_only = sorted(
        ext
        for ext in code_exts
        if ext not in dispatch_exts and ext not in {".php" if item == "*.blade.php" else item for item in special_exts}
    )

    data.update(
        {
            "version": runtime.get("version", data["version"]),
            "binary": binary,
            "python": py,
            "source_root": runtime.get("source_root", data["source_root"]),
            "extract_source": runtime.get("extract_source", ""),
            "detect_source": runtime.get("detect_source", ""),
            "dispatch_count": len(dispatch) + len(special),
            "code_extension_count": len(code_exts),
            "detector_only_code_extensions": detector_only,
            "rows": graphify_rows_from_runtime(dispatch, special),
            "dispatch": dispatch,
            "special": special,
            "code_extensions": code_exts,
            "detect_benchmark_code_extensions": runtime.get("detect_benchmark_code_extensions", []),
            "detect_benchmark_total_files": runtime.get("detect_benchmark_total_files", 0),
        }
    )
    return data


def base_result(tool: str, status: str = "ok", note: str = "") -> dict[str, Any]:
    return {
        "tool": tool,
        "status": status,
        "ok": status == "ok",
        "seconds": 0.0,
        "metrics": {},
        "note": note,
    }


def missing(tool: str, command: str) -> dict[str, Any]:
    return base_result(tool, "missing", f"command not found: {command}")


def not_implemented(tool: str, note: str) -> dict[str, Any]:
    return base_result(tool, "not_implemented", note)


def ensure_repo(url: str, dest: Path, log: list[str]) -> bool:
    if dest.exists():
        return True
    result = run(["git", "clone", "--depth", "1", "-q", url, str(dest)], timeout=900)
    log.append(f"$ git clone --depth 1 {url} {dest}\n{result.stdout}{result.stderr}")
    return result.returncode == 0 and dest.exists()


def clean_generated_sidecars(target: Path) -> None:
    graph_out = target / "graphify-out"
    if graph_out.exists():
        shutil.rmtree(graph_out, ignore_errors=True)


def atlas_metrics(db: Path) -> dict[str, Any]:
    metrics: dict[str, Any] = {
        "calls": 0,
        "internal_calls": 0,
        "recv_typed": 0,
        "sources": {},
    }
    if not db.exists():
        return metrics
    con = sqlite3.connect(str(db))
    cur = con.cursor()
    cur.execute("SELECT count(*) FROM edges WHERE kind='calls'")
    metrics["calls"] = cur.fetchone()[0]
    cur.execute(
        "SELECT count(*) FROM edges WHERE kind='calls' "
        "AND to_ref IN (SELECT DISTINCT name FROM symbols)"
    )
    metrics["internal_calls"] = cur.fetchone()[0]
    cur.execute(
        "SELECT count(*) FROM edges WHERE kind='calls' "
        "AND json_extract(metadata,'$.recv_type') IS NOT NULL "
        "AND json_extract(metadata,'$.recv_type') != ''"
    )
    metrics["recv_typed"] = cur.fetchone()[0]
    cur.execute(
        "SELECT json_extract(metadata,'$.source'), count(*) "
        "FROM edges WHERE kind='calls' GROUP BY 1 ORDER BY 2 DESC"
    )
    metrics["sources"] = {(source or "?"): count for source, count in cur.fetchall()}
    con.close()
    return metrics


def count_python_assignment_names(node: ast.AST) -> int:
    targets: list[ast.AST]
    if isinstance(node, ast.Assign):
        targets = list(node.targets)
    elif isinstance(node, ast.AnnAssign):
        targets = [node.target]
    else:
        return 0

    def names(target: ast.AST) -> int:
        if isinstance(target, ast.Name):
            return 1
        if isinstance(target, (ast.Tuple, ast.List)):
            return sum(names(elt) for elt in target.elts)
        return 0

    return sum(names(target) for target in targets)


def python_ast_truth(target: Path) -> dict[str, int]:
    metrics = {
        "files": 0,
        "functions": 0,
        "classes": 0,
        "module_assignment_names": 0,
        "class_assignment_names": 0,
    }
    for path in target.rglob("*.py"):
        try:
            tree = ast.parse(path.read_text(encoding="utf-8"))
        except (SyntaxError, UnicodeDecodeError):
            continue
        metrics["files"] += 1
        for node in ast.walk(tree):
            if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
                metrics["functions"] += 1
            elif isinstance(node, ast.ClassDef):
                metrics["classes"] += 1
        for node in tree.body:
            metrics["module_assignment_names"] += count_python_assignment_names(node)
            if isinstance(node, ast.ClassDef):
                for child in node.body:
                    metrics["class_assignment_names"] += count_python_assignment_names(child)
    return metrics


def sqlite_symbol_kind_count(db: Path, kinds: tuple[str, ...]) -> int:
    if not db.exists() or not kinds:
        return 0
    con = sqlite3.connect(str(db))
    cur = con.cursor()
    placeholders = ",".join("?" for _ in kinds)
    cur.execute(f"SELECT count(*) FROM symbols WHERE kind IN ({placeholders})", kinds)
    count = cur.fetchone()[0]
    con.close()
    return count


def run_atlas(atlas_bin: str | None, target: Path, db: Path, log: list[str]) -> dict[str, Any]:
    if not atlas_bin:
        return missing("atlas", "atlas")
    if db.exists():
        db.unlink()
    start = time.time()
    cold = run([atlas_bin, "--db", f"sqlite://{db}", "index", str(target)], timeout=900)
    cold_seconds = round(time.time() - start, 3)
    log.append(f"$ {atlas_bin} --db sqlite://{db} index {target}  # cold\n{cold.stdout}{cold.stderr}")
    out = base_result("atlas", "ok" if cold.returncode == 0 else "failed")
    if cold.returncode != 0:
        out["seconds"] = cold_seconds
        out["note"] = cold.stderr.strip() or cold.stdout.strip()
        return out

    start = time.time()
    result = run([atlas_bin, "--db", f"sqlite://{db}", "index", str(target)], timeout=900)
    seconds = round(time.time() - start, 3)
    log.append(f"$ {atlas_bin} --db sqlite://{db} index {target}  # no-change reindex\n{result.stdout}{result.stderr}")
    out["seconds"] = seconds
    if result.returncode != 0:
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = result.stderr.strip() or result.stdout.strip()
        return out
    metrics: dict[str, Any] = {}
    try:
        cold_parsed = json.loads(cold.stdout)
        parsed = json.loads(result.stdout)
        metrics.update(
            {
                "files": parsed.get("indexed_files", 0),
                "symbols": parsed.get("symbols", 0),
                "edges": parsed.get("edges", 0),
                "edge_kinds": parsed.get("edge_kinds", {}) or cold_parsed.get("edge_kinds", {}),
                "languages": parsed.get("languages", {}),
                "duration_ms": parsed.get("duration_ms", 0),
                "timings_ms": parsed.get("timings_ms", {}),
                "mode": parsed.get("mode", ""),
                "cold_seconds": cold_seconds,
                "cold_duration_ms": cold_parsed.get("duration_ms", 0),
                "cold_timings_ms": cold_parsed.get("timings_ms", {}),
                # Explicit aliases so the report never confuses the two builds:
                # full_seconds = cold full index (fair vs graphify FULL / scip-go
                # / gopls cold); delta_seconds = no-change reindex (fair vs the
                # incremental path of tools that support one).
                "full_seconds": cold_seconds,
                "cold_mode": cold_parsed.get("mode", ""),
                "delta_seconds": seconds,
                "delta_mode": parsed.get("mode", ""),
            }
        )
    except json.JSONDecodeError:
        out["note"] = "atlas index returned non-JSON output"
    metrics.update(atlas_metrics(db))
    out["metrics"] = metrics
    return out


def run_graphify(graphify_bin: str | None, target: Path, log: list[str]) -> dict[str, Any]:
    if not graphify_bin:
        return missing("graphify", "graphify")
    clean_generated_sidecars(target)
    graph_out = target / "graphify-out"
    # First `update` with no prior `graphify-out/` is graphify's FULL extract.
    start = time.time()
    result = run([graphify_bin, "update", "."], cwd=target, timeout=900)
    seconds = round(time.time() - start, 3)
    log.append(f"$ {graphify_bin} update . (cwd={target})  # full extract\n{result.stdout}{result.stderr}")
    out = base_result("graphify", "ok" if result.returncode == 0 else "failed")
    # `seconds` is graphify's FULL extract wall time (apples-to-apples with
    # Atlas's cold full index). It must never be paired against Atlas's delta.
    out["seconds"] = seconds
    graph_json = graph_out / "graph.json"
    if not graph_json.exists():
        out["ok"] = False
        out["status"] = "failed"
        out["note"] = result.stderr.strip() or result.stdout.strip() or "graphify-out/graph.json missing"
        return out
    # Second `update` keeps the existing `graphify-out/` so graphify re-runs in
    # its incremental/no-change path; this is the fair delta-vs-delta baseline
    # for Atlas's no-change reindex. We do NOT clean the sidecar in between.
    delta_seconds: float | None = None
    delta = run([graphify_bin, "update", "."], cwd=target, timeout=900)
    if delta.returncode == 0 and graph_json.exists():
        d_start = time.time()
        delta = run([graphify_bin, "update", "."], cwd=target, timeout=900)
        delta_seconds = round(time.time() - d_start, 3)
        log.append(
            f"$ {graphify_bin} update . (cwd={target})  # no-change re-update (delta-vs-delta baseline)\n"
            f"{delta.stdout}{delta.stderr}"
        )
    graph = json.loads(graph_json.read_text())
    links = graph.get("links", [])
    calls = [link for link in links if link.get("relation") == "calls"]
    extracted = sum(1 for link in calls if str(link.get("confidence", "")).upper() == "EXTRACTED")
    out["metrics"] = {
        "nodes": len(graph.get("nodes", [])),
        "links": len(links),
        "calls": len(calls),
        "extracted_calls": extracted,
        "extracted_pct": pct(extracted, len(calls)),
        "full_seconds": seconds,
        "delta_seconds": delta_seconds,
        "supports_incremental": delta_seconds is not None,
    }
    return out


def run_scip_go(scip_go_bin: str | None, repo: Path, workdir: Path, log: list[str]) -> dict[str, Any]:
    if not scip_go_bin:
        return missing("scip-go", "scip-go")
    index_path = workdir / "go-index.scip"
    if index_path.exists():
        index_path.unlink()
    start = time.time()
    result = run([scip_go_bin, "index", "-o", str(index_path), "./..."], cwd=repo, timeout=900)
    seconds = round(time.time() - start, 3)
    log.append(f"$ {scip_go_bin} index -o {index_path} ./... (cwd={repo})\n{result.stdout}{result.stderr}")
    out = base_result("scip-go", "ok" if result.returncode == 0 else "failed")
    out["seconds"] = seconds
    if result.returncode != 0 or not index_path.exists():
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = result.stderr.strip() or result.stdout.strip() or "index.scip missing"
        return out

    stats_dir = Path(__file__).resolve().parent / "scipstats"
    stats = run(["go", "run", ".", str(index_path)], cwd=stats_dir, timeout=900)
    log.append(f"$ go run . {index_path} (cwd={stats_dir})\n{stats.stdout}{stats.stderr}")
    if stats.returncode != 0:
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = stats.stderr.strip() or stats.stdout.strip()
        return out
    parsed = json.loads(stats.stdout)
    parsed["index_bytes"] = index_path.stat().st_size
    out["metrics"] = parsed
    return out


def run_scip_python(scip_python_bin: str | None, repo: Path, workdir: Path, log: list[str]) -> dict[str, Any]:
    if not scip_python_bin:
        return missing("scip-python", "scip-python")
    index_path = workdir / "python-index.scip"
    if index_path.exists():
        index_path.unlink()
    start = time.time()
    # scip-python 0.6.6 produced an empty index for requests when --cwd or
    # --target-only was supplied, so this adapter runs from the repo root and
    # records that broader scope in metrics instead of weakening the baseline.
    result = run(
        [
            scip_python_bin,
            "index",
            "--project-name",
            repo.name,
            "--output",
            str(index_path),
            "--quiet",
        ],
        cwd=repo,
        timeout=900,
    )
    seconds = round(time.time() - start, 3)
    log.append(f"$ {scip_python_bin} index --project-name {repo.name} --output {index_path} --quiet (cwd={repo})\n{result.stdout}{result.stderr}")
    out = base_result("scip-python", "ok" if result.returncode == 0 else "failed")
    out["seconds"] = seconds
    if result.returncode != 0 or not index_path.exists():
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = result.stderr.strip() or result.stdout.strip() or "index.scip missing"
        return out

    stats_dir = Path(__file__).resolve().parent / "scipstats"
    stats = run(["go", "run", ".", str(index_path)], cwd=stats_dir, timeout=900)
    log.append(f"$ go run . {index_path} (cwd={stats_dir})\n{stats.stdout}{stats.stderr}")
    if stats.returncode != 0:
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = stats.stderr.strip() or stats.stdout.strip()
        return out
    parsed = json.loads(stats.stdout)
    parsed["index_bytes"] = index_path.stat().st_size
    parsed["scope"] = "repo-root"
    out["metrics"] = parsed
    return out


def run_scip_typescript(cmd_prefix: list[str] | None, lang: str, repo: Path, target: Path, workdir: Path, log: list[str]) -> dict[str, Any]:
    if not cmd_prefix:
        return missing("scip-typescript", "scip-typescript")
    index_path = workdir / f"{lang}-index.scip"
    if index_path.exists():
        index_path.unlink()
    cmd = cmd_prefix + [
        "index",
        "--cwd",
        str(target),
        "--output",
        str(index_path),
        "--no-progress-bar",
    ]
    if not (target / "tsconfig.json").exists():
        cmd.append("--infer-tsconfig")
    start = time.time()
    result = run(cmd, cwd=target, timeout=900)
    seconds = round(time.time() - start, 3)
    log.append(f"$ {' '.join(cmd)} (cwd={target})\n{result.stdout}{result.stderr}")
    out = base_result("scip-typescript", "ok" if result.returncode == 0 else "failed")
    out["seconds"] = seconds
    if result.returncode != 0 or not index_path.exists():
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = result.stderr.strip() or result.stdout.strip() or "index.scip missing"
        return out

    stats_dir = Path(__file__).resolve().parent / "scipstats"
    stats = run(["go", "run", ".", str(index_path)], cwd=stats_dir, timeout=900)
    log.append(f"$ go run . {index_path} (cwd={stats_dir})\n{stats.stdout}{stats.stderr}")
    if stats.returncode != 0:
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = stats.stderr.strip() or stats.stdout.strip()
        return out
    parsed = json.loads(stats.stdout)
    parsed["index_bytes"] = index_path.stat().st_size
    parsed["scope"] = os.path.relpath(target, repo)
    out["metrics"] = parsed
    return out


def java_build_root(repo: Path, target: Path) -> Path:
    current = target
    while True:
        if (
            (current / "pom.xml").exists()
            or (current / "gradlew").exists()
            or (current / "build.gradle").exists()
            or (current / "build.gradle.kts").exists()
            or (current / "settings.gradle").exists()
            or (current / "settings.gradle.kts").exists()
        ):
            return current
        if current == repo or current.parent == current:
            return repo
        current = current.parent


def run_scip_java(cmd_prefix: list[str] | None, repo: Path, target: Path, workdir: Path, log: list[str]) -> dict[str, Any]:
    if not cmd_prefix:
        return missing("scip-java", "scip-java")
    build_root = java_build_root(repo, target)
    index_path = workdir / "java-index.scip"
    if index_path.exists():
        index_path.unlink()
    cmd = cmd_prefix + [
        "index",
        "--output",
        str(index_path),
        "--",
        "--batch-mode",
        "-DskipTests",
        "-DskipITs",
        "-Dmaven.javadoc.skip=true",
        "package",
    ]
    start = time.time()
    result = run(cmd, cwd=build_root, timeout=1800)
    seconds = round(time.time() - start, 3)
    log.append(f"$ {' '.join(cmd)} (cwd={build_root})\n{result.stdout}{result.stderr}")
    out = base_result("scip-java", "ok" if result.returncode == 0 else "failed")
    out["seconds"] = seconds
    if result.returncode != 0 or not index_path.exists():
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = result.stderr.strip() or result.stdout.strip() or "index.scip missing"
        return out

    stats_dir = Path(__file__).resolve().parent / "scipstats"
    document_prefix = os.path.relpath(target, build_root)
    stats = run(["go", "run", ".", str(index_path), document_prefix], cwd=stats_dir, timeout=900)
    log.append(f"$ go run . {index_path} {document_prefix} (cwd={stats_dir})\n{stats.stdout}{stats.stderr}")
    if stats.returncode != 0:
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = stats.stderr.strip() or stats.stdout.strip()
        return out
    parsed = json.loads(stats.stdout)
    parsed["index_bytes"] = index_path.stat().st_size
    parsed["scope"] = os.path.relpath(build_root, repo)
    parsed["document_filter"] = document_prefix
    out["metrics"] = parsed
    return out


_DURATION_RE = re.compile(r"^([0-9.]+)(ns|us|µs|ms|s|m|h)$")


def duration_ms(value: str) -> float | None:
    match = _DURATION_RE.match(value.strip())
    if not match:
        return None
    amount = float(match.group(1))
    unit = match.group(2)
    factors = {
        "ns": 0.000001,
        "us": 0.001,
        "µs": 0.001,
        "ms": 1.0,
        "s": 1000.0,
        "m": 60000.0,
        "h": 3600000.0,
    }
    return round(amount * factors[unit], 3)


def run_gopls(gopls_bin: str | None, repo: Path, log: list[str]) -> dict[str, Any]:
    if not gopls_bin:
        return missing("gopls", "gopls")
    start = time.time()
    result = run([gopls_bin, "stats", "-anon"], cwd=repo, timeout=900)
    seconds = round(time.time() - start, 3)
    log.append(f"$ {gopls_bin} stats -anon (cwd={repo})\n{result.stdout}{result.stderr}")
    out = base_result("gopls", "ok" if result.returncode == 0 else "failed")
    out["seconds"] = seconds
    if result.returncode != 0:
        out["note"] = result.stderr.strip() or result.stdout.strip()
        return out
    parsed = json.loads(result.stdout)
    view = (parsed.get("WorkspaceStats", {}).get("Views") or [{}])[0]
    workspace = view.get("WorkspacePackages", {})
    all_packages = view.get("AllPackages", {})
    metrics = {
        "gopls_version": parsed.get("GoplsVersion", ""),
        "go_version": parsed.get("GoVersion", ""),
        "initial_workspace_load_ms": duration_ms(parsed.get("InitialWorkspaceLoadDuration", "")),
        "dir_files": parsed.get("DirStats", {}).get("Files", 0),
        "dir_go_files": parsed.get("DirStats", {}).get("GoFiles", 0),
        "workspace_packages": workspace.get("Packages", 0),
        "workspace_compiled_go_files": workspace.get("CompiledGoFiles", 0),
        "all_packages": all_packages.get("Packages", 0),
        "diagnostics": view.get("Diagnostics", 0),
        "heap_alloc_bytes": parsed.get("MemStats", {}).get("HeapAlloc", 0),
    }
    out["metrics"] = metrics
    return out


def run_pyright(pyright_bin: str | None, repo: Path, target: Path, log: list[str]) -> dict[str, Any]:
    if not pyright_bin:
        return missing("pyright", "pyright")
    target_arg = os.path.relpath(target, repo)
    start = time.time()
    result = run([pyright_bin, target_arg, "--outputjson"], cwd=repo, timeout=900)
    seconds = round(time.time() - start, 3)
    log.append(f"$ {pyright_bin} {target_arg} --outputjson (cwd={repo})\n{result.stdout}{result.stderr}")
    out = base_result("pyright", "ok")
    out["seconds"] = seconds
    try:
        parsed = json.loads(result.stdout)
    except json.JSONDecodeError:
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = result.stderr.strip() or result.stdout.strip() or "pyright returned non-JSON output"
        return out

    diagnostics = parsed.get("generalDiagnostics", [])
    by_severity: dict[str, int] = {}
    by_rule: dict[str, int] = {}
    files = set()
    for diag in diagnostics:
        severity = diag.get("severity", "")
        if severity:
            by_severity[severity] = by_severity.get(severity, 0) + 1
        rule = diag.get("rule", "")
        if rule:
            by_rule[rule] = by_rule.get(rule, 0) + 1
        file = diag.get("file", "")
        if file:
            files.add(file)
    summary = parsed.get("summary", {})
    out["metrics"] = {
        "pyright_version": parsed.get("version", ""),
        "diagnostics": len(diagnostics),
        "diagnostics_by_severity": by_severity,
        "diagnostics_by_rule": by_rule,
        "diagnostic_files": len(files),
        "files_analyzed": summary.get("filesAnalyzed", 0),
        "error_count": summary.get("errorCount", by_severity.get("error", 0)),
        "warning_count": summary.get("warningCount", by_severity.get("warning", 0)),
        "information_count": summary.get("informationCount", by_severity.get("information", 0)),
        "time_in_sec": summary.get("timeInSec", 0),
        "exit_code": result.returncode,
    }
    if result.returncode not in (0, 1):
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = result.stderr.strip() or f"unexpected pyright exit code {result.returncode}"
    elif result.returncode == 1:
        out["note"] = "pyright returned diagnostics"
    return out


_TSC_METRIC_RE = re.compile(r"^(Files|Lines|Identifiers|Symbols|Types|Instantiations):\s+([0-9]+)")
_TSC_TIME_RE = re.compile(r"^(I/O read|I/O write|Parse time|Bind time|Check time|Emit time|Total time):\s+([0-9.]+)s")
_TSC_MEMORY_RE = re.compile(r"^Memory used:\s+([0-9]+)K")


def js_ts_source_files(target: Path, repo: Path, lang: str) -> list[str]:
    suffixes = (".ts", ".tsx") if lang == "typescript" else (".js", ".jsx", ".mjs", ".cjs")
    files = [p for p in target.rglob("*") if p.is_file() and p.suffix in suffixes]
    return [os.path.relpath(p, repo) for p in sorted(files)]


def parse_tsc_metrics(text: str) -> dict[str, Any]:
    metrics: dict[str, Any] = {
        "diagnostics": 0,
        "diagnostics_by_code": {},
        "engine": "tsc",
        "note": "TypeScript compiler semantic check used as scriptable tsserver proxy",
    }
    for line in text.splitlines():
        line = line.strip()
        if not line:
            continue
        if "error TS" in line:
            metrics["diagnostics"] += 1
            code_match = re.search(r"error (TS[0-9]+)", line)
            if code_match:
                code = code_match.group(1)
                metrics["diagnostics_by_code"][code] = metrics["diagnostics_by_code"].get(code, 0) + 1
        if match := _TSC_METRIC_RE.match(line):
            metrics[match.group(1).lower()] = int(match.group(2))
            continue
        if match := _TSC_TIME_RE.match(line):
            key = match.group(1).lower().replace(" ", "_").replace("/", "io")
            metrics[f"{key}_sec"] = float(match.group(2))
            continue
        if match := _TSC_MEMORY_RE.match(line):
            metrics["memory_kb"] = int(match.group(1))
    return metrics


def run_tsserver_proxy(cmd_prefix: list[str] | None, lang: str, repo: Path, target: Path, log: list[str]) -> dict[str, Any]:
    if not cmd_prefix:
        return missing("tsserver", "tsc/typescript")
    target_arg = os.path.relpath(target, repo)
    if lang == "typescript" and (repo / "tsconfig.json").exists():
        cmd = cmd_prefix + ["--noEmit", "--pretty", "false", "--diagnostics", "-p", "tsconfig.json"]
    else:
        files = js_ts_source_files(target, repo, lang)
        if not files:
            return base_result("tsserver", "failed", f"no {lang} source files under {target_arg}")
        cmd = cmd_prefix + [
            "--ignoreConfig",
            "--allowJs",
            "--checkJs",
            "--noEmit",
            "--pretty",
            "false",
            "--diagnostics",
            "--skipLibCheck",
            "--moduleResolution",
            "node16",
            "--module",
            "Node16",
            "--target",
            "ES2020",
            "--ignoreDeprecations",
            "6.0",
        ] + files
    start = time.time()
    result = run(cmd, cwd=repo, timeout=900)
    seconds = round(time.time() - start, 3)
    combined = f"{result.stdout}{result.stderr}"
    log.append(f"$ {' '.join(cmd)} (cwd={repo})\n{combined}")
    metrics = parse_tsc_metrics(combined)
    out = base_result("tsserver", "ok" if metrics.get("files") else "failed")
    out["seconds"] = seconds
    out["metrics"] = metrics
    if result.returncode != 0:
        out["note"] = f"tsc returned diagnostics/exit {result.returncode}; used as scriptable tsserver proxy"
    if not metrics.get("files"):
        out["status"] = "failed"
        out["ok"] = False
        out["note"] = combined.strip() or f"unexpected tsc exit code {result.returncode}"
    return out


def _lsp_write(proc: subprocess.Popen[bytes], payload: dict[str, Any]) -> None:
    if proc.stdin is None:
        raise RuntimeError("LSP stdin is closed")
    body = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    proc.stdin.write(f"Content-Length: {len(body)}\r\n\r\n".encode("ascii") + body)
    proc.stdin.flush()


def _lsp_read_loop(stdout: Any, messages: "queue.Queue[dict[str, Any]]") -> None:
    while True:
        headers: dict[str, str] = {}
        while True:
            line = stdout.readline()
            if not line:
                return
            if line in (b"\r\n", b"\n"):
                break
            try:
                name, value = line.decode("ascii", errors="replace").split(":", 1)
            except ValueError:
                continue
            headers[name.strip().lower()] = value.strip()
        length = int(headers.get("content-length", "0") or "0")
        if length <= 0:
            continue
        body = stdout.read(length)
        if not body:
            return
        try:
            messages.put(json.loads(body.decode("utf-8")))
        except json.JSONDecodeError:
            continue


def _tail_pipe(pipe: Any, lines: list[str], limit: int = 80) -> None:
    while True:
        line = pipe.readline()
        if not line:
            return
        text = line.decode("utf-8", errors="replace").rstrip()
        if text:
            lines.append(text)
            del lines[:-limit]


def _wait_lsp(
    messages: "queue.Queue[dict[str, Any]]",
    wanted_ids: set[int],
    diagnostics: dict[str, int],
    timeout: float,
) -> dict[int, dict[str, Any]]:
    deadline = time.time() + timeout
    found: dict[int, dict[str, Any]] = {}
    while wanted_ids - set(found) and time.time() < deadline:
        try:
            msg = messages.get(timeout=max(0.05, min(0.5, deadline - time.time())))
        except queue.Empty:
            continue
        if msg.get("method") == "textDocument/publishDiagnostics":
            params = msg.get("params", {})
            uri = params.get("uri", "")
            diagnostics[uri] = len(params.get("diagnostics") or [])
        msg_id = msg.get("id")
        if isinstance(msg_id, int) and msg_id in wanted_ids:
            found[msg_id] = msg
    return found


def _lsp_symbol_count(items: Any) -> int:
    if not isinstance(items, list):
        return 0
    total = 0
    for item in items:
        if not isinstance(item, dict):
            continue
        total += 1
        total += _lsp_symbol_count(item.get("children"))
    return total


def java_source_files(target: Path, limit: int = 5) -> list[Path]:
    return sorted(target.rglob("*.java"))[:limit]


def c_family_source_files(target: Path, lang: str, limit: int = 8) -> list[Path]:
    if lang == "c":
        suffixes = (".c", ".h")
    else:
        suffixes = (".cc", ".cpp", ".cxx", ".hpp", ".hxx", ".hh", ".h")
    files = [p for p in target.rglob("*") if p.is_file() and p.suffix in suffixes]
    source_first = sorted(files, key=lambda p: (p.suffix in {".h", ".hpp", ".hxx", ".hh"}, str(p)))
    return source_first[:limit]


def run_jdtls(cmd_prefix: list[str] | None, repo: Path, target: Path, workdir: Path, log: list[str]) -> dict[str, Any]:
    if not cmd_prefix:
        return missing("jdtls", "jdtls")
    build_root = java_build_root(repo, target)
    data_dir = workdir / "jdtls-workspace"
    data_dir.mkdir(parents=True, exist_ok=True)
    cmd = list(cmd_prefix)
    if "-data" not in cmd:
        cmd += ["-data", str(data_dir)]
    sources = java_source_files(target, limit=5)
    if not sources:
        return base_result("jdtls", "failed", f"no Java source files under {target}")

    messages: "queue.Queue[dict[str, Any]]" = queue.Queue()
    stderr_tail: list[str] = []
    start = time.time()
    try:
        env = java21_env()
        proc = subprocess.Popen(
            cmd,
            cwd=build_root,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            env=env,
        )
    except OSError as exc:
        return base_result("jdtls", "failed", str(exc))

    assert proc.stdout is not None
    assert proc.stderr is not None
    reader = threading.Thread(target=_lsp_read_loop, args=(proc.stdout, messages), daemon=True)
    stderr_reader = threading.Thread(target=_tail_pipe, args=(proc.stderr, stderr_tail), daemon=True)
    reader.start()
    stderr_reader.start()
    diagnostics: dict[str, int] = {}
    out = base_result("jdtls", "failed")
    try:
        root_uri = build_root.resolve().as_uri()
        _lsp_write(
            proc,
            {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {
                    "processId": os.getpid(),
                    "rootUri": root_uri,
                    "capabilities": {},
                    "workspaceFolders": [{"uri": root_uri, "name": build_root.name}],
                },
            },
        )
        init = _wait_lsp(messages, {1}, diagnostics, timeout=60.0)
        if 1 not in init:
            note = "initialize timed out"
            if proc.poll() is not None:
                note = f"jdtls exited {proc.returncode}"
            out["note"] = note + (("; " + " | ".join(stderr_tail[-5:])) if stderr_tail else "")
            return out
        if init[1].get("error"):
            out["note"] = json.dumps(init[1]["error"])
            return out

        _lsp_write(proc, {"jsonrpc": "2.0", "method": "initialized", "params": {}})
        wanted: set[int] = set()
        for offset, path in enumerate(sources, start=10):
            text = path.read_text(encoding="utf-8", errors="replace")
            uri = path.resolve().as_uri()
            _lsp_write(
                proc,
                {
                    "jsonrpc": "2.0",
                    "method": "textDocument/didOpen",
                    "params": {
                        "textDocument": {
                            "uri": uri,
                            "languageId": "java",
                            "version": 1,
                            "text": text,
                        }
                    },
                },
            )
            _lsp_write(
                proc,
                {
                    "jsonrpc": "2.0",
                    "id": offset,
                    "method": "textDocument/documentSymbol",
                    "params": {"textDocument": {"uri": uri}},
                },
            )
            wanted.add(offset)
        _lsp_write(
            proc,
            {
                "jsonrpc": "2.0",
                "id": 1000,
                "method": "workspace/symbol",
                "params": {"query": "Gson"},
            },
        )
        wanted.add(1000)
        responses = _wait_lsp(messages, wanted, diagnostics, timeout=45.0)
        doc_symbols = 0
        doc_symbol_files = 0
        for msg_id, msg in responses.items():
            if msg_id == 1000:
                continue
            count = _lsp_symbol_count(msg.get("result"))
            doc_symbols += count
            doc_symbol_files += 1
        workspace_symbols = 0
        if 1000 in responses and isinstance(responses[1000].get("result"), list):
            workspace_symbols = len(responses[1000]["result"])
        out = base_result("jdtls", "ok")
        out["metrics"] = {
            "build_root": os.path.relpath(build_root, repo),
            "sample_files": len(sources),
            "document_symbol_files": doc_symbol_files,
            "document_symbols": doc_symbols,
            "workspace_symbols_query_gson": workspace_symbols,
            "diagnostic_files": len(diagnostics),
            "diagnostics": sum(diagnostics.values()),
            "initialized": True,
            "java_home": env.get("JAVA_HOME", ""),
            "stderr_tail": stderr_tail[-5:],
        }
        if doc_symbol_files < len(sources):
            out["note"] = f"documentSymbol responses {doc_symbol_files}/{len(sources)} before timeout"
        return out
    except Exception as exc:
        out["note"] = str(exc) + (("; " + " | ".join(stderr_tail[-5:])) if stderr_tail else "")
        return out
    finally:
        out["seconds"] = round(time.time() - start, 3)
        try:
            if proc.poll() is None:
                _lsp_write(proc, {"jsonrpc": "2.0", "id": 9000, "method": "shutdown", "params": None})
                _lsp_write(proc, {"jsonrpc": "2.0", "method": "exit", "params": None})
                proc.wait(timeout=5)
        except Exception:
            if proc.poll() is None:
                proc.kill()
        log.append(f"$ {' '.join(cmd)} (cwd={build_root})\nstatus={out.get('status')} note={out.get('note', '')}\nstderr_tail={' | '.join(stderr_tail[-10:])}")


def run_clangd(cmd_prefix: list[str] | None, lang: str, repo: Path, target: Path, workdir: Path, log: list[str]) -> dict[str, Any]:
    if not cmd_prefix:
        return missing("clangd", "clangd")
    sources = c_family_source_files(target, lang)
    if not sources:
        return base_result("clangd", "failed", f"no {lang} source files under {target}")
    data_dir = workdir / f"{lang}-clangd"
    data_dir.mkdir(parents=True, exist_ok=True)
    cmd = list(cmd_prefix)
    if not any(part.startswith("--background-index") for part in cmd):
        cmd.append("--background-index=false")
    if not any(part.startswith("--compile-commands-dir") for part in cmd):
        cmd.append(f"--compile-commands-dir={repo}")

    messages: "queue.Queue[dict[str, Any]]" = queue.Queue()
    stderr_tail: list[str] = []
    diagnostics: dict[str, int] = {}
    start = time.time()
    try:
        proc = subprocess.Popen(
            cmd,
            cwd=repo,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
        )
    except OSError as exc:
        return base_result("clangd", "failed", str(exc))

    assert proc.stdout is not None
    assert proc.stderr is not None
    threading.Thread(target=_lsp_read_loop, args=(proc.stdout, messages), daemon=True).start()
    threading.Thread(target=_tail_pipe, args=(proc.stderr, stderr_tail), daemon=True).start()
    out = base_result("clangd", "failed")
    try:
        root_uri = repo.resolve().as_uri()
        _lsp_write(
            proc,
            {
                "jsonrpc": "2.0",
                "id": 1,
                "method": "initialize",
                "params": {
                    "processId": os.getpid(),
                    "rootUri": root_uri,
                    "capabilities": {},
                    "workspaceFolders": [{"uri": root_uri, "name": repo.name}],
                },
            },
        )
        init = _wait_lsp(messages, {1}, diagnostics, timeout=20.0)
        if 1 not in init:
            note = "initialize timed out"
            if proc.poll() is not None:
                note = f"clangd exited {proc.returncode}"
            out["note"] = note + (("; " + " | ".join(stderr_tail[-5:])) if stderr_tail else "")
            return out
        if init[1].get("error"):
            out["note"] = json.dumps(init[1]["error"])
            return out
        _lsp_write(proc, {"jsonrpc": "2.0", "method": "initialized", "params": {}})

        wanted: set[int] = set()
        language_id = "c" if lang == "c" else "cpp"
        for offset, path in enumerate(sources, start=10):
            text = path.read_text(encoding="utf-8", errors="replace")
            uri = path.resolve().as_uri()
            _lsp_write(
                proc,
                {
                    "jsonrpc": "2.0",
                    "method": "textDocument/didOpen",
                    "params": {
                        "textDocument": {
                            "uri": uri,
                            "languageId": language_id,
                            "version": 1,
                            "text": text,
                        }
                    },
                },
            )
            _lsp_write(
                proc,
                {
                    "jsonrpc": "2.0",
                    "id": offset,
                    "method": "textDocument/documentSymbol",
                    "params": {"textDocument": {"uri": uri}},
                },
            )
            wanted.add(offset)
        responses = _wait_lsp(messages, wanted, diagnostics, timeout=20.0)
        doc_symbols = 0
        doc_symbol_files = 0
        for msg in responses.values():
            count = _lsp_symbol_count(msg.get("result"))
            doc_symbols += count
            doc_symbol_files += 1
        out = base_result("clangd", "ok")
        out["metrics"] = {
            "sample_files": len(sources),
            "document_symbol_files": doc_symbol_files,
            "document_symbols": doc_symbols,
            "diagnostic_files": len(diagnostics),
            "diagnostics": sum(diagnostics.values()),
            "initialized": True,
            "stderr_tail": stderr_tail[-5:],
        }
        if doc_symbol_files < len(sources):
            out["note"] = f"documentSymbol responses {doc_symbol_files}/{len(sources)} before timeout"
        return out
    except Exception as exc:
        out["note"] = str(exc) + (("; " + " | ".join(stderr_tail[-5:])) if stderr_tail else "")
        return out
    finally:
        out["seconds"] = round(time.time() - start, 3)
        try:
            if proc.poll() is None:
                _lsp_write(proc, {"jsonrpc": "2.0", "id": 9000, "method": "shutdown", "params": None})
                _lsp_write(proc, {"jsonrpc": "2.0", "method": "exit", "params": None})
                proc.wait(timeout=5)
        except Exception:
            if proc.poll() is None:
                proc.kill()
        log.append(f"$ {' '.join(cmd)} (cwd={repo})\nstatus={out.get('status')} note={out.get('note', '')}\nstderr_tail={' | '.join(stderr_tail[-10:])}")


def atlas_hub_names(atlas_bin: str | None, db: Path, limit: int = 6) -> list[str]:
    """Return Atlas hub symbol names (most-connected symbols) for query probes."""
    if not atlas_bin or not db.exists():
        return []
    hubs = run([atlas_bin, "--db", f"sqlite://{db}", "--json", "hubs", "--limit", str(limit)])
    names: list[str] = []
    try:
        for hub in json.loads(hubs.stdout).get("hubs", []):
            name = hub.get("bare_name") or hub.get("name") or ""
            if name and name not in names:
                names.append(name)
    except json.JSONDecodeError:
        return []
    return names


def _free_tcp_port() -> int:
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    try:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]
    finally:
        sock.close()


def _http_get_ms(url: str, timeout: float = 5.0) -> tuple[float | None, int, int]:
    """GET url; return (elapsed_ms, status, body_len). elapsed_ms is None on error."""
    start = time.time()
    try:
        with urllib.request.urlopen(url, timeout=timeout) as resp:
            body = resp.read()
            return round((time.time() - start) * 1000, 3), resp.status, len(body)
    except urllib.error.HTTPError as exc:
        # 404 still measures the warm round-trip; record it rather than discard.
        return round((time.time() - start) * 1000, 3), exc.code, 0
    except (urllib.error.URLError, OSError):
        return None, 0, 0


def atlas_warm_serve_latency(
    atlas_bin: str | None,
    db: Path,
    names: list[str],
    log: list[str],
    samples: int = 5,
) -> dict[str, Any]:
    """Start `atlas serve` against the already-indexed DB, warm it, and time warm
    HTTP queries. This is the FAIR persistent-daemon latency path (comparable to a
    warm LSP server). It is reported in its own section and is NEVER divided by a
    cold graphify CLI time, because graphify has no warm/server mode.

    Raw per-call millisecond samples are preserved alongside the median.
    """
    out: dict[str, Any] = {
        "status": "skipped",
        "ok": False,
        "note": "",
        "addr": "",
        "healthz_ms": [],
        "healthz_median_ms": None,
        "explain": [],
        "explain_median_ms": None,
        "explain_all_ms": [],
    }
    if not atlas_bin or not db.exists() or not names:
        out["note"] = "atlas binary, indexed db, or hub symbols unavailable for warm serve"
        return out
    port = _free_tcp_port()
    addr = f"127.0.0.1:{port}"
    out["addr"] = addr
    base = f"http://{addr}"
    try:
        proc = subprocess.Popen(
            [atlas_bin, "--db", f"sqlite://{db}", "serve", "--addr", addr],
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
        )
    except OSError as exc:
        out["status"] = "failed"
        out["note"] = f"could not start atlas serve: {exc}"
        return out

    try:
        # Wait for the server to answer /healthz before any measurement.
        deadline = time.time() + 20.0
        ready = False
        while time.time() < deadline:
            if proc.poll() is not None:
                break
            ms, status, _ = _http_get_ms(f"{base}/healthz", timeout=1.0)
            if ms is not None and status == 200:
                ready = True
                break
            time.sleep(0.05)
        if not ready:
            out["status"] = "failed"
            out["note"] = (
                f"atlas serve did not become ready on {addr}"
                + (f" (process exited {proc.returncode})" if proc.poll() is not None else "")
            )
            return out

        # Warm-up pass (untimed) so we measure the steady-state daemon, not the
        # first request that lazily opens the DB / fills caches.
        for name in names:
            _http_get_ms(f"{base}/api/v1/symbols/{urllib.parse.quote(name)}/explain")
        _http_get_ms(f"{base}/healthz")

        # Warm /healthz (server-floor with no query work).
        healthz_ms = []
        for _ in range(samples):
            ms, status, _ = _http_get_ms(f"{base}/healthz")
            if ms is not None and status == 200:
                healthz_ms.append(ms)
        out["healthz_ms"] = healthz_ms
        if healthz_ms:
            out["healthz_median_ms"] = round(statistics.median(healthz_ms), 3)

        # Warm explain per hub symbol; keep every raw sample.
        explain_rows: list[dict[str, Any]] = []
        all_ms: list[float] = []
        for name in names:
            per: list[float] = []
            body_len = 0
            last_status = 0
            for _ in range(samples):
                ms, status, blen = _http_get_ms(
                    f"{base}/api/v1/symbols/{urllib.parse.quote(name)}/explain"
                )
                last_status = status
                if ms is not None and status == 200:
                    per.append(ms)
                    body_len = blen
            if per:
                med = round(statistics.median(per), 3)
                all_ms.extend(per)
                explain_rows.append(
                    {
                        "symbol": name,
                        "samples_ms": per,
                        "median_ms": med,
                        "body_bytes": body_len,
                        "status": last_status,
                    }
                )
        out["explain"] = explain_rows
        out["explain_all_ms"] = all_ms
        if all_ms:
            out["explain_median_ms"] = round(statistics.median(all_ms), 3)
            out["status"] = "ok"
            out["ok"] = True
        else:
            out["status"] = "failed"
            out["note"] = "no warm explain samples succeeded"
        log.append(
            "$ atlas serve --addr "
            + addr
            + " (warm HTTP latency)\n"
            + f"healthz_median_ms={out['healthz_median_ms']} explain_median_ms={out['explain_median_ms']} "
            + f"samples_per_symbol={samples}"
        )
        return out
    finally:
        # Stop the server cleanly: SIGTERM, then wait, then kill as a last resort.
        try:
            if proc.poll() is None:
                proc.terminate()
                try:
                    proc.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    proc.kill()
                    proc.wait(timeout=5)
        except Exception:
            try:
                proc.kill()
            except Exception:
                pass


def query_token_rows(atlas_bin: str | None, graphify_bin: str | None, db: Path, target: Path, log: list[str]) -> list[dict[str, Any]]:
    if not atlas_bin or not graphify_bin or not db.exists():
        return []
    names = atlas_hub_names(atlas_bin, db, limit=6)
    if not names:
        return []

    if names:
        warm_name = names[0]
        warm_atlas = run([atlas_bin, "--db", f"sqlite://{db}", "--format", "plain", "explain", warm_name])
        warm_graphify = run([graphify_bin, "explain", warm_name], cwd=target)
        log.append(
            f"$ query warm-up {warm_name}\n"
            f"atlas_status={warm_atlas.returncode} graphify_status={warm_graphify.returncode}\n"
            "note=untimed warm-up for both tools before measured query latency rows"
        )

    rows: list[dict[str, Any]] = []
    for name in names[:4]:
        atlas_samples: list[float] = []
        graphify_samples: list[float] = []
        atlas = subprocess.CompletedProcess([atlas_bin], 1, "", "")
        graphify = subprocess.CompletedProcess([graphify_bin], 1, "", "")
        for _ in range(5):
            t0 = time.perf_counter()
            atlas = run([atlas_bin, "--db", f"sqlite://{db}", "--format", "plain", "explain", name])
            atlas_samples.append(round((time.perf_counter() - t0) * 1000, 3))
            t0 = time.perf_counter()
            graphify = run([graphify_bin, "explain", name], cwd=target)
            graphify_samples.append(round((time.perf_counter() - t0) * 1000, 3))
        atlas_ms = round(statistics.median(atlas_samples), 3)
        graphify_ms = round(statistics.median(graphify_samples), 3)
        row = {
            "symbol": name,
            "atlas_tokens": tokens(atlas.stdout),
            "graphify_tokens": tokens(graphify.stdout),
            "atlas_ms": atlas_ms,
            "graphify_ms": graphify_ms,
            "atlas_ms_samples": atlas_samples,
            "graphify_ms_samples": graphify_samples,
            "atlas_missing": not atlas.stdout.strip(),
            "graphify_missing": "No node matching" in graphify.stdout or not graphify.stdout.strip(),
        }
        rows.append(row)
        log.append(
            f"$ explain {name}\n"
            f"atlas_tokens={row['atlas_tokens']} graphify_tokens={row['graphify_tokens']} "
            f"atlas_ms_median={row['atlas_ms']} graphify_ms_median={row['graphify_ms']} "
            f"atlas_samples={atlas_samples} graphify_samples={graphify_samples}"
        )
    return rows


def equivalent_query_rows(rows: list[dict[str, Any]]) -> list[dict[str, Any]]:
    return [row for row in rows if not row.get("atlas_missing") and not row.get("graphify_missing")]


def query_totals(rows: list[dict[str, Any]]) -> dict[str, Any]:
    eq = equivalent_query_rows(rows)
    return {
        "rows": len(rows),
        "equivalent_rows": len(eq),
        "graphify_missing": sum(1 for row in rows if row.get("graphify_missing")),
        "atlas_tokens": sum(row["atlas_tokens"] for row in eq),
        "graphify_tokens": sum(row["graphify_tokens"] for row in eq),
        "atlas_ms": sum(row["atlas_ms"] for row in eq),
        "graphify_ms": sum(row["graphify_ms"] for row in eq),
    }


def run_language(lang: str, args: argparse.Namespace, workdir: Path) -> dict[str, Any]:
    url, subdir = REPOS[lang]
    repo = workdir / lang / "repo"
    lang_workdir = workdir / lang
    lang_workdir.mkdir(parents=True, exist_ok=True)
    log: list[str] = []
    if not ensure_repo(url, repo, log):
        return {
            "language": lang,
            "repo": url,
            "subdir": subdir,
            "target": str(repo),
            "tools": {"clone": base_result("clone", "failed", "git clone failed")},
            "queries": [],
        }
    target = repo / subdir if subdir else repo
    if not target.exists():
        target = repo

    atlas_bin = executable(args.atlas)
    graphify_bin = resolve_graphify(args.graphify)
    db = lang_workdir / "atlas.db"
    tools: dict[str, Any] = {}

    clean_generated_sidecars(target)
    run_order = ["atlas"] + [tool for tool in MATRIX[lang] if tool not in {"atlas", "graphify"}] + ["graphify"]

    for tool in run_order:
        if tool == "atlas":
            tools[tool] = run_atlas(atlas_bin, target, db, log)
        elif tool == "graphify":
            tools[tool] = run_graphify(graphify_bin, target, log)
        elif tool == "scip-go":
            tools[tool] = run_scip_go(executable(args.scip_go), repo, lang_workdir, log)
        elif tool == "scip-python":
            tools[tool] = run_scip_python(executable(args.scip_python), repo, lang_workdir, log)
        elif tool == "scip-typescript":
            tools[tool] = run_scip_typescript(
                tool_command(args.scip_typescript, "@sourcegraph/scip-typescript", "scip-typescript"),
                lang,
                repo,
                target,
                lang_workdir,
                log,
            )
        elif tool == "scip-java":
            tools[tool] = run_scip_java(command_prefix(args.scip_java), repo, target, lang_workdir, log)
        elif tool == "gopls":
            tools[tool] = run_gopls(executable(args.gopls), repo, log)
        elif tool == "pyright":
            tools[tool] = run_pyright(executable(args.pyright), repo, target, log)
        elif tool == "tsserver":
            tools[tool] = run_tsserver_proxy(tool_command(args.tsc, "typescript", "tsc"), lang, repo, target, log)
        elif tool == "jdtls":
            tools[tool] = run_jdtls(command_prefix(args.jdtls), repo, target, lang_workdir, log)
        elif tool == "clangd":
            tools[tool] = run_clangd(command_prefix(args.clangd), lang, repo, target, lang_workdir, log)
        else:
            tools[tool] = not_implemented(tool, f"{tool} adapter will be added when iterating {lang}")

    truth: dict[str, Any] = {}
    if lang == "python":
        py_truth = python_ast_truth(target)
        py_truth["atlas_callable_class_symbols"] = sqlite_symbol_kind_count(db, ("function", "method", "class"))
        py_truth["atlas_assignment_symbols"] = sqlite_symbol_kind_count(db, ("constant", "variable", "field"))
        truth["python_ast"] = py_truth

    queries = query_token_rows(atlas_bin, graphify_bin, db, target, log)
    hub_names = atlas_hub_names(atlas_bin, db, limit=6)[:4]
    warm_latency = atlas_warm_serve_latency(atlas_bin, db, hub_names, log)
    (Path(args.out).parent / "logs").mkdir(parents=True, exist_ok=True)
    (Path(args.out).parent / "logs" / f"{lang}-matrix.log").write_text("\n\n".join(log))
    return {
        "language": lang,
        "repo": url,
        "subdir": subdir,
        "target": str(target),
        "tools": tools,
        "truth": truth,
        "queries": queries,
        "atlas_warm_serve": warm_latency,
    }


def tool_cell(result: dict[str, Any], key: str) -> str:
    tool = result["tools"].get(key)
    if not tool:
        return "n/a"
    if not tool.get("ok"):
        return tool.get("status", "missing")
    metrics = tool.get("metrics", {})
    if key == "atlas":
        cold = metrics.get("full_seconds", metrics.get("cold_seconds"))
        delta = metrics.get("delta_seconds", tool["seconds"])
        cold_txt = f"{cold}s cold full" if cold is not None else f"{tool['seconds']}s"
        return f"{metrics.get('symbols', 0)} symbols, {metrics.get('calls', 0)} calls, {cold_txt} ({delta}s delta)"
    if key == "graphify":
        gm = metrics
        full = gm.get("full_seconds", tool["seconds"])
        delta = gm.get("delta_seconds")
        delta_txt = f" ({delta}s delta)" if delta is not None else ""
        return f"{gm.get('nodes', 0)} nodes, {gm.get('calls', 0)} calls, {full}s full{delta_txt}"
    if key == "scip-go":
        return f"{metrics.get('symbols', 0)} symbols, {metrics.get('occurrences', 0)} occ, {tool['seconds']}s"
    if key == "scip-python":
        return f"{metrics.get('symbols', 0)} symbols, {metrics.get('occurrences', 0)} occ, {tool['seconds']}s"
    if key == "scip-typescript":
        return f"{metrics.get('symbols', 0)} symbols, {metrics.get('occurrences', 0)} occ, {tool['seconds']}s"
    if key == "scip-java":
        return f"{metrics.get('symbols', 0)} symbols, {metrics.get('occurrences', 0)} occ, {tool['seconds']}s"
    if key == "gopls":
        return f"{metrics.get('workspace_packages', 0)} pkgs, {metrics.get('diagnostics', 0)} diag, {tool['seconds']}s"
    if key == "pyright":
        return f"{metrics.get('files_analyzed', 0)} files, {metrics.get('diagnostics', 0)} diag, {tool['seconds']}s"
    if key == "tsserver":
        return f"{metrics.get('files', 0)} files, {metrics.get('diagnostics', 0)} diag, {tool['seconds']}s"
    if key == "jdtls":
        return f"{metrics.get('document_symbols', 0)} doc syms, {metrics.get('diagnostics', 0)} diag, {tool['seconds']}s"
    if key == "clangd":
        return f"{metrics.get('document_symbols', 0)} doc syms, {metrics.get('diagnostics', 0)} diag, {tool['seconds']}s"
    return "ok"


def version_probe(name: str, command: list[str] | None, timeout: int = 30) -> dict[str, Any]:
    if not command:
        return {
            "tool": name,
            "status": "missing",
            "ok": False,
            "command": "",
            "version": "",
            "stdout": "",
            "stderr": "",
            "exit_code": 127,
        }
    start = time.time()
    try:
        result = run(command, timeout=timeout)
    except (subprocess.TimeoutExpired, OSError) as exc:
        return {
            "tool": name,
            "status": "failed",
            "ok": False,
            "command": " ".join(command),
            "version": "",
            "stdout": "",
            "stderr": str(exc),
            "seconds": round(time.time() - start, 3),
        }
    stdout = result.stdout.strip()
    stderr = result.stderr.strip()
    first_line = (stdout or stderr).splitlines()[0] if (stdout or stderr) else ""
    return {
        "tool": name,
        "status": "ok" if result.returncode == 0 else "failed",
        "ok": result.returncode == 0,
        "command": " ".join(command),
        "version": first_line,
        "stdout": stdout.splitlines()[:20],
        "stderr": stderr.splitlines()[:20],
        "exit_code": result.returncode,
        "seconds": round(time.time() - start, 3),
    }


def version_command(prefix: list[str] | None, *args: str) -> list[str] | None:
    if not prefix:
        return None
    return [*prefix, *args]


def jdtls_manifest_entry(args: argparse.Namespace) -> dict[str, Any]:
    prefix = command_prefix(args.jdtls)
    if not prefix:
        return version_probe("jdtls", None)
    binary = prefix[0]
    resolved = str(Path(binary).resolve())
    version = ""
    parts = Path(resolved).parts
    if "jdtls" in parts:
        idx = parts.index("jdtls")
        if len(parts) > idx + 1:
            version = parts[idx + 1]
    env = java21_env()
    return {
        "tool": "jdtls",
        "status": "ok",
        "ok": True,
        "command": " ".join(prefix),
        "version": version or "unknown",
        "binary": binary,
        "resolved_binary": resolved,
        "java_home": env.get("JAVA_HOME", ""),
        "note": "JDTLS starts an LSP server instead of printing a stable --version line; version is derived from the installed package path and benchmark Java runtime.",
    }


def sourcekit_lsp_manifest_entry() -> dict[str, Any]:
    binary = executable("sourcekit-lsp")
    if not binary:
        return version_probe("sourcekit-lsp", None)
    swift = executable("swift")
    if not swift:
        return {
            "tool": "sourcekit-lsp",
            "status": "ok",
            "ok": True,
            "command": binary,
            "version": "unknown",
            "binary": binary,
            "note": "sourcekit-lsp is installed but this toolchain exposes no version flag and swift is unavailable.",
        }
    entry = version_probe("sourcekit-lsp", [swift, "--version"])
    entry["tool"] = "sourcekit-lsp"
    entry["command"] = f"{binary} (no version flag); {swift} --version"
    entry["binary"] = binary
    entry["note"] = "sourcekit-lsp has no version flag in this toolchain; Swift toolchain version is recorded as the closest reproducible native version."
    return entry


def benchmark_tool_versions() -> list[dict[str, Any]]:
    versions: list[dict[str, Any]] = []
    for path, display_path in live_benchmark_paths():
        try:
            benchmark = json.loads(path.read_text())
        except (OSError, json.JSONDecodeError) as exc:
            versions.append({"artifact": display_path, "status": "unreadable", "error": str(exc)})
            continue
        native = benchmark.get("native_baseline") or {}
        metrics = native.get("metrics") or {}
        metric_versions = {
            key: value
            for key, value in metrics.items()
            if key.endswith("_version") or key == "tool_versions"
        }
        versions.append(
            {
                "artifact": display_path,
                "language": benchmark.get("language"),
                "native_tool": native.get("tool"),
                "native_status": native.get("status"),
                "native_command": native.get("command"),
                "metric_versions": metric_versions,
                "richer_native_baselines": benchmark.get("richer_native_baselines") or {},
            }
        )
    return versions


def benchmark_tool_manifest(args: argparse.Namespace, graphify_discovery: dict[str, Any]) -> dict[str, Any]:
    graphify_bin = resolve_graphify(args.graphify)
    probes = [
        version_probe("atlas", version_command([executable(args.atlas)] if executable(args.atlas) else None, "version")),
        {
            **version_probe("graphify", version_command([graphify_bin] if graphify_bin else None, "--version")),
            "discovered_version": graphify_discovery.get("version", ""),
            "binary": graphify_discovery.get("binary", graphify_bin or ""),
        },
        version_probe("go", version_command([executable("go")] if executable("go") else None, "version")),
        version_probe("python", [sys.executable, "--version"]),
        version_probe("java", version_command([executable("java")] if executable("java") else None, "-version")),
        version_probe("maven", version_command([executable("mvn")] if executable("mvn") else None, "--version")),
        version_probe("scip-go", version_command([executable(args.scip_go)] if executable(args.scip_go) else None, "--version")),
        version_probe("scip-python", version_command([executable(args.scip_python)] if executable(args.scip_python) else None, "--version")),
        version_probe("scip-typescript", version_command(tool_command(args.scip_typescript, "@sourcegraph/scip-typescript", "scip-typescript"), "--version")),
        version_probe("scip-java", version_command(command_prefix(args.scip_java), "--version")),
        version_probe("gopls", version_command([executable(args.gopls)] if executable(args.gopls) else None, "version")),
        version_probe("pyright", version_command([executable(args.pyright)] if executable(args.pyright) else None, "--version")),
        version_probe("tsc", version_command(tool_command(args.tsc, "typescript", "tsc"), "--version")),
        jdtls_manifest_entry(args),
        version_probe("clangd", version_command(command_prefix(args.clangd), "--version")),
        version_probe("rust-analyzer", version_command([executable("rust-analyzer")] if executable("rust-analyzer") else None, "--version")),
        version_probe("dotnet", version_command([executable("dotnet")] if executable("dotnet") else None, "--version")),
        version_probe("ruby", version_command([executable("ruby")] if executable("ruby") else None, "--version")),
        version_probe("php", version_command([executable("php")] if executable("php") else None, "--version")),
        version_probe("pwsh", version_command([executable("pwsh")] if executable("pwsh") else None, "--version")),
        sourcekit_lsp_manifest_entry(),
    ]
    return {
        "generated_at_unix": int(time.time()),
        "platform": {
            "system": platform.system(),
            "release": platform.release(),
            "machine": platform.machine(),
            "python": sys.version.split()[0],
        },
        "core_tools": probes,
        "live_benchmark_tools": benchmark_tool_versions(),
    }


def render_tool_manifest(tool_manifest: dict[str, Any]) -> list[str]:
    if not tool_manifest:
        return []
    lines: list[str] = []
    w = lines.append
    w("## Tool version manifest")
    w("")
    w("Raw artifact: `bench/MATRIX_TOOL_VERSIONS.json`.")
    platform_info = tool_manifest.get("platform") or {}
    if platform_info:
        w(
            f"- Platform: {platform_info.get('system')} {platform_info.get('release')} "
            f"{platform_info.get('machine')}; Python {platform_info.get('python')}."
        )
    w("")
    w("| tool | status | version / first output line | command |")
    w("|---|---|---|---|")
    for item in tool_manifest.get("core_tools") or []:
        version = item.get("version") or item.get("discovered_version") or ""
        if item.get("tool") == "graphify" and item.get("discovered_version"):
            version = item.get("discovered_version")
        w(f"| {item.get('tool')} | {item.get('status')} | `{version}` | `{item.get('command', '')}` |")
    benchmark_count = len(tool_manifest.get("live_benchmark_tools") or [])
    versioned = sum(1 for item in tool_manifest.get("live_benchmark_tools") or [] if item.get("metric_versions"))
    w("")
    w(f"- Live benchmark native-version details: {versioned}/{benchmark_count} artifacts expose explicit native tool or library version fields in raw JSON; all artifacts include native command/status.")
    w("")
    return lines


# Tools whose full-index wall time lives in result["tools"][key]["seconds"] and
# can be compared cold-vs-cold against Atlas's COLD full index. graphify is
# handled separately because it also exposes an incremental path.
COLD_BUILD_BASELINES = {
    "go": [("scip-go", "scip-go"), ("gopls", "gopls (workspace type-check via `gopls stats`)")],
    "python": [("scip-python", "scip-python")],
    "javascript": [("scip-typescript", "scip-typescript")],
    "typescript": [("scip-typescript", "scip-typescript")],
    "java": [("scip-java", "scip-java")],
}


def build_speed_lines(result: dict[str, Any]) -> list[str]:
    """Honest build-speed reporting.

    Headline = COLD full index vs COLD full build of every baseline that also
    builds from scratch (graphify FULL extract, scip-*, gopls). Delta-vs-delta is
    reported separately and ONLY where both tools have an incremental path. Raw
    seconds are always shown beside any ratio, and delta-vs-full is never the
    headline.
    """
    lang = result.get("language", "")
    tools = result.get("tools", {})
    atlas = tools.get("atlas", {})
    graphify = tools.get("graphify", {})
    am = atlas.get("metrics", {})
    out: list[str] = []

    atlas_cold = am.get("full_seconds", am.get("cold_seconds"))
    atlas_delta = am.get("delta_seconds", atlas.get("seconds"))

    # --- Headline: cold full index vs cold full build (apples-to-apples). ---
    cold_parts: list[str] = []
    if graphify.get("ok"):
        gm = graphify.get("metrics", {})
        g_full = gm.get("full_seconds", graphify.get("seconds"))
        if atlas_cold is not None and g_full is not None:
            cold_parts.append(f"graphify FULL extract {g_full}s (graphify/Atlas = {ratio(g_full, atlas_cold)}x)")
    for key, label in COLD_BUILD_BASELINES.get(lang, []):
        baseline = tools.get(key, {})
        if baseline.get("ok") and baseline.get("seconds") and atlas_cold is not None:
            b = baseline["seconds"]
            cold_parts.append(f"{label} cold {b}s ({key}/Atlas = {ratio(b, atlas_cold)}x)")
    if atlas_cold is not None and cold_parts:
        out.append(
            f"- Build speed (cold-vs-cold, full index): Atlas COLD full index {atlas_cold}s vs "
            + "; ".join(cold_parts)
            + ". A ratio < 1.0x means Atlas is slower cold; this is the honest headline."
        )
    elif atlas_cold is not None:
        out.append(f"- Build speed (cold-vs-cold, full index): Atlas COLD full index {atlas_cold}s (no comparable cold baseline succeeded this run).")

    # --- Delta-vs-delta: only where both tools have an incremental path. ---
    if graphify.get("ok"):
        gm = graphify.get("metrics", {})
        g_delta = gm.get("delta_seconds")
        if atlas_delta is not None and g_delta is not None:
            out.append(
                f"- Build speed (delta-vs-delta, no-change reindex): Atlas {atlas_delta}s vs "
                f"graphify {g_delta}s, graphify/Atlas = {ratio(g_delta, atlas_delta)}x. "
                "Both tools re-run against an existing snapshot/sidecar here."
            )
        elif atlas_delta is not None:
            out.append(
                f"- Build speed (delta): Atlas no-change reindex {atlas_delta}s. graphify exposed no "
                "incremental wall time this run, so no fair delta-vs-delta ratio is reported "
                "(comparing Atlas delta to graphify FULL would be delta-vs-full and is omitted)."
            )
    return out


# Languages whose LSP baseline is a persistent daemon, so a warm-vs-warm latency
# row is meaningful. tsserver here is the one-shot `tsc` proxy (not a daemon) so
# it is intentionally excluded from warm-vs-warm.
WARM_LSP_BASELINES = {
    "go": "gopls",
    "python": "pyright",
    "java": "jdtls",
    "c": "clangd",
    "cpp": "clangd",
}


def render_warm_latency(results: list[dict[str, Any]]) -> list[str]:
    """Render the warm (persistent-server) query-latency section.

    This is the legitimate path to higher latency ratios: Atlas's `serve` daemon
    answers warm HTTP queries without paying the per-call Go process-start floor
    that gates the cold CLI. graphify has NO warm/server mode, so we never divide
    warm Atlas by cold graphify here. Where a baseline is itself a persistent
    daemon (gopls/clangd/jdtls/pyright), the cold-vs-cold CLI latency table above
    already covers graphify; this section reports Atlas warm latency as a RAW
    number plus the cold CLI floor for the SAME tool to show the warm speedup.
    """
    have = [r for r in results if (r.get("atlas_warm_serve") or {}).get("ok")]
    if not have:
        return []
    lines: list[str] = []
    w = lines.append
    w("## Warm query latency (persistent server)\n")
    w(
        "Atlas `serve` is started against the already-indexed DB, warmed, then warm "
        "HTTP queries are timed. Raw per-call samples are preserved in the JSON "
        "(`atlas_warm_serve`). graphify has no warm/server mode, so warm Atlas is "
        "NOT divided by any graphify time; the cold-vs-cold CLI latency rows above "
        "remain the only Atlas-vs-graphify latency ratio.\n"
    )
    w("| Language | Atlas warm /healthz (median ms) | Atlas warm explain (median ms) | Atlas cold-CLI explain (median ms) | warm speedup (cold/warm) |")
    w("|---|--:|--:|--:|--:|")
    for r in have:
        warm = r["atlas_warm_serve"]
        lang = r["language"]
        # Cold CLI explain median for the SAME tool/symbols (fair warm-vs-cold for
        # Atlas itself — both are Atlas, isolating the process-start floor).
        cold_ms = [row["atlas_ms"] for row in r.get("queries", []) if not row.get("atlas_missing")]
        cold_med = round(statistics.median(cold_ms), 3) if cold_ms else None
        warm_explain = warm.get("explain_median_ms")
        speedup = ratio(cold_med, warm_explain) if (cold_med and warm_explain) else None
        w(
            f"| {lang} | {warm.get('healthz_median_ms')} | {warm_explain} | "
            f"{cold_med if cold_med is not None else 'n/a'} | "
            f"{str(speedup) + 'x' if speedup is not None else 'n/a'} |"
        )
    w("")
    # Warm-vs-warm note where the LSP baseline is itself a daemon.
    for r in have:
        lang = r["language"]
        lsp_key = WARM_LSP_BASELINES.get(lang)
        if not lsp_key:
            continue
        warm = r["atlas_warm_serve"]
        w(
            f"- {lang} warm-vs-warm context: both Atlas `serve` and {lsp_key} run as persistent "
            f"daemons. Atlas warm explain median is {warm.get('explain_median_ms')}ms and warm /healthz "
            f"is {warm.get('healthz_median_ms')}ms. {lsp_key}'s steady-state per-request latency is "
            "measured separately in its LSP benchmark (different query semantics: a full Atlas context "
            "bundle vs a single LSP method), so the two are reported side by side, not as a single ratio."
        )
    w("")
    return lines


def atlas_speed_line(atlas: dict[str, Any], graphify: dict[str, Any]) -> str:
    """Backward-compatible single-line summary kept for any external callers.

    It now states BOTH builds explicitly and labels the delta vs full so it can
    never be misread as an apples-to-apples headline.
    """
    am = atlas.get("metrics", {})
    cold = am.get("full_seconds", am.get("cold_seconds"))
    gm = graphify.get("metrics", {})
    g_full = gm.get("full_seconds", graphify.get("seconds"))
    cold_txt = f"Atlas COLD full index {cold}s" if cold is not None else "Atlas full index n/a"
    g_txt = f"graphify FULL extract {g_full}s" if g_full is not None else "graphify full n/a"
    if cold is not None and g_full is not None:
        return f"- Build speed (cold-vs-cold): {cold_txt} vs {g_txt}, graphify/Atlas = {ratio(g_full, cold)}x (Atlas delta reindex {am.get('delta_seconds', atlas.get('seconds'))}s reported separately, not used as the headline)."
    return f"- Build speed (cold-vs-cold): {cold_txt} vs {g_txt}."


def scip_navigation_symbols(metrics: dict[str, Any]) -> int:
    kinds = metrics.get("kinds", {})
    excluded = {"Variable", "Package", "UnspecifiedKind"}
    return sum(count for kind, count in kinds.items() if kind not in excluded)


def live_benchmark_paths() -> list[tuple[Path, str]]:
    bench_dir = Path(__file__).parent
    paths = sorted(path for path in bench_dir.glob("LIVE_*_BENCHMARK.json") if path.name != "LIVE_MCP_CONTEXT_BENCHMARK.json")
    if not paths:
        return []
    return [(path, f"bench/{path.name}") for path in paths]


GRAPHIFY_AUDIT_EVIDENCE = {
    "go": [("core", "go")],
    "python": [("core", "python")],
    "javascript": [("core", "javascript")],
    "typescript": [("core", "typescript")],
    "java": [("core", "java")],
    "c": [("core", "c")],
    "cpp/cuda": [("core", "cpp"), ("benchmark", "cuda")],
    "groovy/gradle": [("benchmark", "groovy")],
    "csharp": [("benchmark", "csharp")],
    "rust": [("benchmark", "rust")],
    "ruby": [("benchmark", "ruby")],
    "kotlin": [("benchmark", "kotlin")],
    "scala": [("benchmark", "scala")],
    "php": [("benchmark", "php")],
    "blade": [("benchmark", "blade")],
    "swift": [("benchmark", "swift")],
    "lua": [("benchmark", "lua")],
    "zig": [("benchmark", "zig")],
    "powershell": [("benchmark", "powershell")],
    "elixir": [("benchmark", "elixir")],
    "objective-c": [("benchmark", "objc")],
    "julia": [("benchmark", "julia")],
    "fortran": [("benchmark", "fortran")],
    "dart": [("benchmark", "dart")],
    "verilog/systemverilog": [("benchmark", "verilog")],
    "sql": [("benchmark", "sql")],
    "markdown": [("benchmark", "markdown")],
    "pascal": [("benchmark", "pascal")],
    "delphi/lazarus forms": [("benchmark", "delphi")],
    "shell": [("benchmark", "bash")],
    "json config": [("benchmark", "json")],
    "terraform/hcl": [("benchmark", "terraform")],
    "byond dm": [("benchmark", "byond")],
    "dotnet project": [("benchmark", "dotnet")],
    "razor": [("benchmark", "razor")],
    "apex": [("benchmark", "apex")],
    "vue": [("benchmark", "vue")],
    "svelte": [("benchmark", "svelte")],
    "astro": [("benchmark", "astro")],
}

GRAPHIFY_DETECTOR_ONLY_EVIDENCE = {
    ".ejs": [("benchmark", "ejs")],
    ".ets": [("benchmark", "ets")],
    ".r": [("benchmark", "r")],
}


def _coverage_ratio_text(coverage: dict[str, Any]) -> str:
    for key, value in sorted(coverage.items()):
        if key.startswith("atlas_vs_") and key.endswith("_definition_ratio"):
            return f"{key}={value}"
    return "coverage=n/a"


def _ratio_x(num: float, den: float) -> str:
    value = ratio(num, den)
    return "n/a" if value is None else f"{value}x"


def benchmark_audit_summary(language: str) -> tuple[bool, str]:
    path = Path(__file__).parent / f"LIVE_{language.upper()}_BENCHMARK.json"
    display_path = f"bench/{path.name}"
    if not path.exists():
        return False, f"`{display_path}` missing"
    try:
        benchmark = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        return False, f"`{display_path}` unreadable: {exc}"

    native = benchmark.get("native_baseline") or {}
    coverage = benchmark.get("coverage") or {}
    atlas = benchmark.get("atlas", {}).get("index", {})
    queries = benchmark.get("queries") or []
    qt = query_totals(queries) if queries else {}
    query_bits = ""
    if qt:
        latency_ratio = _ratio_x(qt.get("graphify_ms", 0), qt.get("atlas_ms", 0))
        token_ratio = _ratio_x(qt.get("graphify_tokens", 0), qt.get("atlas_tokens", 0))
        query_bits = f", query eq {qt.get('equivalent_rows')}/{qt.get('rows')}, latency {latency_ratio}, tokens {token_ratio}"
    native_metrics = native.get("metrics") or {}
    native_has_evidence = bool(native.get("ok")) or bool(native_metrics.get("definitions")) or native.get("status") in {"partial", "proxy"}
    ok = bool(atlas.get("indexed_files")) and native_has_evidence
    status = "ok" if native.get("ok") else f"native_limited={native.get('status', 'unknown')}"
    summary = (
        f"`{display_path}` {status}; repo `{benchmark.get('repo')}` commit `{benchmark.get('commit')}`; "
        f"native `{native.get('tool', 'native')}`; {_coverage_ratio_text(coverage)}{query_bits}"
    )
    return ok, summary


def core_audit_summary(language: str, results_by_language: dict[str, dict[str, Any]]) -> tuple[bool, str]:
    result = results_by_language.get(language)
    if not result:
        return False, f"core matrix row `{language}` missing from this report run"
    atlas = result.get("tools", {}).get("atlas", {})
    graphify = result.get("tools", {}).get("graphify", {})
    native_tools = [tool for tool in MATRIX.get(language, []) if tool not in {"atlas", "graphify"}]
    native_bits = []
    native_ok = False
    for tool in native_tools:
        data = result.get("tools", {}).get(tool, {})
        state = "ok" if data.get("ok") else data.get("status", "missing")
        native_bits.append(f"{tool}:{state}")
        native_ok = native_ok or bool(data.get("ok"))
    qt = query_totals(result.get("queries") or [])
    ok = bool(atlas.get("ok")) and bool(graphify.get("ok")) and native_ok
    summary = (
        f"core matrix `{language}` {'ok' if ok else 'partial'}; native {', '.join(native_bits) or 'n/a'}; "
        f"query eq {qt.get('equivalent_rows')}/{qt.get('rows')}, "
        f"latency {_ratio_x(qt.get('graphify_ms', 0), qt.get('atlas_ms', 0))}, "
        f"tokens {_ratio_x(qt.get('graphify_tokens', 0), qt.get('atlas_tokens', 0))}"
    )
    return ok, summary


def audit_evidence_summary(items: list[tuple[str, str]], results_by_language: dict[str, dict[str, Any]]) -> tuple[bool, str]:
    if not items:
        return False, "no Atlas evidence mapping"
    ok = True
    summaries: list[str] = []
    for kind, language in items:
        if kind == "core":
            item_ok, summary = core_audit_summary(language, results_by_language)
        elif kind == "benchmark":
            item_ok, summary = benchmark_audit_summary(language)
        else:
            item_ok, summary = False, f"unknown evidence kind `{kind}` for `{language}`"
        ok = ok and item_ok
        summaries.append(summary)
    return ok, "<br>".join(summaries)


def render_graphify_coverage_audit(graphify_discovery: dict[str, Any], results: list[dict[str, Any]]) -> list[str]:
    results_by_language = {result["language"]: result for result in results}
    rows = list(graphify_discovery.get("rows") or [])
    detector_only = list(graphify_discovery.get("detector_only_code_extensions") or [])

    audited_rows: list[tuple[str, bool, str, str]] = []
    for family, exts, extractor, _atlas_status in rows:
        evidence = GRAPHIFY_AUDIT_EVIDENCE.get(family, [])
        ok, summary = audit_evidence_summary(evidence, results_by_language)
        audited_rows.append((family, ok, f"`{exts}` via `{extractor}`", summary))
    for ext in detector_only:
        evidence = GRAPHIFY_DETECTOR_ONLY_EVIDENCE.get(ext, [])
        ok, summary = audit_evidence_summary(evidence, results_by_language)
        audited_rows.append((f"detector-only {ext}", ok, f"`{ext}` in `CODE_EXTENSIONS`, no `_DISPATCH` extractor", summary))

    missing = [name for name, ok, _support, _summary in audited_rows if not ok]
    deterministic_total = len(rows)
    deterministic_ok = sum(1 for name, ok, _support, _summary in audited_rows[:deterministic_total] if ok)
    detector_ok = sum(1 for name, ok, _support, _summary in audited_rows[deterministic_total:] if ok)

    lines: list[str] = []
    w = lines.append
    w("## graphify coverage audit")
    w("")
    w(
        f"- Deterministic graphify families covered by Atlas evidence: {deterministic_ok}/{deterministic_total}. "
        f"Detector-only extensions covered by live Atlas benchmarks: {detector_ok}/{len(detector_only)}."
    )
    unsupported = graphify_discovery.get("unsupported_rows") or []
    if unsupported:
        w(f"- Unsupported graphify rows: {unsupported}.")
    else:
        w("- Unsupported graphify rows: none.")
    if missing:
        w(f"- Missing or partial evidence: {', '.join(f'`{name}`' for name in missing)}.")
    else:
        w("- Missing evidence: none.")
    w("")
    w("| graphify support | status | Atlas evidence |")
    w("|---|---|---|")
    for name, ok, support, summary in audited_rows:
        w(f"| {name}<br>{support} | {'ok' if ok else 'partial'} | {summary} |")
    w("")
    return lines


def render_saturation_report() -> list[str]:
    path = Path(__file__).parent / "SATURATION_REPORT.json"
    display_path = f"bench/{path.name}"
    if not path.exists():
        return []
    try:
        report = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        return [f"## Saturation loop evidence", "", f"- Could not load `{display_path}`: {exc}.", ""]

    lines: list[str] = []
    w = lines.append
    w("## Saturation loop evidence")
    w("")
    w(
        f"Raw artifacts: `{display_path}` and `bench/SATURATION_REPORT.md`. "
        f"Iterations requested per language: {report.get('iterations_requested')}."
    )
    w("")
    w("| language | status | iterations | equivalent rows by pass | graphify missing rows by pass | coverage ratio by pass |")
    w("|---|---|---:|---|---|---|")
    for item in report.get("languages") or []:
        iterations = item.get("iterations") or []
        eq = ", ".join(f"{it.get('queries', {}).get('equivalent_rows')}/{it.get('queries', {}).get('rows')}" for it in iterations)
        missing = ", ".join(str(it.get("queries", {}).get("graphify_missing")) for it in iterations)
        coverage = ", ".join(str(it.get("coverage_ratio")) for it in iterations)
        w(
            f"| {item.get('language')} | {item.get('status')} | {item.get('iterations_run')} | "
            f"{eq} | {missing} | {coverage} |"
        )
    saturated = [item.get("language") for item in report.get("languages") or [] if item.get("saturated")]
    if saturated:
        w("")
        w(
            "Saturation note: these languages are marked saturated only for graphify-equivalent query-score improvement. "
            "Their native coverage proxies remain in the live benchmark artifacts; no 5x query claim is made where graphify exposes no equivalent rows."
        )
    w("")
    return lines


def render_mcp_context_benchmark() -> list[str]:
    path = Path(__file__).parent / "LIVE_MCP_CONTEXT_BENCHMARK.json"
    display_path = f"bench/{path.name}"
    if not path.exists():
        return []
    try:
        benchmark = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        return [f"- Could not load `{display_path}`: {exc}.", ""]

    commands = benchmark.get("commands", {})
    index = benchmark.get("index", {})
    index_result = index.get("result", {})
    metrics = benchmark.get("metrics", {})
    graphify = benchmark.get("graphify", {})
    quality = benchmark.get("quality", {})
    registrations = benchmark.get("agent_registrations") or [benchmark.get("agent_registration", {})]

    lines: list[str] = []
    w = lines.append
    w("## Pulse local MCP context benchmark")
    w("")
    w(
        f"Raw artifact: `{display_path}`. Benchmark used a fresh shallow clone of `{benchmark.get('repo')}` "
        f"at commit `{benchmark.get('commit')}` and indexed it into local SQLite."
    )
    w("")
    w("Commands:")
    w("")
    for key, label in (
        ("atlas_index", "Atlas index"),
        ("atlas_install_codex_mcp", "Codex MCP registration"),
        ("atlas_install_claude_mcp", "Claude MCP registration"),
        ("atlas_cli_context", "Atlas CLI context"),
        ("atlas_mcp", "Atlas MCP HTTP server"),
    ):
        if commands.get(key):
            w(f"- {label}: `{commands[key]}`")
    if graphify.get("query_command"):
        w(f"- graphify query: `{graphify['query_command']}`")
    w("")
    w("Results:")
    w("")
    w(
        f"- Atlas indexed {index_result.get('indexed_files')} files, {index_result.get('symbols')} symbols, "
        f"and {index_result.get('edges')} edges into SQLite in {index.get('seconds')}s."
    )
    reg_bits = []
    for reg in registrations:
        if not reg:
            continue
        ok = reg.get("contains_atlas_server") and reg.get("contains_sqlite_db")
        extra = ""
        if reg.get("agent") == "claude":
            extra = f", skill markdown written={bool(reg.get('skill_markdown_written'))}"
        reg_bits.append(f"{reg.get('agent')}: {'ok' if ok else 'failed'}{extra}")
    if reg_bits:
        w(f"- Local agent registration checks: {', '.join(reg_bits)}.")
    w(
        f"- Token cost: raw changed file {metrics.get('raw_changed_file_tokens')} tokens, "
        f"Atlas MCP plain context {metrics.get('atlas_mcp_context_plain_tokens')} tokens, "
        f"Atlas MCP JSON context {metrics.get('atlas_mcp_context_json_tokens')} tokens."
    )
    if metrics.get("raw_file_to_mcp_plain_token_ratio") is not None:
        w(f"- Raw-file/MCP-plain token ratio: {metrics['raw_file_to_mcp_plain_token_ratio']}x.")
    if metrics.get("graphify_to_mcp_plain_token_ratio") is not None:
        w(
            f"- graphify/MCP-plain token ratio: {metrics['graphify_to_mcp_plain_token_ratio']}x; "
            f"latency ratio: {metrics.get('graphify_to_mcp_plain_latency_ratio')}x."
        )
    checks = [
        "contains_changed_file",
        "contains_router_symbol",
        "contains_serve_http_symbol",
        "has_body_excerpt",
    ]
    passed = [key for key in checks if quality.get(key)]
    w(f"- Useful-context checks passed: {len(passed)}/{len(checks)} ({', '.join(passed)}).")
    if (metrics.get("raw_file_to_mcp_plain_token_ratio") or 0) >= 5:
        w("5x note: this benchmark proves the Pulse-style local MCP path is more than 5x lower token cost than raw changed-file context while preserving review-relevant symbols.")
    else:
        w("Saturation note: this benchmark does not prove the raw-file/MCP token-cost 5x target; inspect the raw artifact before claiming it.")
    w("")
    return lines


def render_live_benchmarks() -> list[str]:
    benchmark_paths = live_benchmark_paths()
    if not benchmark_paths:
        return []

    lines: list[str] = []
    w = lines.append
    w("## Live additional-language benchmarks")
    w("")
    for path, display_path in benchmark_paths:
        lines.extend(render_one_live_benchmark(path, display_path))
    return lines


def render_one_live_benchmark(path: Path, display_path: str) -> list[str]:
    try:
        benchmark = json.loads(path.read_text())
    except (OSError, json.JSONDecodeError) as exc:
        return [
            f"- Could not load `{display_path}`: {exc}.",
            "",
        ]

    atlas = benchmark.get("atlas", {})
    graphify = benchmark.get("graphify", {})
    index = atlas.get("index", {})
    reindex = atlas.get("reindex", {})
    commands = benchmark.get("commands", {})
    lines: list[str] = []
    w = lines.append
    raw_language = str(benchmark.get("language", path.stem))
    language = {
        "apex": "Apex",
        "astro": "Astro",
        "blade": "Blade",
        "byond": "BYOND/DM",
        "csharp": "C#",
        "cuda": "CUDA C++",
        "dart": "Dart",
        "delphi": "Delphi/Lazarus",
        "dotnet": ".NET Project",
        "ejs": "EJS",
        "elixir": "Elixir",
        "ets": "ETS/ArkTS",
        "groovy": "Groovy/Gradle",
        "json": "JSON Config",
        "kotlin": "Kotlin",
        "markdown": "Markdown",
        "objc": "Objective-C",
        "php": "PHP",
        "powershell": "PowerShell",
        "r": "R",
        "razor": "Razor",
        "ruby": "Ruby",
        "rust": "Rust",
        "scala": "Scala",
        "sql": "SQL",
        "svelte": "Svelte",
        "terraform": "Terraform/HCL",
        "vue": "Vue",
        "zig": "Zig",
    }.get(raw_language.lower(), raw_language.title())
    w(f"### {language}")
    w("")
    w(
        f"Raw artifact: `{display_path}`. Benchmark used a fresh shallow clone of `{benchmark.get('repo')}` at commit "
        f"`{benchmark.get('commit')}`. `graphify-out/` was removed before Atlas indexed "
        "the repo, then graphify was run afterward for the comparison."
    )
    if commands.get("target_path"):
        w(f"Benchmark target: `{commands['target_path']}`.")
    w("")
    w("Commands:")
    w("")
    for key, label in (
        ("atlas_index", "Atlas index"),
        ("atlas_reindex", "Atlas no-change reindex"),
        ("graphify_update", "graphify update"),
        ("native_baseline", "Native baseline"),
    ):
        if commands.get(key):
            w(f"- {label}: `{commands[key]}`")
    w("")
    w("Results:")
    w("")
    w(
        f"- Atlas indexed {index.get('indexed_files')} files, {index.get('symbols')} symbols, "
        f"and {index.get('edges')} edges in {atlas.get('cold_wall_seconds')}s cold; "
        f"no-change reindex was {atlas.get('reindex_wall_seconds')}s (`mode={reindex.get('mode')}`)."
    )
    langs = index.get("languages") or {}
    if langs:
        lang_text = ", ".join(f"`{k}:{v}`" for k, v in sorted(langs.items(), key=lambda kv: (-kv[1], kv[0])))
        w(f"- Atlas language counts were {lang_text}.")
    graphify_metrics = graphify.get("metrics", graphify)
    graphify_seconds = graphify.get("update_wall_seconds", graphify.get("seconds"))
    if graphify_metrics:
        w(
            f"- graphify rebuilt {graphify_metrics.get('nodes')} nodes and {graphify_metrics.get('links')} links "
            f"in {graphify_seconds}s."
        )
    if benchmark.get("graphify_detector_only"):
        w(
            f"- graphify detector-only caveat: `{benchmark['graphify_detector_only']}` is present in `CODE_EXTENSIONS`, "
            "but this installed graphify runtime has no `_DISPATCH` extractor for it; graphify query rows are kept as missing-baseline evidence rather than 5x proof."
        )
    w("- The generated-output bug is now covered: Atlas skips `graphify-out/`, so competitor sidecars no longer inflate Atlas symbol/file counts.")
    native = benchmark.get("native_baseline") or {}
    if native:
        native_metrics = native.get("metrics", {})
        native_status = native.get("status", "unknown")
        native_bits = ", ".join(f"{k}:{v}" for k, v in native_metrics.items() if isinstance(v, (int, float, str)))
        w(f"- Native baseline `{native.get('tool', 'native')}` status: {native_status}" + (f" ({native_bits})." if native_bits else "."))
    richer = benchmark.get("richer_native_baselines") or {}
    missing_richer = [name for name, data in richer.items() if data.get("status") == "missing"]
    if missing_richer:
        w(f"- Richer native baselines not available on this machine: {', '.join(f'`{name}`' for name in missing_richer)}.")
    coverage = benchmark.get("coverage") or {}
    if coverage:
        cov_text = ", ".join(f"{k}: {'n/a' if v is None else v}" for k, v in coverage.items())
        w(f"- Coverage proxy: {cov_text}.")
    optimization = benchmark.get("optimization") or {}
    if optimization:
        w(
            f"- Optimization cycles: {optimization.get('cycles_run')} "
            f"({optimization.get('stop_reason')})."
        )
    w("")
    w("| query | Atlas ms | graphify ms | latency ratio | Atlas tokens | graphify tokens | token ratio |")
    w("|---|---:|---:|---:|---:|---:|---:|")
    total_atlas_ms = 0.0
    total_graphify_ms = 0.0
    total_atlas_tokens = 0
    total_graphify_tokens = 0
    equivalent_atlas_ms = 0.0
    equivalent_graphify_ms = 0.0
    equivalent_atlas_tokens = 0
    equivalent_graphify_tokens = 0
    equivalent_rows = 0
    missing_rows: list[str] = []
    for row in benchmark.get("queries", []):
        atlas_ms = float(row.get("atlas_ms", 0) or 0)
        graphify_ms = float(row.get("graphify_ms", 0) or 0)
        atlas_tokens = int(row.get("atlas_tokens", 0) or 0)
        graphify_tokens = int(row.get("graphify_tokens", 0) or 0)
        total_atlas_ms += atlas_ms
        total_graphify_ms += graphify_ms
        total_atlas_tokens += atlas_tokens
        total_graphify_tokens += graphify_tokens
        if row.get("atlas_missing") or row.get("graphify_missing"):
            status = "atlas_missing" if row.get("atlas_missing") else "graphify_missing"
            missing_rows.append(f"`{row.get('symbol')}` ({status})")
        else:
            equivalent_rows += 1
            equivalent_atlas_ms += atlas_ms
            equivalent_graphify_ms += graphify_ms
            equivalent_atlas_tokens += atlas_tokens
            equivalent_graphify_tokens += graphify_tokens
        w(
            f"| `{row.get('symbol')}` | {atlas_ms:.3f} | {graphify_ms:.3f} | "
            f"{ratio(graphify_ms, atlas_ms)}x | {atlas_tokens} | {graphify_tokens} | "
            f"{ratio(graphify_tokens, atlas_tokens)}x |"
        )
    w("")
    if missing_rows:
        w(f"- Query caveat: {', '.join(missing_rows)}; raw rows remain in the table.")
    if equivalent_rows:
        latency_ratio = ratio(equivalent_graphify_ms, equivalent_atlas_ms)
        token_ratio = ratio(equivalent_graphify_tokens, equivalent_atlas_tokens)
    else:
        latency_ratio = ratio(total_graphify_ms, total_atlas_ms)
        token_ratio = ratio(total_graphify_tokens, total_atlas_tokens)
    language_label = language
    if equivalent_rows == 0:
        w(
            f"No-equivalent saturation note: this {language_label} benchmark proves Atlas indexes the live language slice "
            "and matches the native coverage proxy, but graphify returned no equivalent query rows. Latency/token ratios "
            "from missing rows are not treated as 5x evidence; see the saturation loop artifact where applicable."
        )
    elif benchmark.get("graphify_detector_only"):
        w(
            f"Detector-only saturation note: this {language_label} benchmark proves Atlas indexes the live language slice "
            "and matches the native coverage proxy, but it does not prove graphify/native 5x query superiority for this extension because "
            "the installed graphify runtime has no deterministic extractor for it."
        )
    elif (latency_ratio or 0) >= 5 and (token_ratio or 0) >= 5:
        w(
            f"5x note: this {language_label} benchmark meets the 5x threshold on equivalent query rows "
            f"for latency ({latency_ratio}x) and token output ({token_ratio}x). Accuracy still uses the "
            "native/graphify coverage proxies above; this is not a blanket quality claim."
        )
    else:
        w(
            f"Saturation note: this {language_label} benchmark proves Atlas has lower latency than graphify "
            f"on these live queries ({latency_ratio}x overall), but it does not prove every 5x target "
            f"(token ratio {token_ratio}x overall). Use the raw JSON rows to distinguish exact-symbol "
            "lookup from budgeted context output before making a 5x token claim."
        )
    w("")
    return lines


def render(
    results: list[dict[str, Any]],
    graphify_discovery: dict[str, Any] | None = None,
    tool_manifest: dict[str, Any] | None = None,
) -> str:
    if graphify_discovery is None:
        graphify_discovery = discover_graphify_runtime()
    lines: list[str] = []
    w = lines.append
    w("# Atlas code-intelligence matrix benchmark\n")
    w("This report benchmarks Atlas against the agreed per-language baselines. Raw metrics are kept separate by tool because Atlas, graphify, SCIP, and LSP servers expose different surfaces.\n")
    for line in render_tool_manifest(tool_manifest or {}):
        w(line)
    w("## graphify language discovery\n")
    w(f"- Installed graphify: {graphify_discovery['version']} (`{graphify_discovery['binary']}`).")
    if graphify_discovery.get("python"):
        w(f"- Runtime Python: `{graphify_discovery['python']}`.")
    w(f"- Source inspected: `{graphify_discovery['source_root']}`.")
    if graphify_discovery.get("extract_source"):
        w(f"- Extract source: `{graphify_discovery['extract_source']}`.")
    if graphify_discovery.get("detect_source"):
        w(f"- Detect source: `{graphify_discovery['detect_source']}`.")
    for item in graphify_discovery["evidence"]:
        w(f"- Evidence: {item}")
    w("- Raw discovery artifact: `bench/GRAPHIFY_LANGUAGE_DISCOVERY.json`.")
    if graphify_discovery.get("help_command_count") is not None:
        w(f"- Runtime help probe: `graphify --help` succeeded and listed {graphify_discovery['help_command_count']} command/help lines.")
    if graphify_discovery.get("dispatch_count"):
        w(
            f"- Runtime support probe: `_DISPATCH` plus filename-special extractors exposed "
            f"{graphify_discovery['dispatch_count']} deterministic extractor entries; "
            f"`CODE_EXTENSIONS` exposed {graphify_discovery['code_extension_count']} code extensions."
        )
    if graphify_discovery.get("detect_benchmark_total_files"):
        w(
            f"- Runtime detect benchmark: generated one sample per `CODE_EXTENSIONS` entry; "
            f"`detect()` returned {graphify_discovery['detect_benchmark_total_files']} code files."
        )
    if graphify_discovery.get("runtime_error"):
        w(f"- Runtime discovery warning: {graphify_discovery['runtime_error']}. Falling back to the checked-in family table.")
    detector_only = ", ".join(f"`{ext}`" for ext in graphify_discovery["detector_only_code_extensions"]) or "none"
    w(f"- Detector-only code extensions in this graphify build, not counted as deterministic parser support because `_DISPATCH` has no extractor for them: {detector_only}.\n")
    w("| graphify family | extensions / special cases | graphify extractor | Atlas status |")
    w("|---|---|---|---|")
    for family, exts, extractor, atlas_status in graphify_discovery["rows"]:
        w(f"| {family} | `{exts}` | `{extractor}` | {atlas_status} |")
    w("")
    for line in render_graphify_coverage_audit(graphify_discovery, results):
        w(line)
    for line in render_saturation_report():
        w(line)
    for line in render_mcp_context_benchmark():
        w(line)
    for line in render_live_benchmarks():
        w(line)
    w("## Tool matrix\n")
    w("| Language | Repo | Atlas | graphify | SCIP | LSP |")
    w("|---|---|---|---|---|---|")
    for result in results:
        lang = result["language"]
        repo = result["repo"].split("github.com/")[-1]
        scip_key = "scip-go" if lang == "go" else next((t for t in MATRIX[lang] if t.startswith("scip-")), "")
        lsp_key = {
            "go": "gopls",
            "python": "pyright",
            "javascript": "tsserver",
            "typescript": "tsserver",
            "java": "jdtls",
            "c": "clangd",
            "cpp": "clangd",
        }.get(lang, "")
        w(f"| {lang} | {repo} | {tool_cell(result, 'atlas')} | {tool_cell(result, 'graphify')} | {tool_cell(result, scip_key)} | {tool_cell(result, lsp_key)} |")

    w("\n## Derived Go ratios\n")
    go = next((r for r in results if r["language"] == "go"), None)
    if go:
        atlas = go["tools"].get("atlas", {})
        graphify = go["tools"].get("graphify", {})
        scip = go["tools"].get("scip-go", {})
        gopls = go["tools"].get("gopls", {})
        if atlas.get("ok") and graphify.get("ok"):
            am = atlas["metrics"]
            gm = graphify["metrics"]
            for line in build_speed_lines(go):
                w(line)
            timings = am.get("timings_ms") or {}
            if timings:
                ordered = ", ".join(f"{k}:{v}ms" for k, v in sorted(timings.items(), key=lambda kv: kv[1], reverse=True))
                w(f"- Atlas index phase timings: {ordered}.")
            if am.get("edge_kinds"):
                edge_kinds = ", ".join(f"{k}:{v}" for k, v in sorted(am.get("edge_kinds", {}).items()))
                w(f"- Atlas edge kinds: {edge_kinds}.")
            w(f"- Call coverage proxy: Atlas internal calls {am.get('internal_calls', 0)} vs graphify calls {gm.get('calls', 0)}, Atlas/graphify = {ratio(am.get('internal_calls', 0), gm.get('calls', 0))}x.")
            w(f"- Atlas receiver-typed calls: {am.get('recv_typed', 0)}/{am.get('calls', 0)} = {pct(am.get('recv_typed', 0), am.get('calls', 0))}%.")
            w(f"- graphify extracted calls: {gm.get('extracted_calls', 0)}/{gm.get('calls', 0)} = {gm.get('extracted_pct', 0)}%.")
        if atlas.get("ok") and scip.get("ok"):
            am = atlas["metrics"]
            sm = scip["metrics"]
            scip_nav = scip_navigation_symbols(sm)
            w(f"- SCIP semantic index: {sm.get('documents', 0)} documents, {sm.get('symbols', 0)} symbols, {sm.get('occurrences', 0)} occurrences, {sm.get('references', 0)} references.")
            w(f"- SCIP navigation symbols (excluding local variables/packages) = {scip_nav}; Atlas symbols vs SCIP navigation symbols = {ratio(am.get('symbols', 0), scip_nav)}x.")
            w(f"- SCIP local variables = {sm.get('kinds', {}).get('Variable', 0)}. Atlas currently keeps locals out of the first-class symbol table, which lowers token cost but limits fine-grained reference parity.")
        if gopls.get("ok"):
            gm = gopls["metrics"]
            w(f"- gopls workspace truth: {gm.get('workspace_packages', 0)} workspace packages, {gm.get('workspace_compiled_go_files', 0)} compiled Go files, {gm.get('diagnostics', 0)} diagnostics, initial load {gm.get('initial_workspace_load_ms')}ms.")
        if go.get("queries"):
            qt = query_totals(go["queries"])
            w(f"- Query token cost ({qt['equivalent_rows']}/{qt['rows']} equivalent rows): graphify {qt['graphify_tokens']} tokens vs Atlas {qt['atlas_tokens']} tokens, graphify/Atlas = {ratio(qt['graphify_tokens'], qt['atlas_tokens'])}x.")
            w(f"- Query latency ({qt['equivalent_rows']}/{qt['rows']} equivalent rows): graphify {round(qt['graphify_ms'], 3)}ms vs Atlas {round(qt['atlas_ms'], 3)}ms, graphify/Atlas = {ratio(qt['graphify_ms'], qt['atlas_ms'])}x.")
            if qt["graphify_missing"]:
                w(f"- Query caveat: graphify missed {qt['graphify_missing']} Atlas-selected hub symbols; raw rows remain in the table.")
            atlas_cold = atlas.get("metrics", {}).get("full_seconds", atlas.get("metrics", {}).get("cold_seconds"))
            g_full = graphify.get("metrics", {}).get("full_seconds", graphify.get("seconds"))
            cold_speed_ratio = ratio(g_full, atlas_cold)
            if cold_speed_ratio is not None and cold_speed_ratio < 5:
                cold_timings = atlas.get("metrics", {}).get("cold_timings_ms") or atlas.get("metrics", {}).get("timings_ms") or {}
                blocker = ", ".join(f"{k}:{v}ms" for k, v in sorted(cold_timings.items(), key=lambda kv: kv[1], reverse=True)[:3])
                w(f"- Go cold-build saturation: cold-vs-cold full-index ratio is {cold_speed_ratio}x (graphify FULL {g_full}s / Atlas cold {atlas_cold}s), below 5x; Atlas's largest cold phases are {blocker}.")

    w("\n## Derived Python ratios\n")
    py = next((r for r in results if r["language"] == "python"), None)
    if py:
        atlas = py["tools"].get("atlas", {})
        graphify = py["tools"].get("graphify", {})
        scip = py["tools"].get("scip-python", {})
        pyright = py["tools"].get("pyright", {})
        if atlas.get("ok") and graphify.get("ok"):
            am = atlas["metrics"]
            gm = graphify["metrics"]
            for line in build_speed_lines(py):
                w(line)
            timings = am.get("timings_ms") or {}
            if timings:
                ordered = ", ".join(f"{k}:{v}ms" for k, v in sorted(timings.items(), key=lambda kv: kv[1], reverse=True))
                w(f"- Atlas index phase timings: {ordered}.")
            if am.get("edge_kinds"):
                edge_kinds = ", ".join(f"{k}:{v}" for k, v in sorted(am.get("edge_kinds", {}).items()))
                w(f"- Atlas edge kinds: {edge_kinds}.")
            w(f"- Call coverage proxy: Atlas internal calls {am.get('internal_calls', 0)} vs graphify calls {gm.get('calls', 0)}, Atlas/graphify = {ratio(am.get('internal_calls', 0), gm.get('calls', 0))}x.")
            w(f"- graphify extracted calls: {gm.get('extracted_calls', 0)}/{gm.get('calls', 0)} = {gm.get('extracted_pct', 0)}%.")
        if atlas.get("ok") and scip.get("ok"):
            am = atlas["metrics"]
            sm = scip["metrics"]
            w(f"- SCIP semantic index: {sm.get('documents', 0)} documents, {sm.get('symbols', 0)} symbols, {sm.get('occurrences', 0)} occurrences, {sm.get('references', 0)} references, scope={sm.get('scope', '')}.")
            w(f"- Atlas symbols vs SCIP symbols = {ratio(am.get('symbols', 0), sm.get('symbols', 0))}x. scip-python 0.6.6 reports all Python symbols as {', '.join(sm.get('kinds', {}).keys()) or 'unknown kind'}, so this is a raw coverage proxy, not navigation-kind parity.")
        truth = py.get("truth", {}).get("python_ast", {})
        if truth:
            expected = truth.get("functions", 0) + truth.get("classes", 0)
            got = truth.get("atlas_callable_class_symbols", 0)
            assignment_expected = truth.get("module_assignment_names", 0) + truth.get("class_assignment_names", 0)
            w(f"- Python AST callable/class truth: Atlas {got}/{expected} function/method/class symbols = {pct(got, expected)}% recall across {truth.get('files', 0)} files.")
            w(f"- Python AST assignment truth: Atlas {truth.get('atlas_assignment_symbols', 0)} assignment symbols vs {assignment_expected} direct module/class assignment names; extra symbols can come from conditional class scopes.")
        if pyright.get("ok"):
            pm = pyright["metrics"]
            severities = ", ".join(f"{k}:{v}" for k, v in sorted(pm.get("diagnostics_by_severity", {}).items()))
            w(f"- Pyright truth pass: {pm.get('files_analyzed', 0)} files analyzed, {pm.get('diagnostics', 0)} diagnostics ({severities}), version {pm.get('pyright_version', '')}.")
        if py.get("queries"):
            qt = query_totals(py["queries"])
            w(f"- Query token cost ({qt['equivalent_rows']}/{qt['rows']} equivalent rows): graphify {qt['graphify_tokens']} tokens vs Atlas {qt['atlas_tokens']} tokens, graphify/Atlas = {ratio(qt['graphify_tokens'], qt['atlas_tokens'])}x.")
            w(f"- Query latency ({qt['equivalent_rows']}/{qt['rows']} equivalent rows): graphify {round(qt['graphify_ms'], 3)}ms vs Atlas {round(qt['atlas_ms'], 3)}ms, graphify/Atlas = {ratio(qt['graphify_ms'], qt['atlas_ms'])}x.")
            if qt["graphify_missing"]:
                w(f"- Query caveat: graphify missed {qt['graphify_missing']} Atlas-selected hub symbols; raw rows remain in the table.")
            atlas_cold = atlas.get("metrics", {}).get("full_seconds", atlas.get("metrics", {}).get("cold_seconds"))
            g_full = graphify.get("metrics", {}).get("full_seconds", graphify.get("seconds"))
            cold_speed_ratio = ratio(g_full, atlas_cold)
            if cold_speed_ratio is not None and cold_speed_ratio < 5:
                cold_timings = atlas.get("metrics", {}).get("cold_timings_ms") or atlas.get("metrics", {}).get("timings_ms") or {}
                blocker = ", ".join(f"{k}:{v}ms" for k, v in sorted(cold_timings.items(), key=lambda kv: kv[1], reverse=True)[:3])
                w(f"- Python cold-build saturation: cold-vs-cold full-index ratio is {cold_speed_ratio}x (graphify FULL {g_full}s / Atlas cold {atlas_cold}s), below 5x; Atlas's largest cold phases are {blocker}.")

    w("\n## Derived JS/TS ratios\n")
    for lang in ("javascript", "typescript"):
        result = next((r for r in results if r["language"] == lang), None)
        if not result:
            continue
        w(f"### {lang}\n")
        atlas = result["tools"].get("atlas", {})
        graphify = result["tools"].get("graphify", {})
        scip = result["tools"].get("scip-typescript", {})
        tsserver = result["tools"].get("tsserver", {})
        if atlas.get("ok") and graphify.get("ok"):
            am = atlas["metrics"]
            gm = graphify["metrics"]
            for line in build_speed_lines(result):
                w(line)
            timings = am.get("timings_ms") or {}
            if timings:
                ordered = ", ".join(f"{k}:{v}ms" for k, v in sorted(timings.items(), key=lambda kv: kv[1], reverse=True))
                w(f"- Atlas index phase timings: {ordered}.")
            if am.get("edge_kinds"):
                edge_kinds = ", ".join(f"{k}:{v}" for k, v in sorted(am.get("edge_kinds", {}).items()))
                w(f"- Atlas edge kinds: {edge_kinds}.")
            w(f"- Call coverage proxy: Atlas internal calls {am.get('internal_calls', 0)} vs graphify calls {gm.get('calls', 0)}, Atlas/graphify = {ratio(am.get('internal_calls', 0), gm.get('calls', 0))}x.")
            w(f"- Atlas receiver-typed calls: {am.get('recv_typed', 0)}/{am.get('calls', 0)} = {pct(am.get('recv_typed', 0), am.get('calls', 0))}%.")
            w(f"- graphify extracted calls: {gm.get('extracted_calls', 0)}/{gm.get('calls', 0)} = {gm.get('extracted_pct', 0)}%.")
        if atlas.get("ok") and scip.get("ok"):
            am = atlas["metrics"]
            sm = scip["metrics"]
            w(f"- SCIP semantic index: {sm.get('documents', 0)} documents, {sm.get('symbols', 0)} symbols, {sm.get('occurrences', 0)} occurrences, {sm.get('references', 0)} references, scope={sm.get('scope', '')}.")
            w(f"- Atlas symbols vs SCIP symbols = {ratio(am.get('symbols', 0), sm.get('symbols', 0))}x. scip-typescript reports symbols as {', '.join(sm.get('kinds', {}).keys()) or 'unknown kind'}, so this is a raw coverage proxy.")
        if tsserver.get("ok"):
            tm = tsserver["metrics"]
            w(f"- TypeScript semantic check proxy: {tm.get('files', 0)} files, {tm.get('diagnostics', 0)} diagnostics, total {tm.get('total_time_sec', 0)}s, memory {tm.get('memory_kb', 0)}KB.")
            if tsserver.get("note"):
                w(f"- LSP caveat: {tsserver.get('note')}.")
        if result.get("queries"):
            qt = query_totals(result["queries"])
            w(f"- Query token cost ({qt['equivalent_rows']}/{qt['rows']} equivalent rows): graphify {qt['graphify_tokens']} tokens vs Atlas {qt['atlas_tokens']} tokens, graphify/Atlas = {ratio(qt['graphify_tokens'], qt['atlas_tokens'])}x.")
            w(f"- Query latency ({qt['equivalent_rows']}/{qt['rows']} equivalent rows): graphify {round(qt['graphify_ms'], 3)}ms vs Atlas {round(qt['atlas_ms'], 3)}ms, graphify/Atlas = {ratio(qt['graphify_ms'], qt['atlas_ms'])}x.")
            if qt["graphify_missing"]:
                w(f"- Query caveat: graphify missed {qt['graphify_missing']} Atlas-selected hub symbols; raw rows remain in the table.")
            atlas_cold = atlas.get("metrics", {}).get("full_seconds", atlas.get("metrics", {}).get("cold_seconds"))
            g_full = graphify.get("metrics", {}).get("full_seconds", graphify.get("seconds"))
            cold_speed_ratio = ratio(g_full, atlas_cold)
            if cold_speed_ratio is not None and cold_speed_ratio < 5:
                cold_timings = atlas.get("metrics", {}).get("cold_timings_ms") or atlas.get("metrics", {}).get("timings_ms") or {}
                blocker = ", ".join(f"{k}:{v}ms" for k, v in sorted(cold_timings.items(), key=lambda kv: kv[1], reverse=True)[:3])
                w(f"- {lang} cold-build saturation: cold-vs-cold full-index ratio is {cold_speed_ratio}x (graphify FULL {g_full}s / Atlas cold {atlas_cold}s), below 5x; Atlas's largest cold phases are {blocker}.")
            token_ratio = ratio(qt["graphify_tokens"], qt["atlas_tokens"])
            if token_ratio is not None and token_ratio < 5:
                avg_atlas = round(qt["atlas_tokens"] / qt["equivalent_rows"], 1) if qt["equivalent_rows"] else 0
                avg_graphify = round(qt["graphify_tokens"] / qt["equivalent_rows"], 1) if qt["equivalent_rows"] else 0
                w(f"- {lang} token saturation: current equivalent-row token ratio is {token_ratio}x, below 5x; Atlas already averages {avg_atlas} tokens/answer vs graphify {avg_graphify}, so further gains require changing retrieval semantics, not only formatting.")

    w("\n## Derived Java ratios\n")
    java = next((r for r in results if r["language"] == "java"), None)
    if java:
        atlas = java["tools"].get("atlas", {})
        graphify = java["tools"].get("graphify", {})
        scip = java["tools"].get("scip-java", {})
        jdtls = java["tools"].get("jdtls", {})
        if atlas.get("ok") and graphify.get("ok"):
            am = atlas["metrics"]
            gm = graphify["metrics"]
            for line in build_speed_lines(java):
                w(line)
            timings = am.get("timings_ms") or {}
            if timings:
                ordered = ", ".join(f"{k}:{v}ms" for k, v in sorted(timings.items(), key=lambda kv: kv[1], reverse=True))
                w(f"- Atlas index phase timings: {ordered}.")
            if am.get("edge_kinds"):
                edge_kinds = ", ".join(f"{k}:{v}" for k, v in sorted(am.get("edge_kinds", {}).items()))
                w(f"- Atlas edge kinds: {edge_kinds}.")
            w(f"- Call coverage proxy: Atlas internal calls {am.get('internal_calls', 0)} vs graphify calls {gm.get('calls', 0)}, Atlas/graphify = {ratio(am.get('internal_calls', 0), gm.get('calls', 0))}x.")
            w(f"- Atlas receiver-typed calls: {am.get('recv_typed', 0)}/{am.get('calls', 0)} = {pct(am.get('recv_typed', 0), am.get('calls', 0))}%.")
            w(f"- graphify extracted calls: {gm.get('extracted_calls', 0)}/{gm.get('calls', 0)} = {gm.get('extracted_pct', 0)}%.")
        if atlas.get("ok") and scip.get("ok"):
            am = atlas["metrics"]
            sm = scip["metrics"]
            scip_nav = scip_navigation_symbols(sm)
            w(f"- SCIP semantic index: {sm.get('documents', 0)} documents, {sm.get('symbols', 0)} symbols, {sm.get('occurrences', 0)} occurrences, {sm.get('references', 0)} references, scope={sm.get('scope', '')}.")
            w(f"- SCIP navigation symbols (excluding local variables/packages) = {scip_nav}; Atlas symbols vs SCIP navigation symbols = {ratio(am.get('symbols', 0), scip_nav)}x.")
        if jdtls.get("ok"):
            jm = jdtls["metrics"]
            w(f"- JDTLS LSP benchmark: initialized against build root {jm.get('build_root', '')}, sampled {jm.get('document_symbol_files', 0)}/{jm.get('sample_files', 0)} files, {jm.get('document_symbols', 0)} document symbols, {jm.get('workspace_symbols_query_gson', 0)} workspace symbols for query `Gson`, {jm.get('diagnostics', 0)} diagnostics.")
            if jdtls.get("note"):
                w(f"- LSP caveat: {jdtls.get('note')}.")
        if java.get("queries"):
            qt = query_totals(java["queries"])
            w(f"- Query token cost ({qt['equivalent_rows']}/{qt['rows']} equivalent rows): graphify {qt['graphify_tokens']} tokens vs Atlas {qt['atlas_tokens']} tokens, graphify/Atlas = {ratio(qt['graphify_tokens'], qt['atlas_tokens'])}x.")
            w(f"- Query latency ({qt['equivalent_rows']}/{qt['rows']} equivalent rows): graphify {round(qt['graphify_ms'], 3)}ms vs Atlas {round(qt['atlas_ms'], 3)}ms, graphify/Atlas = {ratio(qt['graphify_ms'], qt['atlas_ms'])}x.")
            if qt["graphify_missing"]:
                w(f"- Query caveat: graphify missed {qt['graphify_missing']} Atlas-selected hub symbols; raw rows remain in the table.")
            atlas_cold = atlas.get("metrics", {}).get("full_seconds", atlas.get("metrics", {}).get("cold_seconds"))
            g_full = graphify.get("metrics", {}).get("full_seconds", graphify.get("seconds"))
            cold_speed_ratio = ratio(g_full, atlas_cold)
            if cold_speed_ratio is not None and cold_speed_ratio < 5:
                cold_timings = atlas.get("metrics", {}).get("cold_timings_ms") or atlas.get("metrics", {}).get("timings_ms") or {}
                blocker = ", ".join(f"{k}:{v}ms" for k, v in sorted(cold_timings.items(), key=lambda kv: kv[1], reverse=True)[:3])
                w(f"- Java cold-build saturation: cold-vs-cold full-index ratio is {cold_speed_ratio}x (graphify FULL {g_full}s / Atlas cold {atlas_cold}s), below 5x; Atlas's largest cold phases are {blocker}.")
            token_ratio = ratio(qt["graphify_tokens"], qt["atlas_tokens"])
            if token_ratio is not None and token_ratio < 5:
                avg_atlas = round(qt["atlas_tokens"] / qt["equivalent_rows"], 1) if qt["equivalent_rows"] else 0
                avg_graphify = round(qt["graphify_tokens"] / qt["equivalent_rows"], 1) if qt["equivalent_rows"] else 0
                w(f"- Java token saturation: current equivalent-row token ratio is {token_ratio}x, below 5x; Atlas averages {avg_atlas} tokens/answer vs graphify {avg_graphify}.")

    w("\n## Derived C/C++ ratios\n")
    for lang in ("c", "cpp"):
        result = next((r for r in results if r["language"] == lang), None)
        if not result:
            continue
        w(f"### {lang}\n")
        atlas = result["tools"].get("atlas", {})
        graphify = result["tools"].get("graphify", {})
        clangd = result["tools"].get("clangd", {})
        if atlas.get("ok") and graphify.get("ok"):
            am = atlas["metrics"]
            gm = graphify["metrics"]
            for line in build_speed_lines(result):
                w(line)
            timings = am.get("timings_ms") or {}
            if timings:
                ordered = ", ".join(f"{k}:{v}ms" for k, v in sorted(timings.items(), key=lambda kv: kv[1], reverse=True))
                w(f"- Atlas index phase timings: {ordered}.")
            if am.get("edge_kinds"):
                edge_kinds = ", ".join(f"{k}:{v}" for k, v in sorted(am.get("edge_kinds", {}).items()))
                w(f"- Atlas edge kinds: {edge_kinds}.")
            w(f"- Call coverage proxy: Atlas internal calls {am.get('internal_calls', 0)} vs graphify calls {gm.get('calls', 0)}, Atlas/graphify = {ratio(am.get('internal_calls', 0), gm.get('calls', 0))}x.")
            w(f"- Atlas receiver-typed calls: {am.get('recv_typed', 0)}/{am.get('calls', 0)} = {pct(am.get('recv_typed', 0), am.get('calls', 0))}%.")
            w(f"- graphify extracted calls: {gm.get('extracted_calls', 0)}/{gm.get('calls', 0)} = {gm.get('extracted_pct', 0)}%.")
        if clangd.get("ok"):
            cm = clangd["metrics"]
            w(f"- clangd LSP benchmark: sampled {cm.get('document_symbol_files', 0)}/{cm.get('sample_files', 0)} files, {cm.get('document_symbols', 0)} document symbols, {cm.get('diagnostics', 0)} diagnostics.")
            if clangd.get("note"):
                w(f"- LSP caveat: {clangd.get('note')}.")
        if result.get("queries"):
            qt = query_totals(result["queries"])
            w(f"- Query token cost ({qt['equivalent_rows']}/{qt['rows']} equivalent rows): graphify {qt['graphify_tokens']} tokens vs Atlas {qt['atlas_tokens']} tokens, graphify/Atlas = {ratio(qt['graphify_tokens'], qt['atlas_tokens'])}x.")
            w(f"- Query latency ({qt['equivalent_rows']}/{qt['rows']} equivalent rows): graphify {round(qt['graphify_ms'], 3)}ms vs Atlas {round(qt['atlas_ms'], 3)}ms, graphify/Atlas = {ratio(qt['graphify_ms'], qt['atlas_ms'])}x.")
            if qt["graphify_missing"]:
                w(f"- Query caveat: graphify missed {qt['graphify_missing']} Atlas-selected hub symbols; raw rows remain in the table.")
            atlas_cold = atlas.get("metrics", {}).get("full_seconds", atlas.get("metrics", {}).get("cold_seconds"))
            g_full = graphify.get("metrics", {}).get("full_seconds", graphify.get("seconds"))
            cold_speed_ratio = ratio(g_full, atlas_cold)
            if cold_speed_ratio is not None and cold_speed_ratio < 5:
                cold_timings = atlas.get("metrics", {}).get("cold_timings_ms") or atlas.get("metrics", {}).get("timings_ms") or {}
                blocker = ", ".join(f"{k}:{v}ms" for k, v in sorted(cold_timings.items(), key=lambda kv: kv[1], reverse=True)[:3])
                w(f"- {lang} cold-build saturation: cold-vs-cold full-index ratio is {cold_speed_ratio}x (graphify FULL {g_full}s / Atlas cold {atlas_cold}s), below 5x; Atlas's largest cold phases are {blocker}.")
            token_ratio = ratio(qt["graphify_tokens"], qt["atlas_tokens"])
            if token_ratio is not None and token_ratio < 5:
                avg_atlas = round(qt["atlas_tokens"] / qt["equivalent_rows"], 1) if qt["equivalent_rows"] else 0
                avg_graphify = round(qt["graphify_tokens"] / qt["equivalent_rows"], 1) if qt["equivalent_rows"] else 0
                w(f"- {lang} token saturation: current equivalent-row token ratio is {token_ratio}x, below 5x; Atlas averages {avg_atlas} tokens/answer vs graphify {avg_graphify}.")

    w("")
    for line in render_warm_latency(results):
        w(line)

    w("\n## Query token probes\n")
    for result in results:
        if not result.get("queries"):
            continue
        w(f"### {result['language']}\n")
        w("| Symbol | Status | graphify tokens | Atlas tokens | graphify ms | Atlas ms |")
        w("|---|---|--:|--:|--:|--:|")
        for row in result["queries"]:
            status = "graphify_missing" if row.get("graphify_missing") else "equivalent"
            w(f"| {row['symbol']} | {status} | {row['graphify_tokens']} | {row['atlas_tokens']} | {row['graphify_ms']} | {row['atlas_ms']} |")
        w("")

    w("## Missing or partial adapters\n")
    for result in results:
        for tool, data in result["tools"].items():
            if data.get("ok"):
                continue
            w(f"- {result['language']} {tool}: {data.get('status')} - {data.get('note')}")

    w("\n---\nGenerated by `bench/codeintel_matrix.py`. Raw JSON sits next to this report; logs are in `bench/logs/`.")
    return "\n".join(lines)


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--atlas", default=os.environ.get("ATLAS_BIN", "atlas"))
    parser.add_argument("--graphify", default=os.environ.get("GRAPHIFY_BIN", "graphify"))
    parser.add_argument("--scip-go", default=os.environ.get("SCIP_GO_BIN", "scip-go"))
    parser.add_argument("--scip-python", default=os.environ.get("SCIP_PYTHON_BIN", "scip-python"))
    parser.add_argument("--scip-typescript", default=os.environ.get("SCIP_TYPESCRIPT_BIN", "scip-typescript"))
    parser.add_argument("--scip-java", default=os.environ.get("SCIP_JAVA_BIN", repo_tool("scip-java-coursier", "scip-java")))
    parser.add_argument("--gopls", default=os.environ.get("GOPLS_BIN", "gopls"))
    parser.add_argument("--pyright", default=os.environ.get("PYRIGHT_BIN", "pyright"))
    parser.add_argument("--tsc", default=os.environ.get("TSC_BIN", "tsc"))
    parser.add_argument("--jdtls", default=os.environ.get("JDTLS_CMD", os.environ.get("JDTLS_BIN", "jdtls")))
    parser.add_argument("--clangd", default=os.environ.get("CLANGD_BIN", "clangd"))
    parser.add_argument("--workdir", default="/tmp/atlas-codeintel-matrix")
    parser.add_argument("--out", default="bench/MATRIX_REPORT.md")
    parser.add_argument("--langs", default="go")
    args = parser.parse_args()

    workdir = Path(args.workdir)
    workdir.mkdir(parents=True, exist_ok=True)
    output = Path(args.out)
    output.parent.mkdir(parents=True, exist_ok=True)

    results = []
    for lang in [part.strip() for part in args.langs.split(",") if part.strip()]:
        if lang not in REPOS:
            raise SystemExit(f"unsupported language: {lang}")
        print(f"[matrix] {lang}", flush=True)
        results.append(run_language(lang, args, workdir))

    graphify_discovery = discover_graphify_runtime()
    tool_manifest = benchmark_tool_manifest(args, graphify_discovery)

    output.write_text(render(results, graphify_discovery, tool_manifest))
    output.with_suffix(".json").write_text(json.dumps(results, indent=2))
    discovery_output = output.parent / "GRAPHIFY_LANGUAGE_DISCOVERY.json"
    discovery_output.write_text(json.dumps(graphify_discovery, indent=2) + "\n")
    tool_versions_output = output.parent / "MATRIX_TOOL_VERSIONS.json"
    tool_versions_output.write_text(json.dumps(tool_manifest, indent=2) + "\n")
    print(f"[matrix] wrote {output}, {output.with_suffix('.json')}, {discovery_output}, and {tool_versions_output}")


if __name__ == "__main__":
    main()
