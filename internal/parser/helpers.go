package parser

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/dominic097/atlas/internal/graph"
	"github.com/google/uuid"
)

func newUUID() string { return uuid.NewString() }

func itoa(n int) string { return strconv.Itoa(n) }

var (
	callRe   = regexp.MustCompile(`\b([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	callRePy = regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
)

// textCallEdges extracts symbol-granular call edges for non-Go languages by
// scanning each line for call expressions and attributing them to the nearest
// enclosing symbol. Ported from pulse parseTextCallEdges, adapted to
// graph.DependencyEdge with FromSymbol set. Language-specific keywords (if/for/
// def/class/...) are skipped to keep the edges meaningful.
func textCallEdges(filePath, language, content string, syms []symbolDraft) []graph.DependencyEdge {
	pattern := callRe
	if language == "python" {
		pattern = callRePy
	}
	lines := strings.Split(content, "\n")
	var edges []graph.DependencyEdge
	for i, line := range lines {
		lineNumber := i + 1
		caller := nearestSymbolBeforeLine(syms, lineNumber)
		if caller == nil {
			continue
		}
		for _, match := range pattern.FindAllStringSubmatch(line, -1) {
			if len(match) < 2 {
				continue
			}
			callee := match[1]
			if shouldSkipCallName(language, callee) || callee == caller.name {
				continue
			}
			edges = append(edges, graph.DependencyEdge{
				ID:         newUUID(),
				FromFile:   filePath,
				FromSymbol: caller.name,
				ToRef:      callee,
				Kind:       graph.EdgeCalls,
				Language:   language,
				Line:       lineNumber,
				Metadata: graph.JSONBMap{
					"source":         "call_pattern",
					"analysis_level": "call_expression",
				},
			})
		}
	}
	return dedupeEdges(edges)
}

// nearestSymbolBeforeLine returns the symbol whose definition most closely
// precedes the given line — the best-effort enclosing scope.
func nearestSymbolBeforeLine(syms []symbolDraft, line int) *symbolDraft {
	var nearest *symbolDraft
	for i := range syms {
		s := &syms[i]
		if s.startLine <= 0 || s.startLine > line {
			continue
		}
		if nearest == nil || s.startLine > nearest.startLine {
			nearest = s
		}
	}
	return nearest
}

func shouldSkipCallName(language, name string) bool {
	common := map[string]bool{
		"if": true, "for": true, "while": true, "switch": true, "return": true,
		"function": true, "class": true, "new": true, "await": true, "defer": true,
		"make": true, "len": true, "cap": true, "append": true, "copy": true, "delete": true,
	}
	if common[name] {
		return true
	}
	switch language {
	case "python":
		return map[string]bool{
			"def": true, "class": true, "print": true, "len": true, "range": true,
			"str": true, "int": true, "float": true, "list": true, "dict": true, "set": true,
		}[name]
	case "java", "c", "cpp":
		return map[string]bool{
			"else": true, "do": true, "try": true, "catch": true, "sizeof": true,
			"static": true, "public": true, "private": true, "protected": true,
		}[name]
	}
	return false
}

// symbolBodyExcerpts captures a bounded body window for every parsed symbol.
// Review/RCA consumers use this to pack symbol spans instead of whole files.
func symbolBodyExcerpts(content []byte, symbols []symbolDraft) map[string]string {
	lines := strings.Split(string(content), "\n")
	const maxLines = 120
	excerpts := make(map[string]string, len(symbols))
	for _, symbol := range symbols {
		start := symbol.startLine
		end := symbol.endLine
		if start <= 0 || start > len(lines) {
			continue
		}
		if end < start || end > len(lines) {
			end = start
		}
		body := append([]string{}, lines[start-1:end]...)
		if len(body) > maxLines {
			head := (maxLines * 2) / 3
			tail := maxLines - head
			window := append([]string{}, body[:head]...)
			window = append(window, "// ... truncated ...")
			window = append(window, body[len(body)-tail:]...)
			body = window
		}
		excerpts[symbol.key()] = strings.TrimSpace(strings.Join(body, "\n"))
	}
	return excerpts
}

// dedupeEdges collapses identical (file, callee, kind, from-symbol, line) edges.
func dedupeEdges(edges []graph.DependencyEdge) []graph.DependencyEdge {
	seen := map[string]bool{}
	out := make([]graph.DependencyEdge, 0, len(edges))
	for _, e := range edges {
		key := strings.Join([]string{
			e.FromFile, e.ToRef, string(e.Kind), e.FromSymbol, itoa(e.Line),
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, e)
	}
	return out
}

// firstLineSignature returns the trimmed first source line of a symbol as a
// signature fallback (used when the backend did not supply one, e.g. Go types
// or JS arrow functions).
func firstLineSignature(content []byte, startLine int) string {
	if startLine <= 0 {
		return ""
	}
	lines := strings.Split(string(content), "\n")
	if startLine > len(lines) {
		return ""
	}
	sig := strings.TrimSpace(lines[startLine-1])
	sig = strings.TrimSuffix(sig, "{")
	return strings.TrimSpace(sig)
}

const maxDocLines = 20

// leadingComments captures the contiguous comment block immediately above each
// symbol (Go doc comments, JSDoc, leading # comments, etc.) — populating
// CodeSymbol.Doc. Ported from pulse symbolLeadingComments. Keyed by the symbol
// draft's (kind,name,startLine) so it survives the promotion step.
func leadingComments(content []byte, syms []symbolDraft) map[string]string {
	lines := strings.Split(string(content), "\n")
	out := make(map[string]string, len(syms))
	for _, s := range syms {
		start := s.startLine
		if start <= 1 || start-2 >= len(lines) {
			continue
		}
		var collected []string
		for i := start - 2; i >= 0 && len(collected) < maxDocLines; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" || !isCommentLine(line) {
				break
			}
			collected = append(collected, line)
		}
		if len(collected) == 0 {
			continue
		}
		for l, r := 0, len(collected)-1; l < r; l, r = l+1, r-1 {
			collected[l], collected[r] = collected[r], collected[l]
		}
		out[s.key()] = strings.Join(collected, "\n")
	}
	return out
}

func isCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") ||
		strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, "*") ||
		strings.HasPrefix(line, "/*") ||
		strings.HasPrefix(line, `"""`) ||
		strings.HasPrefix(line, "'''") ||
		strings.HasPrefix(line, "--")
}
