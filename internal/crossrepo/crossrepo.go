// Package crossrepo is the cross-repo impact engine — the Atlas USP. It joins
// the PRODUCER route contracts a repo SERVES against the CONSUMER HTTP calls
// other repos MAKE, so a change to a handler in repo A surfaces every OTHER repo
// that calls that endpoint (and the exact calling files).
//
// It is built entirely against the store contract (ListRepos / LatestSnapshot /
// ListRoutes) and graph.Route — the producer/consumer extraction is wired in by
// the indexer in parallel; this package only matches what is already persisted.
//
// The matcher (EndpointMatch) is ported from aziron-pulse
// code_search_service.go endpointMatchesAnyRoute: param/`:name`/`{}` segments
// are wildcards, query strings + trailing slashes + leading host are stripped,
// and consumer method "" matches any producer method.
package crossrepo

import (
	"context"
	"errors"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	"github.com/MsysTechnologiesllc/aziron-atlas/internal/store"
)

// ErrRepoNotFound is returned when repoFullName resolves to no indexed repo.
// The engine maps it to its ErrNoIndex sentinel.
var ErrRepoNotFound = errors.New("crossrepo: repo not found (index it first)")

// ConsumerHit is one calling site in another repo that depends on a route the
// changed repo serves.
type ConsumerHit struct {
	Repo          string `json:"repo"`
	CallingFile   string `json:"calling_file"`
	CallingSymbol string `json:"calling_symbol,omitempty"`
	MatchedRoute  string `json:"matched_route"` // the producer pattern that matched (METHOD path)
	Endpoint      string `json:"endpoint"`      // the consumer's called endpoint (METHOD path)
}

// CrossRepoResult is the blast radius of a change: the routes the repo serves and
// every consumer repo (and calling file) impacted by them.
type CrossRepoResult struct {
	Repo          string        `json:"repo"`
	ServedRoutes  []graph.Route `json:"served_routes"`
	Impacted      []ConsumerHit `json:"impacted"`
	ConsumerRepos []string      `json:"consumer_repos"`
}

// EndpointMatch reports whether a consumer call (consMethod + consPath) hits a
// producer route (prodMethod + prodPath). Ported from pulse
// endpointMatchesAnyRoute / routePathMatches:
//   - method: case-insensitive; an empty consumer method matches any producer.
//   - path: compared segment-by-segment after stripping host, query, fragment
//     and a trailing slash. A producer segment that is a `{param}` / `:param`
//     placeholder, OR a consumer segment that is a dynamic placeholder
//     (`{id}` / `:id` / `{}`), matches any single concrete segment.
func EndpointMatch(prodMethod, prodPath, consMethod, consPath string) bool {
	if !methodMatches(prodMethod, consMethod) {
		return false
	}
	pp := normalizeEndpoint(prodPath)
	cp := normalizeEndpoint(consPath)
	if pp == "" || cp == "" {
		return false
	}
	ps := strings.Split(strings.TrimPrefix(pp, "/"), "/")
	cs := strings.Split(strings.TrimPrefix(cp, "/"), "/")
	// A producer route registered with a trailing slash (net/http subtree, e.g.
	// "/api/v1/users/") serves every deeper path, so it PREFIX-matches a consumer
	// call like "/api/v1/users/{id}" that has extra trailing segments.
	if isSubtree(prodPath) {
		if len(cs) < len(ps) {
			return false
		}
		cs = cs[:len(ps)]
	} else if len(ps) != len(cs) {
		return false
	}
	for i := range ps {
		if isWildcardSegment(ps[i]) || isWildcardSegment(cs[i]) {
			continue
		}
		if !strings.EqualFold(ps[i], cs[i]) {
			return false
		}
	}
	return true
}

// isSubtree reports whether a producer path was registered as a net/http subtree
// (trailing slash) — meaning it serves every deeper path.
func isSubtree(raw string) bool {
	e := strings.TrimSpace(raw)
	if i := strings.IndexAny(e, "?#"); i >= 0 {
		e = e[:i]
	}
	return len(e) > 1 && strings.HasSuffix(e, "/")
}

