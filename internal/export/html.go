package export

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// HTMLOptions tunes the self-contained HTML visualization. The zero value is
// valid: TopN falls back to defaultHTMLTopN, and Title falls back to "Atlas graph".
type HTMLOptions struct {
	// Title is the document/graph title (a prefix; the node/edge counts are
	// always appended). Empty -> "Atlas graph".
	Title string
	// TopN caps the rendered node count: only the top-N nodes by total degree are
	// kept (with their induced edges) so large graphs stay legible and the file
	// stays small. Non-positive -> defaultHTMLTopN. The title notes any capping.
	TopN int
}

// defaultHTMLTopN bounds the rendered node count for large graphs. graphify's
// graph.html gets unusable well before this; capping keeps the SVG and the file
// small while preserving the highest-degree (most architecturally central) nodes.
const defaultHTMLTopN = 300

// htmlNode is the per-node payload embedded in the page: the export.Node fields
// plus the deterministic analytics (total degree -> size, community -> color).
type htmlNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Language  string `json:"language,omitempty"`
	Degree    int    `json:"degree"`
	Community int    `json:"community"`
}

// htmlPayload is the full JSON document embedded in the page's <script>.
type htmlPayload struct {
	Title       string     `json:"title"`
	TotalNodes  int        `json:"totalNodes"`
	TotalEdges  int        `json:"totalEdges"`
	ShownNodes  int        `json:"shownNodes"`
	ShownEdges  int        `json:"shownEdges"`
	Communities int        `json:"communities"`
	Nodes       []htmlNode `json:"nodes"`
	Edges       []Edge     `json:"edges"`
}

// HTML renders a SINGLE self-contained, dependency-free interactive HTML page
// visualizing the graph. No network or CDN is needed to render it: the graph data
// is embedded as JSON and a compact vanilla-JS seeded force-directed renderer is
// inlined. Output is DETERMINISTIC — the same Graph yields a byte-identical page
// (node positions seed from a hash of the node id, fixed iteration count, no
// Math.random). Node SIZE encodes total degree; node COLOR encodes community id
// (deterministic label propagation, categorical palette). For large graphs only
// the top-N nodes by degree are kept (with their induced edges); the title notes
// "showing top N of M".
func (g Graph) HTML(opts HTMLOptions) (string, error) {
	topN := opts.TopN
	if topN <= 0 {
		topN = defaultHTMLTopN
	}
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = "Atlas graph"
	}

	payload := g.buildHTMLPayload(title, topN)
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n")
	b.WriteString(`<html lang="en"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width, initial-scale=1">`)
	b.WriteString("<title>")
	b.WriteString(htmlEscape(payload.Title))
	b.WriteString("</title>\n<style>\n")
	b.WriteString(htmlCSS)
	b.WriteString("\n</style></head>\n<body>\n")
	b.WriteString(htmlBody)
	// Embed the graph data, then the renderer. Closing-tag-injection is impossible
	// here because json.Marshal escapes "<" / ">" via Go's HTMLEscape default only
	// for &<> in strings; we additionally guard "</script" defensively.
	b.WriteString(`<script id="atlas-graph" type="application/json">`)
	b.WriteString(scriptSafe(string(data)))
	b.WriteString("</script>\n<script>\n")
	b.WriteString(htmlJS)
	b.WriteString("\n</script>\n</body></html>\n")
	return b.String(), nil
}

