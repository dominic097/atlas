/**
 * atlas.ts — a hand-written, fetch-based TypeScript client for the Atlas REST
 * API (consumption surface S2, served by `atlas serve`). One method per
 * canonical operation, a base URL plus an optional bearer token, JSON decode of
 * the `{ "data": … }` envelope, and RFC 9457 application/problem+json error
 * parsing into a thrown {@link AtlasApiError}.
 *
 *   const c = new AtlasClient("http://localhost:8080", { token: "secret" });
 *   const status = await c.status();
 *   try {
 *     await c.symbol("Nope");
 *   } catch (e) {
 *     if (e instanceof AtlasApiError) console.error(e.status, e.code, e.detail);
 *   }
 */

// ── result types (the key shapes; matches the Go engine wire format) ─────────

export interface RepoStatus {
  repo_id: string;
  repo_full_name: string;
  snapshot_id: string;
  commit_sha: string;
  symbols: number;
  edges: number;
  indexed_at: string;
}

export interface StatusResult {
  tier: string;
  storage_driver: string;
  vector_backend: string;
  repos_indexed: number;
  repos: RepoStatus[];
}

export interface IndexResult {
  repo_id: string;
  repo_full_name: string;
  snapshot_id: string;
  commit_sha: string;
  indexed_files: number;
  symbols: number;
  edges: number;
  routes: number;
  languages: Record<string, number>;
  mode: string;
  duration_ms: number;
}

export interface SearchHit {
  symbol_id: string;
  symbol: string;
  kind: string;
  repo_id: string;
  path: string;
  line: number;
  signature: string;
  doc?: string;
  score: number;
}

export interface SearchResult {
  results: SearchHit[];
  mode_used: string;
  total: number;
}

export interface SymbolRef {
  symbol_id: string;
  symbol: string;
  kind: string;
  path: string;
  line: number;
  signature?: string;
}

export interface SymbolDef {
  symbol_id: string;
  symbol: string;
  kind: string;
  repo_id: string;
  path: string;
  line: number;
  end_line: number;
  signature?: string;
  doc?: string;
  callers: SymbolRef[];
  callees: SymbolRef[];
}

export interface SymbolResult {
  query: string;
  matches: SymbolDef[];
}

export interface CallersResult {
  symbol: string;
  callers: SymbolRef[];
  total: number;
}

export interface RefsResult {
  symbol: string;
  references: SymbolRef[];
  total: number;
}

export interface NeighborsResult {
  symbol: string;
  callers: SymbolRef[];
  callees: SymbolRef[];
}

export interface ExplainDef {
  symbol_id: string;
  kind: string;
  path: string;
  line: number;
  end_line: number;
  signature?: string;
  doc?: string;
}

export interface ExplainRoute {
  method: string;
  path: string;
  handler_file?: string;
}

export interface ExplainResult {
  symbol: string;
  definitions: ExplainDef[];
  callers: string[];
  callees: string[];
  imports?: string[];
  served_routes?: ExplainRoute[];
  cross_repo_consumers?: string[];
}

export interface FileImpact {
  path: string;
  reason: string;
}

export interface ImpactResult {
  impacted_symbols: string[];
  impacted_files: FileImpact[];
  impacted_tests: string[];
  depth_reached: number;
}

export interface PathResult {
  from: string;
  to: string;
  found: boolean;
  length: number;
  steps: SymbolRef[];
}

export interface CoverageResult {
  target: string;
  direction: string;
  covered: boolean;
  tests?: SymbolRef[];
  symbols?: SymbolRef[];
}

export interface GraphExportResult {
  format: string;
  nodes: number;
  edges: number;
  content: string;
}

export interface SnapshotInfo {
  snapshot_id: string;
  commit_sha: string;
  branch?: string;
  files: number;
  symbols: number;
  edges: number;
  created_at: string;
}

export interface HistoryResult {
  repo_id: string;
  repo_full_name: string;
  snapshots: SnapshotInfo[];
}

export interface SymbolChange {
  name: string;
  path: string;
  kind: string;
  change: string;
}

export interface EdgeChange {
  from: string;
  to: string;
  change: string;
}

export interface SnapshotDiffResult {
  from_commit: string;
  from_snapshot: string;
  to_commit: string;
  to_snapshot: string;
  added_count: number;
  removed_count: number;
  modified_count: number;
  added_symbols: SymbolChange[];
  removed_symbols: SymbolChange[];
  modified_symbols: SymbolChange[];
  changed_files: string[];
  added_edges: EdgeChange[];
  removed_edges: EdgeChange[];
}