// methodMatches is case-insensitive; an empty consumer method (unknown) matches
// any producer method.
func methodMatches(prodMethod, consMethod string) bool {
	c := strings.TrimSpace(consMethod)
	p := strings.TrimSpace(prodMethod)
	// Empty (unknown) or wildcard ("ANY" from net/http HandleFunc, "*") matches any.
	if c == "" || p == "" || isAnyMethod(p) || isAnyMethod(c) {
		return true
	}
	return strings.EqualFold(p, c)
}

func isAnyMethod(m string) bool { return strings.EqualFold(m, "ANY") || m == "*" }

// normalizeEndpoint strips a leading scheme://host, the query string + fragment,
// and a trailing slash, leaving a leading-slash path (e.g. /api/v1/users/{id}).
func normalizeEndpoint(raw string) string {
	e := strings.TrimSpace(raw)
	if e == "" {
		return ""
	}
	// strip scheme://host — keep from the first path slash after the authority.
	if idx := strings.Index(e, "://"); idx >= 0 {
		rest := e[idx+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			e = rest[slash:]
		} else {
			e = "/"
		}
	} else if !strings.HasPrefix(e, "/") {
		// host-without-scheme like api.example.com/v1/x → drop up to the first slash.
		if slash := strings.IndexByte(e, '/'); slash >= 0 && strings.Contains(e[:slash], ".") {
			e = e[slash:]
		}
	}
	if i := strings.IndexAny(e, "?#"); i >= 0 {
		e = e[:i]
	}
	if !strings.HasPrefix(e, "/") {
		e = "/" + e
	}
	if len(e) > 1 {
		e = strings.TrimRight(e, "/")
	}
	return e
}

// isWildcardSegment is true for producer placeholders (`{id}` / `:id`) and
// consumer dynamic segments (`{id}` / `{}` / `:id`).
func isWildcardSegment(seg string) bool {
	if seg == "" {
		return false
	}
	switch {
	case strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}"):
		return true // {id} or {}
	case strings.HasPrefix(seg, ":"):
		return true // :id (chi/gin style)
	case strings.HasPrefix(seg, "%"):
		return true // %s / %d / %v — a fmt.Sprintf placeholder in a consumer URL
	case strings.HasPrefix(seg, "$"):
		return true // ${id} / $id template interpolation
	case isAllDigits(seg):
		return true // a concrete numeric id (/users/1 hits /users/{id})
	}
	return false
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// Impact resolves repoFullName to its latest snapshot, picks the producer routes
// whose handler files are in changedPaths (or ALL producer routes when
// changedPaths is empty), then scans every OTHER indexed repo's consumer routes
// for EndpointMatch hits — returning the impacted consumer repos and calling
// files.
func Impact(ctx context.Context, drv store.StorageDriver, repoFullName string, changedPaths []string) (CrossRepoResult, error) {
	res := CrossRepoResult{
		Repo:          repoFullName,
		ServedRoutes:  []graph.Route{},
		Impacted:      []ConsumerHit{},
		ConsumerRepos: []string{},
	}
	repos, err := drv.ListRepos(ctx, "")
	if err != nil {
		return res, err
	}
	// Resolve the changed repo to its repo_id (by full_name) and latest snapshot.
	var changedRepoID string
	for i := range repos {
		if strings.EqualFold(repos[i].FullName, repoFullName) {
			changedRepoID = repos[i].ID
			res.Repo = repos[i].FullName
			break
		}
	}
	if changedRepoID == "" {
		return res, ErrRepoNotFound
	}
	snap, err := drv.LatestSnapshot(ctx, changedRepoID)
	if err != nil {
		return res, err
	}
	if snap == nil {
		return res, nil
	}
	producers, err := drv.ListRoutes(ctx, snap.ID, "producer")
	if err != nil {
		return res, err
	}

	// Filter producers to the changed handler files (canonical-path compare). An
	// empty changedPaths means "this whole repo changed" → all producer routes.
	changedSet := canonicalSet(changedPaths)
	served := make([]graph.Route, 0, len(producers))
	for _, p := range producers {
		if len(changedSet) == 0 || changedSet[canonicalPath(p.HandlerFile)] {
			served = append(served, p)
		}
	}
	res.ServedRoutes = served
	if len(served) == 0 {
		return res, nil
	}

	// For every OTHER repo with a latest snapshot, match its consumer routes.
	repoSeen := map[string]bool{}
	hitSeen := map[string]bool{}
	for i := range repos {
		if repos[i].ID == changedRepoID {
			continue
		}
		csnap, err := drv.LatestSnapshot(ctx, repos[i].ID)
		if err != nil || csnap == nil {
			continue
		}
		consumers, err := drv.ListRoutes(ctx, csnap.ID, "consumer")
		if err != nil {
			continue
		}
		repoName := repos[i].FullName
		for _, c := range consumers {
			for _, p := range served {
				if !EndpointMatch(p.Method, p.PathPattern, c.Method, c.PathPattern) {
					continue
				}
				hit := ConsumerHit{
					Repo:          consumerRepoName(repoName, c),
					CallingFile:   c.HandlerFile,
					CallingSymbol: metaString(c.Metadata, "calling_symbol"),
					MatchedRoute:  routeLabel(p.Method, p.PathPattern),
					Endpoint:      routeLabel(c.Method, c.PathPattern),
				}
				key := hit.Repo + "\x00" + hit.CallingFile + "\x00" + hit.MatchedRoute + "\x00" + hit.Endpoint
				if hitSeen[key] {
					break
				}
				hitSeen[key] = true
				res.Impacted = append(res.Impacted, hit)
				if rk := strings.ToLower(hit.Repo); !repoSeen[rk] {
					repoSeen[rk] = true
					res.ConsumerRepos = append(res.ConsumerRepos, hit.Repo)
				}
				break // one match per consumer route is enough
			}
		}
	}
	sort.Slice(res.Impacted, func(i, j int) bool {
		if res.Impacted[i].Repo != res.Impacted[j].Repo {
			return res.Impacted[i].Repo < res.Impacted[j].Repo
		}
		if res.Impacted[i].CallingFile != res.Impacted[j].CallingFile {
			return res.Impacted[i].CallingFile < res.Impacted[j].CallingFile
		}
		return res.Impacted[i].MatchedRoute < res.Impacted[j].MatchedRoute
	})
	sort.Strings(res.ConsumerRepos)
	return res, nil
}

