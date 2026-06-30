#!/usr/bin/env python3
"""Count Razor directive/component symbols for Atlas validation remeasurement."""

from __future__ import annotations

import json
import re
import sys
from pathlib import Path


RAZOR_EXTS = {".razor", ".cshtml"}
DIRECTIVE_PATTERNS = {
    "route": re.compile(r"(?m)^\s*@page\b"),
    "import": re.compile(r"(?m)^\s*@(using|namespace)\b"),
    "service": re.compile(r"(?m)^\s*@inject\b"),
    "base": re.compile(r"(?m)^\s*@inherits\b"),
    "model": re.compile(r"(?m)^\s*@model\b"),
}
COMPONENT_RE = re.compile(r"<([A-Z][A-Za-z0-9_.]*)\b")
METHOD_RE = re.compile(
    r"(?m)^\s*"
    r"(?:public|private|protected|internal|static|async|override|virtual|sealed|partial|new|\s)+"
    r"[\w<>\[\],?]+\s+(\w+)\s*\([^;{}]*\)\s*(?:=>|\{)"
)
CONTROL_WORDS = {"if", "for", "while", "switch", "catch", "using", "foreach", "lock"}


def iter_razor_files(root: Path):
    for path in sorted(root.rglob("*")):
        if not path.is_file():
            continue
        if "graphify-out" in path.parts:
            continue
        if path.suffix.lower() in RAZOR_EXTS:
            yield path


def add_definition(stats: dict, kind: str, name: str, path: Path, root: Path) -> None:
    stats["definition_counts"][kind] += 1
    if len(stats["sample_definitions"]) < 30:
        stats["sample_definitions"].append({
            "kind": kind,
            "name": name,
            "path": str(path.relative_to(root)),
        })


def file_view_name(path: Path, root: Path) -> str:
    rel = path.relative_to(root)
    name = str(rel)
    for suffix in (".razor", ".cshtml"):
        if name.endswith(suffix):
            name = name[: -len(suffix)]
            break
    return name.replace("/", ".").replace("\\", ".")


def count_file(path: Path, root: Path, stats: dict) -> None:
    text = path.read_text(encoding="utf-8", errors="ignore")
    add_definition(stats, "view", file_view_name(path, root), path, root)

    for kind, pattern in DIRECTIVE_PATTERNS.items():
        for match in pattern.finditer(text):
            add_definition(stats, kind, match.group(0).strip(), path, root)

    for match in COMPONENT_RE.finditer(text):
        add_definition(stats, "component", match.group(1), path, root)

    # Match the benchmark-owned proxy denominator: method declarations inside
    # Razor files, excluding parameter-heavy component-library internals.
    if "[Parameter]" in text:
        return
    for match in METHOD_RE.finditer(text):
        name = match.group(1)
        if name in CONTROL_WORDS:
            continue
        add_definition(stats, "method", name, path, root)


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: razor_stats.py <root>", file=sys.stderr)
        return 2

    root = Path(sys.argv[1]).resolve()
    stats = {
        "files": 0,
        "parsed_files": 0,
        "parse_errors": 0,
        "definition_counts": {
            "view": 0,
            "component": 0,
            "route": 0,
            "import": 0,
            "service": 0,
            "base": 0,
            "model": 0,
            "method": 0,
        },
        "definitions": 0,
        "sample_definitions": [],
    }

    for path in iter_razor_files(root):
        stats["files"] += 1
        stats["parsed_files"] += 1
        count_file(path, root, stats)

    stats["definitions"] = sum(stats["definition_counts"].values())
    print(json.dumps(stats, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
