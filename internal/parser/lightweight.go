package parser

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/MsysTechnologiesllc/aziron-atlas/internal/graph"
)

// parseDocSymbols mirrors Pulse's doc/config indexing: markdown is chunked by
// headings, while config-like files become one retrievable document symbol.
func parseDocSymbols(path, language string, content []byte) []symbolDraft {
	lines := strings.Split(string(content), "\n")
	if language == "markdown" {
		type section struct {
			name  string
			start int
		}
		sections := make([]section, 0)
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "#") {
				title := strings.TrimSpace(strings.TrimLeft(trimmed, "# "))
				if title == "" {
					title = filepath.Base(path)
				}
				sections = append(sections, section{name: title, start: i + 1})
			}
		}
		if len(sections) > 0 {
			out := make([]symbolDraft, 0, len(sections))
			for i, section := range sections {
				end := len(lines)
				if i+1 < len(sections) {
					end = sections[i+1].start - 1
				}
				body := strings.Join(lines[section.start-1:end], "\n")
				out = append(out, docDraft(language, "section", section.name, section.start, end, body))
			}
			return out
		}
	}
	return []symbolDraft{docDraft(language, "document", filepath.Base(path), 1, len(lines), string(content))}
}

func docDraft(language, kind, name string, start, end int, body string) symbolDraft {
	body = strings.TrimSpace(body)
	if len(body) > 4000 {
		body = body[:4000]
	}
	return symbolDraft{
		kind:      kind,
		name:      name,
		doc:       body,
		startLine: start,
		endLine:   end,
		metadata:  graph.JSONBMap{"body_excerpt": body},
	}
}

func parseRegexFallback(path, language string, content []byte) ([]symbolDraft, []string) {
	switch language {
	case "csharp":
		return parseCSharpRegex(path, content)
	case "groovy":
		return parseGroovyRegex(path, content)
	case "bash":
		return parseBashRegex(path, content)
	default:
		return nil, nil
	}
}

func parseCSharpRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	imports := regexCaptures(text, regexp.MustCompile(`using\s+([\w.]+);`))
	symbols := make([]symbolDraft, 0)
	add := func(kind string, re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 4 {
				continue
			}
			line := lineForOffset(text, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      kind,
				name:      text[match[2]:match[3]],
				startLine: line,
				endLine:   line,
			})
		}
	}
	add("class", regexp.MustCompile(`(?m)^\s*(?:public|private|protected|internal|abstract|sealed)?\s+class\s+([A-Za-z_]\w*)`))
	add("interface", regexp.MustCompile(`(?m)^\s*(?:public|private|protected|internal)?\s+interface\s+([A-Za-z_]\w*)`))
	add("method", regexp.MustCompile(`(?m)^\s*(?:public|private|protected|internal|static|virtual|override|async)(?:\s+\w+)+\s+([A-Za-z_]\w*)\s*\(`))
	return symbols, imports
}

func parseGroovyRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	imports := regexCaptures(text, regexp.MustCompile(`import\s+([\w.]+)`))
	symbols := make([]symbolDraft, 0)
	add := func(kind string, re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 4 {
				continue
			}
			line := lineForOffset(text, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      kind,
				name:      text[match[2]:match[3]],
				startLine: line,
				endLine:   line,
			})
		}
	}
	add("class", regexp.MustCompile(`(?m)^\s*class\s+([A-Za-z_]\w*)`))
	add("function", regexp.MustCompile(`(?m)^\s*(?:def\s+)?([A-Za-z_]\w*)\s*\([^)]*\)\s*\{`))
	return symbols, imports
}

func parseBashRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	imports := regexCaptures(text, regexp.MustCompile(`(?m)^\s*(?:\.|source)\s+([^\s;]+)`))
	symbols := make([]symbolDraft, 0)
	re := regexp.MustCompile(`(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(\s*\)\s*\{`)
	for _, match := range re.FindAllStringSubmatchIndex(text, -1) {
		if len(match) < 4 {
			continue
		}
		line := lineForOffset(text, match[0])
		symbols = append(symbols, symbolDraft{
			kind:      "function",
			name:      text[match[2]:match[3]],
			startLine: line,
			endLine:   line,
		})
	}
	return symbols, imports
}

func parseProtoSymbols(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	imports := regexCaptures(text, regexp.MustCompile(`(?m)^\s*import\s+(?:public\s+|weak\s+)?\"([^\"]+)\"`))
	symbols := make([]symbolDraft, 0)
	addBlocks := func(kind string, re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 4 {
				continue
			}
			start := lineForOffset(text, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      kind,
				name:      text[match[2]:match[3]],
				startLine: start,
				endLine:   blockEndLine(text, match[0], start),
			})
		}
	}
	addBlocks("message", regexp.MustCompile(`(?m)^\s*message\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`))
	addBlocks("service", regexp.MustCompile(`(?m)^\s*service\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`))
	addBlocks("enum", regexp.MustCompile(`(?m)^\s*enum\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`))
	rpcRe := regexp.MustCompile(`(?m)^\s*rpc\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	for _, match := range rpcRe.FindAllStringSubmatchIndex(text, -1) {
		if len(match) < 4 {
			continue
		}
		line := lineForOffset(text, match[0])
		symbols = append(symbols, symbolDraft{
			kind:      "rpc",
			name:      text[match[2]:match[3]],
			startLine: line,
			endLine:   line,
		})
	}
	return symbols, imports
}

func parseMakefileSymbols(path string, content []byte) []symbolDraft {
	lines := strings.Split(string(content), "\n")
	targetRe := regexp.MustCompile(`^([A-Za-z0-9_./%@+-]+)\s*:(?:\s|$)`)
	starts := make([]struct {
		name string
		line int
	}, 0)
	for i, line := range lines {
		if strings.HasPrefix(line, "\t") || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		match := targetRe.FindStringSubmatch(line)
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" || strings.HasPrefix(name, ".") {
			continue
		}
		starts = append(starts, struct {
			name string
			line int
		}{name: name, line: i + 1})
	}
	out := make([]symbolDraft, 0, len(starts))
	for i, target := range starts {
		end := len(lines)
		if i+1 < len(starts) {
			end = starts[i+1].line - 1
		}
		out = append(out, symbolDraft{
			kind:      "target",
			name:      target.name,
			startLine: target.line,
			endLine:   end,
		})
	}
	return out
}

func regexCaptures(text string, patterns ...*regexp.Regexp) []string {
	values := make([]string, 0)
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllStringSubmatch(text, -1) {
			if len(match) <= 1 {
				continue
			}
			for _, item := range strings.Split(match[1], ",") {
				if trimmed := strings.TrimSpace(item); trimmed != "" {
					values = append(values, trimmed)
				}
			}
		}
	}
	return uniqueStrings(values)
}

func lineForOffset(text string, offset int) int {
	if offset <= 0 {
		return 1
	}
	if offset > len(text) {
		offset = len(text)
	}
	line := 1
	for i := 0; i < offset; i++ {
		if text[i] == '\n' {
			line++
		}
	}
	return line
}

func blockEndLine(text string, startOffset, fallbackLine int) int {
	open := strings.IndexByte(text[startOffset:], '{')
	if open < 0 {
		return fallbackLine
	}
	depth := 0
	for i := startOffset + open; i < len(text); i++ {
		switch text[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return lineForOffset(text, i)
			}
		}
	}
	return fallbackLine
}