// Consumers is Impact with no changed-path filter: every consumer of any route
// this repo serves.
func Consumers(ctx context.Context, drv store.StorageDriver, repoFullName string) (CrossRepoResult, error) {
	return Impact(ctx, drv, repoFullName, nil)
}

// RouteContracts returns the producer routes the repo serves (its public HTTP
// contract).
func RouteContracts(ctx context.Context, drv store.StorageDriver, repoFullName string) ([]graph.Route, error) {
	repos, err := drv.ListRepos(ctx, "")
	if err != nil {
		return nil, err
	}
	var repoID, fullName string
	for i := range repos {
		if strings.EqualFold(repos[i].FullName, repoFullName) {
			repoID = repos[i].ID
			fullName = repos[i].FullName
			break
		}
	}
	if repoID == "" {
		return []graph.Route{}, nil
	}
	snap, err := drv.LatestSnapshot(ctx, repoID)
	if err != nil {
		return nil, err
	}
	if snap == nil {
		return []graph.Route{}, nil
	}
	producers, err := drv.ListRoutes(ctx, snap.ID, "producer")
	if err != nil {
		return nil, err
	}
	for i := range producers {
		if producers[i].RepoFullName == "" {
			producers[i].RepoFullName = fullName
		}
	}
	return producers, nil
}

// routeLabel renders "METHOD path" (METHOD omitted when unknown).
func routeLabel(method, path string) string {
	m := strings.TrimSpace(strings.ToUpper(method))
	if m == "" {
		return path
	}
	return m + " " + path
}

// consumerRepoName prefers the route's own RepoFullName (the indexer stamps it),
// falling back to the repo row's full name.
func consumerRepoName(repoName string, c graph.Route) string {
	if strings.TrimSpace(c.RepoFullName) != "" {
		return c.RepoFullName
	}
	return repoName
}

// canonicalSet builds a set of canonical-form changed paths.
func canonicalSet(paths []string) map[string]bool {
	set := make(map[string]bool, len(paths))
	for _, p := range paths {
		if c := canonicalPath(p); c != "" {
			set[c] = true
		}
	}
	return set
}

// canonicalPath normalizes a file path for cross-form comparison: forward
// slashes, no leading "./", lowercased (case-insensitive filesystems / repos).
func canonicalPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = filepath.ToSlash(p)
	p = strings.TrimPrefix(p, "./")
	return strings.ToLower(p)
}

func metaString(m graph.JSONBMap, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