export interface RouteContract {
  method: string;
  path_pattern: string;
  handler_file?: string;
  handler_symbol?: string;
  source?: string;
  confidence?: string;
}

export interface RouteContractsResult {
  repo: string;
  routes: RouteContract[];
  total: number;
}

export interface ConsumerHit {
  repo: string;
  calling_file: string;
  calling_symbol?: string;
  matched_route: string;
  endpoint: string;
}

export interface ConsumersResult {
  repo: string;
  impacted: ConsumerHit[];
  consumer_repos: string[];
}

export interface CrossRepoImpactResult {
  repo: string;
  served_routes: RouteContract[];
  impacted: ConsumerHit[];
  consumer_repos: string[];
}

// ── params ───────────────────────────────────────────────────────────────────

export interface StatusParams {
  repoId?: string;
  verbose?: boolean;
}

export interface IndexParams {
  projectPath?: string;
  repo?: string;
  reindex?: boolean;
}

export interface SearchParams {
  query: string;
  repoId?: string;
  kind?: string;
  limit?: number;
  mode?: string;
}

export interface ImpactParams {
  changedPaths?: string[];
  symbols?: string[];
  maxDepth?: number;
  repoId?: string;
}

export interface CoverageParams {
  target: string;
  repoId?: string;
  direction?: string;
}

export interface ExportParams {
  repoId?: string;
  symbol?: string;
  depth?: number;
  format?: string;
  all?: boolean;
}

// ── error + client ────────────────────────────────────────────────────────────

/** AtlasApiError is the typed form of an RFC 9457 problem+json response. */
export class AtlasApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly detail: string;
  readonly title?: string;
  readonly type?: string;

  constructor(status: number, code: string, detail: string, title?: string, type?: string) {
    super(`atlas api error ${status} (${code}): ${detail}`);
    this.name = "AtlasApiError";
    this.status = status;
    this.code = code;
    this.detail = detail;
    this.title = title;
    this.type = type;
    Object.setPrototypeOf(this, AtlasApiError.prototype);
  }
}

export interface AtlasClientOptions {
  /** Bearer token sent as `Authorization: Bearer <token>` on /api/v1 requests. */
  token?: string;
  /** Custom fetch implementation (defaults to the global `fetch`). */
  fetch?: typeof fetch;
}

interface Envelope<T> {
  data: T;
}

type QueryValue = string | number | boolean | undefined;

/** AtlasClient is a typed HTTP client for the Atlas REST API. */
export class AtlasClient {
  private readonly baseURL: string;
  private readonly token?: string;
  private readonly fetchImpl: typeof fetch;

  constructor(baseURL: string, opts: AtlasClientOptions = {}) {
    const trimmed = baseURL.trim().replace(/\/+$/, "");
    if (!trimmed) {
      throw new Error("AtlasClient: empty base URL");
    }
    this.baseURL = trimmed;
    this.token = opts.token;
    const f = opts.fetch ?? (typeof fetch !== "undefined" ? fetch : undefined);
    if (!f) {
      throw new Error("AtlasClient: no fetch implementation available; pass opts.fetch");
    }
    this.fetchImpl = f;
  }

  // ── operations ──────────────────────────────────────────────────────────────

  status(params: StatusParams = {}): Promise<StatusResult> {
    return this.get<StatusResult>("/api/v1/status", {
      repo_id: params.repoId,
      verbose: params.verbose ? "true" : undefined,
    });
  }

  index(params: IndexParams = {}): Promise<IndexResult> {
    return this.post<IndexResult>("/api/v1/index", {
      project_path: params.projectPath,
      repo: params.repo,
      reindex: params.reindex,
    });
  }

  search(params: SearchParams): Promise<SearchResult> {
    return this.get<SearchResult>("/api/v1/search", {
      q: params.query,
      repo_id: params.repoId,
      kind: params.kind,
      limit: params.limit,
      mode: params.mode,
    });
  }

  symbol(name: string, repoId?: string): Promise<SymbolResult> {
    return this.get<SymbolResult>(`/api/v1/symbols/${this.seg(name)}`, { repo_id: repoId });
  }

  callers(name: string, repoId?: string, limit?: number): Promise<CallersResult> {
    return this.get<CallersResult>(`/api/v1/symbols/${this.seg(name)}/callers`, {
      repo_id: repoId,
      limit,
    });
  }

  refs(name: string, repoId?: string): Promise<RefsResult> {
    return this.get<RefsResult>(`/api/v1/symbols/${this.seg(name)}/refs`, { repo_id: repoId });
  }

