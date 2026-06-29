// Package parser is Atlas's multi-language code parser. It extracts symbols
// (functions, methods, types, classes, ...), import edges, and — the keystone
// Atlas adds over the pulse engine it is ported from — symbol-granular CALL
// edges that name the enclosing (caller) symbol and the callee reference.
//
// Go is parsed with the native go/parser for compiler-grade fidelity; every
// other supported language (python, javascript, typescript, java, c, cpp) is
// parsed with tree-sitter.
//
// Ported and adapted from the original parser service:
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

	"github.com/dominic097/atlas/internal/graph"
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
	base := strings.ToLower(filepath.Base(path))
	if strings.HasSuffix(base, ".blade.php") {
		return "blade"
	}
	if base == "dockerfile" || strings.HasPrefix(base, "dockerfile.") {
		return "dockerfile"
	}
	if base == "makefile" || base == "gnumakefile" {
		return "makefile"
	}
	if base == "postinstall" || base == "preinstall" {
		return "bash"
	}
	if base == "jenkinsfile" || strings.HasPrefix(base, "jenkinsfile.") {
		return "groovy"
	}
	switch base {
	case "go.mod":
		return "gomod"
	case "go.sum":
		return "gosum"
	case ".dockerignore", ".gitignore", ".nojekyll", ".python-version":
		return "config"
	}
	if strings.HasPrefix(base, ".env") || strings.Contains(base, ".env.") {
		return "config"
	}
	if lang := languageForBackupName(base); lang != "" {
		return lang
	}
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
	case ".cpp", ".cc", ".cxx", ".hpp", ".hxx", ".hh", ".cu", ".cuh":
		return "cpp"
	case ".cs":
		return "csharp"
	case ".rs":
		return "rust"
	case ".rb":
		return "ruby"
	case ".kt", ".kts":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".lua", ".luau", ".toc":
		return "lua"
	case ".zig":
		return "zig"
	case ".ex", ".exs":
		return "elixir"
	case ".m", ".mm":
		return "objc"
	case ".jl":
		return "julia"
	case ".f", ".f90", ".f95", ".f03", ".f08":
		return "fortran"
	case ".dart":
		return "dart"
	case ".v", ".sv", ".svh":
		return "verilog"
	case ".pas", ".pp", ".dpr", ".dpk", ".lpr", ".inc":
		return "pascal"
	case ".dfm", ".lfm", ".lpk":
		return "delphi"
	case ".tf", ".tfvars", ".hcl":
		return "terraform"
	case ".dm", ".dme", ".dmi", ".dmm", ".dmf":
		return "byond"
	case ".sln", ".slnx", ".csproj", ".fsproj", ".vbproj":
		return "dotnet"
	case ".razor", ".cshtml":
		return "razor"
	case ".cls", ".trigger":
		return "apex"
	case ".vue":
		return "vue"
	case ".svelte":
		return "svelte"
	case ".astro":
		return "astro"
	case ".p4":
		return "p4"
	case ".ejs":
		return "ejs"
	case ".ets":
		return "ets"
	case ".r":
		return "r"
	case ".html", ".htm":
		return "html"
	case ".css", ".scss", ".sass":
		return "css"
	case ".sh", ".bash":
		return "bash"
	case ".groovy", ".gvy", ".gradle":
		return "groovy"
	case ".md", ".markdown", ".mdown", ".mkd", ".qmd":
		return "markdown"
	case ".mdx":
		return "mdx"
	case ".yaml", ".yml":
		return "yaml"
	case ".json", ".jsonc", ".jsonl", ".code-workspace":
		return "json"
	case ".proto":
		return "proto"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".plist":
		return "plist"
	case ".ini", ".cfg", ".conf", ".cnf", ".service", ".properties", ".example", ".iss", ".alloy", ".lock":
		return "config"
	case ".bat", ".cmd":
		return "batch"
	case ".ps1", ".psm1", ".psd1":
		return "powershell"
	case ".sql":
		return "sql"
	case ".csv", ".tsv":
		return "csv"
	case ".txt", ".rst":
		return "text"
	case ".pptx":
		return "pptx"
	case ".docx":
		return "docx"
	case ".xlsx":
		return "xlsx"
	case ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff", ".webp":
		return "image"
	case ".pdf":
		return "pdf"
	default:
		return ""
	}
}

func languageForBackupName(base string) string {
	for _, suffix := range []string{".go.disabled", ".go.backup", ".go.bak"} {
		if strings.HasSuffix(base, suffix) {
			return "go"
		}
	}
	for _, suffix := range []string{".sql.bak", ".sql.backup"} {
		if strings.HasSuffix(base, suffix) {
			return "sql"
		}
	}
	for _, suffix := range []string{".json.orig", ".json.bak", ".json.backup"} {
		if strings.HasSuffix(base, suffix) {
			return "json"
		}
	}
	return ""
}

