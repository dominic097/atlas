// Package export serializes a slice of the code knowledge graph into portable
// formats: a graphify-compatible JSON graph, a Mermaid flowchart, and Graphviz
// DOT. It is deterministic (callers sort inputs) and depends only on its own
// Node/Edge types, so the engine maps graph.CodeSymbol -> export.Node.
package export

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Node is one symbol in the exported graph.
type Node struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Language string `json:"language,omitempty"`
}

// Edge is one directed relationship (caller -> callee) between two node IDs.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}

// Graph is a portable node/edge graph.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Render serializes the graph in the requested format ("json"|"mermaid"|"dot").
func (g Graph) Render(format string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", "json":
		return g.JSON()
	case "mermaid", "mmd":
		return g.Mermaid(), nil
	case "dot", "graphviz":
		return g.DOT(), nil
	default:
		return "", fmt.Errorf("export: unknown format %q (want json|mermaid|dot)", format)
	}
}

// JSON renders an indented node/edge JSON document.
func (g Graph) JSON() (string, error) {
	b, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Mermaid renders a flowchart (LR). Node IDs are remapped to safe nN tokens.
func (g Graph) Mermaid() string {
	id := make(map[string]string, len(g.Nodes))
	var b strings.Builder
	b.WriteString("flowchart LR\n")
	for i, n := range g.Nodes {
		sid := fmt.Sprintf("n%d", i)
		id[n.ID] = sid
		label := n.Name
		if n.Kind != "" {
			label = n.Name + " (" + n.Kind + ")"
		}
		fmt.Fprintf(&b, "  %s[%q]\n", sid, mermaidLabel(label))
	}
	for _, e := range g.Edges {
		f, ok1 := id[e.From]
		t, ok2 := id[e.To]
		if ok1 && ok2 {
			fmt.Fprintf(&b, "  %s --> %s\n", f, t)
		}
	}
	return b.String()
}

// DOT renders a Graphviz digraph.
func (g Graph) DOT() string {
	id := make(map[string]string, len(g.Nodes))
	var b strings.Builder
	b.WriteString("digraph atlas {\n  rankdir=LR;\n  node [shape=box];\n")
	for i, n := range g.Nodes {
		sid := fmt.Sprintf("n%d", i)
		id[n.ID] = sid
		fmt.Fprintf(&b, "  %s [label=%q];\n", sid, n.Name)
	}
	for _, e := range g.Edges {
		if f, ok := id[e.From]; ok {
			if t, ok2 := id[e.To]; ok2 {
				fmt.Fprintf(&b, "  %s -> %s;\n", f, t)
			}
		}
	}
	b.WriteString("}\n")
	return b.String()
}

func mermaidLabel(s string) string {
	s = strings.ReplaceAll(s, "\"", "'")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}
