#!/usr/bin/env python3
"""Count .NET project metadata symbols for Atlas validation remeasurement."""

from __future__ import annotations

import json
import re
import sys
import xml.etree.ElementTree as ET
from pathlib import Path


PROJECT_EXTS = {".csproj", ".fsproj", ".vbproj"}
SOLUTION_EXTS = {".sln", ".slnx"}


def iter_files(root: Path):
    for path in sorted(root.rglob("*")):
        if not path.is_file():
            continue
        if "graphify-out" in path.parts:
            continue
        if path.suffix.lower() in PROJECT_EXTS or path.suffix.lower() in SOLUTION_EXTS:
            yield path


def local_name(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def count_solution_projects(path: Path) -> int:
    text = path.read_text(encoding="utf-8", errors="ignore")
    if path.suffix.lower() == ".slnx":
        try:
            root = ET.fromstring(text)
        except ET.ParseError:
            return 0
        return sum(1 for elem in root.iter() if local_name(elem.tag).lower() == "project")
    return len(re.findall(r'^Project\("[^"]+"\)\s*=\s*"[^"]+",\s*"[^"]+"', text, flags=re.MULTILINE))


def count_project_xml(path: Path, stats: dict) -> None:
    stats["definition_counts"]["project"] += 1
    try:
        root = ET.parse(path).getroot()
    except ET.ParseError as error:
        stats["parse_errors"] += 1
        if len(stats["parse_error_samples"]) < 8:
            stats["parse_error_samples"].append({
                "path": str(path),
                "error": str(error),
            })
        return

    stats["parsed_files"] += 1
    if root.attrib.get("Sdk"):
        stats["definition_counts"]["sdk"] += 1
    for elem in root.iter():
        name = local_name(elem.tag)
        if name == "PackageReference":
            stats["definition_counts"]["package"] += 1
        elif name == "ProjectReference":
            stats["definition_counts"]["project_reference"] += 1
        elif name == "TargetFramework" and (elem.text or "").strip():
            stats["definition_counts"]["target_framework"] += 1
        elif name == "TargetFrameworks" and (elem.text or "").strip():
            stats["definition_counts"]["target_framework"] += len(
                [part for part in re.split(r"[;,]", elem.text) if part.strip()]
            )


def main() -> int:
    if len(sys.argv) != 2:
        print("Usage: dotnet_project_stats.py <root>", file=sys.stderr)
        return 2

    root = Path(sys.argv[1]).resolve()
    stats = {
        "files": 0,
        "parsed_files": 0,
        "parse_errors": 0,
        "definition_counts": {
            "project": 0,
            "sdk": 0,
            "package": 0,
            "project_reference": 0,
            "target_framework": 0,
        },
        "definitions": 0,
        "parse_error_samples": [],
    }

    for path in iter_files(root):
        stats["files"] += 1
        if path.suffix.lower() in SOLUTION_EXTS:
            stats["parsed_files"] += 1
            stats["definition_counts"]["project"] += count_solution_projects(path)
        elif path.suffix.lower() in PROJECT_EXTS:
            count_project_xml(path, stats)

    stats["definitions"] = sum(stats["definition_counts"].values())
    print(json.dumps(stats, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