// Supported reports whether the language has a first-class parser.
func Supported(lang string) bool {
	switch lang {
	case "go", "python", "javascript", "typescript", "java", "c", "cpp",
		"rust", "ruby", "kotlin", "scala", "php", "swift", "lua", "zig",
		"elixir", "objc", "julia", "fortran", "dart", "verilog", "pascal",
		"delphi", "terraform", "byond", "dotnet", "razor", "apex", "blade",
		"vue", "svelte", "astro", "ejs", "ets", "r", "p4",
		"csharp", "groovy", "bash", "html", "css",
		"markdown", "mdx", "yaml", "json", "proto", "toml", "xml", "plist",
		"gomod", "gosum", "config", "makefile", "batch", "powershell",
		"sql", "csv", "text", "dockerfile",
		"pptx", "docx", "xlsx", "image", "pdf":
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
		goEdges   []graph.DependencyEdge
		textEdges []graph.DependencyEdge
	)
	defer func() { tsCleanup() }()

	switch language {
	case "pptx", "docx", "xlsx":
		// Binary office documents: extract searchable text directly into document
		// symbols. They have no source-code comments/bodies, so they bypass the
		// symbolDraft enrichment below and return their own Result.
		return parseOfficeDocument(repoID, repoFullName, filePath, language, content)
	case "image":
		// Images: catalog + optional OCR text, also returning their own Result.
		return parseImage(repoID, repoFullName, filePath, content)
	case "pdf":
		// PDF: extract page text into document/page symbols.
		return parsePDF(repoID, repoFullName, filePath, content)
	case "go":
		// Native go/parser is the highest-fidelity path. Parse once, then reuse
		// the AST for both symbol and call-edge extraction.
		rawSyms, imports, goEdges = parseGoFile(filePath, content)
	case "python", "javascript", "typescript", "java", "c", "cpp":
		rawSyms, imports, tsRoot, tsCleanup = parseTreeSitter(filePath, language, content)
	case "rust", "ruby", "csharp", "php", "kotlin", "scala", "swift", "lua", "zig":
		// NATIVE AST path: the generic tree-sitter tags-query extractor
		// (tagsquery.go) replaces the regex fallback for these languages. Imports
		// are not modeled by the tags query, so they stay empty here; call edges
		// keep using the line-scan textCallEdges for now (parity with the other
		// non-AST-call languages until per-language AST call extractors land).
		if grammar, query, ok := tagsGrammar(language); ok {
			rawSyms = tagsSymbols(language, grammar, query, content)
		} else {
			// Grammar/query unexpectedly unavailable: fall back rather than drop
			// the file entirely.
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		if len(imports) == 0 {
			imports = parseLightweightImports(language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "elixir":
		if syms, ok := parseElixirNative(content); ok {
			rawSyms = syms
			imports = parseLightweightImports(language, content)
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "objc":
		if syms, ok := parseObjCNative(content); ok {
			rawSyms = syms
			imports = parseLightweightImports(language, content)
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "julia":
		if syms, ok := parseJuliaNative(content); ok {
			rawSyms = syms
			imports = parseLightweightImports(language, content)
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "dart":
		if syms, ok := parseDartNative(content); ok {
			rawSyms = syms
			imports = parseLightweightImports(language, content)
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "r":
		if syms, ok := parseRNative(content); ok {
			rawSyms = syms
			imports = parseLightweightImports(language, content)
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "fortran":
		if syms, ok := parseFortranNative(content); ok {
			rawSyms = syms
			imports = parseLightweightImports(language, content)
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "verilog":
		if syms, ok := parseVerilogNative(content); ok {
			rawSyms = syms
			imports = parseLightweightImports(language, content)
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "groovy":
		if syms, groovyImports, ok := parseGroovyNative(content); ok {
			rawSyms = syms
			imports = groovyImports
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "bash":
		if syms, bashImports, ok := parseBashNative(content); ok {
			rawSyms = syms
			imports = bashImports
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "powershell":
		if syms, powerShellImports, ok := parsePowerShellNative(content); ok {
			rawSyms = syms
			imports = powerShellImports
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "vue":
		if syms, vueImports, ok := parseVueNative(content); ok {
			rawSyms = syms
			imports = vueImports
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "pascal":
		if syms, ok := parsePascalNative(content); ok {
			rawSyms = syms
			imports = parseLightweightImports(language, content)
		} else {
			rawSyms, imports = parseRegexFallback(filePath, language, content)
		}
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "delphi", "terraform", "byond", "dotnet", "razor", "apex", "blade",
		"svelte", "astro", "ejs", "ets", "sql", "p4":
		rawSyms, imports = parseRegexFallback(filePath, language, content)
		textEdges = textCallEdges(filePath, language, string(content), rawSyms)
	case "html", "css":
	// Pulse records these as file-level context even though there is no
	// first-class symbol parser. Keep the file row; symbols remain empty.
	case "proto":
		rawSyms, imports = parseProtoSymbols(filePath, content)
	case "makefile":
		rawSyms = parseMakefileSymbols(filePath, content)
	case "markdown", "mdx", "yaml", "json", "toml", "xml", "plist", "gomod", "gosum",
		"config", "batch", "csv", "text", "dockerfile":
		rawSyms = parseDocSymbols(filePath, language, content)
	default:
		return Result{}, nil
	}

	// Enrich each symbol with leading-comment Doc and a first-line Signature
	// fallback, then promote to the shared graph type with a stable NodeID.
	docs := leadingComments(content, rawSyms)
	bodies := symbolBodyExcerpts(content, rawSyms)
	symbols := make([]graph.CodeSymbol, 0, len(rawSyms))
	for _, d := range rawSyms {
		sig := d.signature
		if sig == "" {
			sig = firstLineSignature(content, d.startLine)
		}
		doc := d.doc
		if leading := docs[d.key()]; leading != "" {
			doc = leading
		}
		meta := graph.JSONBMap{}
		for k, v := range d.metadata {
			meta[k] = v
		}
		if body := bodies[d.key()]; body != "" {
			meta["body_excerpt"] = body
		}
		if doc != "" {
			meta["doc"] = doc
		}
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
			Metadata:  meta,
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
	var edges []graph.DependencyEdge
	if language == "go" {
		edges = goEdges
	} else if len(textEdges) > 0 {
		edges = textEdges
	} else {
		edges = callEdges(repoID, repoFullName, filePath, language, content, tsRoot, symbols)
	}

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
	doc       string
	startLine int
	endLine   int
	metadata  graph.JSONBMap
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
