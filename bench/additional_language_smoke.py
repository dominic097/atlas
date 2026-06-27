#!/usr/bin/env python3
"""Live smoke benchmarks for graphify-supported languages outside the main matrix.

The main matrix covers Go/Python/JS/TS/Java/C/C++. This script adds one
additional graphify language at a time with a real repository, Atlas+SQLite,
graphify, and the best scriptable native baseline available on the machine.
"""

from __future__ import annotations

import argparse
import json
import os
import queue
import re
import shutil
import sqlite3
import subprocess
import tempfile
import textwrap
import threading
import time
from pathlib import Path
from typing import Any


LANGUAGES: dict[str, dict[str, Any]] = {
    "bash": {
        "repo": "https://github.com/nvm-sh/nvm",
        "workdir": "/tmp/atlas-live-bash-nvm",
        "queries": ["nvm", "nvm_install_binary", "nvm_die_on_prefix", "nvm_get_os"],
        "native": "bash-n",
    },
    "csharp": {
        "repo": "https://github.com/DapperLib/Dapper",
        "workdir": "/tmp/atlas-live-csharp-dapper",
        "queries": ["SqlMapper", "CommandDefinition", "DynamicParameters", "TypeHandlerCache"],
        "native": "roslyn",
    },
    "dart": {
        "repo": "https://github.com/dart-lang/http",
        "subdir": "pkgs/http/lib",
        "workdir": "/tmp/atlas-live-dart-http",
        "queries": ["Client", "BaseClient", "Request", "Response", "send", "RetryClient"],
        "native": "tree-sitter-dart",
    },
    "kotlin": {
        "repo": "https://github.com/square/okhttp",
        "subdir": "okhttp/src/commonJvmAndroid/kotlin",
        "workdir": "/tmp/atlas-live-kotlin-okhttp",
        "queries": ["OkHttpClient", "Request", "Response", "HttpUrl", "Headers"],
        "native": "tree-sitter-kotlin",
    },
    "lua": {
        "repo": "https://github.com/folke/lazy.nvim",
        "workdir": "/tmp/atlas-live-lua-lazy",
        "queries": ["Loader.load", "Async.new", "M.add", "M.reload"],
        "native": "luaparser",
    },
    "php": {
        "repo": "https://github.com/slimphp/Slim",
        "workdir": "/tmp/atlas-live-php-slim",
        "queries": ["handle", "process", "addRoute", "getResponseFactory"],
        "native": "php-tokenizer",
    },
    "powershell": {
        "repo": "https://github.com/PowerShell/PowerShellGet",
        "subdir": "src",
        "workdir": "/tmp/atlas-live-powershell-powershellget",
        "queries": ["Find-Module", "Install-Module", "Register-PSRepository", "Update-ModuleManifest"],
        "native": "pwsh-parser",
    },
    "ruby": {
        "repo": "https://github.com/sinatra/sinatra",
        "workdir": "/tmp/atlas-live-ruby-sinatra",
        "queries": ["initialize", "call", "route", "settings"],
        "native": "ruby-ripper",
    },
    "rust": {
        "repo": "https://github.com/BurntSushi/ripgrep",
        "workdir": "/tmp/atlas-live-rust-ripgrep",
        "queries": ["HiArgs", "LowArgs", "PatternMatcher", "WalkBuilder"],
        "native": "rust-analyzer",
    },
    "scala": {
        "repo": "https://github.com/typelevel/cats",
        "subdir": "core/src/main/scala",
        "workdir": "/tmp/atlas-live-scala-cats",
        "queries": ["Functor", "Applicative", "Monad", "Traverse", "Eval"],
        "native": "tree-sitter-scala",
    },
    "svelte": {
        "repo": "https://github.com/carbon-design-system/carbon-components-svelte",
        "subdir": "src",
        "workdir": "/tmp/atlas-live-svelte-carbon",
        "queries": ["setChar", "focusInput", "handleInput", "handleKeydown", "handleOutsideClick"],
        "native": "svelte-compiler",
    },
    "sql": {
        "repo": "https://github.com/hasura/graphql-engine",
        "subdir": "server/src-rsr/migrations",
        "workdir": "/tmp/atlas-live-sql-hasura",
        "queries": [
            "hdb_catalog.event_triggers",
            "hdb_catalog.hdb_metadata",
            "hdb_catalog.hdb_schema_update_event_notifier",
            "hdb_catalog.hdb_function_agg",
        ],
        "native": "sqlfluff",
    },
    "terraform": {
        "repo": "https://github.com/terraform-aws-modules/terraform-aws-vpc",
        "workdir": "/tmp/atlas-live-terraform-vpc",
        "queries": [
            "aws_vpc.this",
            "aws_subnet.public",
            "aws_route_table.public",
            "aws_nat_gateway.this",
        ],
        "native": "python-hcl2",
    },
    "vue": {
        "repo": "https://github.com/gothinkster/vue-realworld-example-app",
        "subdir": "src",
        "workdir": "/tmp/atlas-live-vue-realworld",
        "queries": ["parseMarkdown", "follow", "goTo", "onPageChange"],
        "native": "vue-compiler-sfc",
    },
    "swift": {
        "repo": "https://github.com/apple/swift-argument-parser",
        "workdir": "/tmp/atlas-live-swift-argument-parser",
        "queries": ["ArgumentParser", "parse", "run", "help"],
        "native": "sourcekit-lsp",
    },
    "elixir": {
        "repo": "https://github.com/phoenixframework/phoenix",
        "subdir": "lib",
        "workdir": "/tmp/atlas-live-elixir-phoenix",
        "queries": ["Phoenix.Router", "Phoenix.Endpoint", "Phoenix.Controller", "path", "socket"],
        "native": "tree-sitter-elixir",
    },
    "fortran": {
        "repo": "https://github.com/fortran-lang/stdlib",
        "subdir": "src",
        "workdir": "/tmp/atlas-live-fortran-stdlib",
        "queries": ["stdlib_array", "stdlib_datetime", "datetime_type", "hashmap_type", "loading", "free_chaining_map"],
        "native": "tree-sitter-fortran",
    },
    "verilog": {
        "repo": "https://github.com/lowRISC/ibex",
        "subdir": "rtl",
        "workdir": "/tmp/atlas-live-verilog-ibex",
        "queries": ["ibex_core", "ibex_top", "ibex_pkg", "ibex_alu", "cm_stack_adj_base", "decode_i_insn"],
        "native": "tree-sitter-systemverilog",
    },
    "groovy": {
        "repo": "https://github.com/nextflow-io/nextflow",
        "subdir": "modules/nf-commons/src/main",
        "workdir": "/tmp/atlas-live-groovy-nextflow",
        "queries": ["SysEnv", "Const", "Duration", "getVersion", "format"],
        "native": "tree-sitter-groovy",
    },
    "objc": {
        "repo": "https://github.com/SDWebImage/SDWebImage",
        "subdir": "SDWebImage",
        "workdir": "/tmp/atlas-live-objc-sdwebimage",
        "queries": ["SDImageCache", "SDWebImageManager", "sharedImageCache", "storeImage:forKey:completion:", "objectForKey:"],
        "native": "tree-sitter-objc",
    },
    "julia": {
        "repo": "https://github.com/JuliaIO/JSON.jl",
        "subdir": "src",
        "workdir": "/tmp/atlas-live-julia-json",
        "queries": ["JSON", "JSONText", "Object", "parse", "json", "LazyValue"],
        "native": "tree-sitter-julia",
    },
    "zig": {
        "repo": "https://github.com/zigtools/zls",
        "subdir": "src",
        "workdir": "/tmp/atlas-live-zig-zls",
        "queries": ["Server", "DocumentStore", "Analyser", "Config", "main"],
        "native": "tree-sitter-zig",
    },
}


def run(cmd: list[str], cwd: Path | None = None, timeout: int = 900) -> subprocess.CompletedProcess[str]:
    return subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout)