// buildHTMLPayload computes per-node degree + community, applies the top-N cap,
// and assembles the embeddable payload. Pure + deterministic.
func (g Graph) buildHTMLPayload(title string, topN int) htmlPayload {
	// Degree (undirected, distinct neighbors) over the full graph's edges.
	idSet := make(map[string]struct{}, len(g.Nodes))
	for _, n := range g.Nodes {
		idSet[n.ID] = struct{}{}
	}
	neighbors := make(map[string]map[string]struct{}, len(g.Nodes))
	addAdj := func(a, b string) {
		s := neighbors[a]
		if s == nil {
			s = make(map[string]struct{})
			neighbors[a] = s
		}
		s[b] = struct{}{}
	}
	for _, e := range g.Edges {
		_, ok1 := idSet[e.From]
		_, ok2 := idSet[e.To]
		if !ok1 || !ok2 || e.From == e.To {
			continue
		}
		addAdj(e.From, e.To)
		addAdj(e.To, e.From)
	}
	degree := make(map[string]int, len(g.Nodes))
	for _, n := range g.Nodes {
		degree[n.ID] = len(neighbors[n.ID])
	}

	community := communityLabels(g.Nodes, neighbors)

	// Top-N selection by total degree, ties by ascending id (deterministic).
	kept := append([]Node(nil), g.Nodes...)
	sort.SliceStable(kept, func(i, j int) bool {
		di, dj := degree[kept[i].ID], degree[kept[j].ID]
		if di != dj {
			return di > dj
		}
		return kept[i].ID < kept[j].ID
	})
	capped := topN > 0 && topN < len(kept)
	if capped {
		kept = kept[:topN]
	}
	keptSet := make(map[string]struct{}, len(kept))
	for _, n := range kept {
		keptSet[n.ID] = struct{}{}
	}

	// Emit nodes in stable id order so the JSON (and thus the page) is identical
	// across runs regardless of the input node ordering.
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].ID < kept[j].ID })
	nodes := make([]htmlNode, 0, len(kept))
	for _, n := range kept {
		nodes = append(nodes, htmlNode{
			ID:        n.ID,
			Name:      n.Name,
			Kind:      n.Kind,
			Path:      n.Path,
			Line:      n.Line,
			Language:  n.Language,
			Degree:    degree[n.ID],
			Community: community[n.ID],
		})
	}

	// Induced edges among kept nodes, deduped + stably ordered.
	type ek struct{ from, to string }
	seen := make(map[ek]struct{})
	edges := make([]Edge, 0, len(g.Edges))
	for _, e := range g.Edges {
		if _, ok := keptSet[e.From]; !ok {
			continue
		}
		if _, ok := keptSet[e.To]; !ok {
			continue
		}
		k := ek{e.From, e.To}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		edges = append(edges, Edge{From: e.From, To: e.To, Kind: e.Kind})
	}
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		if edges[i].To != edges[j].To {
			return edges[i].To < edges[j].To
		}
		return edges[i].Kind < edges[j].Kind
	})

	// Distinct community count among the kept nodes (for the legend/title).
	distinct := make(map[int]struct{}, len(nodes))
	for _, n := range nodes {
		distinct[n.Community] = struct{}{}
	}

	fullTitle := fmt.Sprintf("%s — %d nodes, %d edges", title, len(nodes), len(edges))
	if capped {
		fullTitle = fmt.Sprintf("%s — showing top %d of %d nodes, %d edges", title, len(nodes), len(g.Nodes), len(edges))
	}

	return htmlPayload{
		Title:       fullTitle,
		TotalNodes:  len(g.Nodes),
		TotalEdges:  len(g.Edges),
		ShownNodes:  len(nodes),
		ShownEdges:  len(edges),
		Communities: len(distinct),
		Nodes:       nodes,
		Edges:       edges,
	}
}

// htmlCommunityIters bounds the deterministic label-propagation sweep used to
// color nodes. Mirrors internal/analytics/community.go's approach but operates
// directly on the export node-id graph so the export package stays dependency-light.
const htmlCommunityIters = 20

// communityLabels assigns a small deterministic integer community id to every node
// via label propagation over the undirected adjacency. Final ids are dense and
// assigned by DESCENDING community size (ties by smallest member id), so the
// largest cluster is community 0 — stable across runs.
func communityLabels(nodes []Node, neighbors map[string]map[string]struct{}) map[string]int {
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	sort.Strings(ids)

	// Sorted neighbor lists for deterministic sweeps.
	adj := make(map[string][]string, len(ids))
	for _, id := range ids {
		if set := neighbors[id]; len(set) > 0 {
			lst := make([]string, 0, len(set))
			for nb := range set {
				lst = append(lst, nb)
			}
			sort.Strings(lst)
			adj[id] = lst
		}
	}

	label := make(map[string]string, len(ids))
	for _, id := range ids {
		label[id] = id
	}
	for iter := 0; iter < htmlCommunityIters; iter++ {
		changed := false
		for _, id := range ids {
			nbs := adj[id]
			if len(nbs) == 0 {
				continue
			}
			best := dominantHTMLLabel(label, id, nbs)
			if best != label[id] {
				label[id] = best
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	// Group by final label and rank groups by descending size (ties by smallest
	// member id), then assign dense ids 0..k-1.
	groups := make(map[string][]string)
	for _, id := range ids {
		groups[label[id]] = append(groups[label[id]], id)
	}
	type grp struct {
		smallest string
		members  []string
	}
	gs := make([]grp, 0, len(groups))
	for _, members := range groups {
		sort.Strings(members)
		gs = append(gs, grp{smallest: members[0], members: members})
	}
	sort.SliceStable(gs, func(i, j int) bool {
		if len(gs[i].members) != len(gs[j].members) {
			return len(gs[i].members) > len(gs[j].members)
		}
		return gs[i].smallest < gs[j].smallest
	})
	out := make(map[string]int, len(ids))
	for cid, g := range gs {
		for _, m := range g.members {
			out[m] = cid
		}
	}
	return out
}

// dominantHTMLLabel returns the most-frequent label among self+neighbors, ties
// broken by smallest label — the deterministic label-propagation update rule.
func dominantHTMLLabel(label map[string]string, self string, neighbors []string) string {
	counts := make(map[string]int, len(neighbors)+1)
	counts[label[self]]++
	for _, nb := range neighbors {
		counts[label[nb]]++
	}
	cands := make([]string, 0, len(counts))
	for l := range counts {
		cands = append(cands, l)
	}
	sort.Strings(cands)
	best := cands[0]
	bestCount := counts[best]
	for _, l := range cands[1:] {
		if counts[l] > bestCount {
			best, bestCount = l, counts[l]
		}
	}
	return best
}

// htmlEscape escapes the five HTML metacharacters for text in element bodies.
func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return r.Replace(s)
}

// scriptSafe neutralizes any literal "</script" sequence inside embedded JSON so
// the data block can't terminate its own <script> element. The escaped form is
// still valid JSON (the parser un-escapes "<\/script").
func scriptSafe(s string) string {
	return strings.ReplaceAll(s, "</script", "<\\/script")
}
