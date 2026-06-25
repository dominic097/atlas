// Package parser is Atlas's multi-language code parser. It extracts symbols
// (functions, methods, types, classes, ...), import edges, and — the keystone
// Atlas adds over the pulse engine it is ported from — symbol-granular CALL
// edges that name the enclosing (caller) symbol and the callee reference.
//
// Go is parsed with the native go/parser for compiler-grade fidelity; every
// other supported language (python, javascript, typescript, java, c, cpp) is
// parsed with tree-sitter.
//
// Ported and adapted from aziron-pulse internal/service:
//   - tree_sitter_parser.go (walk*AST / extract*Import)
//   - code_intelligence_service.go (parseGoFile, parseGoCallEdges,
//     parseTextCallEdges, parseCodeFile, languageForPath,
//     symbolLeadingComments, symbolBodyExcerpts)
//
// Atlas additions over pulse: stable line-independent NodeID (ComputeNodeID),
// first-class graph.DependencyEdge call/import edges, doc/signature population.
package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// Result is the parse output for a single file.
type Result struct {
	Symbols []graph.CodeSymbol
	Edges   []graph.DependencyEdge
	Imports []string
}

// LanguageForPath maps a file path to its parser language, or "" when the
// extension is not one of the supported tree-sitter / native grammars.
func LanguageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".cxx", ".hpp", ".hxx", ".hh":
		return "cpp"
	default:
		return ""
	}
}

// Supported reports whether the language has a first-class parser.
func Supported(lang string) bool {
	switch lang {
	case "go", "python", "javascript", "typescript", "java", "c", "cpp":
		return true
	default:
		return false
	}
}

// ComputeNodeID derives a content-stable identity for a symbol node. It is the
// sha256 hex of repoFullName|path|kind|name|signature and — deliberately — does
// NOT include line numbers, so a symbol keeps the same NodeID across snapshots
// even when surrounding edits shift its position. This stability is what lets
// the temporal diff layer compute set differences between snapshots.
func ComputeNodeID(repoFullName, path, kind, name, signature string) graph.NodeID {
	h := sha256.Sum256([]byte(strings.Join([]string{
		repoFullName, path, kind, name, signature,
	}, "|")))
	return graph.NodeID(hex.EncodeToString(h[:]))
}

// Parse extracts symbols, import edges, and symbol-granular call edges from a
// single source file. repoFullName feeds the stable NodeID; repoID stamps each
// symbol's RepoID. SnapshotID is left empty here — the index layer assigns it
// when persisting (symbols/edges are snapshot-scoped at save time).
func Parse(repoID, repoFullName, filePath, language string, content []byte) (Result, error) {
	if language == "" {
		language = LanguageForPath(filePath)
	}

	var (
		rawSyms []symbolDraft
		imports []string
		// tsRoot is the live tree-sitter AST root for non-Go languages, reused
		// by extractCallEdges below so we never re-parse. tsCleanup releases the
		// parser/tree and MUST run before Parse returns.
		tsRoot    *tree_sitter.Node
		tsCleanup = func() {}
	)
	defer func() { tsCleanup() }()

	switch language {
	case "go":
		// Native go/parser is the highest-fidelity path.
		rawSyms, imports = parseGoSymbols(filePath, content)
	case "python", "javascript", "typescript", "java", "c", "cpp":
		rawSyms, imports, tsRoot, tsCleanup = parseTreeSitter(filePath, language, content)
	default:
		return Result{}, nil
	}

	// Enrich each symbol with leading-comment Doc and a first-line Signature
	// fallback, then promote to the shared graph type with a stable NodeID.
	docs := leadingComments(content, rawSyms)
	symbols := make([]graph.CodeSymbol, 0, len(rawSyms))
	for _, d := range rawSyms {
		sig := d.signature
		if sig == "" {
			sig = firstLineSignature(content, d.startLine)
		}
		doc := docs[d.key()]
		sym := graph.CodeSymbol{
			ID:        newUUID(),
			RepoID:    repoID,
			Path:      filePath,
			Language:  language,
			Kind:      d.kind,
			Name:      d.name,
			Signature: sig,
			Doc:       doc,
			StartLine: d.startLine,
			EndLine:   d.endLine,
			Metadata:  graph.JSONBMap{},
		}
		sym.NodeID = ComputeNodeID(repoFullName, filePath, d.kind, d.name, sig)
		// SHARED METADATA CONTRACT: method symbols carry the base receiver
		// type so resolveTargets can match a call's receiver type to the
		// declaring type (defeats method-name collisions).
		if d.recvType != "" {
			sym.Metadata["recv_type"] = d.recvType
		}
		symbols = append(symbols, sym)
	}

	// Call edges (Atlas keystone): per call expression, FromSymbol = enclosing
	// symbol, ToRef = callee. Go uses the native go/parser path; the tree-sitter
	// languages walk the ALREADY-PARSED root (tsRoot) and attribute calls to the
	// promoted symbols by line span.
	edges := callEdges(repoID, repoFullName, filePath, language, content, tsRoot, symbols)

	// Import edges, one EdgeImports per imported module.
	imports = uniqueStrings(imports)
	for _, imp := range imports {
		edges = append(edges, graph.DependencyEdge{
			ID:       newUUID(),
			FromFile: filePath,
			ToRef:    imp,
			Kind:     graph.EdgeImports,
			Language: language,
			Metadata: graph.JSONBMap{"source": "import_declaration"},
		})
	}

	return Result{Symbols: symbols, Edges: edges, Imports: imports}, nil
}

// symbolDraft is the language-agnostic intermediate produced by each backend
// before promotion to graph.CodeSymbol.
type symbolDraft struct {
	name      string
	kind      string
	signature string
	startLine int
	endLine   int
	// recvType is the base receiver type a method is declared on (Go only,
	// per the SHARED METADATA CONTRACT). Empty for non-methods. Promoted to
	// CodeSymbol.Metadata["recv_type"] so the query layer can disambiguate
	// same-named methods on different types.
	recvType string
}

func (d symbolDraft) key() string {
	return d.kind + "\x00" + d.name + "\x00" + itoa(d.startLine)
}

// callEdges dispatches to the language-appropriate call-edge extractor.
//
//   - go: native go/parser (precise qualified_ref + recv_type).
//   - java/cpp/python/javascript/typescript: AST-based extractCallEdges over the
//     already-parsed tree-sitter root (tsRoot), deduped here. This REPLACES the
//     old line-scan textCallEdges, which is retained only as a fallback for any
//     future language that has no AST extractor (none of the five use it now).
func callEdges(repoID, repoFullName, filePath, language string, content []byte, tsRoot *tree_sitter.Node, symbols []graph.CodeSymbol) []graph.DependencyEdge {
	switch language {
	case "go":
		return goCallEdges(filePath, content)
	case "java", "cpp", "python", "javascript", "typescript":
		return dedupeEdges(extractCallEdges(language, tsRoot, content, repoID, repoFullName, filePath, symbols))
	case "c":
		// C shares the cpp grammar walker for symbols; route call edges through
		// the cpp extractor as well (same call_expression node shape).
		return dedupeEdges(extractCallEdges("cpp", tsRoot, content, repoID, repoFullName, filePath, symbols))
	default:
		return nil
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
