# Phase 3 spike — compact integer-ID + dictionary schema

**Status:** spike complete. Design locked, win confirmed empirically. The full
storage compaction is the next focused build (not done in this commit).

This is the measurement spike the plan called for: build the compact schema, load a
real repo, measure DB-size vs the TEXT-uuid baseline, lock the design, *then* migrate.
It was run against a real 643 MB Atlas index (`.atlas/pulse-review-live.db`):
**128,825 symbols, 743,343 edges, 7,269 files, 4 snapshots.**

## 1. Where the bytes actually go (measured, `dbstat`)

| Object | Bytes |
|---|---:|
| `edges` table | 218.6 MB |
| `symbols` table | 178.4 MB |
| edge indexes (`fromfile`/`fromsymbol`/`toref` + uuid-PK autoindex) | 226.4 MB |
| symbol indexes (`node`/`path`/`name` + uuid-PK autoindex) | 46.5 MB |
| files + routes | ~3 MB |

**Indexes are ~42% of the DB.** The uuid-PK autoindexes
(`sqlite_autoindex_edges_1` = 38 MB, `…symbols_1` = 6.5 MB) are pure overhead —
**no query filters or joins on `edges.id` / `symbols.id`** (verified: the only
`WHERE id = ?` in the store is on the 4-row `snapshots` table). The uuid `id`
columns are write-only at the SQL layer.

### Average bytes per column (measured)

- **Edges (743k rows):** `id` 36 · `snapshot_id` 36 · `from_file` 41.8 ·
  `from_symbol` 29.1 · `to_ref` 9.6 · `kind` 5.2 (**3 distinct**) ·
  `language` 2.4 (**5 distinct**) · `metadata` 102.6 (repetitive enum-ish JSON:
  `analysis_level`/`source`/`recv_source`/`recv_type`/`qualified_ref`).
- **Symbols (129k rows):** `id` 36 · `snapshot_id` 36 · `node_id` 64 (all 64-hex) ·
  `repo_id` 36 · `path` 40 · `name` 27.7 · `kind` 7.1 (11 distinct) ·
  `language` 3.3 (23 distinct) · `signature` 56.7 · `doc` 110.3 ·
  **`metadata` 731.7** (`body_excerpt` source code — dominates the row).

## 2. Measured win (compacted copy built from the real rows, row-parity verified)

Built `edges_c` / `symbols_c` with: implicit **rowid** PK (drop uuid `id`),
`snapshot_id`→int FK, `kind`/`language`→dictionary int, edge hot-metadata
(`source`/`recv_source`/`recv_type`/`qualified_ref`)→typed columns,
`node_id`→32-byte BLOB (`unhex`), payload (`signature`/`doc`/`metadata`) verbatim.
Same secondary indexes as prod.

| Storage | Original | Compacted | Saved |
|---|---:|---:|---:|
| edges + indexes | 424.4 MB | 170.6 MB | **−253.8 MB (−60%)** |
| symbols + indexes | 214.4 MB | 163.6 MB | −50.8 MB (−24%) |
| **total** | **638.8 MB** | **334.2 MB** | **−304.6 MB (−48%)** |

Row counts identical on both tables (743343, 128825). The win is **edge-dominated**
(edges are ID/ref/enum bytes; symbols are dominated by `metadata` body_excerpt,
which integer IDs don't shrink) — and edges grow fastest with repo size, so the win
*grows* on larger codebases. Less on-disk also means proportionally smaller
in-memory edge slices (4 uuid string fields/edge → ints) and less GC pressure on
the 743k-edge whole-graph loads.

## 3. Design decision — storage-layer compaction, NOT a model migration

The spike changes the original plan. The win is almost entirely capturable **inside
the store package**, because:

- The uuid `id` columns are **write-only** at SQL → droppable.
- `snapshot_id` / `kind` / `language` / edge-metadata are **encode-on-write,
  decode-on-read** → the in-memory `graph.CodeSymbol` / `graph.DependencyEdge`
  model (string IDs) and **all engine/MCP output stay byte-identical**.

This avoids the high-risk model migration (re-typing `*.ID` to int and rippling
through engine/query/analytics/gotypes/mcp) the plan originally assumed.

**Hard constraint surfaced:** `CodeSymbol.ID` is *public* — returned as `symbol_id`
in search output and used as a node key (`query.go:899`, `byID[s.ID]`). So the
symbol id's **bytes must not change**. Therefore symbols keep a stored string id
(or a deterministically-formatted one preserving current output); only the columns
that never appear in output (`snapshot_id` internal mapping, `kind`/`language`
encoding, edge `metadata` recomposed to an identical map) get compacted. Edge `id`
*is* unused (read nowhere) and is the one id safe to drop to rowid.

**Migration strategy — reindex, don't in-place-migrate.** The local `.atlas.db` is a
*derived cache*; the git working tree is the source of truth. So a schema-version
bump that finds an old DB should **rebuild via reindex**, not run a risky in-place
743k-row data migration. This eliminates the most dangerous part of the migration.

## 4. Recommended build order (each slice: store-internal, output byte-identical, parity-tested)

1. **`snapshot_id` → internal int** (store maps uuid↔int via the snapshots table;
   model still returns the uuid string). Largest single safe chunk (~120 MB: edge
   table + all 3 edge secondary-index prefixes + symbol indexes). Touches every
   `WHERE snapshot_id = ?` in `sqlite.go`/`postgres.go` + `SaveSnapshot` + scan.
2. **Edge `metadata` → typed columns** (`source`/`recv_source`/`recv_type`/
   `qualified_ref`) + remainder JSON; recompose an identical `JSONBMap` on read
   (map re-marshal is key-sorted/deterministic). ~38 MB.
3. **Dictionary-encode `kind` + `language`** (both tables). ~10 MB; trivial + safe.
4. **Drop edge uuid `id` → rowid** (edge id read nowhere). ~65 MB (table + 38 MB
   autoindex). Symbols keep their string id (public output).
5. **`node_id` → 32-byte BLOB** (all 64-hex today). ~4 MB table + halves
   `idx_symbols_node`.

Gate the new schema behind a version bump; on an old DB, reindex. Verify each slice
with: `go test ./...` green, the existing full-vs-delta parity tests, and an
explicit old-vs-new byte-identical check on every touched op's output (the same bar
that caught the Phase 1 parity bug).

## 5. Reproduce

```
DB=.atlas/pulse-review-live.db
# table/index byte breakdown:
sqlite3 $DB "SELECT name, SUM(pgsize) FROM dbstat GROUP BY name ORDER BY 2 DESC;"
# the compaction prototype lives in this spike's shell history; rebuild edges_c/
# symbols_c from src.edges/src.symbols with rowid PK + dict + typed metadata cols.
```
