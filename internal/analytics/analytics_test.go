package analytics

import (
	"reflect"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// bridgedGraph builds two clearly-separate clusters linked by one bridge edge:
//
//	Cluster 1: A <-> B <-> C  (A-B, B-C calls)
//	Cluster 2: X <-> Y <-> Z  (X-Y, Y-Z calls)
//	Bridge:    C -> X         (the single edge joining the two clusters)
//
// The graph also includes:
//   - an ISOLATED symbol "Lonely" with no call edges,
//   - an unresolved/external call edge (A -> External) that must be dropped,
//   - an import edge (counted in EdgeKinds but not in the call graph).
func bridgedGraph() ([]graph.CodeSymbol, []graph.DependencyEdge) {
	syms := []graph.CodeSymbol{
		{ID: "s1", Path: "c1.go", Language: "go", Kind: "function", Name: "A", StartLine: 1, EndLine: 3},
		{ID: "s2", Path: "c1.go", Language: "go", Kind: "function", Name: "B", StartLine: 5, EndLine: 7},
		{ID: "s3", Path: "c1.go", Language: "go", Kind: "function", Name: "C", StartLine: 9, EndLine: 11},
		{ID: "s4", Path: "c2.py", Language: "python", Kind: "function", Name: "X", StartLine: 1, EndLine: 3},
		{ID: "s5", Path: "c2.py", Language: "python", Kind: "function", Name: "Y", StartLine: 5, EndLine: 7},
		{ID: "s6", Path: "c2.py", Language: "python", Kind: "function", Name: "Z", StartLine: 9, EndLine: 11},
		{ID: "s7", Path: "lonely.go", Language: "go", Kind: "function", Name: "Lonely", StartLine: 1, EndLine: 2},
	}
	edges := []graph.DependencyEdge{
		// Cluster 1.
		{ID: "e1", FromFile: "c1.go", FromSymbol: "A", ToRef: "B", Kind: graph.EdgeCalls},
		{ID: "e2", FromFile: "c1.go", FromSymbol: "B", ToRef: "C", Kind: graph.EdgeCalls},
		// Cluster 2.
		{ID: "e3", FromFile: "c2.py", FromSymbol: "X", ToRef: "Y", Kind: graph.EdgeCalls},
		{ID: "e4", FromFile: "c2.py", FromSymbol: "Y", ToRef: "Z", Kind: graph.EdgeCalls},
		// Bridge.
		{ID: "e5", FromFile: "c1.go", FromSymbol: "C", ToRef: "X", Kind: graph.EdgeCalls},
		// Unresolved external callee — must be dropped from the call graph.
		{ID: "e6", FromFile: "c1.go", FromSymbol: "A", ToRef: "External", Kind: graph.EdgeCalls},
		// Non-call edge — counted in EdgeKinds, not in the call graph.
		{ID: "e7", FromFile: "c1.go", FromSymbol: "A", ToRef: "B", Kind: graph.EdgeImports},
	}
	return syms, edges
}

func TestCommunitiesFindTwoClusters(t *testing.T) {
	syms, edges := bridgedGraph()
	g := Build(syms, edges)

	comms := g.Communities()

	// With one bridge edge, label propagation may merge across the bridge depending
	// on the topology; the firm invariant is that the isolated node is its OWN
	// community and each cluster's members stay together. We assert the partition
	// keeps cluster-1 ({A,B,C}) members in one community and cluster-2 ({X,Y,Z}) in
	// one community, and Lonely alone.
	commOf := map[string]int{}
	for _, c := range comms {
		for _, m := range c.Members {
			commOf[m] = c.ID
		}
	}

	if commOf["A"] != commOf["B"] || commOf["B"] != commOf["C"] {
		t.Errorf("cluster 1 {A,B,C} split across communities: %v", commOf)
	}
	if commOf["X"] != commOf["Y"] || commOf["Y"] != commOf["Z"] {
		t.Errorf("cluster 2 {X,Y,Z} split across communities: %v", commOf)
	}
	if commOf["Lonely"] == commOf["A"] || commOf["Lonely"] == commOf["X"] {
		t.Errorf("isolated node Lonely was merged into a cluster: %v", commOf)
	}

	// The isolated node forms a singleton community.
	var lonely *Community
	for i := range comms {
		if len(comms[i].Members) == 1 && comms[i].Members[0] == "Lonely" {
			lonely = &comms[i]
		}
	}
	if lonely == nil {
		t.Fatalf("expected a singleton community for Lonely, communities = %+v", comms)
	}
	if lonely.Size != 1 {
		t.Errorf("Lonely community size = %d, want 1", lonely.Size)
	}

	// IDs are assigned by descending size; the first community is the largest.
	for i := 1; i < len(comms); i++ {
		if comms[i-1].Size < comms[i].Size {
			t.Errorf("communities not ordered by descending size: %+v", comms)
		}
		if comms[i].ID != i {
			t.Errorf("community ID = %d at index %d, want stable index id", comms[i].ID, i)
		}
	}
}

func TestBridgeNodeDegree(t *testing.T) {
	syms, edges := bridgedGraph()
	g := Build(syms, edges)

	// C is called by B (in=1) and calls X (out=1) -> total 2.
	in, out, total := g.Degree("C")
	if in != 1 || out != 1 || total != 2 {
		t.Errorf("C degree = in%d/out%d/total%d, want in1/out1/total2", in, out, total)
	}

	// X is called by C (in=1) and calls Y (out=1) -> total 2.
	in, out, total = g.Degree("X")
	if in != 1 || out != 1 || total != 2 {
		t.Errorf("X degree = in%d/out%d/total%d, want in1/out1/total2", in, out, total)
	}

	// B is the busiest non-bridge node: called by A (in=1), calls C (out=1) -> 2.
	in, out, total = g.Degree("B")
	if in != 1 || out != 1 || total != 2 {
		t.Errorf("B degree = in%d/out%d/total%d, want in1/out1/total2", in, out, total)
	}

	// The dropped external edge must NOT inflate A's out-degree (A calls only B).
	in, out, total = g.Degree("A")
	if in != 0 || out != 1 || total != 1 {
		t.Errorf("A degree = in%d/out%d/total%d, want in0/out1/total1 (External dropped)", in, out, total)
	}

	// Isolated node has zero degree.
	in, out, total = g.Degree("Lonely")
	if in != 0 || out != 0 || total != 0 {
		t.Errorf("Lonely degree = in%d/out%d/total%d, want all zero", in, out, total)
	}
}

func TestHubsOrdering(t *testing.T) {
	syms, edges := bridgedGraph()
	g := Build(syms, edges)

	hubs := g.Hubs(3)
	if len(hubs) != 3 {
		t.Fatalf("Hubs(3) returned %d, want 3", len(hubs))
	}

	// Degrees: B,C,X,Y all = 2; A,Z = 1; Lonely = 0.
	// Top-3 by total degree, ties by name => B, C, X.
	wantNames := []string{"B", "C", "X"}
	for i, w := range wantNames {
		if hubs[i].Name != w {
			t.Errorf("Hubs[%d].Name = %q, want %q (full=%+v)", i, hubs[i].Name, w, hubs)
		}
		if hubs[i].TotalDegree != 2 {
			t.Errorf("Hubs[%d] (%s) TotalDegree = %d, want 2", i, hubs[i].Name, hubs[i].TotalDegree)
		}
	}

	// Hub carries the representative symbol's path/kind/language.
	if hubs[0].Path != "c1.go" || hubs[0].Language != "go" || hubs[0].Kind != "function" {
		t.Errorf("Hub B metadata = %+v, want path=c1.go lang=go kind=function", hubs[0])
	}

	// topN <= 0 returns all nodes, still ranked.
	all := g.Hubs(0)
	if len(all) != 7 {
		t.Errorf("Hubs(0) returned %d, want all 7 nodes", len(all))
	}
	// Lonely (degree 0) ranks last.
	if all[len(all)-1].Name != "Lonely" {
		t.Errorf("last hub = %q, want Lonely (lowest degree)", all[len(all)-1].Name)
	}
}

func TestStatsTotals(t *testing.T) {
	syms, edges := bridgedGraph()
	g := Build(syms, edges)

	s := g.Stats()

	if s.Symbols != 7 {
		t.Errorf("Symbols = %d, want 7 distinct names", s.Symbols)
	}
	if s.Files != 3 {
		t.Errorf("Files = %d, want 3 (c1.go, c2.py, lonely.go)", s.Files)
	}
	// Resolved name-level call edges: A->B, B->C, X->Y, Y->Z, C->X = 5.
	// External callee dropped; import edge not a call edge.
	if s.Edges != 5 {
		t.Errorf("Edges = %d, want 5 resolved call edges", s.Edges)
	}
	if s.RawEdges != 7 {
		t.Errorf("RawEdges = %d, want 7 input edges", s.RawEdges)
	}
	if s.EdgeKinds["calls"] != 6 {
		t.Errorf("EdgeKinds[calls] = %d, want 6 raw call edges (incl. dropped External)", s.EdgeKinds["calls"])
	}
	if s.EdgeKinds["imports"] != 1 {
		t.Errorf("EdgeKinds[imports] = %d, want 1", s.EdgeKinds["imports"])
	}
	if s.Languages["go"] != 4 {
		t.Errorf("Languages[go] = %d, want 4 (A,B,C,Lonely)", s.Languages["go"])
	}
	if s.Languages["python"] != 3 {
		t.Errorf("Languages[python] = %d, want 3 (X,Y,Z)", s.Languages["python"])
	}
	if s.IsolatedNodes != 1 {
		t.Errorf("IsolatedNodes = %d, want 1 (Lonely)", s.IsolatedNodes)
	}
	if s.Communities < 2 {
		t.Errorf("Communities = %d, want at least 2 clusters + isolated", s.Communities)
	}
}

func TestPageRankDeterministicAndScored(t *testing.T) {
	syms, edges := bridgedGraph()
	g := Build(syms, edges)

	pr := g.PageRank()
	if len(pr) != 7 {
		t.Fatalf("PageRank returned %d scores, want 7", len(pr))
	}

	// Scores should sum to ~1.0 (mass conserving with dangling redistribution).
	var sum float64
	for _, p := range pr {
		sum += p.Score
		if p.Score <= 0 {
			t.Errorf("PageRank[%s] = %g, want positive", p.Name, p.Score)
		}
	}
	if sum < 0.999 || sum > 1.001 {
		t.Errorf("PageRank sum = %g, want ~1.0", sum)
	}

	// Sink-ish nodes that receive the chain's flow should outrank pure sources.
	top := g.TopByPageRank(3)
	if len(top) != 3 {
		t.Fatalf("TopByPageRank(3) returned %d, want 3", len(top))
	}
	// X and Z receive flow from the chain (C->X->Y->Z), so they should rank above A
	// (a pure source called by no one).
	scoreOf := map[string]float64{}
	for _, p := range pr {
		scoreOf[p.Name] = p.Score
	}
	if scoreOf["X"] <= scoreOf["A"] {
		t.Errorf("expected X (%g) to outrank source A (%g)", scoreOf["X"], scoreOf["A"])
	}
}

// TestRunToRunStability asserts every accessor is bit-for-bit stable across two
// independently-built Graphs from equal input — the determinism contract.
func TestRunToRunStability(t *testing.T) {
	syms, edges := bridgedGraph()

	g1 := Build(syms, edges)
	g2 := Build(syms, edges)

	if !reflect.DeepEqual(g1.Communities(), g2.Communities()) {
		t.Errorf("Communities() not stable across runs:\n%+v\n%+v", g1.Communities(), g2.Communities())
	}
	if !reflect.DeepEqual(g1.Hubs(5), g2.Hubs(5)) {
		t.Errorf("Hubs(5) not stable across runs")
	}
	if !reflect.DeepEqual(g1.Stats(), g2.Stats()) {
		t.Errorf("Stats() not stable across runs:\n%+v\n%+v", g1.Stats(), g2.Stats())
	}
	if !reflect.DeepEqual(g1.PageRank(), g2.PageRank()) {
		t.Errorf("PageRank() not stable across runs")
	}
	if !reflect.DeepEqual(g1.TopByPageRank(4), g2.TopByPageRank(4)) {
		t.Errorf("TopByPageRank(4) not stable across runs")
	}

	// Same Graph, called twice, must also be identical.
	if !reflect.DeepEqual(g1.Communities(), g1.Communities()) {
		t.Errorf("Communities() not idempotent on the same Graph")
	}
}

// TestEmptyGraph guards the degenerate input path.
func TestEmptyGraph(t *testing.T) {
	g := Build(nil, nil)
	if len(g.Communities()) != 0 {
		t.Errorf("empty Communities() = %d, want 0", len(g.Communities()))
	}
	if len(g.Hubs(5)) != 0 {
		t.Errorf("empty Hubs() = %d, want 0", len(g.Hubs(5)))
	}
	if len(g.PageRank()) != 0 {
		t.Errorf("empty PageRank() = %d, want 0", len(g.PageRank()))
	}
	s := g.Stats()
	if s.Symbols != 0 || s.Edges != 0 || s.Files != 0 || s.Communities != 0 {
		t.Errorf("empty Stats = %+v, want all zero", s)
	}
}

// TestSelfRecursionDirectedNotUndirected verifies a self-call adds a directed
// self-loop (affecting in/out degree) without creating a phantom undirected edge.
func TestSelfRecursionDirectedNotUndirected(t *testing.T) {
	syms := []graph.CodeSymbol{
		{ID: "s1", Path: "r.go", Language: "go", Kind: "function", Name: "Rec", StartLine: 1, EndLine: 5},
	}
	edges := []graph.DependencyEdge{
		{ID: "e1", FromFile: "r.go", FromSymbol: "Rec", ToRef: "Rec", Kind: graph.EdgeCalls},
	}
	g := Build(syms, edges)

	in, out, total := g.Degree("Rec")
	if in != 1 || out != 1 || total != 2 {
		t.Errorf("Rec self-call degree = in%d/out%d/total%d, want in1/out1/total2", in, out, total)
	}
	// Self-recursion must not crash community detection; Rec is its own community.
	comms := g.Communities()
	if len(comms) != 1 || comms[0].Members[0] != "Rec" {
		t.Errorf("self-recursive node communities = %+v, want single {Rec}", comms)
	}
}

// TestIdentityAwareSplitsSameNamedMethods is the keystone regression: two distinct
// Close methods (on different receiver types, in different files) must NOT collapse
// into one fake god-node. They become two DISTINCT identity nodes — each qualified
// by its receiver type — with their own per-identity degree, and the report shows
// "aCloser.Close" / "bCloser.Close" so they are distinguishable.
func TestIdentityAwareSplitsSameNamedMethods(t *testing.T) {
	syms := []graph.CodeSymbol{
		// Two types, each with its own Close method (same bare name).
		{ID: "ta", Path: "a/a.go", Language: "go", Kind: "type", Name: "aCloser", StartLine: 1, EndLine: 2},
		{ID: "tb", Path: "b/b.go", Language: "go", Kind: "type", Name: "bCloser", StartLine: 1, EndLine: 2},
		{ID: "ca", Path: "a/a.go", Language: "go", Kind: "method", Name: "Close", StartLine: 5, EndLine: 7,
			Metadata: graph.JSONBMap{"recv_type": "aCloser"}},
		{ID: "cb", Path: "b/b.go", Language: "go", Kind: "method", Name: "Close", StartLine: 5, EndLine: 7,
			Metadata: graph.JSONBMap{"recv_type": "bCloser"}},
		// Two callers, each calling a different Close via a typed receiver.
		{ID: "ua", Path: "a/a.go", Language: "go", Kind: "function", Name: "UseA", StartLine: 10, EndLine: 12},
		{ID: "ub", Path: "b/b.go", Language: "go", Kind: "function", Name: "UseB", StartLine: 10, EndLine: 12},
	}
	edges := []graph.DependencyEdge{
		// UseA calls a.Close() on an aCloser — resolves ONLY to aCloser.Close.
		{ID: "e1", FromFile: "a/a.go", FromSymbol: "UseA", ToRef: "Close", Kind: graph.EdgeCalls,
			Metadata: graph.JSONBMap{"qualified_ref": "a.Close", "recv_type": "aCloser"}},
		// UseB calls b.Close() on a bCloser — resolves ONLY to bCloser.Close.
		{ID: "e2", FromFile: "b/b.go", FromSymbol: "UseB", ToRef: "Close", Kind: graph.EdgeCalls,
			Metadata: graph.JSONBMap{"qualified_ref": "b.Close", "recv_type": "bCloser"}},
	}
	g := Build(syms, edges)

	// There must be TWO distinct Close hubs, not one collapsed node.
	var closeHubs []Hub
	for _, h := range g.Hubs(0) {
		if h.BareName == "Close" {
			closeHubs = append(closeHubs, h)
		}
	}
	if len(closeHubs) != 2 {
		t.Fatalf("expected 2 distinct Close identities, got %d: %+v", len(closeHubs), closeHubs)
	}
	byLabel := map[string]Hub{}
	for _, h := range closeHubs {
		byLabel[h.Name] = h
	}
	a, okA := byLabel["aCloser.Close"]
	b, okB := byLabel["bCloser.Close"]
	if !okA || !okB {
		t.Fatalf("Close identities not qualified by receiver type: %+v", closeHubs)
	}
	// Each Close has in-degree 1 (its own caller), NOT 2 (the collapsed total).
	if a.InDegree != 1 || a.Path != "a/a.go" {
		t.Errorf("aCloser.Close = in%d %s, want in1 a/a.go", a.InDegree, a.Path)
	}
	if b.InDegree != 1 || b.Path != "b/b.go" {
		t.Errorf("bCloser.Close = in%d %s, want in1 b/b.go", b.InDegree, b.Path)
	}

	// The precise receiver-type resolution must NOT cross-link: UseA does not reach
	// bCloser.Close. So aCloser.Close and bCloser.Close are in SEPARATE communities.
	comms := g.Communities()
	commOf := map[string]int{}
	for _, c := range comms {
		for _, m := range c.Members {
			commOf[m] = c.ID
		}
	}
	if commOf["aCloser.Close"] == commOf["bCloser.Close"] {
		t.Errorf("distinct Close methods wrongly merged into one community: %v", commOf)
	}
}