  neighbors(name: string, repoId?: string): Promise<NeighborsResult> {
    return this.get<NeighborsResult>(`/api/v1/symbols/${this.seg(name)}/neighbors`, {
      repo_id: repoId,
    });
  }

  explain(name: string, repoId?: string): Promise<ExplainResult> {
    return this.get<ExplainResult>(`/api/v1/symbols/${this.seg(name)}/explain`, {
      repo_id: repoId,
    });
  }

  impact(params: ImpactParams = {}): Promise<ImpactResult> {
    return this.post<ImpactResult>("/api/v1/impact", {
      changed_paths: params.changedPaths ?? [],
      symbols: params.symbols ?? [],
      max_depth: params.maxDepth ?? 0,
      repo_id: params.repoId ?? "",
    });
  }

  path(from: string, to: string, maxDepth?: number, repoId?: string): Promise<PathResult> {
    return this.get<PathResult>("/api/v1/path", {
      from,
      to,
      repo_id: repoId,
      max_depth: maxDepth,
    });
  }

  coverage(params: CoverageParams): Promise<CoverageResult> {
    return this.get<CoverageResult>("/api/v1/coverage", {
      target: params.target,
      repo_id: params.repoId,
      direction: params.direction,
    });
  }

  export(params: ExportParams = {}): Promise<GraphExportResult> {
    return this.get<GraphExportResult>("/api/v1/export", {
      repo_id: params.repoId,
      symbol: params.symbol,
      depth: params.depth,
      format: params.format,
      all: params.all ? "true" : undefined,
    });
  }

  /** history is keyed on the `repo` query param (repo full_name or id). */
  history(repo?: string, limit?: number): Promise<HistoryResult> {
    return this.get<HistoryResult>("/api/v1/history", { repo, limit });
  }

  snapshotDiff(repo?: string, from?: string, to?: string): Promise<SnapshotDiffResult> {
    return this.get<SnapshotDiffResult>("/api/v1/snapshot-diff", { repo, from, to });
  }

  /** repos returns the bare repo slice under `{ data: [...] }`. */
  repos(): Promise<RepoStatus[]> {
    return this.get<RepoStatus[]>("/api/v1/repos", {});
  }

  routeContracts(repo: string): Promise<RouteContractsResult> {
    return this.get<RouteContractsResult>(`/api/v1/repos/${this.seg(repo)}/route-contracts`, {});
  }

  consumers(repo: string): Promise<ConsumersResult> {
    return this.get<ConsumersResult>(`/api/v1/repos/${this.seg(repo)}/consumers`, {});
  }

  crossRepoImpact(repo: string, changedPaths: string[] = []): Promise<CrossRepoImpactResult> {
    return this.post<CrossRepoImpactResult>(`/api/v1/repos/${this.seg(repo)}/cross-repo-impact`, {
      changed_paths: changedPaths,
    });
  }

  // ── transport ───────────────────────────────────────────────────────────────

  private get<T>(path: string, query: Record<string, QueryValue>): Promise<T> {
    return this.request<T>("GET", path + this.queryString(query));
  }

  private post<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>("POST", path, body);
  }

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { Accept: "application/json" };
    if (this.token) {
      headers["Authorization"] = `Bearer ${this.token}`;
    }
    const init: RequestInit = { method, headers };
    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
      init.body = JSON.stringify(body);
    }

    const resp = await this.fetchImpl(this.baseURL + path, init);
    const text = await resp.text();

    if (!resp.ok) {
      throw this.toApiError(resp.status, text);
    }

    const env = JSON.parse(text) as Envelope<T>;
    return env.data;
  }

  private toApiError(status: number, text: string): AtlasApiError {
    try {
      const p = JSON.parse(text) as {
        code?: string;
        detail?: string;
        title?: string;
        type?: string;
      };
      if (p && (p.code || p.detail)) {
        return new AtlasApiError(status, p.code ?? "", p.detail ?? "", p.title, p.type);
      }
    } catch {
      // fall through to the raw-text fallback
    }
    return new AtlasApiError(status, String(status), text.trim());
  }

  private queryString(query: Record<string, QueryValue>): string {
    const usp = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === "" || value === false) {
        continue;
      }
      if (typeof value === "number" && value <= 0) {
        continue;
      }
      usp.set(key, String(value));
    }
    const s = usp.toString();
    return s ? `?${s}` : "";
  }

  private seg(s: string): string {
    return encodeURIComponent(s);
  }
}
