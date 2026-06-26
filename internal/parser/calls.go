package parser

import (
	"strings"

	"github.com/dominic097/atlas/internal/graph"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// extractCallEdges is the AST-based call-edge entry point for the tree-sitter
// languages (java/cpp/python/javascript/typescript). It REPLACES the line-scan
// textCallEdges: instead of regex-matching call-shaped text, each per-language
// extractor walks the ALREADY-PARSED AST root, attributes every call expression
// to its enclosing symbol, and — the precision win — records the callee's
// qualified reference (and a receiver type where the grammar makes it feasible)
// in edge metadata so the query layer's resolveTargets can defeat method-name
// collisions exactly as it already does for Go.
//
// The per-language bodies live in calls_<lang>.go and are filled in by separate
// agents; the dispatcher and the shared edge/lookup helpers live here.
func extractCallEdges(lang string, root *tree_sitter.Node, source []byte, repoID, repoFullName, filePath string, syms []graph.CodeSymbol) []graph.DependencyEdge {
	if root == nil {
		return nil
	}
	switch lang {
	case "java":
		return javaCallEdges(root, source, repoID, repoFullName, filePath, syms)
	case "cpp":
		return cppCallEdges(root, source, repoID, repoFullName, filePath, syms)
	case "python":
		return pyCallEdges(root, source, repoID, repoFullName, filePath, syms)
	case "javascript", "typescript":
		// jsCallEdges handles both JavaScript and TypeScript (shared grammar shape).
		return jsCallEdges(root, source, repoID, repoFullName, filePath, syms)
	default:
		return nil
	}
}

// enclosingSymbolName returns the name of the innermost symbol whose
// [StartLine, EndLine] span contains the given (1-based) line — the caller
// attribution for a call at that line. "Innermost" = the smallest containing
// span, so a method inside a class wins over the class. Returns "" when no
// symbol contains the line (e.g. a top-level call outside any function).
func enclosingSymbolName(syms []graph.CodeSymbol, line int) string {
	if line <= 0 {
		return ""
	}
	bestName := ""
	bestSpan := 0
	for _, s := range syms {
		if s.StartLine <= 0 || s.EndLine <= 0 {
			continue
		}
		if line < s.StartLine || line > s.EndLine {
			continue
		}
		span := s.EndLine - s.StartLine
		if bestName == "" || span < bestSpan {
			bestName = s.Name
			bestSpan = span
		}
	}
	return bestName
}

// newCallEdge builds one EdgeCalls edge for a tree-sitter language, mirroring the
// metadata contract goCallEdges produces so resolveTargets treats AST call edges
// from every language uniformly:
//   - qualified_ref: the qualified callee form (e.g. "obj.method", "pkg.Foo") or
//     the bare name when unqualified; "" is allowed.
//   - recv_type: the statically inferred base receiver type, OMITTED when "" so
//     edgeRecvType reads it as unknown (best-effort dispatch).
//   - source: "<lang>_ts" tags the AST tree-sitter origin (vs Go's "go_ast").
//   - analysis_level: "ts_call_expression".
//
// Dedupe is the CALLER's responsibility (use dedupeEdges over the collected set).
func newCallEdge(repoID, filePath, fromSymbol, toRef, qualifiedRef, recvType, lang string, line int) graph.DependencyEdge {
	meta := graph.JSONBMap{
		"qualified_ref":  qualifiedRef,
		"source":         lang + "_ts",
		"analysis_level": "ts_call_expression",
	}
	if rt := strings.TrimSpace(recvType); rt != "" {
		meta["recv_type"] = rt
	}
	return graph.DependencyEdge{
		ID:         newUUID(),
		FromFile:   filePath,
		FromSymbol: fromSymbol,
		ToRef:      toRef,
		Kind:       graph.EdgeCalls,
		Language:   lang,
		Line:       line,
		Metadata:   meta,
	}
}
