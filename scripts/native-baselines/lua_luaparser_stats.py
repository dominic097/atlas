#!/usr/bin/env python3
"""Count Lua function definitions with luaparser for Atlas validation replay."""

from __future__ import annotations

import json
import sys
from pathlib import Path

try:
    from luaparser import ast, astnodes
except ImportError as error:
    raise SystemExit(
        "luaparser is required. Run with: "
        "uv run --with luaparser==4.0.1 python scripts/native-baselines/lua_luaparser_stats.py <root>"
    ) from error


def iter_lua_files(root: Path):
    for path in sorted(root.rglob("*.lua")):
        if "graphify-out" in path.parts:
            continue
        if path.is_file():
            yield path


def add_sample(stats: dict, kind: str, name: str, path: Path, root: Path) -> None:
    if len(stats["sample_definitions"]) >= 30:
        return
    stats["sample_definitions"].append({
        "kind": kind,
        "name": name,
        "path": str(path.relative_to(root)),
    })


def node_name(node) -> str:
    name = getattr(node, "name", None)
    if isinstance(name, astnodes.Name):
        return name.id
    if isinstance(name, astnodes.Index):
        idx = getattr(name, "idx", None)
        if isinstance(idx, astnodes.Name):
            return idx.id
    return "anonymous"


def assignment_name(node) -> str:
    targets = getattr(node, "targets", []) or []
    if not targets:
        return "anonymous"
    target = targets[0]
    if isinstance(target, astnodes.Name):
        return target.id
    if isinstance(target, astnodes.Index):
        idx = getattr(target, "idx", None)
        if isinstance(idx, astnodes.Name):
            return idx.id
    return "anonymous"


def count_file(path: Path, root: Path, stats: dict) -> None:
    tree = ast.parse(path.read_text(encoding="utf-8", errors="ignore"))
    for node in ast.walk(tree):
        if isinstance(node, astnodes.Function):
            stats["definition_counts"]["function"] += 1
            add_sample(stats, "function", node_name(node), path, root)
        elif isinstance(node, astnodes.LocalFunction):
            stats["definition_counts"]["local_function"] += 1
            add_sample(stats, "local_function", node_name(node), path, root)
        elif isinstance(node, (astnodes.Assign, astnodes.LocalAssign)):
            values = getattr(node, "values", []) or []
            if any(isinstance(value, astnodes.AnonymousFunction) for value in values):
                stats["definition_counts"]["assigned_function"] += 1
                add_sample(stats, "assigned_function", assignment_name(node), path, root)


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: lua_luaparser_stats.py <root>", file=sys.stderr)
        return 2

    root = Path(sys.argv[1]).resolve()
    stats = {
        "files": 0,
        "parsed_files": 0,
        "parse_errors": 0,
        "definition_counts": {
            "function": 0,
            "local_function": 0,
            "assigned_function": 0,
        },
        "definitions": 0,
        "luaparser_version": getattr(sys.modules.get("luaparser"), "__version__", "unknown"),
        "parse_error_samples": [],
        "sample_definitions": [],
    }

    for path in iter_lua_files(root):
        stats["files"] += 1
        try:
            count_file(path, root, stats)
        except Exception as error:  # luaparser raises parser-specific exceptions.
            stats["parse_errors"] += 1
            if len(stats["parse_error_samples"]) < 8:
                stats["parse_error_samples"].append({
                    "path": str(path.relative_to(root)),
                    "error": str(error),
                })
            continue
        stats["parsed_files"] += 1

    stats["definitions"] = sum(stats["definition_counts"].values())
    print(json.dumps(stats, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
