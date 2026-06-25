"""atlas_client — a hand-written, stdlib-only HTTP client for the Atlas REST API.

Atlas (consumption surface S2, served by ``atlas serve``) is a deterministic
code-intelligence engine. This module is a thin, typed wrapper over
``urllib.request``: one method per canonical operation, a base URL plus an
optional bearer token, JSON decode of the ``{"data": ...}`` envelope, and
RFC 9457 ``application/problem+json`` error parsing into an :class:`AtlasApiError`.

No third-party dependencies.

Example::

    from atlas_client import AtlasClient, AtlasApiError

    client = AtlasClient("http://localhost:8080", token="secret")
    status = client.status()
    try:
        client.symbol("Nope")
    except AtlasApiError as e:
        print(e.status, e.code, e.detail)
"""

from __future__ import annotations

import json
import urllib.error
import urllib.parse
import urllib.request
from typing import Any, Dict, List, Optional

__all__ = ["AtlasClient", "AtlasApiError"]


class AtlasApiError(Exception):
    """Typed form of an RFC 9457 problem+json error response."""

    def __init__(
        self,
        status: int,
        code: str = "",
        detail: str = "",
        title: Optional[str] = None,
        type: Optional[str] = None,
    ) -> None:
        self.status = status
        self.code = code
        self.detail = detail
        self.title = title
        self.type = type
        super().__init__(f"atlas api error {status} ({code}): {detail}")