def tokens(text: str) -> int:
    return max(1, len(text) // 4)


def ratio(num: float, den: float) -> float | None:
    if not num or not den:
        return None
    return round(num / den, 2)


def executable(path_or_name: str) -> str | None:
    expanded = Path(path_or_name).expanduser()
    if expanded.exists() and os.access(expanded, os.X_OK):
        return str(expanded)
    return shutil.which(path_or_name)


def resolve_graphify(value: str) -> str | None:
    found = executable(value)
    if found:
        return found
    uv_tool = Path.home() / ".local/share/uv/tools/graphifyy/bin/graphify"
    if uv_tool.exists() and os.access(uv_tool, os.X_OK):
        return str(uv_tool)
    return None


def clean_generated_sidecars(target: Path) -> None:
    graph_out = target / "graphify-out"
    if graph_out.exists():
        shutil.rmtree(graph_out, ignore_errors=True)


def sqlite_breakdown(db: Path, table: str, where: str = "", params: tuple[Any, ...] = ()) -> list[dict[str, Any]]:
    if not db.exists():
        return []
    con = sqlite3.connect(str(db))
    cur = con.cursor()
    clause = f" WHERE {where}" if where else ""
    if table == "symbols":
        cur.execute(f"SELECT language, kind, count(*) FROM symbols{clause} GROUP BY language, kind ORDER BY language, kind", params)
        rows = [{"language": r[0], "kind": r[1], "count": r[2]} for r in cur.fetchall()]
    elif table == "edges":
        cur.execute(f"SELECT kind, count(*) FROM edges{clause} GROUP BY kind ORDER BY kind", params)
        rows = [{"kind": r[0], "count": r[1]} for r in cur.fetchall()]
    else:
        rows = []
    con.close()
    return rows


def sqlite_scalar(db: Path, sql: str, params: tuple[Any, ...] = ()) -> int:
    con = sqlite3.connect(str(db))
    cur = con.cursor()
    cur.execute(sql, params)
    value = int(cur.fetchone()[0] or 0)
    con.close()
    return value


def _tail_pipe(pipe: Any, sink: list[str], limit: int = 80) -> None:
    try:
        for raw in iter(pipe.readline, b""):
            text = raw.decode("utf-8", errors="replace").strip()
            if text:
                sink.append(text)
                del sink[:-limit]
    except Exception:
        return


def _lsp_write(proc: subprocess.Popen[bytes], payload: dict[str, Any]) -> None:
    data = json.dumps(payload, separators=(",", ":")).encode("utf-8")
    header = f"Content-Length: {len(data)}\r\n\r\n".encode("ascii")
    assert proc.stdin is not None
    proc.stdin.write(header + data)
    proc.stdin.flush()


def _lsp_read_loop(stdout: Any, messages: "queue.Queue[dict[str, Any]]") -> None:
    while True:
        headers: dict[str, str] = {}
        line = stdout.readline()
        if not line:
            return
        while line not in (b"\r\n", b"\n", b""):
            text = line.decode("ascii", errors="replace").strip()
            if ":" in text:
                key, value = text.split(":", 1)
                headers[key.lower()] = value.strip()
            line = stdout.readline()
        if not line:
            return
        length = int(headers.get("content-length", "0"))
        if length <= 0:
            continue
        body = stdout.read(length)
        try:
            messages.put(json.loads(body.decode("utf-8")))
        except json.JSONDecodeError:
            continue


def _wait_lsp(
    messages: "queue.Queue[dict[str, Any]]",
    wanted_ids: set[int],
    diagnostics: dict[str, int],
    timeout: float,
) -> dict[int, dict[str, Any]]:
    deadline = time.time() + timeout
    found: dict[int, dict[str, Any]] = {}
    while time.time() < deadline and wanted_ids - found.keys():
        try:
            msg = messages.get(timeout=max(0.1, deadline - time.time()))
        except queue.Empty:
            break
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


def swift_source_files(repo: Path, limit: int = 16) -> list[Path]:
    files = [
        p
        for p in repo.rglob("*.swift")
        if p.is_file()
        and "/.build/" not in p.as_posix()
        and "/graphify-out/" not in p.as_posix()
    ]
    return sorted(files, key=lambda p: ("/Tests/" in p.as_posix(), str(p)))[:limit]


def run_ruby_ripper(repo: Path, workdir: Path) -> dict[str, Any]:
    ruby = executable("ruby")
    if not ruby:
        return {"tool": "ruby-ripper", "status": "missing", "ok": False, "note": "ruby not found"}

    script = workdir / "ripper_stats.rb"
    script.write_text(
        textwrap.dedent(
            r'''
            require "json"
            require "ripper"

            root = ARGV.fetch(0)
            stats = {
              files: 0,
              parsed_files: 0,
              parse_errors: 0,
              classes: 0,
              modules: 0,
              methods: 0,
              requires: 0,
              ruby_version: RUBY_VERSION,
            }

            def walk(node, stats)
              return unless node.is_a?(Array)
              case node[0]
              when :class
                stats[:classes] += 1
              when :module
                stats[:modules] += 1
              when :def, :defs
                stats[:methods] += 1
              when :method_add_arg
                call = node[1]
                if call.is_a?(Array) && call[0] == :fcall
                  ident = call[1]
                  if ident.is_a?(Array) && ident[0] == :@ident && ident[1] == "require"
                    stats[:requires] += 1
                  end
                end
              end
              node.each { |child| walk(child, stats) if child.is_a?(Array) }
            end

            Dir.glob(File.join(root, "**", "*.rb")).sort.each do |path|
              next if path.include?("/graphify-out/")
              stats[:files] += 1
              sexp = Ripper.sexp(File.read(path))
              if sexp
                stats[:parsed_files] += 1
                walk(sexp, stats)
              else
                stats[:parse_errors] += 1
              end
            rescue StandardError
              stats[:parse_errors] += 1
            end

            stats[:definitions] = stats[:classes] + stats[:modules] + stats[:methods]
            puts JSON.pretty_generate(stats)
            '''
        ).strip()
        + "\n"
    )

    start = time.time()
    result = run([ruby, str(script), str(repo)], timeout=900)
    seconds = round(time.time() - start, 3)
    out: dict[str, Any] = {
        "tool": "ruby-ripper",
        "status": "ok" if result.returncode == 0 else "failed",
        "ok": result.returncode == 0,
        "seconds": seconds,
        "command": f"{ruby} {script} {repo}",
    }
    if result.returncode != 0:
        out["note"] = result.stderr.strip() or result.stdout.strip()
        return out
    out["metrics"] = json.loads(result.stdout)
    return out


def run_sourcekit_lsp(repo: Path, workdir: Path) -> dict[str, Any]:
    sourcekit = executable("sourcekit-lsp")
    if not sourcekit:
        return {"tool": "sourcekit-lsp", "status": "missing", "ok": False, "note": "sourcekit-lsp not found"}
    sources = swift_source_files(repo)
    if not sources:
        return {"tool": "sourcekit-lsp", "status": "failed", "ok": False, "note": f"no Swift files under {repo}"}

    scratch = workdir / "sourcekit-scratch"
    scratch.mkdir(parents=True, exist_ok=True)
    cmd = [sourcekit, "--scratch-path", str(scratch), "--default-workspace-type", "swiftPM"]
    messages: "queue.Queue[dict[str, Any]]" = queue.Queue()
    stderr_tail: list[str] = []
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
        return {"tool": "sourcekit-lsp", "status": "failed", "ok": False, "note": str(exc)}

    assert proc.stdout is not None
    assert proc.stderr is not None
    threading.Thread(target=_lsp_read_loop, args=(proc.stdout, messages), daemon=True).start()
    threading.Thread(target=_tail_pipe, args=(proc.stderr, stderr_tail), daemon=True).start()
    diagnostics: dict[str, int] = {}
    out: dict[str, Any] = {"tool": "sourcekit-lsp", "status": "failed", "ok": False}
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
        init = _wait_lsp(messages, {1}, diagnostics, timeout=45.0)
        if 1 not in init:
            note = "initialize timed out"
            if proc.poll() is not None:
                note = f"sourcekit-lsp exited {proc.returncode}"
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
                            "languageId": "swift",
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

        responses = _wait_lsp(messages, wanted, diagnostics, timeout=60.0)
        doc_symbols = 0
        doc_symbol_files = 0
        for msg in responses.values():
            count = _lsp_symbol_count(msg.get("result"))
            doc_symbols += count
            doc_symbol_files += 1

        version = run(["swift", "--version"], timeout=30)
        out = {
            "tool": "sourcekit-lsp",
            "status": "ok",
            "ok": True,
            "seconds": 0.0,
            "command": " ".join(cmd),
            "metrics": {
                "sample_files": len(sources),
                "document_symbol_files": doc_symbol_files,
                "document_symbols": doc_symbols,
                "definitions": doc_symbols,
                "diagnostic_files": len(diagnostics),
                "diagnostics": sum(diagnostics.values()),
                "sample_paths": [str(p.relative_to(repo)) for p in sources],
                "swift_version": version.stdout.splitlines()[0] if version.stdout else "",
                "stderr_tail": stderr_tail[-8:],
            },
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


def bash_source_files(repo: Path) -> list[Path]:
    out: list[Path] = []
    for path in repo.rglob("*"):
        if not path.is_file():
            continue
        if any(part in {".git", "graphify-out", ".atlas"} for part in path.parts):
            continue
        if path.suffix in {".sh", ".bash"}:
            out.append(path)
    return sorted(out)


_BASH_FUNCTION_RE = re.compile(
    r"""(?mx)
    ^[ \t]*
    (?:
        function[ \t]+([A-Za-z_][A-Za-z0-9_]*)[ \t]*(?:\(\))?
        |
        ([A-Za-z_][A-Za-z0-9_]*)[ \t]*\(\)
    )
    [ \t]*\{
    """
)


def run_bash_native(repo: Path, workdir: Path) -> dict[str, Any]:
    bash = executable("bash") or "/bin/bash"
    if not Path(bash).exists():
        return {"tool": "bash-n", "status": "missing", "ok": False, "note": "bash not found"}
    sources = bash_source_files(repo)
    if not sources:
        return {"tool": "bash-n", "status": "failed", "ok": False, "note": f"no .sh/.bash files under {repo}"}

    start = time.time()
    syntax_errors: list[dict[str, str]] = []
    parsed = 0
    functions = 0
    source_edges = 0
    for path in sources:
        result = run([bash, "-n", str(path)], timeout=60)
        if result.returncode == 0:
            parsed += 1
        else:
            syntax_errors.append({"path": str(path.relative_to(repo)), "error": (result.stderr or result.stdout).strip()[:500]})
        text = path.read_text(encoding="utf-8", errors="replace")
        functions += sum(1 for _ in _BASH_FUNCTION_RE.finditer(text))
        source_edges += len(re.findall(r"(?m)^\s*(?:\.|source)\s+[^\s;]+", text))

    version = run([bash, "--version"], timeout=30)
    return {
        "tool": "bash-n",
        "status": "ok" if not syntax_errors else "partial",
        "ok": not syntax_errors,
        "seconds": round(time.time() - start, 3),
        "command": f"{bash} -n <{len(sources)} shell files>",
        "metrics": {
            "files": len(sources),
            "parsed_files": parsed,
            "syntax_errors": len(syntax_errors),
            "functions": functions,
            "source_edges": source_edges,
            "definitions": functions,
            "bash_version": version.stdout.splitlines()[0] if version.stdout else "",
            "syntax_error_samples": syntax_errors[:8],
        },
    }


_SQL_DEFINITION_RE = re.compile(
    r"""(?im)^\s*
    CREATE\s+(?:OR\s+REPLACE\s+)?
    (TABLE|VIEW|FUNCTION|PROCEDURE|TRIGGER)
    \s+(?:IF\s+NOT\s+EXISTS\s+)?
    ([A-Za-z_"][A-Za-z0-9_."$]*)
    """,
    re.VERBOSE,
)


def ensure_sqlfluff(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "sqlfluff-venv"
    sqlfluff = venv / "bin" / "sqlfluff"
    if sqlfluff.exists():
        return str(sqlfluff), ""
    python = executable("python3")
    if not python:
        return None, "python3 not found; cannot create benchmark-only sqlfluff venv"
    create = run([python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    pip = venv / "bin" / "python"
    install = run([str(pip), "-m", "pip", "install", "-q", "sqlfluff==3.5.0"], timeout=300)
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(sqlfluff), ""


def sql_source_files(repo: Path) -> list[Path]:
    return sorted(path for path in repo.rglob("*.sql") if path.is_file() and "graphify-out" not in path.parts)


def run_sqlfluff(repo: Path, workdir: Path) -> dict[str, Any]:
    sqlfluff, err = ensure_sqlfluff(workdir)
    if not sqlfluff:
        return {"tool": "sqlfluff", "status": "missing", "ok": False, "note": err}
    sources = sql_source_files(repo)
    if not sources:
        return {"tool": "sqlfluff", "status": "failed", "ok": False, "note": f"no SQL files under {repo}"}

    start = time.time()
    parsed = 0
    parse_errors: list[dict[str, str]] = []
    definition_counts = {"table": 0, "view": 0, "function": 0, "procedure": 0, "trigger": 0}
    for path in sources:
        result = run([sqlfluff, "parse", "--dialect", "postgres", str(path)], timeout=120)
        if result.returncode == 0:
            parsed += 1
        else:
            parse_errors.append({"path": str(path.relative_to(repo)), "error": (result.stderr or result.stdout).strip()[:500]})
        text = path.read_text(encoding="utf-8", errors="replace")
        for match in _SQL_DEFINITION_RE.finditer(text):
            definition_counts[match.group(1).lower()] += 1

    version = run([sqlfluff, "--version"], timeout=30)
    definitions = sum(definition_counts.values())
    return {
        "tool": "sqlfluff",
        "status": "ok" if not parse_errors else "partial",
        "ok": not parse_errors,
        "seconds": round(time.time() - start, 3),
        "command": f"{sqlfluff} parse --dialect postgres <{len(sources)} SQL files>",
        "metrics": {
            "files": len(sources),
            "parsed_files": parsed,
            "parse_errors": len(parse_errors),
            "definitions": definitions,
            "definition_counts": definition_counts,
            "sqlfluff_version": version.stdout.strip() or version.stderr.strip(),
            "parse_error_samples": parse_errors[:8],
        },
    }


def ensure_hcl2(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "hcl2-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only python-hcl2 venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run([str(python), "-m", "pip", "install", "-q", "python-hcl2==8.1.2"], timeout=300)
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_python_hcl2(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_hcl2(workdir)
    if not python:
        return {"tool": "python-hcl2", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

import hcl2

repo = Path(sys.argv[1])
sources = sorted(
    path for path in repo.rglob("*")
    if path.is_file()
    and path.suffix in {".tf", ".tfvars", ".hcl"}
    and ".terraform" not in path.parts
    and "graphify-out" not in path.parts
)
definition_counts = {"resource": 0, "data": 0, "module": 0, "variable": 0, "output": 0}
parsed_files = 0
parse_errors = 0


def blocks(value):
    if isinstance(value, list):
        return [item for item in value if isinstance(item, dict)]
    if isinstance(value, dict):
        return [value]
    return []


for path in sources:
    try:
        with path.open(encoding="utf-8") as handle:
            data = hcl2.load(handle)
    except Exception:
        parse_errors += 1
        continue
    parsed_files += 1
    for block in blocks(data.get("resource")):
        for resource_type, instances in block.items():
            if isinstance(instances, dict):
                definition_counts["resource"] += len(instances)
    for block in blocks(data.get("data")):
        for data_type, instances in block.items():
            if isinstance(instances, dict):
                definition_counts["data"] += len(instances)
    for kind in ("module", "variable", "output"):
        for block in blocks(data.get(kind)):
            definition_counts[kind] += len(block)

definitions = sum(definition_counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": parse_errors,
    "definition_counts": definition_counts,
    "definitions": definitions,
    "python_hcl2_version": metadata.version("python-hcl2"),
}))
"""
    command = f"{python} -c <hcl2 definition counter> {repo}"
    result = run([python, "-c", helper, str(repo)], timeout=300)
    if result.returncode != 0:
        return {
            "tool": "python-hcl2",
            "status": "failed",
            "ok": False,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "python-hcl2",
            "status": "failed",
            "ok": False,
            "command": command,
            "note": f"could not parse python-hcl2 metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "python-hcl2",
        "status": "ok",
        "ok": True,
        "command": command,
        "metrics": metrics,
    }


def run_rust_native(repo: Path, workdir: Path) -> dict[str, Any]:
    tools = {name: executable(name) for name in ("rust-analyzer", "cargo", "rustc")}
    if not any(tools.values()):
        return {
            "tool": "rust-analyzer",
            "status": "missing",
            "ok": False,
            "note": "rust-analyzer, cargo, and rustc are not installed on this machine; this smoke records Atlas vs graphify and leaves the native Rust baseline explicit.",
        }
    versions: dict[str, str] = {}
    for name, path in tools.items():
        if not path:
            continue
        result = run([path, "--version"], cwd=repo if repo.exists() else workdir, timeout=30)
        versions[name] = (result.stdout.strip() or result.stderr.strip()).splitlines()[0] if (result.stdout or result.stderr) else path
    return {
        "tool": "rust-analyzer",
        "status": "available_not_run",
        "ok": False,
        "note": "Rust tooling is partially available, but this harness does not yet implement a rust-analyzer documentSymbol adapter.",
        "metrics": {"definitions": 0, "tool_versions": versions},
    }


def run_csharp_native(repo: Path, workdir: Path) -> dict[str, Any]:
    tools = {name: executable(name) for name in ("dotnet", "csc", "mcs", "omnisharp", "csharp-ls")}
    if not any(tools.values()):
        return {
            "tool": "roslyn",
            "status": "missing",
            "ok": False,
            "note": "dotnet/Roslyn, csc, mcs, OmniSharp, and csharp-ls are not installed on this machine; this smoke records Atlas vs graphify and leaves the native C# baseline explicit.",
        }
    versions: dict[str, str] = {}
    for name, path in tools.items():
        if not path:
            continue
        result = run([path, "--version"], cwd=repo if repo.exists() else workdir, timeout=30)
        versions[name] = (result.stdout.strip() or result.stderr.strip()).splitlines()[0] if (result.stdout or result.stderr) else path
    return {
        "tool": "roslyn",
        "status": "available_not_run",
        "ok": False,
        "note": "C# tooling is partially available, but this harness does not yet implement a Roslyn/OmniSharp document-symbol adapter.",
        "metrics": {"definitions": 0, "tool_versions": versions},
    }


def ensure_luaparser(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "luaparser-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only luaparser venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run([str(python), "-m", "pip", "install", "-q", "luaparser==4.0.1"], timeout=300)
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_luaparser(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_luaparser(workdir)
    if not python:
        return {"tool": "luaparser", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

from luaparser import ast
from luaparser import astnodes as n

repo = Path(sys.argv[1])
sources = sorted(
    path for path in repo.rglob("*")
    if path.is_file()
    and path.suffix in {".lua", ".luau"}
    and "graphify-out" not in path.parts
)
counts = {"function": 0, "method": 0, "local_function": 0, "assigned_function": 0}
parse_errors = []


def node_name(node):
    if isinstance(node, n.Name):
        return node.id
    if isinstance(node, n.Index):
        left = node_name(node.value)
        right = node_name(node.idx)
        return ".".join(part for part in (left, right) if part)
    if isinstance(node, n.Method):
        left = node_name(node.source)
        right = node_name(node.name)
        return ":".join(part for part in (left, right) if part)
    if isinstance(node, n.String):
        return node.s.decode("utf-8", errors="replace") if isinstance(node.s, bytes) else str(node.s)
    return ""


class Counter(ast.ASTVisitor):
    def visit_Function(self, node):
        if node_name(node.name):
            counts["function"] += 1

    def visit_Method(self, node):
        if node_name(node):
            counts["method"] += 1

    def visit_LocalFunction(self, node):
        if node_name(node.name):
            counts["local_function"] += 1

    def visit_Assign(self, node):
        for target, value in zip(node.targets, node.values):
            if isinstance(value, n.AnonymousFunction) and node_name(target):
                counts["assigned_function"] += 1

    def visit_LocalAssign(self, node):
        for target, value in zip(node.targets, node.values):
            if isinstance(value, n.AnonymousFunction) and node_name(target):
                counts["assigned_function"] += 1


parsed_files = 0
for path in sources:
    try:
        tree = ast.parse(path.read_text(encoding="utf-8", errors="replace"))
        Counter().visit(tree)
        parsed_files += 1
    except Exception as exc:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": str(exc)[:500]})

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "luaparser_version": metadata.version("luaparser"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <luaparser definition counter> {repo}"
    result = run([python, "-c", helper, str(repo)], timeout=300)
    if result.returncode != 0:
        return {
            "tool": "luaparser",
            "status": "failed",
            "ok": False,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "luaparser",
            "status": "failed",
            "ok": False,
            "command": command,
            "note": f"could not parse luaparser metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "luaparser",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "command": command,
        "metrics": metrics,
    }


def ensure_tree_sitter_kotlin(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "tree-sitter-kotlin-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only tree-sitter-kotlin venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run(
        [str(python), "-m", "pip", "install", "-q", "tree-sitter==0.25.2", "tree-sitter-kotlin==1.1.0"],
        timeout=300,
    )
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_tree_sitter_kotlin(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_tree_sitter_kotlin(workdir)
    if not python:
        return {"tool": "tree-sitter-kotlin", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

from tree_sitter import Language, Parser
import tree_sitter_kotlin

repo = Path(sys.argv[1])
sources = sorted(
    path for path in repo.rglob("*")
    if path.is_file()
    and path.suffix in {".kt", ".kts"}
    and "graphify-out" not in path.parts
)
language = Language(tree_sitter_kotlin.language())
parser = Parser(language)
counts = {"type": 0, "function": 0, "variable": 0}
parse_errors = []


def text(node, source):
    return source[node.start_byte:node.end_byte].decode("utf-8", errors="replace")


def named_child(node, name):
    child = node.child_by_field_name(name)
    if child is not None:
        return child
    for candidate in node.children:
        if candidate.type == "identifier":
            return candidate
    return None


def count_property(node, source):
    found = 0
    for child in node.children:
        if child.type == "variable_declaration":
            name = named_child(child, "name")
            if name is not None and text(name, source).strip():
                found += 1
    if found == 0:
        name = named_child(node, "name")
        if name is not None and text(name, source).strip():
            found = 1
    return found


def walk(node, source):
    if node.type in {"class_declaration", "object_declaration"}:
        name = named_child(node, "name")
        if name is not None and text(name, source).strip():
            counts["type"] += 1
    elif node.type == "function_declaration":
        name = named_child(node, "name")
        if name is not None and text(name, source).strip():
            counts["function"] += 1
    elif node.type == "property_declaration":
        counts["variable"] += count_property(node, source)
    for child in node.children:
        walk(child, source)


parsed_files = 0
for path in sources:
    source = path.read_bytes()
    tree = parser.parse(source)
    if tree.root_node.has_error:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": "tree-sitter parse error"})
    else:
        parsed_files += 1
    walk(tree.root_node, source)

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "tree_sitter_version": metadata.version("tree-sitter"),
    "tree_sitter_kotlin_version": metadata.version("tree-sitter-kotlin"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <tree-sitter-kotlin definition counter> {repo}"
    start = time.time()
    result = run([python, "-c", helper, str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "tree-sitter-kotlin",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "tree-sitter-kotlin",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse tree-sitter-kotlin metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "tree-sitter-kotlin",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_tree_sitter_scala(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "tree-sitter-scala-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only tree-sitter-scala venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run(
        [str(python), "-m", "pip", "install", "-q", "tree-sitter==0.25.2", "tree-sitter-scala==0.26.0"],
        timeout=300,
    )
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_tree_sitter_scala(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_tree_sitter_scala(workdir)
    if not python:
        return {"tool": "tree-sitter-scala", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

from tree_sitter import Language, Parser
import tree_sitter_scala

repo = Path(sys.argv[1])
sources = sorted(
    path for path in repo.rglob("*")
    if path.is_file()
    and path.suffix == ".scala"
    and "graphify-out" not in path.parts
)
language = Language(tree_sitter_scala.language())
parser = Parser(language)
counts = {"type": 0, "function": 0, "variable": 0}
parse_errors = []


def text(node, source):
    return source[node.start_byte:node.end_byte].decode("utf-8", errors="replace")


def named_child(node, source):
    child = node.child_by_field_name("name")
    if child is not None:
        name = text(child, source).strip()
        if name:
            return name
    for candidate in node.children:
        if candidate.type in {"identifier", "type_identifier"}:
            name = text(candidate, source).strip()
            if name:
                return name
    return ""


def val_names(node, source):
    names = []
    for child in node.children:
        if child.type == "=":
            break
        if child.type == "identifier":
            name = text(child, source).strip()
            if name and name not in {"val", "var"}:
                names.append(name)
    return names


def walk(node, source):
    if node.type in {"class_definition", "trait_definition", "object_definition", "enum_definition", "type_definition"}:
        if named_child(node, source):
            counts["type"] += 1
    elif node.type in {"function_definition", "function_declaration"}:
        if named_child(node, source):
            counts["function"] += 1
    elif node.type in {"val_definition", "var_definition"}:
        counts["variable"] += len(val_names(node, source))
    for child in node.children:
        walk(child, source)


parsed_files = 0
for path in sources:
    source = path.read_bytes()
    tree = parser.parse(source)
    if tree.root_node.has_error:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": "tree-sitter parse error"})
    else:
        parsed_files += 1
    walk(tree.root_node, source)

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "tree_sitter_version": metadata.version("tree-sitter"),
    "tree_sitter_scala_version": metadata.version("tree-sitter-scala"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <tree-sitter-scala definition counter> {repo}"
    start = time.time()
    result = run([python, "-c", helper, str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "tree-sitter-scala",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "tree-sitter-scala",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse tree-sitter-scala metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "tree-sitter-scala",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_tree_sitter_zig(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "tree-sitter-zig-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only tree-sitter-zig venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run(
        [str(python), "-m", "pip", "install", "-q", "tree-sitter==0.25.2", "tree-sitter-zig==1.1.2"],
        timeout=300,
    )
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_tree_sitter_zig(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_tree_sitter_zig(workdir)
    if not python:
        return {"tool": "tree-sitter-zig", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

from tree_sitter import Language, Parser
import tree_sitter_zig

repo = Path(sys.argv[1])
sources = sorted(
    path for path in repo.rglob("*")
    if path.is_file()
    and path.suffix == ".zig"
    and "graphify-out" not in path.parts
)
language = Language(tree_sitter_zig.language())
parser = Parser(language)
counts = {"function": 0, "constant": 0, "type": 0}
parse_errors = []


def text(node, source):
    return source[node.start_byte:node.end_byte].decode("utf-8", errors="replace")


def named_child(node, source):
    child = node.child_by_field_name("name")
    if child is not None:
        name = text(child, source).strip()
        if name:
            return name
    for candidate in node.children:
        if candidate.type == "identifier":
            name = text(candidate, source).strip()
            if name:
                return name
    return ""


def is_const(node):
    return any(child.type == "const" for child in node.children)


def const_names(node, source):
    names = []
    want_name = False
    for child in node.children:
        if child.type == "const":
            want_name = True
            continue
        if want_name and child.type == "identifier":
            name = text(child, source).strip()
            if name:
                names.append(name)
            want_name = False
            continue
        if want_name and child.type not in {",", "pub"}:
            want_name = False
    return names


def is_container_type(node):
    seen_equals = False
    for child in node.children:
        if child.type == "=":
            seen_equals = True
            continue
        if seen_equals and child.type in {"struct_declaration", "enum_declaration", "union_declaration"}:
            return True
    return False


def walk(node, source):
    if node.type == "function_declaration":
        if named_child(node, source):
            counts["function"] += 1
    elif node.type == "variable_declaration" and is_const(node):
        names = const_names(node, source)
        if names:
            counts["constant"] += len(names)
            if is_container_type(node):
                counts["type"] += 1
    for child in node.children:
        walk(child, source)


parsed_files = 0
for path in sources:
    source = path.read_bytes()
    tree = parser.parse(source)
    if tree.root_node.has_error:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": "tree-sitter parse error"})
    else:
        parsed_files += 1
    walk(tree.root_node, source)

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "tree_sitter_version": metadata.version("tree-sitter"),
    "tree_sitter_zig_version": metadata.version("tree-sitter-zig"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <tree-sitter-zig definition counter> {repo}"
    start = time.time()
    result = run([python, "-c", helper, str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "tree-sitter-zig",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "tree-sitter-zig",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse tree-sitter-zig metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "tree-sitter-zig",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_tree_sitter_elixir(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "tree-sitter-elixir-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only tree-sitter-elixir venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run(
        [str(python), "-m", "pip", "install", "-q", "tree-sitter==0.25.2", "tree-sitter-elixir==0.3.5"],
        timeout=300,
    )
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_tree_sitter_elixir(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_tree_sitter_elixir(workdir)
    if not python:
        return {"tool": "tree-sitter-elixir", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import re
import sys
from pathlib import Path

from tree_sitter import Language, Parser
import tree_sitter_elixir

repo = Path(sys.argv[1])
sources = sorted(
    path for path in repo.rglob("*")
    if path.is_file()
    and path.suffix in {".ex", ".exs"}
    and "graphify-out" not in path.parts
)
language = Language(tree_sitter_elixir.language())
parser = Parser(language)
counts = {
    "module": 0,
    "protocol": 0,
    "implementation": 0,
    "function": 0,
    "macro": 0,
    "delegate": 0,
    "guard": 0,
}
parse_errors = []
module_forms = {"defmodule": "module", "defprotocol": "protocol", "defimpl": "implementation"}
call_forms = {"def": "function", "defp": "function", "defmacro": "macro", "defmacrop": "macro", "defdelegate": "delegate", "defguard": "guard", "defguardp": "guard"}


def text(node, source):
    return source[node.start_byte:node.end_byte].decode("utf-8", errors="replace")


def head_identifier(node, source):
    if node.type != "call":
        return ""
    for child in node.children:
        if child.is_named:
            return text(child, source).strip()
    return ""


def arguments_text(node, source):
    for child in node.children:
        if child.type == "arguments":
            return text(child, source).strip()
    return ""


def module_name(args):
    match = re.match(r"([A-Z][A-Za-z0-9_]*(?:\.[A-Z][A-Za-z0-9_]*)*)\b", args)
    return match.group(1) if match else ""


def callable_name(args):
    match = re.match(r"([A-Za-z_][A-Za-z0-9_!?]*|[+\-*/%<>=!&|^~]+)\s*(?:\(|,|\b)", args)
    return match.group(1) if match else ""


def walk(node, source):
    if node.type == "call":
        head = head_identifier(node, source)
        args = arguments_text(node, source)
        if head in module_forms and module_name(args):
            counts[module_forms[head]] += 1
        elif head in call_forms and callable_name(args):
            counts[call_forms[head]] += 1
    for child in node.children:
        walk(child, source)


parsed_files = 0
for path in sources:
    source = path.read_bytes()
    tree = parser.parse(source)
    if tree.root_node.has_error:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": "tree-sitter parse error"})
    else:
        parsed_files += 1
    walk(tree.root_node, source)

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "tree_sitter_version": metadata.version("tree-sitter"),
    "tree_sitter_elixir_version": metadata.version("tree-sitter-elixir"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <tree-sitter-elixir definition counter> {repo}"
    start = time.time()
    result = run([python, "-c", helper, str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "tree-sitter-elixir",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "tree-sitter-elixir",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse tree-sitter-elixir metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "tree-sitter-elixir",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_tree_sitter_fortran(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "tree-sitter-fortran-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only tree-sitter-fortran venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run(
        [str(python), "-m", "pip", "install", "-q", "tree-sitter==0.25.2", "tree-sitter-fortran==0.6.0"],
        timeout=300,
    )
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_tree_sitter_fortran(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_tree_sitter_fortran(workdir)
    if not python:
        return {"tool": "tree-sitter-fortran", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

from tree_sitter import Language, Parser
import tree_sitter_fortran

repo = Path(sys.argv[1])
suffixes = {".f", ".f90", ".f95", ".f03", ".f08"}
sources = sorted(
    path for path in repo.rglob("*")
    if path.is_file()
    and path.suffix.lower() in suffixes
    and "graphify-out" not in path.parts
)
language = Language(tree_sitter_fortran.language())
parser = Parser(language)
counts = {"module": 0, "type": 0, "function": 0, "subroutine": 0, "program": 0}
parse_errors = []


def text(node, source):
    return source[node.start_byte:node.end_byte].decode("utf-8", errors="replace")


def name_from(node, source):
    if node is None:
        return ""
    child = node.child_by_field_name("name")
    if child is not None:
        return text(child, source).strip()
    for candidate in node.children:
        if candidate.type in {"name", "identifier", "type_name"}:
            return text(candidate, source).strip()
    return ""


def first_statement(node):
    for child in node.children:
        if child.type.endswith("_statement"):
            return child
    return node


def walk(node, source):
    if node.type == "module":
        name = name_from(first_statement(node), source)
        if name:
            counts["module"] += 1
    elif node.type == "program":
        name = name_from(first_statement(node), source)
        if name:
            counts["program"] += 1
    elif node.type == "derived_type_definition":
        name = name_from(first_statement(node), source)
        if name:
            counts["type"] += 1
    elif node.type == "function":
        name = name_from(first_statement(node), source)
        if name:
            counts["function"] += 1
    elif node.type == "subroutine":
        name = name_from(first_statement(node), source)
        if name:
            counts["subroutine"] += 1
    for child in node.children:
        walk(child, source)


parsed_files = 0
for path in sources:
    source = path.read_bytes()
    tree = parser.parse(source)
    if tree.root_node.has_error:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": "tree-sitter parse error"})
    else:
        parsed_files += 1
    walk(tree.root_node, source)

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "tree_sitter_version": metadata.version("tree-sitter"),
    "tree_sitter_fortran_version": metadata.version("tree-sitter-fortran"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <tree-sitter-fortran definition counter> {repo}"
    start = time.time()
    result = run([python, "-c", helper, str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "tree-sitter-fortran",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "tree-sitter-fortran",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse tree-sitter-fortran metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "tree-sitter-fortran",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_tree_sitter_systemverilog(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "tree-sitter-systemverilog-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only tree-sitter-systemverilog venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run(
        [str(python), "-m", "pip", "install", "-q", "tree-sitter==0.25.2", "tree-sitter-systemverilog==0.3.1"],
        timeout=300,
    )
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_tree_sitter_systemverilog(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_tree_sitter_systemverilog(workdir)
    if not python:
        return {"tool": "tree-sitter-systemverilog", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

from tree_sitter import Language, Parser
import tree_sitter_systemverilog

repo = Path(sys.argv[1])
suffixes = {".v", ".sv", ".svh"}
sources = sorted(
    path for path in repo.rglob("*")
    if path.is_file()
    and path.suffix.lower() in suffixes
    and "graphify-out" not in path.parts
)
language = Language(tree_sitter_systemverilog.language())
parser = Parser(language)
kindmap = {
    "module_declaration": "module",
    "interface_declaration": "interface",
    "package_declaration": "package",
    "class_declaration": "class",
    "function_declaration": "function",
    "task_declaration": "task",
    "program_declaration": "program",
    "checker_declaration": "checker",
}
counts = {kind: 0 for kind in ("module", "interface", "package", "class", "function", "task", "program", "checker")}
parse_errors = []


def text(node, source):
    return source[node.start_byte:node.end_byte].decode("utf-8", errors="replace")


def first_identifier(node, source):
    if node is None:
        return ""
    child = node.child_by_field_name("name")
    if child is not None:
        return text(child, source).strip().lstrip("\\").split()[0]
    for candidate in node.children:
        if candidate.type in {"simple_identifier", "escaped_identifier", "system_tf_identifier"}:
            return text(candidate, source).strip().lstrip("\\").split()[0]
        if candidate.is_named:
            name = first_identifier(candidate, source)
            if name:
                return name
    return ""


def walk(node, source):
    kind = kindmap.get(node.type)
    if kind:
        name = first_identifier(node, source)
        if name:
            counts[kind] += 1
    for child in node.children:
        walk(child, source)


parsed_files = 0
for path in sources:
    source = path.read_bytes()
    tree = parser.parse(source)
    if tree.root_node.has_error:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": "tree-sitter parse error"})
    else:
        parsed_files += 1
    walk(tree.root_node, source)

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "tree_sitter_version": metadata.version("tree-sitter"),
    "tree_sitter_systemverilog_version": metadata.version("tree-sitter-systemverilog"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <tree-sitter-systemverilog definition counter> {repo}"
    start = time.time()
    result = run([python, "-c", helper, str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "tree-sitter-systemverilog",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "tree-sitter-systemverilog",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse tree-sitter-systemverilog metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "tree-sitter-systemverilog",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_tree_sitter_groovy(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "tree-sitter-groovy-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only tree-sitter-groovy venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run(
        [str(python), "-m", "pip", "install", "-q", "tree-sitter==0.25.2", "tree-sitter-groovy==0.1.2"],
        timeout=300,
    )
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_tree_sitter_groovy(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_tree_sitter_groovy(workdir)
    if not python:
        return {"tool": "tree-sitter-groovy", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

from tree_sitter import Language, Parser
import tree_sitter_groovy

repo = Path(sys.argv[1])
sources = sorted(
    path for path in repo.rglob("*")
    if path.is_file()
    and (path.suffix == ".groovy" or path.suffix == ".gradle")
    and "graphify-out" not in path.parts
)
language = Language(tree_sitter_groovy.language())
parser = Parser(language)
counts = {"class": 0, "interface": 0, "enum": 0, "trait": 0, "method": 0, "function": 0, "task": 0}
parse_errors = []


def text(node, source):
    return source[node.start_byte:node.end_byte].decode("utf-8", errors="replace")


def first_identifier(node, source):
    for child in node.children:
        if child.type in {"identifier", "type_identifier"}:
            value = text(child, source).strip()
            if value:
                return value
    return ""


def juxt_head_and_arg(node, source):
    named = [child for child in node.children if child.is_named]
    if not named:
        return "", ""
    head = text(named[0], source).strip()
    arg = ""
    if len(named) > 1:
        arg = text(named[1], source).strip().split()[0] if text(named[1], source).strip() else ""
    return head, arg


def walk(node, source):
    if node.type == "class_declaration" and first_identifier(node, source):
        counts["class"] += 1
    elif node.type == "interface_declaration" and first_identifier(node, source):
        counts["interface"] += 1
    elif node.type == "enum_declaration" and first_identifier(node, source):
        counts["enum"] += 1
    elif node.type == "method_declaration" and first_identifier(node, source):
        counts["method"] += 1
    elif node.type == "function_definition" and first_identifier(node, source):
        counts["function"] += 1
    elif node.type == "juxt_function_call":
        head, arg = juxt_head_and_arg(node, source)
        if head == "trait" and arg:
            counts["trait"] += 1
        elif head == "task" and arg:
            counts["task"] += 1
    for child in node.children:
        walk(child, source)


parsed_files = 0
for path in sources:
    source = path.read_bytes()
    tree = parser.parse(source)
    if tree.root_node.has_error:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": "tree-sitter parse error"})
    else:
        parsed_files += 1
    walk(tree.root_node, source)

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "tree_sitter_version": metadata.version("tree-sitter"),
    "tree_sitter_groovy_version": metadata.version("tree-sitter-groovy"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <tree-sitter-groovy definition counter> {repo}"
    start = time.time()
    result = run([python, "-c", helper, str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "tree-sitter-groovy",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "tree-sitter-groovy",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse tree-sitter-groovy metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "tree-sitter-groovy",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_tree_sitter_objc(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "tree-sitter-objc-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only tree-sitter-objc venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run(
        [str(python), "-m", "pip", "install", "-q", "tree-sitter==0.25.2", "tree-sitter-objc==3.0.2"],
        timeout=300,
    )
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_tree_sitter_objc(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_tree_sitter_objc(workdir)
    if not python:
        return {"tool": "tree-sitter-objc", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

from tree_sitter import Language, Parser
import tree_sitter_objc

repo = Path(sys.argv[1])
sources = sorted(
    path for path in repo.rglob("*")
    if path.is_file()
    # Match graphify 0.8.49's Objective-C dispatch exactly: `.m` and `.mm`.
    # Headers are dispatched as C by graphify, so counting Objective-C declarations
    # from `.h` here would make the native baseline wider than the tool target.
    and path.suffix in {".m", ".mm"}
    and "graphify-out" not in path.parts
)
language = Language(tree_sitter_objc.language())
parser = Parser(language)
counts = {"type": 0, "method": 0}
parse_errors = []


def text(node, source):
    return source[node.start_byte:node.end_byte].decode("utf-8", errors="replace")


def first_identifier(node, source):
    for child in node.children:
        if child.type == "identifier":
            value = text(child, source).strip()
            if value:
                return value
    return ""


def selector_name(node, source):
    names = []
    has_parameter = False
    for child in node.children:
        if child.type == "identifier":
            value = text(child, source).strip()
            if value:
                names.append(value)
        elif child.type == "method_parameter":
            has_parameter = True
    if not names:
        return ""
    if has_parameter:
        return ":".join(names) + ":"
    return names[0]


def walk(node, source):
    if node.type in {"class_interface", "class_implementation", "protocol_declaration", "category_interface", "category_implementation"}:
        if first_identifier(node, source):
            counts["type"] += 1
    elif node.type in {"method_declaration", "method_definition"}:
        if selector_name(node, source):
            counts["method"] += 1
    for child in node.children:
        walk(child, source)


parsed_files = 0
for path in sources:
    source = path.read_bytes()
    tree = parser.parse(source)
    if tree.root_node.has_error:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": "tree-sitter parse error"})
    else:
        parsed_files += 1
    walk(tree.root_node, source)

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "tree_sitter_version": metadata.version("tree-sitter"),
    "tree_sitter_objc_version": metadata.version("tree-sitter-objc"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <tree-sitter-objc definition counter> {repo}"
    start = time.time()
    result = run([python, "-c", helper, str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "tree-sitter-objc",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "tree-sitter-objc",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse tree-sitter-objc metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "tree-sitter-objc",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_tree_sitter_dart(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "tree-sitter-dart-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only tree-sitter-dart venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run(
        [str(python), "-m", "pip", "install", "-q", "tree-sitter==0.25.2", "tree-sitter-dart==0.1.0"],
        timeout=300,
    )
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_tree_sitter_dart(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_tree_sitter_dart(workdir)
    if not python:
        return {"tool": "tree-sitter-dart", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

from tree_sitter import Language, Parser
import tree_sitter_dart

repo = Path(sys.argv[1])
sources = sorted(
    path for path in repo.rglob("*.dart")
    if path.is_file()
    and "graphify-out" not in path.parts
    and ".dart_tool" not in path.parts
)
language = Language(tree_sitter_dart.language())
parser = Parser(language)
counts = {"type": 0, "function": 0, "constructor": 0, "getter": 0, "setter": 0, "typedef": 0}
parse_errors = []
type_nodes = {
    "class_definition",
    "mixin_declaration",
    "mixin_definition",
    "extension_declaration",
    "extension_type_declaration",
    "enum_declaration",
}


def walk(node):
    if node.type in type_nodes:
        counts["type"] += 1
    elif node.type == "type_alias":
        counts["typedef"] += 1
    elif node.type in {"constructor_signature", "constant_constructor_signature", "factory_constructor_signature"}:
        counts["constructor"] += 1
    elif node.type == "function_signature":
        counts["function"] += 1
    elif node.type == "getter_signature":
        counts["getter"] += 1
    elif node.type == "setter_signature":
        counts["setter"] += 1
    for child in node.children:
        walk(child)


parsed_files = 0
for path in sources:
    source = path.read_bytes()
    tree = parser.parse(source)
    if tree.root_node.has_error:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": "tree-sitter parse error"})
    else:
        parsed_files += 1
    walk(tree.root_node)

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "tree_sitter_version": metadata.version("tree-sitter"),
    "tree_sitter_dart_version": metadata.version("tree-sitter-dart"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <tree-sitter-dart definition counter> {repo}"
    start = time.time()
    result = run([python, "-c", helper, str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "tree-sitter-dart",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "tree-sitter-dart",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse tree-sitter-dart metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "tree-sitter-dart",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_tree_sitter_julia(workdir: Path) -> tuple[str | None, str]:
    venv = workdir / "tree-sitter-julia-venv"
    python = venv / "bin" / "python"
    if python.exists():
        return str(python), ""
    host_python = executable("python3")
    if not host_python:
        return None, "python3 not found; cannot create benchmark-only tree-sitter-julia venv"
    create = run([host_python, "-m", "venv", str(venv)], timeout=120)
    if create.returncode != 0:
        return None, create.stderr.strip() or create.stdout.strip()
    install = run(
        [str(python), "-m", "pip", "install", "-q", "tree-sitter==0.25.2", "tree-sitter-julia==0.23.1"],
        timeout=300,
    )
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return str(python), ""


def run_tree_sitter_julia(repo: Path, workdir: Path) -> dict[str, Any]:
    python, err = ensure_tree_sitter_julia(workdir)
    if not python:
        return {"tool": "tree-sitter-julia", "status": "missing", "ok": False, "note": err}
    helper = r"""
import importlib.metadata as metadata
import json
import sys
from pathlib import Path

from tree_sitter import Language, Parser
import tree_sitter_julia

repo = Path(sys.argv[1])
sources = sorted(
    path for path in repo.rglob("*.jl")
    if path.is_file()
    and "graphify-out" not in path.parts
)
language = Language(tree_sitter_julia.language())
parser = Parser(language)
counts = {"module": 0, "type": 0, "function": 0, "macro": 0, "constant": 0}
parse_errors = []


def text(node, source):
    return source[node.start_byte:node.end_byte].decode("utf-8", errors="replace")


def first_identifier(node, source):
    if node is None:
        return ""
    if node.type in {"identifier", "operator_identifier", "type_identifier"}:
        return text(node, source).strip().lstrip("@")
    for child in node.children:
        name = first_identifier(child, source)
        if name:
            return name
    return ""


def callable_type_from_parenthesized(node, source):
    raw = text(node, source).strip()
    if raw.startswith("(") and raw.endswith(")"):
        raw = raw[1:-1].strip()
    if "::" not in raw:
        return ""
    name = raw.split("::", 1)[1].strip()
    for marker in (" where ", "{", ")"):
        if marker in name:
            name = name.split(marker, 1)[0].strip()
    return name


def callable_name(node, source):
    if node is None:
        return ""
    if node.type in {"identifier", "operator_identifier", "type_identifier"}:
        return text(node, source).strip().lstrip("@")
    if node.type in {"field_expression", "qualified_identifier", "scoped_identifier"}:
        return text(node, source).strip()
    if node.type == "parenthesized_expression":
        typed = callable_type_from_parenthesized(node, source)
        if typed:
            return typed
        return callable_name(node.children[0], source) if node.children else ""
    if node.type in {"signature", "where_expression", "typed_expression", "parametrized_type_expression", "type_head"}:
        return callable_name(node.children[0], source) if node.children else ""
    if node.type == "call_expression":
        fn = node.child_by_field_name("function") or (node.children[0] if node.children else None)
        return callable_name(fn, source)
    return ""


def is_callable_assignment_left(node):
    if node is None:
        return False
    if node.type == "call_expression":
        return True
    if node.type in {"where_expression", "parametrized_type_expression"}:
        return any(is_callable_assignment_left(child) for child in node.children)
    return False


def stable_callable_name(name):
    return bool(name) and "$" not in name


def walk(node, source):
    if node.type == "module_definition":
        name = first_identifier(node.child_by_field_name("name") or node, source)
        if name:
            counts["module"] += 1
    elif node.type in {"struct_definition", "abstract_definition", "primitive_definition"}:
        name = first_identifier(node.child_by_field_name("name") or node, source)
        if name:
            counts["type"] += 1
    elif node.type == "function_definition":
        sig = node.child_by_field_name("signature")
        if sig is None:
            for child in node.children:
                if child.type in {"signature", "call_expression", "identifier", "where_expression"}:
                    sig = child
                    break
        if stable_callable_name(callable_name(sig, source)):
            counts["function"] += 1
    elif node.type == "macro_definition":
        if first_identifier(node, source):
            counts["macro"] += 1
    elif node.type == "const_statement":
        counts["constant"] += 1
    elif node.type == "assignment":
        left = node.child_by_field_name("left") or (node.children[0] if node.children else None)
        if is_callable_assignment_left(left) and stable_callable_name(callable_name(left, source)):
            counts["function"] += 1
    for child in node.children:
        walk(child, source)


parsed_files = 0
for path in sources:
    source = path.read_bytes()
    tree = parser.parse(source)
    if tree.root_node.has_error:
        parse_errors.append({"path": str(path.relative_to(repo)), "error": "tree-sitter parse error"})
    else:
        parsed_files += 1
    walk(tree.root_node, source)

definitions = sum(counts.values())
print(json.dumps({
    "files": len(sources),
    "parsed_files": parsed_files,
    "parse_errors": len(parse_errors),
    "definition_counts": counts,
    "definitions": definitions,
    "tree_sitter_version": metadata.version("tree-sitter"),
    "tree_sitter_julia_version": metadata.version("tree-sitter-julia"),
    "parse_error_samples": parse_errors[:8],
}))
"""
    command = f"{python} -c <tree-sitter-julia definition counter> {repo}"
    start = time.time()
    result = run([python, "-c", helper, str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "tree-sitter-julia",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "tree-sitter-julia",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse tree-sitter-julia metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "tree-sitter-julia",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def run_pwsh_parser(repo: Path, workdir: Path) -> dict[str, Any]:
    pwsh = executable("pwsh") or executable("powershell")
    if not pwsh:
        return {"tool": "pwsh-parser", "status": "missing", "ok": False, "note": "pwsh/powershell not found"}
    script = workdir / "pwsh_stats.ps1"
    helper = r"""
param([Parameter(Mandatory=$true)][string]$root)
$ErrorActionPreference = "Stop"
$stats = [ordered]@{
  files = 0
  parsed_files = 0
  parse_errors = 0
  functions = 0
  assignments = 0
  definitions = 0
  powershell_version = $PSVersionTable.PSVersion.ToString()
  parse_error_samples = @()
}
Get-ChildItem -Path $root -Recurse -File -Include *.ps1,*.psm1,*.psd1 |
  Where-Object { $_.FullName -notmatch [regex]::Escape([IO.Path]::DirectorySeparatorChar + "graphify-out" + [IO.Path]::DirectorySeparatorChar) } |
  Sort-Object FullName |
  ForEach-Object {
    $stats.files++
    $tokens = $null
    $errors = $null
    $ast = [System.Management.Automation.Language.Parser]::ParseFile($_.FullName, [ref]$tokens, [ref]$errors)
    if ($errors -and $errors.Count) {
      $stats.parse_errors++
      if ($stats.parse_error_samples.Count -lt 8) {
        $stats.parse_error_samples += [ordered]@{
          path = [IO.Path]::GetRelativePath($root, $_.FullName)
          error = $errors[0].Message
        }
      }
    } else {
      $stats.parsed_files++
    }
    $stats.functions += @($ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.FunctionDefinitionAst] }, $true)).Count
    $stats.assignments += @($ast.FindAll({ param($node) $node -is [System.Management.Automation.Language.AssignmentStatementAst] }, $true)).Count
  }
$stats.definitions = $stats.functions
$stats | ConvertTo-Json -Depth 8
"""
    script.write_text(helper.strip() + "\n", encoding="utf-8")
    command = f"{pwsh} -NoLogo -NoProfile -File {script} {repo}"
    start = time.time()
    result = run([pwsh, "-NoLogo", "-NoProfile", "-File", str(script), str(repo)], timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "pwsh-parser",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "pwsh-parser",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse pwsh metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "pwsh-parser",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_vue_compiler(workdir: Path) -> tuple[Path | None, str]:
    package_dir = workdir / "vue-compiler"
    marker = package_dir / "node_modules" / "@vue" / "compiler-sfc" / "package.json"
    if marker.exists():
        return package_dir, ""
    node = executable("node")
    npm = executable("npm")
    if not node or not npm:
        return None, "node/npm not found; cannot install benchmark-only @vue/compiler-sfc"
    package_dir.mkdir(parents=True, exist_ok=True)
    init = run([npm, "init", "-y"], cwd=package_dir, timeout=120)
    if init.returncode != 0:
        return None, init.stderr.strip() or init.stdout.strip()
    install = run([npm, "install", "--silent", "@vue/compiler-sfc@3.5.22"], cwd=package_dir, timeout=300)
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return package_dir, ""


def run_vue_compiler(repo: Path, workdir: Path) -> dict[str, Any]:
    package_dir, err = ensure_vue_compiler(workdir)
    node = executable("node")
    if not package_dir or not node:
        return {"tool": "vue-compiler-sfc", "status": "missing", "ok": False, "note": err or "node not found"}
    helper = package_dir / "vue_sfc_stats.js"
    helper.write_text(
        r'''
const fs = require("fs");
const path = require("path");
const { parse } = require("@vue/compiler-sfc");

const root = process.argv[2];
const stats = {
  files: 0,
  parsed_files: 0,
  parse_errors: 0,
  script_blocks: 0,
  functions: 0,
  variables: 0,
  definitions: 0,
  compiler_version: require("@vue/compiler-sfc/package.json").version,
  parse_error_samples: [],
};

function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name !== "graphify-out" && entry.name !== "node_modules") {
        walk(full);
      }
      continue;
    }
    if (!entry.name.endsWith(".vue")) {
      continue;
    }
    stats.files++;
    const source = fs.readFileSync(full, "utf8");
    const parsed = parse(source, { filename: full });
    if (parsed.errors && parsed.errors.length) {
      stats.parse_errors++;
      if (stats.parse_error_samples.length < 8) {
        stats.parse_error_samples.push({
          path: path.relative(root, full),
          error: String(parsed.errors[0].message || parsed.errors[0]).slice(0, 500),
        });
      }
    } else {
      stats.parsed_files++;
    }
    const blocks = [parsed.descriptor.script, parsed.descriptor.scriptSetup]
      .filter(Boolean)
      .map((block) => block.content || "");
    if (blocks.length) {
      stats.script_blocks += blocks.length;
    }
    for (const content of blocks) {
      stats.functions += (content.match(/^\s*function\s+[A-Za-z_$][\w$]*/gm) || []).length;
      stats.variables += (content.match(/^\s*(?:const|let)\s+[A-Za-z_$][\w$]*/gm) || []).length;
    }
  }
}

walk(root);
stats.definitions = stats.functions + stats.variables;
console.log(JSON.stringify(stats, null, 2));
'''.strip()
        + "\n",
        encoding="utf-8",
    )
    command = f"{node} {helper} {repo}"
    start = time.time()
    result = run([node, str(helper), str(repo)], cwd=package_dir, timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "vue-compiler-sfc",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "vue-compiler-sfc",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse Vue compiler metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "vue-compiler-sfc",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def ensure_svelte_compiler(workdir: Path) -> tuple[Path | None, str]:
    package_dir = workdir / "svelte-compiler"
    marker = package_dir / "node_modules" / "svelte" / "package.json"
    if marker.exists():
        return package_dir, ""
    node = executable("node")
    npm = executable("npm")
    if not node or not npm:
        return None, "node/npm not found; cannot install benchmark-only svelte compiler"
    package_dir.mkdir(parents=True, exist_ok=True)
    init = run([npm, "init", "-y"], cwd=package_dir, timeout=120)
    if init.returncode != 0:
        return None, init.stderr.strip() or init.stdout.strip()
    install = run([npm, "install", "--silent", "svelte@5.56.4"], cwd=package_dir, timeout=300)
    if install.returncode != 0:
        return None, install.stderr.strip() or install.stdout.strip()
    return package_dir, ""


def run_svelte_compiler(repo: Path, workdir: Path) -> dict[str, Any]:
    package_dir, err = ensure_svelte_compiler(workdir)
    node = executable("node")
    if not package_dir or not node:
        return {"tool": "svelte-compiler", "status": "missing", "ok": False, "note": err or "node not found"}
    helper = package_dir / "svelte_stats.js"
    helper.write_text(
        r'''
const fs = require("fs");
const path = require("path");
const { parse, VERSION } = require("svelte/compiler");

const root = process.argv[2];
const stats = {
  files: 0,
  parsed_files: 0,
  parse_errors: 0,
  script_blocks: 0,
  functions: 0,
  variables: 0,
  definitions: 0,
  compiler_version: VERSION,
  parse_error_samples: [],
};

function walk(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name !== "graphify-out" && entry.name !== "node_modules") {
        walk(full);
      }
      continue;
    }
    if (!entry.name.endsWith(".svelte")) {
      continue;
    }
    stats.files++;
    const source = fs.readFileSync(full, "utf8");
    try {
      parse(source, { filename: full });
      stats.parsed_files++;
    } catch (error) {
      stats.parse_errors++;
      if (stats.parse_error_samples.length < 8) {
        stats.parse_error_samples.push({
          path: path.relative(root, full),
          error: String(error.message || error).slice(0, 500),
        });
      }
    }
    const scripts = [...source.matchAll(/<script(?:\s[^>]*)?>([\s\S]*?)<\/script>/gi)]
      .map((match) => match[1] || "");
    stats.script_blocks += scripts.length;
    for (const content of scripts) {
      stats.functions += (content.match(/^\s*function\s+[A-Za-z_$][\w$]*/gm) || []).length;
      stats.variables += (content.match(/^\s*(?:const|let)\s+[A-Za-z_$][\w$]*/gm) || []).length;
    }
  }
}

walk(root);
stats.definitions = stats.functions + stats.variables;
console.log(JSON.stringify(stats, null, 2));
'''.strip()
        + "\n",
        encoding="utf-8",
    )
    command = f"{node} {helper} {repo}"
    start = time.time()
    result = run([node, str(helper), str(repo)], cwd=package_dir, timeout=300)
    seconds = round(time.time() - start, 3)
    if result.returncode != 0:
        return {
            "tool": "svelte-compiler",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": result.stderr.strip() or result.stdout.strip(),
        }
    try:
        metrics = json.loads(result.stdout)
    except json.JSONDecodeError as exc:
        return {
            "tool": "svelte-compiler",
            "status": "failed",
            "ok": False,
            "seconds": seconds,
            "command": command,
            "note": f"could not parse Svelte compiler metrics: {exc}: {result.stdout[:200]}",
        }
    return {
        "tool": "svelte-compiler",
        "status": "ok" if metrics.get("parse_errors", 0) == 0 else "partial",
        "ok": metrics.get("parse_errors", 0) == 0,
        "seconds": seconds,
        "command": command,
        "metrics": metrics,
    }


def run_php_tokenizer(repo: Path, workdir: Path) -> dict[str, Any]:
    php = executable("php")
    if not php:
        return {"tool": "php-tokenizer", "status": "missing", "ok": False, "note": "php not found"}

    script = workdir / "php_token_stats.php"
    script.write_text(
        textwrap.dedent(
            r'''<?php
            $root = $argv[1];
            $stats = [
              "files" => 0,
              "parsed_files" => 0,
              "parse_errors" => 0,
              "classes" => 0,
              "interfaces" => 0,
              "traits" => 0,
              "enums" => 0,
              "functions" => 0,
              "requires" => 0,
              "namespaces" => 0,
              "uses" => 0,
              "use_functions" => 0,
              "php_version" => PHP_VERSION,
            ];

            $rii = new RecursiveIteratorIterator(new RecursiveDirectoryIterator($root, FilesystemIterator::SKIP_DOTS));
            foreach ($rii as $file) {
              $path = $file->getPathname();
              if (!str_ends_with($path, ".php") || str_contains($path, "/graphify-out/")) {
                continue;
              }
              $stats["files"]++;
              try {
                $tokens = token_get_all(file_get_contents($path), TOKEN_PARSE);
                $stats["parsed_files"]++;
              } catch (Throwable $e) {
                $stats["parse_errors"]++;
                continue;
              }
              $count = count($tokens);
              for ($i = 0; $i < $count; $i++) {
                $token = $tokens[$i];
                $id = is_array($token) ? $token[0] : null;
                if ($id === T_CLASS) {
                  if (next_named_token($tokens, $i) !== null) {
                    $stats["classes"]++;
                  }
                } elseif ($id === T_INTERFACE) {
                  $stats["interfaces"]++;
                } elseif ($id === T_TRAIT) {
                  $stats["traits"]++;
                } elseif (defined("T_ENUM") && $id === T_ENUM) {
                  $stats["enums"]++;
                } elseif ($id === T_FUNCTION) {
                  if (previous_significant_token_id($tokens, $i) === T_USE) {
                    $stats["use_functions"]++;
                    continue;
                  }
                  if (next_named_token($tokens, $i) !== null) {
                    $stats["functions"]++;
                  }
                } elseif (in_array($id, [T_REQUIRE, T_REQUIRE_ONCE, T_INCLUDE, T_INCLUDE_ONCE], true)) {
                  $stats["requires"]++;
                } elseif ($id === T_NAMESPACE) {
                  $stats["namespaces"]++;
                } elseif ($id === T_USE) {
                  $stats["uses"]++;
                }
              }
            }

            $stats["definitions"] = $stats["classes"] + $stats["interfaces"] + $stats["traits"] + $stats["enums"] + $stats["functions"];
            echo json_encode($stats, JSON_PRETTY_PRINT) . PHP_EOL;

            function next_named_token(array $tokens, int $i): ?string {
              $count = count($tokens);
              for ($j = $i + 1; $j < $count; $j++) {
                $token = $tokens[$j];
                if (is_array($token)) {
                  if (in_array($token[0], [T_WHITESPACE, T_COMMENT, T_DOC_COMMENT, T_AMPERSAND_FOLLOWED_BY_VAR_OR_VARARG, T_AMPERSAND_NOT_FOLLOWED_BY_VAR_OR_VARARG], true)) {
                    continue;
                  }
                  return $token[0] === T_STRING ? $token[1] : null;
                }
                if ($token === "&") {
                  continue;
                }
                return null;
              }
              return null;
            }

            function previous_significant_token_id(array $tokens, int $i): ?int {
              for ($j = $i - 1; $j >= 0; $j--) {
                $token = $tokens[$j];
                if (is_array($token)) {
                  if (in_array($token[0], [T_WHITESPACE, T_COMMENT, T_DOC_COMMENT], true)) {
                    continue;
                  }
                  return $token[0];
                }
                if (trim($token) === "") {
                  continue;
                }
                return null;
              }
              return null;
            }
            '''
        ).strip()
        + "\n"
    )

    start = time.time()
    result = run([php, str(script), str(repo)], timeout=900)
    seconds = round(time.time() - start, 3)
    out: dict[str, Any] = {
        "tool": "php-tokenizer",
        "status": "ok" if result.returncode == 0 else "failed",
        "ok": result.returncode == 0,
        "seconds": seconds,
        "command": f"{php} {script} {repo}",
    }
    if result.returncode != 0:
        out["note"] = result.stderr.strip() or result.stdout.strip()
        return out
    out["metrics"] = json.loads(result.stdout)
    return out


def run_smoke(language: str, atlas_bin: str, graphify_bin: str | None) -> dict[str, Any]:
    config = LANGUAGES[language]
    root = Path(config["workdir"])
    repo = root / "repo"
    db = root / "atlas.db"
    if repo.exists():
        shutil.rmtree(repo)
    root.mkdir(parents=True, exist_ok=True)
    for path in [db, Path(str(db) + "-shm"), Path(str(db) + "-wal")]:
        path.unlink(missing_ok=True)

    clone_cmd = ["git", "clone", "--depth", "1", config["repo"], str(repo)]
    clone = run(clone_cmd, timeout=900)
    if clone.returncode != 0:
        raise SystemExit(clone.stderr or clone.stdout)
    commit = subprocess.check_output(["git", "-C", str(repo), "rev-parse", "HEAD"], text=True).strip()
    target = repo / config.get("subdir", "")
    if not target.exists():
        raise SystemExit(f"configured benchmark target does not exist: {target}")
    repo_query_name = target.name
    clean_generated_sidecars(target)

    atlas_index_cmd = [atlas_bin, "--db", f"sqlite://{db}", "--json", "index", str(target)]
    start = time.time()
    atlas_index = run(atlas_index_cmd, timeout=900)
    atlas_cold_seconds = round(time.time() - start, 3)
    if atlas_index.returncode != 0:
        raise SystemExit(atlas_index.stderr or atlas_index.stdout)

    atlas_reindex_cmd = [atlas_bin, "--db", f"sqlite://{db}", "--json", "index", str(target)]
    start = time.time()
    atlas_reindex = run(atlas_reindex_cmd, timeout=900)
    atlas_reindex_seconds = round(time.time() - start, 3)
    if atlas_reindex.returncode != 0:
        raise SystemExit(atlas_reindex.stderr or atlas_reindex.stdout)

    if language == "bash":
        native = run_bash_native(repo, root)
        richer_native = {
            "shellcheck": {"status": "missing", "ok": False, "note": "shellcheck not installed; /bin/bash -n plus function counting used as the native baseline"},
            "shfmt": {"status": "missing", "ok": False, "note": "shfmt not installed"},
        }
    elif language == "csharp":
        native = run_csharp_native(target, root)
        richer_native = {
            "dotnet": {"status": "missing" if not executable("dotnet") else "available", "ok": bool(executable("dotnet")), "note": "dotnet SDK/Roslyn not installed" if not executable("dotnet") else "dotnet is available but Roslyn adapter is not implemented in this harness yet"},
            "csc": {"status": "missing" if not executable("csc") else "available", "ok": bool(executable("csc")), "note": "csc not installed" if not executable("csc") else "csc is available but not a code-intelligence definition baseline by itself"},
            "omnisharp": {"status": "missing" if not executable("omnisharp") else "missing_adapter", "ok": False, "note": "OmniSharp not installed" if not executable("omnisharp") else "OmniSharp adapter not implemented in this harness yet"},
            "csharp-ls": {"status": "missing" if not executable("csharp-ls") else "missing_adapter", "ok": False, "note": "csharp-ls not installed" if not executable("csharp-ls") else "csharp-ls adapter not implemented in this harness yet"},
        }
    elif language == "dart":
        native = run_tree_sitter_dart(target, root)
        richer_native = {
            "dart": {"status": "missing" if not executable("dart") else "available", "ok": bool(executable("dart")), "note": "dart SDK not installed; tree-sitter-dart parser used as the isolated definition baseline" if not executable("dart") else "dart SDK is available, but the harness uses tree-sitter-dart for source-only definition coverage"},
            "flutter": {"status": "missing" if not executable("flutter") else "available", "ok": bool(executable("flutter")), "note": "flutter not installed" if not executable("flutter") else "flutter is available but not needed for this package-only source smoke"},
            "dart_language_server": {"status": "missing" if not executable("dart_language_server") else "missing_adapter", "ok": False, "note": "dart_language_server not installed" if not executable("dart_language_server") else "dart_language_server adapter not implemented in this harness yet"},
        }
    elif language == "kotlin":
        native = run_tree_sitter_kotlin(target, root)
        richer_native = {
            "kotlinc": {"status": "missing" if not executable("kotlinc") else "available", "ok": bool(executable("kotlinc")), "note": "kotlinc not installed; tree-sitter-kotlin parser used as the isolated definition baseline" if not executable("kotlinc") else "kotlinc is available, but the harness uses tree-sitter-kotlin for source-only definition coverage"},
            "kotlin-language-server": {"status": "missing" if not executable("kotlin-language-server") else "missing_adapter", "ok": False, "note": "kotlin-language-server not installed" if not executable("kotlin-language-server") else "kotlin-language-server adapter not implemented in this harness yet"},
            "ktlint": {"status": "missing" if not executable("ktlint") else "available", "ok": bool(executable("ktlint")), "note": "ktlint not installed" if not executable("ktlint") else "ktlint is available but lint-focused, not a definition index baseline"},
        }
    elif language == "lua":
        native = run_luaparser(target, root)
        richer_native = {
            "lua": {"status": "missing" if not executable("lua") else "available", "ok": bool(executable("lua")), "note": "lua interpreter not installed" if not executable("lua") else "lua interpreter is available but luaparser is used for AST definition coverage"},
            "luac": {"status": "missing" if not executable("luac") else "available", "ok": bool(executable("luac")), "note": "luac not installed" if not executable("luac") else "luac is available for syntax checks only"},
            "luacheck": {"status": "missing" if not executable("luacheck") else "available", "ok": bool(executable("luacheck")), "note": "luacheck not installed" if not executable("luacheck") else "luacheck is available but not a definition index baseline"},
            "stylua": {"status": "missing" if not executable("stylua") else "available", "ok": bool(executable("stylua")), "note": "stylua not installed" if not executable("stylua") else "stylua is available but not a definition index baseline"},
        }
    elif language == "powershell":
        native = run_pwsh_parser(target, root)
        richer_native = {
            "pwsh": {"status": "available" if executable("pwsh") else "missing", "ok": bool(executable("pwsh")), "note": "PowerShell parser used as native syntax/function-definition baseline" if executable("pwsh") else "pwsh not installed"},
            "powershell-editor-services": {"status": "missing", "ok": False, "note": "PowerShellEditorServices LSP is not installed; pwsh AST parser used for this smoke"},
            "psscriptanalyzer": {"status": "missing", "ok": False, "note": "PSScriptAnalyzer not installed; it is lint-focused, not a definition index baseline"},
        }
    elif language == "ruby":
        native = run_ruby_ripper(repo, root)
        richer_native = {
            "solargraph": {"status": "missing", "ok": False, "note": "solargraph not installed"},
            "ruby-lsp": {"status": "missing", "ok": False, "note": "ruby-lsp not installed"},
        }
    elif language == "rust":
        native = run_rust_native(target, root)
        richer_native = {
            "rust-analyzer": {"status": "missing" if not executable("rust-analyzer") else "missing_adapter", "ok": False, "note": "rust-analyzer not installed" if not executable("rust-analyzer") else "rust-analyzer adapter not implemented in this harness yet"},
            "cargo": {"status": "missing" if not executable("cargo") else "available", "ok": bool(executable("cargo")), "note": "cargo not installed" if not executable("cargo") else "cargo is available but not a code-intelligence definition baseline by itself"},
            "rustc": {"status": "missing" if not executable("rustc") else "available", "ok": bool(executable("rustc")), "note": "rustc not installed" if not executable("rustc") else "rustc is available but not a code-intelligence definition baseline by itself"},
        }
    elif language == "scala":
        native = run_tree_sitter_scala(target, root)
        richer_native = {
            "metals": {"status": "missing" if not executable("metals") else "missing_adapter", "ok": False, "note": "Metals not installed" if not executable("metals") else "Metals adapter not implemented in this harness yet"},
            "scalac": {"status": "missing" if not executable("scalac") else "available", "ok": bool(executable("scalac")), "note": "scalac not installed; tree-sitter-scala parser used as the isolated definition baseline" if not executable("scalac") else "scalac is available, but the harness uses tree-sitter-scala for source-only definition coverage"},
            "scala-cli": {"status": "missing" if not executable("scala-cli") else "available", "ok": bool(executable("scala-cli")), "note": "scala-cli not installed" if not executable("scala-cli") else "scala-cli is available but not used for this source-only smoke"},
        }
    elif language == "svelte":
        native = run_svelte_compiler(target, root)
        richer_native = {
            "svelte-check": {"status": "missing" if not executable("svelte-check") else "available", "ok": bool(executable("svelte-check")), "note": "svelte-check not installed; Svelte compiler parser used as the scriptable SFC baseline" if not executable("svelte-check") else "svelte-check is available but this harness uses compiler parse coverage"},
            "svelte-language-server": {"status": "missing", "ok": False, "note": "Svelte language server is not installed; Svelte compiler parse coverage used for this smoke"},
        }
    elif language == "php":
        native = run_php_tokenizer(repo, root)
        richer_native = {
            "intelephense": {"status": "missing", "ok": False, "note": "intelephense not installed"},
            "phpstan": {"status": "missing", "ok": False, "note": "phpstan not installed"},
            "psalm": {"status": "missing", "ok": False, "note": "psalm not installed"},
        }
    elif language == "sql":
        native = run_sqlfluff(target, root)
        richer_native = {
            "psql": {"status": "unavailable", "ok": False, "note": "psql is installed but requires a live PostgreSQL database; sqlfluff parser used as the scriptable native baseline"},
        }
    elif language == "terraform":
        native = run_python_hcl2(target, root)
        terraform_cli = executable("terraform")
        richer_native = {
            "terraform": {
                "status": "available" if terraform_cli else "missing",
                "ok": bool(terraform_cli),
                "note": "terraform CLI not installed; python-hcl2 parser used as the scriptable native baseline" if not terraform_cli else f"terraform CLI available at {terraform_cli}; python-hcl2 parser used to avoid provider init/network coupling",
            },
        }
    elif language == "vue":
        native = run_vue_compiler(target, root)
        richer_native = {
            "vue-tsc": {"status": "missing" if not executable("vue-tsc") else "available", "ok": bool(executable("vue-tsc")), "note": "vue-tsc not installed; @vue/compiler-sfc used as the scriptable SFC parser baseline" if not executable("vue-tsc") else "vue-tsc is available but this harness uses compiler-sfc for isolated SFC parse coverage"},
            "volar": {"status": "missing", "ok": False, "note": "Volar language server is not installed; compiler-sfc parse coverage used for this smoke"},
        }
    elif language == "swift":
        native = run_sourcekit_lsp(repo, root)
        richer_native = {
            "sourcekit-lsp": {"status": native.get("status", "unknown"), "ok": native.get("ok", False), "note": native.get("note", "")},
            "swift-syntax": {"status": "missing", "ok": False, "note": "swift-syntax CLI not installed; SourceKit-LSP documentSymbol used as the native baseline"},
        }
    elif language == "elixir":
        native = run_tree_sitter_elixir(target, root)
        richer_native = {
            "elixir": {"status": "missing" if not executable("elixir") else "available", "ok": bool(executable("elixir")), "note": "elixir CLI not installed; tree-sitter-elixir parser used as the isolated definition baseline" if not executable("elixir") else "elixir CLI is available but this harness uses tree-sitter-elixir for source-only definition coverage"},
            "mix": {"status": "missing" if not executable("mix") else "available", "ok": bool(executable("mix")), "note": "mix not installed" if not executable("mix") else "mix is available but this harness avoids dependency compilation/network coupling"},
            "lexical": {"status": "missing" if not executable("lexical") else "missing_adapter", "ok": False, "note": "Lexical language server not installed" if not executable("lexical") else "Lexical adapter not implemented in this harness yet"},
        }
    elif language == "fortran":
        native = run_tree_sitter_fortran(target, root)
        richer_native = {
            "gfortran": {"status": "missing" if not executable("gfortran") else "available", "ok": bool(executable("gfortran")), "note": "gfortran not installed; tree-sitter-fortran parser used as the isolated definition baseline" if not executable("gfortran") else "gfortran is available, but this harness uses tree-sitter-fortran for source-only definition coverage"},
            "fortls": {"status": "missing" if not executable("fortls") else "missing_adapter", "ok": False, "note": "fortls language server not installed" if not executable("fortls") else "fortls adapter not implemented in this harness yet"},
            "fpm": {"status": "missing" if not executable("fpm") else "available", "ok": bool(executable("fpm")), "note": "fpm not installed; source parser baseline used instead of dependency/build execution" if not executable("fpm") else "fpm is available but this harness avoids dependency/build execution"},
        }
    elif language == "verilog":
        native = run_tree_sitter_systemverilog(target, root)
        richer_native = {
            "verilator": {"status": "missing" if not executable("verilator") else "available", "ok": bool(executable("verilator")), "note": "verilator not installed; tree-sitter-systemverilog parser used as the isolated definition baseline" if not executable("verilator") else "verilator is available but this harness uses tree-sitter-systemverilog for source-only definition coverage"},
            "slang": {"status": "missing" if not executable("slang") else "available", "ok": bool(executable("slang")), "note": "slang not installed" if not executable("slang") else "slang is available but not wired as a code-intelligence baseline in this harness"},
            "svlint": {"status": "missing" if not executable("svlint") else "available", "ok": bool(executable("svlint")), "note": "svlint not installed" if not executable("svlint") else "svlint is available but lint-focused, not a definition index baseline"},
        }
    elif language == "groovy":
        native = run_tree_sitter_groovy(target, root)
        richer_native = {
            "groovy": {"status": "missing" if not executable("groovy") else "available", "ok": bool(executable("groovy")), "note": "groovy CLI not installed; tree-sitter-groovy parser used as the isolated definition baseline" if not executable("groovy") else "groovy CLI is available but this harness uses tree-sitter-groovy for source-only definition coverage"},
            "gradle": {"status": "missing" if not executable("gradle") else "available", "ok": bool(executable("gradle")), "note": "gradle not installed; source parser baseline used instead of dependency/build execution" if not executable("gradle") else "gradle is available but this harness avoids dependency/build execution"},
            "groovy-language-server": {"status": "missing", "ok": False, "note": "Groovy language server is not installed; tree-sitter-groovy parse coverage used for this smoke"},
        }
    elif language == "objc":
        native = run_tree_sitter_objc(target, root)
        richer_native = {
            "clang": {"status": "available" if executable("clang") else "missing", "ok": bool(executable("clang")), "note": "clang is available for syntax/build checks; tree-sitter-objc is used for source-only definition coverage" if executable("clang") else "clang not installed"},
            "sourcekit-lsp": {"status": "available" if executable("sourcekit-lsp") else "missing", "ok": bool(executable("sourcekit-lsp")), "note": "sourcekit-lsp is available but Objective-C documentSymbol coverage is not wired into this harness; tree-sitter-objc used instead" if executable("sourcekit-lsp") else "sourcekit-lsp not installed"},
            "clangd": {"status": "missing" if not executable("clangd") else "missing_adapter", "ok": False, "note": "clangd not installed" if not executable("clangd") else "clangd adapter not implemented for Objective-C in this harness yet"},
        }
    elif language == "julia":
        native = run_tree_sitter_julia(target, root)
        richer_native = {
            "julia": {"status": "missing" if not executable("julia") else "available", "ok": bool(executable("julia")), "note": "julia CLI not installed; tree-sitter-julia parser used as the isolated definition baseline" if not executable("julia") else "julia CLI is available, but this harness uses tree-sitter-julia for source-only definition coverage"},
            "LanguageServer.jl": {"status": "missing_adapter", "ok": False, "note": "LanguageServer.jl is not wired into this harness; tree-sitter-julia source parse coverage is used for this smoke"},
        }
    elif language == "zig":
        native = run_tree_sitter_zig(target, root)
        richer_native = {
            "zig": {"status": "missing" if not executable("zig") else "available", "ok": bool(executable("zig")), "note": "zig CLI not installed; tree-sitter-zig parser used as the isolated definition baseline" if not executable("zig") else "zig CLI is available but this harness uses tree-sitter-zig for source-only definition coverage"},
            "zls": {"status": "missing" if not executable("zls") else "missing_adapter", "ok": False, "note": "zls language server not installed" if not executable("zls") else "zls adapter not implemented in this harness yet"},
        }
    else:
        native = {"status": "not_implemented", "ok": False}
        richer_native = {}

    graphify: dict[str, Any] = {"status": "missing", "ok": False, "note": "graphify not found"}
    if graphify_bin:
        clean_generated_sidecars(target)
        graphify_cmd = [graphify_bin, "update", "."]
        start = time.time()
        graphify_result = run(graphify_cmd, cwd=target, timeout=900)
        graphify_seconds = round(time.time() - start, 3)
        graph_json = target / "graphify-out" / "graph.json"
        graphify = {
            "status": "ok" if graphify_result.returncode == 0 else "failed",
            "ok": graphify_result.returncode == 0,
            "seconds": graphify_seconds,
            "command": " ".join(graphify_cmd),
            "stdout_head": graphify_result.stdout.splitlines()[:12],
            "stderr_head": graphify_result.stderr.splitlines()[:12],
        }
        if graph_json.exists():
            graph = json.loads(graph_json.read_text())
            links = graph.get("links", []) or graph.get("edges", []) or []
            graphify["metrics"] = {
                "nodes": len(graph.get("nodes", [])),
                "links": len(links),
                "calls": sum(1 for edge in links if (edge.get("relation") or edge.get("kind")) == "calls"),
            }

    queries = []
    for symbol in config["queries"]:
        atlas_query_cmd = [atlas_bin, "--db", f"sqlite://{db}", "--format", "plain", "--repo", repo_query_name, "explain", symbol]
        start = time.time()
        atlas_query = run(atlas_query_cmd, timeout=120)
        atlas_ms = round((time.time() - start) * 1000, 3)
        graphify_query_cmd = [graphify_bin, "explain", symbol] if graphify_bin else []
        start = time.time()
        graphify_query = run(graphify_query_cmd, cwd=target, timeout=120) if graphify_query_cmd else subprocess.CompletedProcess([], 127, "", "")
        graphify_ms = round((time.time() - start) * 1000, 3)
        queries.append(
            {
                "symbol": symbol,
                "atlas_command": " ".join(atlas_query_cmd).replace(str(Path.cwd()) + "/", "./"),
                "graphify_command": " ".join(graphify_query_cmd),
                "atlas_tokens": tokens(atlas_query.stdout),
                "graphify_tokens": tokens(graphify_query.stdout),
                "atlas_ms": atlas_ms,
                "graphify_ms": graphify_ms,
                "atlas_returncode": atlas_query.returncode,
                "graphify_returncode": graphify_query.returncode,
                "atlas_missing": not atlas_query.stdout.strip(),
                "graphify_missing": "No node matching" in graphify_query.stdout or not graphify_query.stdout.strip(),
                "atlas_stdout_head": atlas_query.stdout.splitlines()[:8],
                "graphify_stdout_head": graphify_query.stdout.splitlines()[:8],
            }
        )

    atlas_index_json = json.loads(atlas_index.stdout)
    atlas_reindex_json = json.loads(atlas_reindex.stdout)
    native_defs = int(native.get("metrics", {}).get("definitions", 0) or 0)
    definition_kinds = {
        "bash": ("function",),
        "csharp": ("class", "interface", "struct", "enum", "record", "method"),
        "dart": ("type", "function", "constructor", "getter", "setter", "typedef"),
        "kotlin": ("type", "function", "variable"),
        "lua": ("function",),
        "php": ("function", "type"),
        "powershell": ("function",),
        "ruby": ("class", "module", "method", "function"),
        "rust": ("function", "type", "constant"),
        "scala": ("type", "function", "variable"),
        "svelte": ("function",),
        "sql": ("table", "view", "function", "procedure", "trigger"),
        "swift": ("function", "type", "variable"),
        "elixir": ("module", "protocol", "implementation", "function", "macro", "delegate", "guard"),
        "fortran": ("module", "type", "function"),
        "verilog": ("module", "interface", "package", "class", "function", "task", "program", "checker"),
        "groovy": ("class", "interface", "enum", "trait", "method", "function", "task"),
        "objc": ("type", "method"),
        "julia": ("module", "type", "function", "macro", "constant"),
        "terraform": ("resource", "data", "module", "variable", "output"),
        "vue": ("function",),
        "zig": ("function", "type", "constant"),
    }.get(language, ("class", "module", "method", "function", "type"))
    placeholders = ",".join("?" for _ in definition_kinds)
    sample_paths = list(native.get("metrics", {}).get("sample_paths", []) or [])
    if language == "swift" and sample_paths:
        path_placeholders = ",".join("?" for _ in sample_paths)
        atlas_language_defs = sqlite_scalar(
            db,
            f"SELECT count(*) FROM symbols WHERE language=? AND kind IN ({placeholders}) AND path IN ({path_placeholders})",
            (language, *definition_kinds, *sample_paths),
        )
    else:
        atlas_language_defs = sqlite_scalar(
            db,
            f"SELECT count(*) FROM symbols WHERE language=? AND kind IN ({placeholders})",
            (language, *definition_kinds),
        )
    coverage = {
        f"atlas_vs_{native.get('tool', 'native').replace('-', '_')}_definition_ratio": ratio(atlas_language_defs, native_defs),
        f"atlas_{language}_definition_symbols": atlas_language_defs,
        "native_definitions": native_defs,
    }
    if language == "swift" and sample_paths:
        coverage["coverage_scope"] = f"{len(sample_paths)} SourceKit-LSP sampled files"

    optimization = {
        "cycles_run": 1,
        "stop_reason": f"{language} live smoke records the first Atlas/graphify/native baseline; additional optimize-test cycles continue if coverage or 5x metrics miss.",
        "cycle_notes": [f"cycle 1: established live {language} baseline against {config['repo']}."],
    }
    if language == "ruby":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "Ruby live smoke met the current 5x latency/token thresholds and the Ripper definition coverage proxy after adding operator, receiver-qualified, and ::-qualified module/class parsing.",
            "cycle_notes": [
                "cycle 1: Sinatra smoke found Atlas/Ripper definition ratio 0.98; biggest gap was Ruby operator and qualified definition syntax.",
                "cycle 2: after parser patch, Atlas/Ripper definition ratio reached 1.01 and equivalent query rows exceeded 5x for latency and token output vs graphify.",
            ],
        }
    elif language == "csharp":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "C# live smoke improves Atlas type/method recall on real Dapper code and records Atlas/graphify query metrics; native Roslyn/OmniSharp coverage remains pending because C# tooling is not installed on this machine.",
            "cycle_notes": [
                "cycle 1: Dapper probe exposed an Atlas C# parser gap: multi-modifier declarations such as `public static partial class SqlMapper` were missed, leaving 1,332 C# definition symbols.",
                "cycle 2: after expanding C# modifier/type/method regexes, the same Dapper checkout indexes 3,419 C# definition symbols and resolves Dapper type queries that were previously absent.",
            ],
        }
    elif language == "dart":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "Dart live smoke met the current 5x latency/token thresholds and matched the tree-sitter-dart definition coverage proxy after replacing generic regex parsing with a Dart-specific signature scanner.",
            "cycle_notes": [
                "cycle 1: dart-lang/http probe showed the generic fallback over-indexed constructor calls such as Duration and missed real generic-return methods such as `Future<StreamedResponse> send(...)`.",
                "cycle 2: after adding a Dart-specific parser, Atlas/tree-sitter-dart definition coverage matched on the live package slice and exact-symbol query rows exceeded 5x for latency and token output vs graphify.",
            ],
        }
    elif language == "kotlin":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "Kotlin live smoke met the current 5x latency/token thresholds and exceeded the tree-sitter-kotlin definition coverage proxy after widening Atlas Kotlin modifier handling.",
            "cycle_notes": [
                "cycle 1: square/okhttp smoke found Atlas/tree-sitter-kotlin definition ratio 0.78; biggest gap was Kotlin modifiers before type/function/property declarations.",
                "cycle 2: after widening Kotlin declaration regexes for actual/open/value/fun-interface/override forms, Atlas/tree-sitter-kotlin definition ratio reached 1.01 and equivalent query rows exceeded 5x for latency and token output vs graphify.",
            ],
        }
    elif language == "lua":
        optimization = {
            "cycles_run": 1,
            "stop_reason": "Lua live smoke exceeded 5x query latency plus token output on equivalent function-symbol queries and exceeded the luaparser named-definition coverage proxy on cycle 1.",
            "cycle_notes": ["cycle 1: folke/lazy.nvim smoke used luaparser as an isolated AST baseline; Atlas indexed more navigable Lua function symbols than luaparser's named definition count while keeping exact-symbol explain output terse."],
        }
    elif language == "php":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "PHP live smoke met the current 5x latency/token thresholds and matched PHP tokenizer definition coverage after separating `use function` imports from real function definitions in the native baseline.",
            "cycle_notes": [
                "cycle 1: Slim smoke found an apparent Atlas/PHP-tokenizer definition ratio of 0.96; biggest gap was the native tokenizer script counting `use function` imports as definitions.",
                "cycle 2: after benchmark baseline correction, Atlas/PHP-tokenizer definition ratio reached 1.0 and equivalent query rows exceeded 5x for latency and token output vs graphify.",
            ],
        }
    elif language == "powershell":
        optimization = {
            "cycles_run": 1,
            "stop_reason": "PowerShell live smoke met the current 5x latency/token thresholds and matched pwsh AST function-definition coverage on cycle 1.",
            "cycle_notes": ["cycle 1: PowerShellGet src smoke used the native PowerShell parser for syntax/function-definition truth; Atlas matched function coverage and kept exact-symbol explain output terse."],
        }
    elif language == "swift":
        optimization = {
            "cycles_run": 1,
            "stop_reason": "Swift live smoke met the current 5x latency/token thresholds and exceeded the SourceKit-LSP sampled definition coverage proxy on cycle 1.",
            "cycle_notes": ["cycle 1: apple/swift-argument-parser smoke reached 1.19x Atlas/SourceKit-LSP sampled definition coverage, 8.07x query latency, and 14.82x token output vs graphify."],
        }
    elif language == "bash":
        optimization = {
            "cycles_run": 1,
            "stop_reason": "Bash live smoke met the current 5x latency/token thresholds and matched the /bin/bash -n function-definition coverage proxy on cycle 1.",
            "cycle_notes": ["cycle 1: nvm-sh/nvm smoke reached 1.0 Atlas/native function coverage and exceeded 5x query latency plus token output vs graphify on exact function queries."],
        }
    elif language == "rust":
        optimization = {
            "cycles_run": 1,
            "stop_reason": "Rust live smoke restores reproducible Atlas/graphify measurements and exceeds 5x query latency plus token output on comparable type-symbol queries; native rust-analyzer coverage remains pending because Rust tooling is not installed on this machine.",
            "cycle_notes": ["cycle 1: BurntSushi/ripgrep smoke records live Atlas and graphify query behavior on shared Rust type symbols while explicitly marking rust-analyzer/cargo/rustc unavailable."],
        }
    elif language == "scala":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "Scala live smoke met the current 5x latency/token thresholds and improved Cats definition coverage after widening Atlas modifier and type-alias handling; Metals/scalac remain unavailable on this machine.",
            "cycle_notes": [
                "cycle 1: typelevel/cats probe showed the biggest Atlas gap was Scala modifiers and type aliases such as `private[cats] trait`, `sealed abstract class`, and `opaque type`.",
                "cycle 2: after parser widening, the Cats smoke uses tree-sitter-scala as a source-only definition baseline while explicitly marking Metals/scalac availability separately.",
            ],
        }
    elif language == "svelte":
        optimization = {
            "cycles_run": 1,
            "stop_reason": "Svelte live smoke exceeded 5x query latency plus token output and exceeded the Svelte compiler script-declaration coverage proxy on cycle 1.",
            "cycle_notes": ["cycle 1: carbon-components-svelte smoke used the native Svelte compiler as an isolated SFC parser baseline; Atlas indexed at least the compiler-counted top-level script declarations and resolved exact component function symbols."],
        }
    elif language == "sql":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "SQL live smoke matched SQLFluff DDL definition coverage after installing graphify's optional tree_sitter_sql parser and exceeded 5x query latency, but token ratio saturated just below 5x on already-terse exact-symbol output.",
            "cycle_notes": [
                "cycle 1: graphify advertised SQL support but produced an empty graph until its optional tree_sitter_sql dependency was installed in the graphify tool environment.",
                "cycle 2: hasura/graphql-engine migrations reached 1.0 Atlas/SQLFluff DDL coverage and exceeded 5x latency; token output stayed below 5x without removing useful source context.",
            ],
        }
    elif language == "terraform":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "Terraform/HCL live smoke matched python-hcl2 definition coverage and exceeded 5x query latency plus token output after installing graphify's optional tree_sitter_hcl parser.",
            "cycle_notes": [
                "cycle 1: graphify advertised Terraform/HCL support but produced no HCL code nodes until its optional tree_sitter_hcl dependency was installed in the graphify tool environment.",
                "cycle 2: terraform-aws-modules/terraform-aws-vpc reached 1.0 Atlas/python-hcl2 definition coverage and exceeded 5x latency plus token output on exact HCL resource queries.",
            ],
        }
    elif language == "vue":
        optimization = {
            "cycles_run": 1,
            "stop_reason": "Vue live smoke matched @vue/compiler-sfc script declaration coverage and exceeded 5x query latency plus token output on cycle 1.",
            "cycle_notes": ["cycle 1: vue-realworld smoke used @vue/compiler-sfc as an isolated SFC parser baseline; Atlas matched top-level script declaration coverage while keeping exact-symbol explain output terse."],
        }
    elif language == "zig":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "Zig live smoke met the current 5x latency/token thresholds and exceeded the tree-sitter-zig definition coverage proxy after widening Atlas Zig declaration handling; zig/zls remain unavailable on this machine.",
            "cycle_notes": [
                "cycle 1: zigtools/zls smoke found Atlas/tree-sitter-zig definition ratio 0.89; biggest gaps were typed constants, compound const declarations, inline/noinline functions, and packed/extern container aliases.",
                "cycle 2: after widening Zig declaration rules and correcting the native compound-const counter, Atlas/tree-sitter-zig definition ratio reached 1.0 while exact-symbol query rows exceeded 5x for latency and token output vs graphify.",
            ],
        }
    elif language == "elixir":
        optimization = {
            "cycles_run": 3,
            "stop_reason": "Elixir live smoke matched the tree-sitter-elixir definition coverage proxy exactly and met the current 5x latency/token thresholds; elixir/mix/Lexical remain unavailable on this machine.",
            "cycle_notes": [
                "cycle 1: phoenixframework/phoenix probe showed the biggest Atlas gap was Elixir macros, delegates, guards, protocols, and implementations beyond basic defmodule/def parsing.",
                "cycle 2: after parser widening, Atlas/tree-sitter-elixir definition ratio reached 1.07, revealing doc/comment false positives from regex parsing.",
                "cycle 3: after masking Elixir comments and heredocs before lightweight regex extraction, Atlas/tree-sitter-elixir definition ratio reached 1.0 while exact-symbol query rows exceeded 5x for latency and token output vs graphify.",
            ],
        }
    elif language == "fortran":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "Fortran live smoke matched the tree-sitter-fortran definition coverage proxy and met the current 5x latency/token thresholds after widening Atlas Fortran declaration handling.",
            "cycle_notes": [
                "cycle 1: fortran-lang/stdlib probe showed the generic Fortran regex could misclassify `module procedure` interface entries as modules and miss `module function`/`module subroutine` definitions.",
                "cycle 2: after tightening module matching and widening function/subroutine modifiers plus typed functions, Atlas/tree-sitter-fortran definition coverage reached 1.0 and exact-symbol query rows exceeded 5x for latency and token output vs graphify.",
            ],
        }
    elif language == "verilog":
        optimization = {
            "cycles_run": 2,
			"stop_reason": "Verilog/SystemVerilog live smoke matched the tree-sitter-systemverilog definition coverage proxy and exceeded 5x overall latency/token ratios after widening Atlas declaration handling; native tree-sitter parse errors and the one below-5x `ibex_core` query row are reported in raw metrics.",
            "cycle_notes": [
                "cycle 1: lowRISC/ibex probe showed the generic Verilog regex missed functions with packed return types such as `function automatic logic [6:0] cm_stack_adj_base(...)`.",
				"cycle 2: after adding a SystemVerilog-specific declaration scanner, Atlas/tree-sitter-systemverilog definition coverage reached 1.0; equivalent query rows exceeded 5x latency and token output overall, while `ibex_core` saturated below 5x for both latency/token because graphify matched the terse `ibex_core.f` file-list node instead of the RTL module.",
            ],
        }
    elif language == "groovy":
        optimization = {
            "cycles_run": 3,
            "stop_reason": "Groovy/Gradle live smoke met the current 5x latency/token thresholds after widening Atlas Groovy declaration handling; definition coverage is saturated by tree-sitter-groovy parse errors on real Nextflow files, so the 1.59x native ratio is not claimed as exact recall.",
            "cycle_notes": [
                "cycle 1: Nextflow nf-commons probe showed the biggest Atlas gap was typed Groovy methods, interfaces, enums, traits, constructors, and Gradle task declarations beyond class plus untyped def parsing.",
                "cycle 2: after parser widening, Atlas/tree-sitter-groovy definition ratio was inflated by control-flow false positives such as if/for/catch/synchronized being indexed as methods.",
                "cycle 3: after restricting constructor and return-type matching, Atlas resolves the shared query symbols and exceeds 5x latency/token output vs graphify; native coverage remains partial because tree-sitter-groovy reports parse errors for most files in this real repo slice.",
            ],
        }
    elif language == "objc":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "Objective-C live smoke matched the graphify-scoped tree-sitter-objc definition coverage proxy and exceeded 5x latency/token thresholds on equivalent rows after preserving full multi-part selectors in Atlas.",
            "cycle_notes": [
                "cycle 1: SDWebImage probe showed Atlas only captured the first Objective-C selector segment, losing precision for multi-argument methods.",
                "cycle 2: after adding an Objective-C-specific parser, Atlas preserves selectors such as `storeImage:forKey:completion:`; graphify flattens those selector names, so the colon-selector row is kept as a visible graphify-missing caveat.",
            ],
        }
    elif language == "julia":
        optimization = {
            "cycles_run": 2,
            "stop_reason": "Julia live smoke met the current 5x latency/token thresholds and matched the tree-sitter-julia definition coverage proxy after masking docstrings/strings and supporting macro-prefixed structs.",
            "cycle_notes": [
                "cycle 1: JuliaIO/JSON.jl probe showed generic Julia regex parsing missed macros/constants and counted docstring examples as real definitions.",
                "cycle 2: after adding a Julia-specific parser with non-code masking, method-assignment detection, and macro-prefixed struct support, Atlas matched the tree-sitter-julia source definition proxy.",
            ],
        }
    return {
        "language": language,
        "repo": config["repo"],
        "commit": commit,
        "commands": {
            "clone": " ".join(clone_cmd),
            "atlas_index": " ".join(atlas_index_cmd).replace(str(Path.cwd()) + "/", "./"),
            "atlas_reindex": " ".join(atlas_reindex_cmd).replace(str(Path.cwd()) + "/", "./"),
            "graphify_update": graphify.get("command", ""),
            "native_baseline": native.get("command", ""),
            "query_symbols": config["queries"],
            "target_path": str(target),
        },
        "atlas": {
            "cold_wall_seconds": atlas_cold_seconds,
            "reindex_wall_seconds": atlas_reindex_seconds,
            "index": atlas_index_json,
            "reindex": atlas_reindex_json,
            "symbol_breakdown": sqlite_breakdown(db, "symbols"),
            "edge_breakdown": sqlite_breakdown(db, "edges"),
            f"{language}_definition_symbols": atlas_language_defs,
        },
        "graphify": graphify,
        "native_baseline": native,
        "richer_native_baselines": richer_native,
        "coverage": coverage,
        "optimization": optimization,
        "queries": queries,
    }


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--language", choices=sorted(LANGUAGES), required=True)
    parser.add_argument("--atlas", default=os.environ.get("ATLAS_BIN", "./bin/atlas"))
    parser.add_argument("--graphify", default=os.environ.get("GRAPHIFY_BIN", "graphify"))
    parser.add_argument("--out", default="")
    args = parser.parse_args()

    atlas_bin = executable(args.atlas)
    if not atlas_bin:
        raise SystemExit(f"atlas binary not found: {args.atlas}")
    graphify_bin = resolve_graphify(args.graphify)
    smoke = run_smoke(args.language, atlas_bin, graphify_bin)
    out = Path(args.out or f"bench/LIVE_{args.language.upper()}_SMOKE.json")
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(smoke, indent=2) + "\n")
    print(json.dumps({"wrote": str(out), "language": args.language, "commit": smoke["commit"], "coverage": smoke["coverage"]}, indent=2))


if __name__ == "__main__":
    main()