class AtlasClient:
    """A typed HTTP client for the Atlas REST API."""

    def __init__(
        self,
        base_url: str,
        token: Optional[str] = None,
        timeout: float = 30.0,
    ) -> None:
        trimmed = (base_url or "").strip().rstrip("/")
        if not trimmed:
            raise ValueError("AtlasClient: empty base URL")
        parsed = urllib.parse.urlparse(trimmed)
        if not parsed.scheme or not parsed.netloc:
            raise ValueError(f"AtlasClient: base URL must be absolute, got {base_url!r}")
        self.base_url = trimmed
        self.token = token
        self.timeout = timeout

    # ── operations ────────────────────────────────────────────────────────────

    def status(
        self, repo_id: Optional[str] = None, verbose: bool = False
    ) -> Dict[str, Any]:
        return self._get(
            "/api/v1/status",
            {"repo_id": repo_id, "verbose": "true" if verbose else None},
        )

    def index(
        self,
        project_path: Optional[str] = None,
        repo: Optional[str] = None,
        reindex: bool = False,
    ) -> Dict[str, Any]:
        return self._post(
            "/api/v1/index",
            {"project_path": project_path, "repo": repo, "reindex": reindex},
        )

    def search(
        self,
        query: str,
        repo_id: Optional[str] = None,
        kind: Optional[str] = None,
        limit: Optional[int] = None,
        mode: Optional[str] = None,
    ) -> Dict[str, Any]:
        return self._get(
            "/api/v1/search",
            {
                "q": query,
                "repo_id": repo_id,
                "kind": kind,
                "limit": limit,
                "mode": mode,
            },
        )

    def symbol(self, name: str, repo_id: Optional[str] = None) -> Dict[str, Any]:
        return self._get(
            "/api/v1/symbols/" + self._seg(name), {"repo_id": repo_id}
        )

    def callers(
        self, name: str, repo_id: Optional[str] = None, limit: Optional[int] = None
    ) -> Dict[str, Any]:
        return self._get(
            "/api/v1/symbols/" + self._seg(name) + "/callers",
            {"repo_id": repo_id, "limit": limit},
        )

    def refs(self, name: str, repo_id: Optional[str] = None) -> Dict[str, Any]:
        return self._get(
            "/api/v1/symbols/" + self._seg(name) + "/refs", {"repo_id": repo_id}
        )

    def neighbors(self, name: str, repo_id: Optional[str] = None) -> Dict[str, Any]:
        return self._get(
            "/api/v1/symbols/" + self._seg(name) + "/neighbors",
            {"repo_id": repo_id},
        )

    def explain(self, name: str, repo_id: Optional[str] = None) -> Dict[str, Any]:
        return self._get(
            "/api/v1/symbols/" + self._seg(name) + "/explain",
            {"repo_id": repo_id},
        )

    def impact(
        self,
        changed_paths: Optional[List[str]] = None,
        symbols: Optional[List[str]] = None,
        max_depth: int = 0,
        repo_id: str = "",
    ) -> Dict[str, Any]:
        return self._post(
            "/api/v1/impact",
            {
                "changed_paths": changed_paths or [],
                "symbols": symbols or [],
                "max_depth": max_depth,
                "repo_id": repo_id,
            },
        )

    def path(
        self,
        from_symbol: str,
        to_symbol: str,
        max_depth: Optional[int] = None,
        repo_id: Optional[str] = None,
    ) -> Dict[str, Any]:
        return self._get(
            "/api/v1/path",
            {
                "from": from_symbol,
                "to": to_symbol,
                "repo_id": repo_id,
                "max_depth": max_depth,
            },
        )

    def coverage(
        self,
        target: str,
        repo_id: Optional[str] = None,
        direction: Optional[str] = None,
    ) -> Dict[str, Any]:
        return self._get(
            "/api/v1/coverage",
            {"target": target, "repo_id": repo_id, "direction": direction},
        )

    def export(
        self,
        repo_id: Optional[str] = None,
        symbol: Optional[str] = None,
        depth: Optional[int] = None,
        fmt: Optional[str] = None,
        all_nodes: bool = False,
    ) -> Dict[str, Any]:
        return self._get(
            "/api/v1/export",
            {
                "repo_id": repo_id,
                "symbol": symbol,
                "depth": depth,
                "format": fmt,
                "all": "true" if all_nodes else None,
            },
        )

    def history(
        self, repo: Optional[str] = None, limit: Optional[int] = None
    ) -> Dict[str, Any]:
        # history is keyed on the `repo` query param (repo full_name or id).
        return self._get("/api/v1/history", {"repo": repo, "limit": limit})

    def snapshot_diff(
        self,
        repo: Optional[str] = None,
        from_ref: Optional[str] = None,
        to_ref: Optional[str] = None,
    ) -> Dict[str, Any]:
        return self._get(
            "/api/v1/snapshot-diff",
            {"repo": repo, "from": from_ref, "to": to_ref},
        )

    def repos(self) -> List[Dict[str, Any]]:
        # repos returns the bare repo slice under {"data": [...]}.
        return self._get("/api/v1/repos", {})

    def route_contracts(self, repo: str) -> Dict[str, Any]:
        return self._get(
            "/api/v1/repos/" + self._seg(repo) + "/route-contracts", {}
        )

    def consumers(self, repo: str) -> Dict[str, Any]:
        return self._get("/api/v1/repos/" + self._seg(repo) + "/consumers", {})

    def cross_repo_impact(
        self, repo: str, changed_paths: Optional[List[str]] = None
    ) -> Dict[str, Any]:
        return self._post(
            "/api/v1/repos/" + self._seg(repo) + "/cross-repo-impact",
            {"changed_paths": changed_paths or []},
        )

    # ── transport ─────────────────────────────────────────────────────────────

    def _get(self, path: str, query: Dict[str, Any]) -> Any:
        return self._request("GET", path + self._query_string(query), None)

    def _post(self, path: str, body: Any) -> Any:
        return self._request("POST", path, body)

    def _request(self, method: str, path: str, body: Optional[Any]) -> Any:
        url = self.base_url + path
        data: Optional[bytes] = None
        headers = {"Accept": "application/json"}
        if body is not None:
            data = json.dumps(body).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if self.token:
            headers["Authorization"] = "Bearer " + self.token

        req = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with urllib.request.urlopen(req, timeout=self.timeout) as resp:
                raw = resp.read()
        except urllib.error.HTTPError as exc:
            raise self._to_api_error(exc) from None

        envelope = json.loads(raw.decode("utf-8"))
        return envelope.get("data")

    def _to_api_error(self, exc: "urllib.error.HTTPError") -> AtlasApiError:
        status = exc.code
        try:
            raw = exc.read()
        except Exception:
            raw = b""
        text = raw.decode("utf-8", errors="replace") if raw else ""
        if text:
            try:
                problem = json.loads(text)
            except json.JSONDecodeError:
                problem = None
            if isinstance(problem, dict) and (problem.get("code") or problem.get("detail")):
                return AtlasApiError(
                    status=status,
                    code=problem.get("code", ""),
                    detail=problem.get("detail", ""),
                    title=problem.get("title"),
                    type=problem.get("type"),
                )
        return AtlasApiError(status=status, code=str(status), detail=text.strip())

    @staticmethod
    def _query_string(query: Dict[str, Any]) -> str:
        pairs: List[tuple] = []
        for key, value in query.items():
            if value is None or value == "" or value is False:
                continue
            if isinstance(value, int) and not isinstance(value, bool) and value <= 0:
                continue
            pairs.append((key, str(value)))
        encoded = urllib.parse.urlencode(pairs)
        return "?" + encoded if encoded else ""

    @staticmethod
    def _seg(value: str) -> str:
        return urllib.parse.quote(value, safe="")
