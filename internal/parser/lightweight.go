package parser

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/dominic097/atlas/internal/graph"
)

// parseDocSymbols mirrors Pulse's doc/config indexing: markdown is chunked by
// headings, while config-like files become one retrievable document symbol.
func parseDocSymbols(path, language string, content []byte) []symbolDraft {
	lines := strings.Split(string(content), "\n")
	if language == "markdown" || language == "mdx" {
		type section struct {
			name  string
			start int
		}
		sections := make([]section, 0)
		inFence := false
		fenceMarker := byte(0)
		fenceLen := 0
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if marker, markerLen, ok := markdownFence(trimmed); ok {
				if !inFence {
					inFence = true
					fenceMarker = marker
					fenceLen = markerLen
					continue
				}
				if marker == fenceMarker && markerLen >= fenceLen {
					inFence = false
					fenceMarker = 0
					fenceLen = 0
				}
				continue
			}
			if inFence {
				continue
			}
			if title, ok := markdownHeadingTitle(trimmed); ok {
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
	if language == "json" {
		if symbols := parseJSONSymbols(path, content); len(symbols) > 0 {
			return symbols
		}
	}
	return []symbolDraft{docDraft(language, "document", filepath.Base(path), 1, len(lines), string(content))}
}

func markdownFence(trimmed string) (byte, int, bool) {
	if len(trimmed) < 3 {
		return 0, 0, false
	}
	marker := trimmed[0]
	if marker != '`' && marker != '~' {
		return 0, 0, false
	}
	count := 0
	for count < len(trimmed) && trimmed[count] == marker {
		count++
	}
	return marker, count, count >= 3
}

func markdownHeadingTitle(trimmed string) (string, bool) {
	if trimmed == "" || trimmed[0] != '#' {
		return "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level > 6 {
		return "", false
	}
	if level < len(trimmed) && trimmed[level] != ' ' && trimmed[level] != '\t' {
		return "", false
	}
	title := strings.TrimSpace(trimmed[level:])
	hashStart := len(title)
	for hashStart > 0 && title[hashStart-1] == '#' {
		hashStart--
	}
	if hashStart < len(title) && (hashStart == 0 || title[hashStart-1] == ' ' || title[hashStart-1] == '\t') {
		title = strings.TrimSpace(title[:hashStart])
	}
	return title, true
}

func parseJSONSymbols(path string, content []byte) []symbolDraft {
	text := string(content)
	var value any
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	if err := dec.Decode(&value); err != nil {
		return nil
	}

	symbols := make([]symbolDraft, 0)
	var walk func(prefix string, node any, depth int)
	walk = func(prefix string, node any, depth int) {
		if depth > 8 || len(symbols) >= 1000 {
			return
		}
		switch typed := node.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				child := typed[key]
				name := jsonPathJoin(prefix, key)
				line := jsonKeyLine(text, key)
				symbols = append(symbols, symbolDraft{
					kind:      "key",
					name:      name,
					startLine: line,
					endLine:   line,
					metadata: graph.JSONBMap{
						"source":     "json_parser",
						"value_kind": jsonValueKind(child),
					},
				})
				walk(name, child, depth+1)
			}
		case []any:
			for _, child := range typed {
				walk(prefix+"[]", child, depth+1)
				if len(symbols) >= 1000 {
					return
				}
			}
		}
	}
	walk("", value, 0)
	if len(symbols) == 0 {
		return []symbolDraft{docDraft("json", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text)}
	}
	return dedupeSymbolDrafts(symbols)
}

func jsonPathJoin(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

func jsonKeyLine(text, key string) int {
	needle := `"` + strings.ReplaceAll(key, `"`, `\"`) + `"`
	if offset := strings.Index(text, needle); offset >= 0 {
		return lineForOffset(text, offset)
	}
	return 1
}

func jsonValueKind(value any) string {
	switch value.(type) {
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case json.Number:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "null"
	default:
		return "value"
	}
}

func parseDotnetRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	scanText := maskXMLComments(text)
	symbols := make([]symbolDraft, 0)
	addAt := func(kind, name string, offset int) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		line := lineForOffset(text, offset)
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: line,
			endLine:   line,
			metadata:  graph.JSONBMap{"source": "dotnet_project_parser"},
		})
	}
	baseProjectName := func(value string) string {
		value = strings.ReplaceAll(strings.TrimSpace(value), `\`, `/`)
		base := filepath.Base(value)
		ext := filepath.Ext(base)
		if ext != "" {
			base = strings.TrimSuffix(base, ext)
		}
		return base
	}

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".sln":
		re := regexp.MustCompile(`(?m)^Project\("[^"\n]+"\)\s*=\s*"([^"]+)"\s*,\s*"([^"]+)"`)
		for _, match := range re.FindAllStringSubmatchIndex(scanText, -1) {
			if len(match) >= 4 {
				addAt("project", scanText[match[2]:match[3]], match[0])
			}
		}
	case ".slnx":
		re := regexp.MustCompile(`(?i)<Project\b[^>]*\bPath="([^"]+)"`)
		for _, match := range re.FindAllStringSubmatchIndex(scanText, -1) {
			if len(match) >= 4 {
				addAt("project", baseProjectName(scanText[match[2]:match[3]]), match[0])
			}
		}
	default:
		addAt("project", baseProjectName(path), 0)
		for _, spec := range []struct {
			kind string
			re   *regexp.Regexp
		}{
			{"sdk", regexp.MustCompile(`(?i)<Project\b[^>]*\bSdk="([^"]+)"`)},
			{"package", regexp.MustCompile(`(?i)<PackageReference\b[^>]*\b(?:Include|Update)="([^"]+)"`)},
			{"project_reference", regexp.MustCompile(`(?i)<ProjectReference\b[^>]*\bInclude="([^"]+)"`)},
		} {
			for _, match := range spec.re.FindAllStringSubmatchIndex(scanText, -1) {
				if len(match) < 4 {
					continue
				}
				name := scanText[match[2]:match[3]]
				if spec.kind == "project_reference" {
					name = baseProjectName(name)
				}
				addAt(spec.kind, name, match[0])
			}
		}
		reFramework := regexp.MustCompile(`(?is)<TargetFrameworks?>([^<]+)</TargetFrameworks?>`)
		for _, match := range reFramework.FindAllStringSubmatchIndex(scanText, -1) {
			if len(match) < 4 {
				continue
			}
			for _, framework := range strings.Split(scanText[match[2]:match[3]], ";") {
				addAt("target_framework", framework, match[0])
			}
		}
	}

	imports := regexCaptures(scanText, lightweightImportRules["dotnet"]...)
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("dotnet", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), imports
}

func maskXMLComments(text string) string {
	buf := []byte(text)
	for start := 0; start < len(buf); {
		open := strings.Index(string(buf[start:]), "<!--")
		if open < 0 {
			break
		}
		open += start
		closeRel := strings.Index(string(buf[open+4:]), "-->")
		close := len(buf)
		if closeRel >= 0 {
			close = open + 4 + closeRel + 3
		}
		for i := open; i < close; i++ {
			if buf[i] != '\n' && buf[i] != '\r' {
				buf[i] = ' '
			}
		}
		start = close
	}
	return string(buf)
}

func parseBladeRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	symbols := make([]symbolDraft, 0)
	imports := make([]string, 0)
	add := func(kind, name string, offset int, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: lineForOffset(text, offset),
			endLine:   lineForOffset(text, offset),
			metadata:  graph.JSONBMap{"source": source},
		})
	}
	addImport := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			imports = append(imports, value)
		}
	}

	add("template", bladeViewName(path), 0, "blade_file")

	directives := []struct {
		kind    string
		source  string
		re      *regexp.Regexp
		imports bool
	}{
		{"include", "blade_include", regexp.MustCompile(`@include(?:If|When|Unless|First)?\s*\(\s*['"]([^'"]+)['"]`), true},
		{"layout", "blade_extends", regexp.MustCompile(`@extends\s*\(\s*['"]([^'"]+)['"]`), true},
		{"section", "blade_section", regexp.MustCompile(`@section\s*\(\s*['"]([^'"]+)['"]`), false},
		{"slot", "blade_yield", regexp.MustCompile(`@yield\s*\(\s*['"]([^'"]+)['"]`), false},
		{"component", "blade_component_directive", regexp.MustCompile(`@component\s*\(\s*['"]([^'"]+)['"]`), true},
		{"component", "blade_livewire_tag", regexp.MustCompile(`<livewire:([A-Za-z0-9_.-]+)`), false},
		{"component", "blade_anonymous_component", regexp.MustCompile(`<x-([A-Za-z0-9_.:-]+)`), false},
		{"handler", "blade_wire_handler", regexp.MustCompile(`wire:[A-Za-z0-9_.:-]+\s*=\s*(?:"([^"]*)"|'([^']*)')`), false},
	}
	for _, directive := range directives {
		for _, match := range directive.re.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 4 {
				continue
			}
			name := regexFirstCapture(text, match)
			add(directive.kind, name, match[0], directive.source)
			if directive.imports {
				addImport(name)
			}
		}
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("blade", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), uniqueStrings(imports)
}

func bladeViewName(path string) string {
	slashed := filepath.ToSlash(path)
	if idx := strings.Index(slashed, "resources/views/"); idx >= 0 {
		slashed = slashed[idx+len("resources/views/"):]
	}
	slashed = strings.TrimSuffix(slashed, ".blade.php")
	slashed = strings.TrimSuffix(slashed, filepath.Ext(slashed))
	return strings.ReplaceAll(strings.Trim(slashed, "/"), "/", ".")
}

func parseEJSRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	symbols := make([]symbolDraft, 0)
	imports := make([]string, 0)
	add := func(kind, name string, offset int, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		line := lineForOffset(text, offset)
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: line,
			endLine:   line,
			metadata:  graph.JSONBMap{"source": source},
		})
	}
	addImport := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			imports = append(imports, value)
		}
	}

	add("template", ejsViewName(path), 0, "ejs_file")

	directives := []struct {
		kind    string
		source  string
		re      *regexp.Regexp
		imports bool
	}{
		{"include", "ejs_include", regexp.MustCompile(`<%\s*(?:-|=|_)?\s*(?:include|await\s+include)\s*\(?\s*['"]([^'"]+)['"]`), true},
		{"function", "ejs_function", regexp.MustCompile(`<%[^%]*?\bfunction\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`), false},
		{"variable", "ejs_variable", regexp.MustCompile(`<%[^%]*?\b(?:const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=`), false},
	}
	for _, directive := range directives {
		for _, match := range directive.re.FindAllStringSubmatchIndex(text, -1) {
			name := regexFirstCapture(text, match)
			add(directive.kind, name, match[0], directive.source)
			if directive.imports {
				addImport(name)
			}
		}
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("ejs", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), uniqueStrings(imports)
}

func ejsViewName(path string) string {
	slashed := filepath.ToSlash(path)
	if idx := strings.Index(slashed, "views/"); idx >= 0 {
		slashed = slashed[idx+len("views/"):]
	}
	slashed = strings.TrimSuffix(slashed, ".ejs")
	slashed = strings.TrimSuffix(slashed, filepath.Ext(slashed))
	return strings.ReplaceAll(strings.Trim(slashed, "/"), "/", ".")
}

var etsControlWords = map[string]bool{
	"break": true, "case": true, "catch": true, "continue": true, "default": true,
	"do": true, "else": true, "for": true, "if": true, "new": true, "return": true,
	"super": true, "switch": true, "this": true, "throw": true, "try": true, "while": true,
}

func parseETSRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	imports := regexCaptures(text, lightweightImportRules["ets"]...)
	symbols := make([]symbolDraft, 0)
	add := func(kind, name string, offset int, source string) {
		name = strings.TrimSpace(name)
		if name == "" || etsControlWords[name] {
			return
		}
		start := lineForOffset(text, offset)
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: start,
			endLine:   blockEndLine(text, offset, start),
			metadata:  graph.JSONBMap{"source": source},
		})
	}
	addMatches := func(kind, source string, re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(text, -1) {
			name := regexFirstCapture(text, match)
			add(kind, name, match[0], source)
		}
	}

	ident := `([A-Za-z_][A-Za-z0-9_]*)`
	addMatches("type", "ets_type", regexp.MustCompile(`(?m)^\s*(?:@[A-Za-z_][A-Za-z0-9_]*(?:\([^)\r\n]*\))?\s*)*(?:export\s+)?(?:abstract\s+)?(?:class|struct|interface|enum|namespace)\s+`+ident))
	addMatches("function", "ets_function", regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+`+ident+`\s*\(`))
	addMatches("variable", "ets_decorated_variable", regexp.MustCompile(`(?m)^\s*(?:@[A-Za-z_][A-Za-z0-9_]*(?:\([^)\r\n]*\))?\s*)+`+ident+`\s*[:=]`))
	addMatches("variable", "ets_field", regexp.MustCompile(`(?m)^\s*(?:(?:public|private|protected|static|readonly)\s+)+`+ident+`\s*[:=]`))
	addMatches("variable", "ets_variable", regexp.MustCompile(`(?m)^\s*(?:const|let|var)\s+`+ident+`\s*[:=]`))

	methodRe := regexp.MustCompile(`^\s*(?:(?:public|private|protected|static|async|override)\s+)*(constructor|build|aboutToAppear|aboutToDisappear|aboutToReuse|onPageShow|onPageHide|onBackPress|[a-z_][A-Za-z0-9_]*)\s*\(`)
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		clean := strings.TrimSpace(stripVerilogLineComment(line))
		if match := methodRe.FindStringSubmatchIndex(clean); len(match) >= 4 {
			name := clean[match[2]:match[3]]
			kind := "method"
			if name == "constructor" {
				kind = "constructor"
			}
			add(kind, name, offset+strings.Index(line, strings.TrimLeft(line, " \t")), "ets_method")
		}
		offset += len(line)
	}

	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("ets", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), uniqueStrings(imports)
}

func regexFirstCapture(text string, match []int) string {
	for i := 2; i+1 < len(match); i += 2 {
		if match[i] >= 0 && match[i+1] >= match[i] {
			return text[match[i]:match[i+1]]
		}
	}
	return ""
}

func parseRazorRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	symbols := make([]symbolDraft, 0)
	imports := make([]string, 0)
	add := func(kind, name string, offset int, source string) {
		name = strings.TrimSpace(name)
		if name == "" || razorHTMLTags[name] {
			return
		}
		line := lineForOffset(text, offset)
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: line,
			endLine:   line,
			metadata:  graph.JSONBMap{"source": source},
		})
	}
	addImport := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			imports = append(imports, value)
		}
	}

	fileKind := "view"
	if strings.EqualFold(filepath.Ext(path), ".razor") {
		fileKind = "component"
	}
	add(fileKind, razorViewName(path), 0, "razor_file")

	directives := []struct {
		kind    string
		source  string
		re      *regexp.Regexp
		imports bool
	}{
		{"route", "razor_page", regexp.MustCompile(`(?m)^\s*@page\s+"([^"]+)"`), false},
		{"import", "razor_using", regexp.MustCompile(`(?m)^\s*@using\s+([A-Za-z0-9_.]+)`), true},
		{"service", "razor_inject", regexp.MustCompile(`(?m)^\s*@inject\s+([A-Za-z0-9_.<>\[\]]+)\s+[A-Za-z_][A-Za-z0-9_]*`), true},
		{"base", "razor_inherits", regexp.MustCompile(`(?m)^\s*@inherits\s+([A-Za-z0-9_.<>\[\]]+)`), false},
		{"model", "razor_model", regexp.MustCompile(`(?m)^\s*@model\s+([A-Za-z0-9_.<>\[\]]+)`), false},
		{"component", "razor_component_tag", regexp.MustCompile(`<([A-Z][A-Za-z0-9]+)(?:\s|/?>)`), false},
	}
	for _, directive := range directives {
		for _, match := range directive.re.FindAllStringSubmatchIndex(text, -1) {
			name := regexFirstCapture(text, match)
			add(directive.kind, name, match[0], directive.source)
			if directive.imports {
				addImport(name)
			}
		}
	}

	methodRe := regexp.MustCompile(`(?m)(?:public|private|protected|internal|static|async|override|virtual|abstract)\s+(?:[A-Za-z0-9_<>,\[\]?]+\s+)+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	for _, block := range razorCodeBlocks(text) {
		for _, match := range methodRe.FindAllStringSubmatchIndex(block.text, -1) {
			name := regexFirstCapture(block.text, match)
			add("method", name, block.offset+match[0], "razor_code")
		}
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("razor", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), uniqueStrings(imports)
}

func razorViewName(path string) string {
	slashed := filepath.ToSlash(path)
	if idx := strings.Index(slashed, "/src/"); idx >= 0 {
		slashed = slashed[idx+len("/src/"):]
	} else if strings.HasPrefix(slashed, "src/") {
		slashed = strings.TrimPrefix(slashed, "src/")
	}
	slashed = strings.TrimSuffix(slashed, ".cshtml")
	slashed = strings.TrimSuffix(slashed, ".razor")
	return strings.ReplaceAll(strings.Trim(slashed, "/"), "/", ".")
}

type razorCodeBlock struct {
	offset int
	text   string
}

func razorCodeBlocks(text string) []razorCodeBlock {
	re := regexp.MustCompile(`(?m)@(code|functions)\s*\{`)
	blocks := make([]razorCodeBlock, 0)
	for _, match := range re.FindAllStringSubmatchIndex(text, -1) {
		start := match[1]
		depth := 1
		pos := start
		for pos < len(text) && depth > 0 {
			switch text[pos] {
			case '{':
				depth++
			case '}':
				depth--
			}
			pos++
		}
		if depth == 0 && pos-1 >= start {
			blocks = append(blocks, razorCodeBlock{offset: start, text: text[start : pos-1]})
		}
	}
	return blocks
}

var razorHTMLTags = map[string]bool{
	"DOCTYPE": true, "Html": true, "Head": true, "Body": true, "Div": true,
	"Span": true, "Table": true, "Form": true, "Input": true, "Button": true,
	"Select": true, "Option": true, "Label": true, "Textarea": true,
	"Script": true, "Style": true, "Link": true, "Meta": true, "Title": true,
	"Header": true, "Footer": true, "Nav": true, "Main": true, "Section": true,
	"Article": true, "Aside": true,
}

func parseAstroRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	imports := regexCaptures(text, lightweightImportRules["astro"]...)
	symbols := make([]symbolDraft, 0)
	add := func(kind, name string, line int, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: line,
			endLine:   line,
			metadata:  graph.JSONBMap{"source": source},
		})
	}

	componentName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	add("component", componentName, 1, "astro_file")

	frontmatter, frontmatterLine := astroFrontmatter(text)
	if frontmatter != "" {
		for _, match := range regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`).FindAllStringSubmatchIndex(frontmatter, -1) {
			add("function", frontmatter[match[2]:match[3]], frontmatterLine+lineForOffset(frontmatter, match[0])-1, "astro_frontmatter")
		}
		for _, match := range regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?::|=|,)`).FindAllStringSubmatchIndex(frontmatter, -1) {
			add("variable", frontmatter[match[2]:match[3]], frontmatterLine+lineForOffset(frontmatter, match[0])-1, "astro_frontmatter")
		}
		destructured := regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s*\{([^}]+)\}\s*=`)
		for _, match := range destructured.FindAllStringSubmatchIndex(frontmatter, -1) {
			line := frontmatterLine + lineForOffset(frontmatter, match[0]) - 1
			for _, name := range astroDestructureNames(frontmatter[match[2]:match[3]]) {
				add("variable", name, line, "astro_frontmatter")
			}
		}
	}

	componentTag := regexp.MustCompile(`(?m)<([A-Z][A-Za-z0-9_.:-]*)\b`)
	for _, match := range componentTag.FindAllStringSubmatchIndex(text, -1) {
		add("component", text[match[2]:match[3]], lineForOffset(text, match[0]), "astro_component_tag")
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("astro", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), imports
}

func parseApexRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	lines := strings.Split(text, "\n")
	symbols := make([]symbolDraft, 0)
	seenContext := map[string]bool{}
	currentType := ""
	pendingAnnotations := make([]string, 0)
	offset := 0

	add := func(kind, name string, line, lineOffset int, source string, annotations []string) {
		name = strings.TrimSpace(name)
		if name == "" || apexControlWords[strings.ToLower(name)] {
			return
		}
		meta := graph.JSONBMap{"source": source}
		if len(annotations) > 0 {
			meta["annotations"] = strings.Join(uniqueStrings(annotations), ",")
		}
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: line,
			endLine:   blockEndLine(text, lineOffset, line),
			metadata:  meta,
		})
	}
	addContext := func(kind, name string, line, lineOffset int, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := kind + "\x00" + name + "\x00" + itoa(line)
		if seenContext[key] {
			return
		}
		seenContext[key] = true
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: line,
			endLine:   line,
			metadata:  graph.JSONBMap{"source": source},
		})
	}

	for i, lineText := range lines {
		lineNo := i + 1
		stripped := strings.TrimSpace(lineText)
		if stripped == "" {
			offset += len(lineText) + 1
			continue
		}
		if apexCommentLine(stripped) {
			offset += len(lineText) + 1
			continue
		}

		annotations := apexAnnotations(stripped)
		if strings.HasPrefix(stripped, "@") && len(annotations) > 0 && !apexHasDeclaration(stripped) {
			pendingAnnotations = append(pendingAnnotations, annotations...)
			offset += len(lineText) + 1
			continue
		}
		allAnnotations := append([]string{}, pendingAnnotations...)
		allAnnotations = append(allAnnotations, annotations...)

		if match := apexTriggerRe.FindStringSubmatch(stripped); len(match) > 0 {
			add("trigger", match[1], lineNo, offset, "regex_apex", allAnnotations)
			addContext("sobject", match[2], lineNo, offset, "apex_trigger_target")
			currentType = match[1]
			pendingAnnotations = pendingAnnotations[:0]
			offset += len(lineText) + 1
			continue
		}
		if match := apexTypeRe.FindStringSubmatch(stripped); len(match) > 0 {
			apexKind := strings.ToLower(match[1])
			name := match[2]
			add("type", name, lineNo, offset, "regex_apex", allAnnotations)
			if apexKind == "class" || currentType == "" {
				currentType = name
			}
			pendingAnnotations = pendingAnnotations[:0]
			offset += len(lineText) + 1
			continue
		}
		if currentType != "" {
			if match := apexConstructorRe.FindStringSubmatch(stripped); len(match) > 0 && match[1] == currentType {
				add("constructor", match[1], lineNo, offset, "regex_apex", allAnnotations)
				pendingAnnotations = pendingAnnotations[:0]
				offset += len(lineText) + 1
				continue
			}
			if match := apexMethodRe.FindStringSubmatch(stripped); len(match) > 0 {
				add("method", match[1], lineNo, offset, "regex_apex", allAnnotations)
				pendingAnnotations = pendingAnnotations[:0]
				offset += len(lineText) + 1
				continue
			}
		}
		pendingAnnotations = pendingAnnotations[:0]

		for _, match := range apexSOQLRe.FindAllStringSubmatch(lineText, -1) {
			addContext("sobject", match[1], lineNo, offset, "apex_soql")
		}
		for _, match := range apexDMLRe.FindAllStringSubmatch(lineText, -1) {
			addContext("dml", strings.ToLower(match[1]), lineNo, offset, "apex_dml")
		}
		offset += len(lineText) + 1
	}

	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("apex", "document", filepath.Base(path), 1, len(lines), text))
	}
	return dedupeSymbolDrafts(symbols), nil
}

var (
	apexTypeRe        = regexp.MustCompile(`(?i)^(?:@\w+(?:\s*\([^)]*\))?\s*)*(?:(?:public|private|protected|global|webService|abstract|virtual|override|static|final|transient|testMethod|with\s+sharing|without\s+sharing|inherited\s+sharing)\s+)*(class|interface|enum)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	apexTriggerRe     = regexp.MustCompile(`(?i)^\s*trigger\s+([A-Za-z_][A-Za-z0-9_]*)\s+on\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	apexMethodRe      = regexp.MustCompile(`(?i)^(?:@\w+(?:\s*\([^)]*\))?\s*)*(?:(?:public|private|protected|global|webService|abstract|virtual|override|static|final|transient|testMethod)\s+)*(?:[A-Za-z_][A-Za-z0-9_.]*(?:<[^>{};]+>)?(?:\[\])?)\s+([A-Za-z_][A-Za-z0-9_]*)\s*\([^)]*\)\s*(?:throws\s+[A-Za-z_][A-Za-z0-9_]*\s*)?(?:\{|$|;)`)
	apexConstructorRe = regexp.MustCompile(`(?i)^(?:@\w+(?:\s*\([^)]*\))?\s*)*(?:(?:public|private|protected|global)\s+)*([A-Za-z_][A-Za-z0-9_]*)\s*\([^)]*\)\s*(?:\{|$|;)`)
	apexAnnotationRe  = regexp.MustCompile(`@([A-Za-z_][A-Za-z0-9_]*)`)
	apexSOQLRe        = regexp.MustCompile(`(?i)\[\s*SELECT\b[^\]]+FROM\s+([A-Za-z_][A-Za-z0-9_]*)`)
	apexDMLRe         = regexp.MustCompile(`(?i)\b(?:Database\s*\.\s*)?(insert|update|delete|upsert|merge|undelete)\s*(?:\(|\s+[A-Za-z_][A-Za-z0-9_]*)`)
)

var apexControlWords = map[string]bool{
	"if": true, "else": true, "for": true, "while": true, "do": true,
	"switch": true, "try": true, "catch": true, "finally": true,
	"return": true, "throw": true, "new": true, "void": true, "null": true,
	"true": true, "false": true, "this": true, "super": true,
	"class": true, "interface": true, "enum": true, "trigger": true, "on": true,
}

func apexAnnotations(line string) []string {
	matches := apexAnnotationRe.FindAllStringSubmatch(line, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			out = append(out, strings.ToLower(match[1]))
		}
	}
	return out
}

func apexHasDeclaration(line string) bool {
	return apexTypeRe.MatchString(line) || apexMethodRe.MatchString(line) || apexTriggerRe.MatchString(line)
}

func apexCommentLine(line string) bool {
	return strings.HasPrefix(line, "//") ||
		strings.HasPrefix(line, "/*") ||
		strings.HasPrefix(line, "*") ||
		strings.HasPrefix(line, "*/")
}

func parseByondRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".dm", ".dme":
		return parseByondDM(path, text)
	case ".dmf":
		return parseByondDMF(path, text)
	case ".dmi":
		return parseByondDMI(path, text)
	case ".dmm":
		return parseByondDMM(path, text)
	default:
		return parseByondDM(path, text)
	}
}

func parseByondDM(path, text string) ([]symbolDraft, []string) {
	lines := strings.Split(text, "\n")
	symbols := make([]symbolDraft, 0)
	imports := make([]string, 0)
	seen := map[string]bool{}
	seenTypes := map[string]bool{}
	type byondTypeCtx struct {
		path   string
		indent int
	}
	typeStack := make([]byondTypeCtx, 0)
	inBlockComment := false

	add := func(kind, name string, line int, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := kind + "\x00" + name + "\x00" + itoa(line)
		if seen[key] {
			return
		}
		seen[key] = true
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: line,
			endLine:   line,
			metadata:  graph.JSONBMap{"source": source},
		})
	}
	addType := func(typePath string, line int, source string) {
		typePath = byondNormalizePath(typePath)
		if typePath == "" || seenTypes[typePath] {
			return
		}
		seenTypes[typePath] = true
		add("type", typePath, line, source)
	}
	addMethod := func(ownerPath, procName string, line int, source string) {
		ownerPath = byondNormalizePath(ownerPath)
		procName = strings.TrimSpace(procName)
		if ownerPath == "" || procName == "" || byondControlWords[strings.ToLower(procName)] {
			return
		}
		addType(ownerPath, line, source+"_owner")
		add("method", ownerPath+"/"+procName, line, source)
	}

	for i, rawLine := range lines {
		lineNo := i + 1
		code := byondCodeLine(rawLine, &inBlockComment)
		trimmed := strings.TrimSpace(code)
		if trimmed == "" {
			continue
		}
		indent := byondIndent(rawLine)
		for len(typeStack) > 0 && indent <= typeStack[len(typeStack)-1].indent {
			typeStack = typeStack[:len(typeStack)-1]
		}

		if match := byondIncludeRe.FindStringSubmatch(trimmed); len(match) > 0 {
			imports = append(imports, strings.TrimSpace(match[1]))
			continue
		}
		if match := byondAbsoluteProcRe.FindStringSubmatch(trimmed); len(match) > 0 {
			addMethod(match[1], match[2], lineNo, "byond_dm_absolute_proc")
			continue
		}
		if match := byondAbsoluteOverrideRe.FindStringSubmatch(trimmed); len(match) > 0 {
			ownerPath, procName := match[1], match[2]
			if !byondExcludedOwnerPath(ownerPath) {
				addMethod(ownerPath, procName, lineNo, "byond_dm_absolute_override")
			}
			continue
		}
		if match := byondRelativeProcRe.FindStringSubmatch(trimmed); len(match) > 0 && len(typeStack) == 0 {
			add("proc", match[2], lineNo, "byond_dm_global_proc")
			continue
		}
		if typePath, ok := byondAbsoluteTypePath(trimmed); ok {
			addType(typePath, lineNo, "byond_dm_type")
			typeStack = append(typeStack, byondTypeCtx{path: byondNormalizePath(typePath), indent: indent})
			continue
		}
		if len(typeStack) == 0 || indent != typeStack[len(typeStack)-1].indent+1 {
			continue
		}
		owner := typeStack[len(typeStack)-1].path
		if match := byondRelativeProcRe.FindStringSubmatch(trimmed); len(match) > 0 {
			addMethod(owner, match[2], lineNo, "byond_dm_relative_proc")
			continue
		}
		if match := byondRelativeOverrideRe.FindStringSubmatch(trimmed); len(match) > 0 {
			addMethod(owner, match[1], lineNo, "byond_dm_relative_override")
		}
	}

	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("byond", "document", filepath.Base(path), 1, len(lines), text))
	}
	return dedupeSymbolDrafts(symbols), uniqueStrings(imports)
}

func parseByondDMF(path, text string) ([]symbolDraft, []string) {
	lines := strings.Split(text, "\n")
	symbols := make([]symbolDraft, 0)
	currentWindow := ""
	currentElement := ""
	add := func(kind, name string, line int, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: line,
			endLine:   line,
			metadata:  graph.JSONBMap{"source": source},
		})
	}
	for i, line := range lines {
		lineNo := i + 1
		if match := byondDMFWindowRe.FindStringSubmatch(line); len(match) > 0 {
			currentWindow = match[1]
			currentElement = ""
			add("window", currentWindow, lineNo, "byond_dmf_window")
			continue
		}
		if match := byondDMFElementRe.FindStringSubmatch(line); len(match) > 0 && currentWindow != "" {
			currentElement = match[1]
			add("element", currentWindow+"/"+currentElement, lineNo, "byond_dmf_element")
			continue
		}
		if match := byondDMFTypeRe.FindStringSubmatch(line); len(match) > 0 && currentWindow != "" && currentElement != "" {
			add("element_type", currentWindow+"/"+currentElement+":"+match[1], lineNo, "byond_dmf_type")
		}
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("byond", "document", filepath.Base(path), 1, len(lines), text))
	}
	return dedupeSymbolDrafts(symbols), nil
}

func parseByondDMI(path, text string) ([]symbolDraft, []string) {
	lines := strings.Split(text, "\n")
	symbols := make([]symbolDraft, 0)
	for i, line := range lines {
		if match := byondDMIStateRe.FindStringSubmatch(line); len(match) > 0 {
			symbols = append(symbols, symbolDraft{
				kind:      "state",
				name:      strings.Trim(match[1], `"`),
				startLine: i + 1,
				endLine:   i + 1,
				metadata:  graph.JSONBMap{"source": "byond_dmi_state"},
			})
		}
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("byond", "document", filepath.Base(path), 1, len(lines), text))
	}
	return dedupeSymbolDrafts(symbols), nil
}

func parseByondDMM(path, text string) ([]symbolDraft, []string) {
	lines := strings.Split(text, "\n")
	symbols := make([]symbolDraft, 0)
	seen := map[string]bool{}
	for i, line := range lines {
		if byondDMMGridRe.MatchString(line) {
			break
		}
		for _, match := range byondTypePathRefRe.FindAllStringSubmatch(line, -1) {
			name := byondNormalizePath(match[0])
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true
			symbols = append(symbols, symbolDraft{
				kind:      "map_reference",
				name:      name,
				startLine: i + 1,
				endLine:   i + 1,
				metadata:  graph.JSONBMap{"source": "byond_dmm_reference"},
			})
		}
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("byond", "document", filepath.Base(path), 1, len(lines), text))
	}
	return dedupeSymbolDrafts(symbols), nil
}

var (
	byondIncludeRe          = regexp.MustCompile(`(?i)^#include\s+["<]([^">]+)[">]`)
	byondAbsoluteProcRe     = regexp.MustCompile(`^(/[A-Za-z_][A-Za-z0-9_/]*)/(?:proc|verb)/([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	byondAbsoluteOverrideRe = regexp.MustCompile(`^(/[A-Za-z_][A-Za-z0-9_/]*)/([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	byondRelativeProcRe     = regexp.MustCompile(`^(proc|verb)/([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	byondRelativeOverrideRe = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*\([^)]*\)\s*$`)
	byondTypePathRefRe      = regexp.MustCompile(`/[A-Za-z_][A-Za-z0-9_]*(?:/[A-Za-z_][A-Za-z0-9_]*)+`)
	byondDMFWindowRe        = regexp.MustCompile(`^\s*window\s+"([^"]+)"\s*$`)
	byondDMFElementRe       = regexp.MustCompile(`^\s*elem\s+"([^"]+)"\s*$`)
	byondDMFTypeRe          = regexp.MustCompile(`^\s*type\s*=\s*(\S+)\s*$`)
	byondDMIStateRe         = regexp.MustCompile(`(?m)^\s*state\s*=\s*("[^"]*"|[^\r\n]+)`)
	byondDMMGridRe          = regexp.MustCompile(`^\(\s*\d+\s*,\s*\d+\s*,\s*\d+\s*\)\s*=`)
)

var byondControlWords = map[string]bool{
	"if": true, "for": true, "while": true, "switch": true, "return": true,
	"spawn": true, "sleep": true, "set": true, "var": true, "new": true,
	"else": true, "do": true, "try": true, "catch": true,
}

func byondAbsoluteTypePath(line string) (string, bool) {
	if strings.ContainsAny(line, "(=") {
		return "", false
	}
	line = strings.TrimSpace(line)
	if byondTypePathRefRe.FindString(line) != line || !strings.HasPrefix(line, "/") || strings.Contains(line, " ") || strings.Contains(line, "\t") {
		return "", false
	}
	if strings.HasSuffix(line, "/proc") || strings.HasSuffix(line, "/verb") || strings.Contains(line, "/var/") {
		return "", false
	}
	return line, true
}

func byondExcludedOwnerPath(path string) bool {
	path = byondNormalizePath(path)
	return path == "" || strings.HasSuffix(path, "/var") || strings.Contains(path, "/var/")
}

func byondNormalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return strings.TrimRight(path, "/")
}

func byondIndent(line string) int {
	indent := 0
	for _, r := range line {
		switch r {
		case '\t':
			indent++
		case ' ':
			indent++
		default:
			return indent
		}
	}
	return indent
}

func byondCodeLine(line string, inBlockComment *bool) string {
	var out strings.Builder
	inString := false
	quote := rune(0)
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := rune(line[i])
		if *inBlockComment {
			if ch == '*' && i+1 < len(line) && line[i+1] == '/' {
				*inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			out.WriteRune(ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				inString = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inString = true
			quote = ch
			out.WriteRune(ch)
			continue
		}
		if ch == '/' && i+1 < len(line) {
			switch line[i+1] {
			case '/':
				return out.String()
			case '*':
				*inBlockComment = true
				i++
				continue
			}
		}
		out.WriteRune(ch)
	}
	return out.String()
}

func parseDelphiRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".lpk":
		return parseLazarusPackageRegex(path, text)
	default:
		return parseDelphiFormRegex(path, text)
	}
}

func parseDelphiFormRegex(path, text string) ([]symbolDraft, []string) {
	lines := strings.Split(text, "\n")
	symbols := make([]symbolDraft, 0)
	seen := map[string]bool{}
	add := func(kind, name string, line int, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := kind + "\x00" + name + "\x00" + itoa(line)
		if seen[key] {
			return
		}
		seen[key] = true
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: line,
			endLine:   line,
			metadata:  graph.JSONBMap{"source": source},
		})
	}
	for i, line := range lines {
		lineNo := i + 1
		if match := delphiComponentRe.FindStringSubmatch(line); len(match) > 0 {
			add("component", match[1], lineNo, "delphi_form_component")
			add("component_type", match[2], lineNo, "delphi_form_component_type")
			continue
		}
		if match := delphiEventRe.FindStringSubmatch(line); len(match) > 0 {
			add("event", match[1], lineNo, "delphi_form_event")
		}
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("delphi", "document", filepath.Base(path), 1, len(lines), text))
	}
	return dedupeSymbolDrafts(symbols), nil
}

func parseLazarusPackageRegex(path, text string) ([]symbolDraft, []string) {
	lines := strings.Split(text, "\n")
	symbols := make([]symbolDraft, 0)
	imports := make([]string, 0)
	seen := map[string]bool{}
	add := func(kind, name string, line int, source string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		key := kind + "\x00" + name + "\x00" + itoa(line)
		if seen[key] {
			return
		}
		seen[key] = true
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: line,
			endLine:   line,
			metadata:  graph.JSONBMap{"source": source},
		})
	}
	for i, line := range lines {
		lineNo := i + 1
		if match := delphiPackageNameRe.FindStringSubmatch(line); len(match) > 0 {
			add("package", match[1], lineNo, "lazarus_package")
			continue
		}
		if match := delphiPackageDependencyRe.FindStringSubmatch(line); len(match) > 0 {
			add("dependency", match[1], lineNo, "lazarus_package_dependency")
			imports = append(imports, match[1])
			continue
		}
		if match := delphiPackageUnitRe.FindStringSubmatch(line); len(match) > 0 {
			add("unit", match[1], lineNo, "lazarus_package_unit")
		}
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("delphi", "document", filepath.Base(path), 1, len(lines), text))
	}
	return dedupeSymbolDrafts(symbols), uniqueStrings(imports)
}

var (
	delphiComponentRe         = regexp.MustCompile(`(?i)^\s*(?:object|inherited)\s+([A-Za-z_][A-Za-z0-9_]*)\s*:\s*([A-Za-z_][A-Za-z0-9_]*)`)
	delphiEventRe             = regexp.MustCompile(`(?i)^\s*On[A-Za-z0-9_]+\s*=\s*([A-Za-z_][A-Za-z0-9_]*)`)
	delphiPackageNameRe       = regexp.MustCompile(`(?i)<Name\s+Value="([^"]+)"`)
	delphiPackageDependencyRe = regexp.MustCompile(`(?i)<PackageName\s+Value="([^"]+)"`)
	delphiPackageUnitRe       = regexp.MustCompile(`(?i)<UnitName\s+Value="([^"]+)"`)
)

func astroFrontmatter(text string) (string, int) {
	lines := strings.SplitAfter(text, "\n")
	offset := 0
	for i, line := range lines {
		if strings.TrimSpace(line) != "---" {
			offset += len(line)
			continue
		}
		startOffset := offset + len(line)
		lineNo := i + 2
		offset += len(line)
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "---" {
				return text[startOffset:offset], lineNo
			}
			offset += len(lines[j])
		}
		return "", 0
	}
	return "", 0
}

func astroDestructureNames(list string) []string {
	out := make([]string, 0)
	for _, part := range strings.Split(list, ",") {
		part = strings.TrimSpace(part)
		if part == "" || strings.ContainsAny(part, "{}[]") {
			continue
		}
		if colon := strings.Index(part, ":"); colon >= 0 {
			part = strings.TrimSpace(part[:colon])
		}
		part = strings.Trim(part, ". ")
		if isIdent(part) {
			out = append(out, part)
		}
	}
	return out
}

func isIdent(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') {
				return false
			}
			continue
		}
		if r != '_' && (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
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
	case "objc":
		return parseObjCRegex(path, content)
	case "dart":
		return parseDartRegex(path, content)
	case "julia":
		return parseJuliaRegex(path, content)
	case "fortran":
		return parseFortranRegex(path, content)
	case "verilog":
		return parseVerilogRegex(path, content)
	case "dotnet":
		return parseDotnetRegex(path, content)
	case "delphi":
		return parseDelphiRegex(path, content)
	case "blade":
		return parseBladeRegex(path, content)
	case "ejs":
		return parseEJSRegex(path, content)
	case "ets":
		return parseETSRegex(path, content)
	case "razor":
		return parseRazorRegex(path, content)
	case "astro":
		return parseAstroRegex(path, content)
	case "apex":
		return parseApexRegex(path, content)
	case "byond":
		return parseByondRegex(path, content)
	default:
		return parseLightweightCodeSymbols(path, language, content)
	}
}

type regexSymbolRule struct {
	kind   string
	re     *regexp.Regexp
	groups []int
	sep    string
}

func symbolRule(kind, pattern string, groups ...int) regexSymbolRule {
	if len(groups) == 0 {
		groups = []int{1}
	}
	return regexSymbolRule{kind: kind, re: regexp.MustCompile(pattern), groups: groups, sep: "."}
}

var lightweightSymbolRules = map[string][]regexSymbolRule{
	"rust": {
		symbolRule("function", `(?m)^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?fn\s+((?:r#)?[A-Za-z_][A-Za-z0-9_]*)\s*(?:<[^\n{;]*>)?\s*\(`),
		symbolRule("type", `(?m)^\s*(?:pub(?:\([^)]*\))?\s+)?(?:struct|enum|trait|type|mod)\s+([A-Za-z_][A-Za-z0-9_]*)`),
		symbolRule("constant", `(?m)^\s*(?:pub(?:\([^)]*\))?\s+)?(?:const|static)\s+([A-Za-z_][A-Za-z0-9_]*)\s*:`),
	},
	"ruby": {
		symbolRule("class", `(?m)^\s*(?:class|module)\s+(?:::)?([A-Z][A-Za-z0-9_:]*)`),
		symbolRule("method", `(?m)(?:^|;)\s*def\s+(?:(?:self|@@?[A-Za-z_][A-Za-z0-9_]*)\.)?([A-Za-z_][A-Za-z0-9_!?=]*)`),
		symbolRule("method", `(?m)(?:^|;)\s*def\s+(?:(?:self|@@?[A-Za-z_][A-Za-z0-9_]*)\.)?(\[\]=|\[\]|<<|<=>|==|===|!=|=~|!~|[+\-*/%&|^~]|[<>]=?)`),
	},
	"kotlin": {
		symbolRule("type", `(?m)^\s*(?:(?:public|private|protected|internal|actual|expect|data|sealed|enum|open|abstract|value|annotation|fun)\s+)*(?:class|interface|object)\s+([A-Za-z_][A-Za-z0-9_]*)`),
		symbolRule("function", `(?m)^\s*(?:(?:public|private|protected|internal|actual|expect|override|suspend|inline|tailrec|operator|infix|open|final|abstract|external)\s+)*fun\s+(?:[A-Za-z0-9_.<>?]+\.)?([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		symbolRule("variable", `(?m)^\s*(?:(?:public|private|protected|internal|actual|expect|override|open|final|const|lateinit|external)\s+)*(?:val|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*[:=]`),
	},
	"scala": {
		symbolRule("type", `(?m)^\s*(?:(?:private(?:\[[^\]]+\])?|protected(?:\[[^\]]+\])?|final|sealed|abstract|case|implicit|given|transparent|open)\s+)*(?:class|trait|object|enum)\s+([^\s\[{]+)`),
		symbolRule("type", `(?m)^\s*(?:(?:override|private(?:\[[^\]]+\])?|protected(?:\[[^\]]+\])?|final|sealed|abstract|opaque)\s+)*type\s+([^\s\[\]=]+)`),
		symbolRule("function", "(?m)(?:^|[;{])\\s*(?:@[A-Za-z_][A-Za-z0-9_]*(?:\\([^\\n]*\\))?\\s+)*(?:(?:override|private(?:\\[[^\\]]+\\])?|protected(?:\\[[^\\]]+\\])?|final|implicit|inline|transparent|extension)\\s+)*def\\s+([A-Za-z_][A-Za-z0-9_]*|[!#%&*+./:<=>?@^|~-]+|`[^`]+`)\\s*(?:\\[|\\(|:|=|$)"),
		symbolRule("variable", `(?m)^\s*(?:(?:override|private(?:\[[^\]]+\])?|protected(?:\[[^\]]+\])?|final|implicit|lazy|inline)\s+)*(?:val|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*[:=]`),
	},
	"php": {
		symbolRule("type", `(?m)^\s*(?:final\s+|abstract\s+)?(?:class|interface|trait|enum)\s+([A-Za-z_][A-Za-z0-9_]*)`),
		symbolRule("function", `(?m)^\s*(?:public|private|protected|static|final|abstract|\s)*function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		symbolRule("namespace", `(?m)^\s*namespace\s+([A-Za-z_\\][A-Za-z0-9_\\]*)\s*;`),
	},
	"p4": {
		// P4_16 top-level declarations (the packet-processing DSL, github.com/p4lang).
		symbolRule("parser", `(?m)^\s*parser\s+([A-Za-z_][A-Za-z0-9_]*)\s*[(<]`),
		symbolRule("control", `(?m)^\s*control\s+([A-Za-z_][A-Za-z0-9_]*)\s*[(<]`),
		symbolRule("package", `(?m)^\s*package\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		symbolRule("action", `(?m)^\s*action\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		symbolRule("table", `(?m)^\s*table\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`),
		symbolRule("header_union", `(?m)^\s*header_union\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`),
		symbolRule("header", `(?m)^\s*header\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`),
		symbolRule("struct", `(?m)^\s*struct\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`),
		symbolRule("enum", `(?m)^\s*enum\s+(?:bit<\d+>\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*\{`),
		symbolRule("extern", `(?m)^\s*extern\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`),
		symbolRule("type", `(?m)^\s*(?:typedef|type)\s+.+\s+([A-Za-z_][A-Za-z0-9_]*)\s*;`),
		symbolRule("constant", `(?m)^\s*const\s+.+\s+([A-Za-z_][A-Za-z0-9_]*)\s*=`),
		symbolRule("state", `(?m)^\s*state\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`),
	},
	"swift": {
		symbolRule("type", `(?m)^\s*(?:public|private|internal|open|final|\s)*(?:class|struct|enum|protocol|actor|extension)\s+([A-Za-z_][A-Za-z0-9_]*)`),
		symbolRule("function", `(?m)^\s*(?:public|private|internal|open|static|class|mutating|async|override|\s)*func\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		symbolRule("function", `(?m)^\s*(?:public|private|internal|open|convenience|required|override|\s)*(init)\s*\(`),
		symbolRule("variable", `(?m)^\s*(?:public|private|internal|open|static|\s)*(?:let|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*[:=]`),
	},
	"lua": {
		symbolRule("function", `(?m)^\s*(?:local\s+)?function\s+([A-Za-z_][A-Za-z0-9_.:]*)\s*\(`),
		symbolRule("function", `(?m)^\s*([A-Za-z_][A-Za-z0-9_.:]*)\s*=\s*function\s*\(`),
		symbolRule("variable", `(?m)^\s*(?:local\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=`),
	},
	"zig": {
		symbolRule("function", `(?m)^\s*(?:(?:pub|inline|noinline|export|extern)\s+)*fn\s+([A-Za-z_][A-Za-z0-9_]*|@"[^"]+")\s*\(`),
		symbolRule("type", `(?m)^\s*(?:pub\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:(?:packed|extern)\s+)?(?:struct|enum|union)\b`),
		symbolRule("constant", `(?m)^\s*(?:pub\s+)?const\s+([A-Za-z_][A-Za-z0-9_]*|@"[^"]+")\s*(?:[:=]|,)`),
		symbolRule("constant", `,\s*const\s+([A-Za-z_][A-Za-z0-9_]*|@"[^"]+")\s*(?:[:=]|,)`),
	},
	"powershell": {
		symbolRule("function", `(?mi)^\s*function\s+([A-Za-z_][A-Za-z0-9_-]*)`),
		symbolRule("variable", `(?m)^\s*\$([A-Za-z_][A-Za-z0-9_]*)\s*=`),
	},
	"elixir": {
		symbolRule("module", `(?m)^\s*defmodule\s+([A-Za-z_][A-Za-z0-9_.]*)\s+do`),
		symbolRule("protocol", `(?m)^\s*defprotocol\s+([A-Za-z_][A-Za-z0-9_.]*)\s+do`),
		symbolRule("implementation", `(?m)^\s*defimpl\s+([A-Za-z_][A-Za-z0-9_.]*)\b`),
		symbolRule("function", `(?m)^\s*defp?\s+([A-Za-z_][A-Za-z0-9_]*(?:[!?])?)\s*(?:\(|,|\b)`),
		symbolRule("macro", `(?m)^\s*defmacrop?\s+([A-Za-z_][A-Za-z0-9_]*(?:[!?])?)\s*(?:\(|,|\b)`),
		symbolRule("delegate", `(?m)^\s*defdelegate\s+([A-Za-z_][A-Za-z0-9_]*(?:[!?])?)\s*(?:\(|,|\b)`),
		symbolRule("guard", `(?m)^\s*defguardp?\s+([A-Za-z_][A-Za-z0-9_]*(?:[!?])?)\s*(?:\(|,|\b)`),
	},
	"fortran": {
		symbolRule("module", `(?mi)^\s*module\s+([A-Za-z_][A-Za-z0-9_]*)`),
		symbolRule("type", `(?mi)^\s*type\s*(?:,\s*(?:abstract|public|private|extends\([^)]+\)))?\s*::\s*([A-Za-z_][A-Za-z0-9_]*)`),
		symbolRule("function", `(?mi)^\s*(?:(?:recursive|pure|elemental|impure|module)\s+)*(?:subroutine|function)\s+([A-Za-z_][A-Za-z0-9_]*)`),
	},
	"verilog": {
		symbolRule("module", `(?m)^\s*module\s+([A-Za-z_][A-Za-z0-9_$]*)`),
		symbolRule("interface", `(?m)^\s*interface\s+([A-Za-z_][A-Za-z0-9_$]*)`),
		symbolRule("package", `(?m)^\s*package\s+([A-Za-z_][A-Za-z0-9_$]*)`),
		symbolRule("function", `(?m)^\s*(?:function|task)\s+(?:automatic\s+)?(?:[A-Za-z_][A-Za-z0-9_$]*\s+)?([A-Za-z_][A-Za-z0-9_$]*)`),
	},
	"pascal": {
		symbolRule("unit", `(?mi)^\s*(?:unit|program|library|package)\s+([A-Za-z_][A-Za-z0-9_]*)`),
		symbolRule("type", `(?mi)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:class|record|interface|object)`),
		symbolRule("function", `(?mi)^\s*(?:class\s+)?(?:procedure|function|constructor|destructor)\s+([A-Za-z_][A-Za-z0-9_.]*)`),
	},
	"delphi": {
		symbolRule("component", `(?mi)^\s*(?:object|inherited)\s+([A-Za-z_][A-Za-z0-9_]*)\s*:`),
	},
	"sql": {
		symbolRule("table", `(?mi)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?([A-Za-z_"][A-Za-z0-9_."$]*)`),
		symbolRule("view", `(?mi)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+([A-Za-z_"][A-Za-z0-9_."$]*)`),
		symbolRule("function", `(?mi)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?(?:FUNCTION|PROCEDURE)\s+([A-Za-z_"][A-Za-z0-9_."$]*)`),
		symbolRule("trigger", `(?mi)^\s*CREATE\s+(?:OR\s+REPLACE\s+)?TRIGGER\s+([A-Za-z_"][A-Za-z0-9_."$]*)`),
	},
	"terraform": {
		symbolRule("resource", `(?m)^\s*resource\s+"([^"]+)"\s+"([^"]+)"`, 1, 2),
		symbolRule("data", `(?m)^\s*data\s+"([^"]+)"\s+"([^"]+)"`, 1, 2),
		symbolRule("module", `(?m)^\s*module\s+"([^"]+)"`),
		symbolRule("variable", `(?m)^\s*variable\s+"([^"]+)"`),
		symbolRule("output", `(?m)^\s*output\s+"([^"]+)"`),
	},
	"byond": {
		symbolRule("type", `(?m)^\s*/([A-Za-z_][A-Za-z0-9_/]*)\s*$`),
		symbolRule("function", `(?m)^\s*(?:/[A-Za-z0-9_/]+/)?proc/([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		symbolRule("function", `(?m)^\s*([A-Za-z_][A-Za-z0-9_]*)\s*\([^)]*\)\s*$`),
	},
	"dotnet": {
		symbolRule("project", `(?i)<Project\s+Sdk="([^"]+)"`),
		symbolRule("package", `(?i)<PackageReference\s+Include="([^"]+)"`),
		symbolRule("project_reference", `(?i)<ProjectReference\s+Include="([^"]+)"`),
	},
	"razor": {
		symbolRule("route", `(?m)^\s*@page\s+"([^"]+)"`),
		symbolRule("method", `(?m)^\s*(?:public|private|protected|internal|static|async|override|virtual|\s)+[A-Za-z0-9_<>,\[\]?]+\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		symbolRule("component", `(?m)<([A-Z][A-Za-z0-9]+)[\s/>]`),
	},
	"apex": {
		symbolRule("type", `(?mi)^\s*(?:public|private|global|with\s+sharing|without\s+sharing|abstract|virtual|\s)*(?:class|interface|enum)\s+([A-Za-z_][A-Za-z0-9_]*)`),
		symbolRule("trigger", `(?mi)^\s*trigger\s+([A-Za-z_][A-Za-z0-9_]*)\s+on\s+([A-Za-z_][A-Za-z0-9_]*)`, 1),
		symbolRule("method", `(?mi)^\s*(?:public|private|global|protected|static|override|virtual|\s)+[A-Za-z0-9_<>,\[\]]+\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
	},
	"blade": {
		symbolRule("include", `@include\(['"]([^'"]+)['"]\)`),
		symbolRule("component", `<livewire:([A-Za-z0-9_.-]+)`),
		symbolRule("handler", `wire:click=['"]([^'"]+)['"]`),
	},
	"vue": {
		symbolRule("function", `(?m)^\s*(?:function|const|let)\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:=|\()`),
	},
	"svelte": {
		symbolRule("function", `(?m)^\s*(?:function|const|let)\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:=|\()`),
	},
	"astro": {
		symbolRule("function", `(?m)^\s*(?:function|const|let)\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:=|\()`),
	},
	"ejs": {
		symbolRule("template", `(?m)<%\s*(?:-|=|_)?\s*(?:include|await\s+include)\s*\(?\s*['"]([^'"]+)['"]`),
		symbolRule("function", `(?m)<%[\s\S]*?\bfunction\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		symbolRule("variable", `(?m)<%[\s\S]*?\b(?:const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*=`),
	},
	"ets": {
		symbolRule("type", `(?m)^\s*(?:@[A-Za-z_][A-Za-z0-9_]*\s*)*(?:export\s+)?(?:abstract\s+)?(?:class|struct|interface|enum|namespace)\s+([A-Za-z_][A-Za-z0-9_]*)`),
		symbolRule("function", `(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`),
		symbolRule("method", `(?m)^\s*(?:public|private|protected|static|async|override|\s)*(build|aboutToAppear|aboutToDisappear|onPageShow|onPageHide|onBackPress|[a-z_][A-Za-z0-9_]*)\s*\(`),
		symbolRule("variable", `(?m)^\s*(?:@(?:State|Prop|Link|Provide|Consume|StorageLink|StorageProp|LocalStorageLink|LocalStorageProp|ObjectLink|Watch)\s*)?(?:private\s+|public\s+|protected\s+)?(?:const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)\s*[:=]`),
		symbolRule("variable", `(?m)^\s*@(?:State|Prop|Link|Provide|Consume|StorageLink|StorageProp|LocalStorageLink|LocalStorageProp|ObjectLink|Watch)\s+([A-Za-z_][A-Za-z0-9_]*)\s*[:=]`),
	},
	"r": {
		symbolRule("function", `(?m)^\s*([A-Za-z.][A-Za-z0-9._]*)\s*(?:<-|=)\s*function\s*\(`),
		symbolRule("function", `(?m)^\s*([A-Za-z.][A-Za-z0-9._]*)\s*<<-\s*function\s*\(`),
		symbolRule("type", `(?m)^\s*setClass\s*\(\s*["']([^"']+)["']`),
		symbolRule("type", `(?m)^\s*([A-Za-z.][A-Za-z0-9._]*)\s*<-\s*R6::R6Class\s*\(\s*["']([^"']+)["']`, 1),
		symbolRule("type", `(?m)^\s*([A-Za-z.][A-Za-z0-9._]*)\s*<-\s*ggproto\s*\(\s*["']([^"']+)["']`, 1),
		symbolRule("variable", `(?m)^\s*([A-Za-z.][A-Za-z0-9._]*)\s*(?:<-|=)\s*(?:new\.env\s*\(|c\s*\(|list\s*\(|data\.frame\s*\(|tibble\s*\(|["'0-9\[])`),
	},
}

var lightweightImportRules = map[string][]*regexp.Regexp{
	"rust":   {regexp.MustCompile(`(?m)^\s*use\s+([^;]+);`)},
	"ruby":   {regexp.MustCompile(`(?m)^\s*require(?:_relative)?\s+['"]([^'"]+)['"]`)},
	"kotlin": {regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z0-9_.*]+)`)},
	"scala":  {regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z0-9_.*{}]+)`)},
	"php":    {regexp.MustCompile(`(?m)^\s*use\s+([^;]+);`), regexp.MustCompile(`(?m)^\s*(?:require|include)(?:_once)?\s*\(?\s*['"]([^'"]+)['"]`)},
	"swift":  {regexp.MustCompile(`(?m)^\s*import\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	"lua":    {regexp.MustCompile(`require\s*\(\s*['"]([^'"]+)['"]\s*\)`)},
	"zig":    {regexp.MustCompile(`@import\s*\(\s*"([^"]+)"\s*\)`)},
	"powershell": {
		regexp.MustCompile(`(?mi)^\s*(?:Import-Module|using\s+module)\s+['"]?([^'"\r\n]+)`),
	},
	"elixir":  {regexp.MustCompile(`(?m)^\s*(?:alias|import|require|use)\s+([A-Za-z0-9_.]+)`)},
	"objc":    {regexp.MustCompile(`(?m)^\s*#import\s+[<"]([^>"]+)[>"]`)},
	"julia":   {regexp.MustCompile(`(?m)^\s*(?:using|import)\s+([A-Za-z0-9_.,: ]+)`)},
	"fortran": {regexp.MustCompile(`(?mi)^\s*use\s+([A-Za-z_][A-Za-z0-9_]*)`)},
	"dart":    {regexp.MustCompile(`(?m)^\s*import\s+['"]([^'"]+)['"]`)},
	"verilog": {regexp.MustCompile(`(?m)^\s*import\s+([^;]+);`)},
	"pascal":  {regexp.MustCompile(`(?mi)^\s*uses\s+([^;]+);`)},
	"dotnet":  {regexp.MustCompile(`(?i)<ProjectReference\s+Include="([^"]+)"`), regexp.MustCompile(`(?i)<PackageReference\s+Include="([^"]+)"`)},
	"razor":   {regexp.MustCompile(`(?m)^\s*@using\s+([A-Za-z0-9_.]+)`), regexp.MustCompile(`(?m)^\s*@inject\s+([A-Za-z0-9_.<>\[\]]+)`)},
	"blade":   {regexp.MustCompile(`@include\(['"]([^'"]+)['"]\)`)},
	"vue":     {regexp.MustCompile(`(?m)^\s*import\s+[^'"]*['"]([^'"]+)['"]`)},
	"svelte":  {regexp.MustCompile(`(?m)^\s*import\s+[^'"]*['"]([^'"]+)['"]`)},
	"astro":   {regexp.MustCompile(`(?m)^\s*import\s+[^'"]*['"]([^'"]+)['"]`)},
	"ejs":     {regexp.MustCompile(`<%\s*(?:-|=|_)?\s*(?:include|await\s+include)\s*\(?\s*['"]([^'"]+)['"]`)},
	"ets":     {regexp.MustCompile(`(?m)^\s*import\s+[^'"]*['"]([^'"]+)['"]`)},
	"r": {
		regexp.MustCompile(`(?m)^\s*(?:library|require)\s*\(\s*['"]?([A-Za-z0-9._]+)['"]?`),
		regexp.MustCompile(`(?m)^\s*source\s*\(\s*['"]([^'"]+)['"]`),
	},
}

func parseLightweightCodeSymbols(path, language string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	scanText := text
	if language == "elixir" {
		scanText = maskElixirNonCode(text)
	}
	imports := regexCaptures(scanText, lightweightImportRules[language]...)
	rules := lightweightSymbolRules[language]
	symbols := make([]symbolDraft, 0)
	for _, rule := range rules {
		for _, match := range rule.re.FindAllStringSubmatchIndex(scanText, -1) {
			name := regexRuleName(scanText, match, rule)
			if name == "" {
				continue
			}
			start := lineForOffset(scanText, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      rule.kind,
				name:      name,
				startLine: start,
				endLine:   blockEndLine(text, match[0], start),
				metadata:  graph.JSONBMap{"source": "regex_lightweight"},
			})
		}
	}
	if len(symbols) == 0 && language != "" {
		symbols = append(symbols, docDraft(language, "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), imports
}

func maskElixirNonCode(text string) string {
	buf := []byte(text)
	for i := 0; i < len(buf); {
		switch {
		case hasAt(buf, i, `"""`):
			i = blankThroughDelimiter(buf, i, `"""`)
		case hasAt(buf, i, `'''`):
			i = blankThroughDelimiter(buf, i, `'''`)
		case buf[i] == '#':
			for i < len(buf) && buf[i] != '\n' {
				buf[i] = ' '
				i++
			}
		default:
			i++
		}
	}
	return string(buf)
}

func parseFortranRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	imports := regexCaptures(text, lightweightImportRules["fortran"]...)
	symbols := make([]symbolDraft, 0)
	addNamed := func(kind string, re *regexp.Regexp, skip map[string]bool) {
		for _, match := range re.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 4 {
				continue
			}
			lineStart := strings.LastIndex(text[:match[0]], "\n") + 1
			lineEnd := strings.IndexByte(text[match[0]:], '\n')
			if lineEnd < 0 {
				lineEnd = len(text)
			} else {
				lineEnd += match[0]
			}
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(text[lineStart:lineEnd])), "end ") {
				continue
			}
			name := strings.TrimSpace(text[match[2]:match[3]])
			if name == "" || skip[strings.ToLower(name)] {
				continue
			}
			start := lineForOffset(text, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      kind,
				name:      name,
				startLine: start,
				endLine:   blockEndLine(text, match[0], start),
				metadata:  graph.JSONBMap{"source": "regex_fortran"},
			})
		}
	}
	ident := `([A-Za-z_][A-Za-z0-9_]*)`
	addNamed("module", regexp.MustCompile(`(?mi)^[ \t]*module[ \t]+`+ident), map[string]bool{
		"procedure":  true,
		"function":   true,
		"subroutine": true,
	})
	addNamed("type", regexp.MustCompile(`(?mi)^[ \t]*type[ \t]*(?:,[ \t]*(?:abstract|public|private|extends\([^)]+\)))*[ \t]*::[ \t]*`+ident), nil)
	addNamed("type", regexp.MustCompile(`(?mi)^[ \t]*type[ \t]+`+ident+`\b`), map[string]bool{"is": true})
	fortranTypePrefix := `(?:(?:[A-Za-z_][A-Za-z0-9_]*(?:\([^)\r\n]*\))?|type\([^)\r\n]+\)|class\([^)\r\n]+\))[ \t]+)*`
	addNamed("function", regexp.MustCompile(`(?mi)^[ \t]*(?:(?:recursive|pure|elemental|impure)[ \t]+)*`+fortranTypePrefix+`(?:module[ \t]+)?(?:subroutine|function)[ \t]+`+ident), nil)
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("fortran", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), imports
}

func parseVerilogRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	imports := regexCaptures(text, lightweightImportRules["verilog"]...)
	symbols := make([]symbolDraft, 0)
	addNamed := func(kind string, re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 4 {
				continue
			}
			name := strings.TrimSpace(text[match[2]:match[3]])
			if name == "" {
				continue
			}
			start := lineForOffset(text, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      kind,
				name:      name,
				startLine: start,
				endLine:   blockEndLine(text, match[0], start),
				metadata:  graph.JSONBMap{"source": "regex_verilog"},
			})
		}
	}
	ident := `([A-Za-z_][A-Za-z0-9_$]*)`
	addNamed("module", regexp.MustCompile(`(?m)^[ \t]*(?:module|macromodule)[ \t]+`+ident))
	addNamed("interface", regexp.MustCompile(`(?m)^[ \t]*interface[ \t]+`+ident))
	addNamed("package", regexp.MustCompile(`(?m)^[ \t]*package[ \t]+`+ident))
	addNamed("class", regexp.MustCompile(`(?m)^[ \t]*(?:virtual[ \t]+)?class[ \t]+`+ident))

	for i, line := range strings.Split(text, "\n") {
		trimmed := stripVerilogLineComment(line)
		kind, name := verilogCallableSymbol(trimmed)
		if name == "" {
			continue
		}
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: i + 1,
			endLine:   i + 1,
			metadata:  graph.JSONBMap{"source": "regex_verilog"},
		})
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("verilog", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), imports
}

func stripVerilogLineComment(line string) string {
	if idx := strings.Index(line, "//"); idx >= 0 {
		return line[:idx]
	}
	return line
}

func verilogCallableSymbol(line string) (string, string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "endfunction") || strings.HasPrefix(trimmed, "endtask") {
		return "", ""
	}
	kind := ""
	switch {
	case strings.HasPrefix(trimmed, "function"):
		kind = "function"
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "function"))
	case strings.HasPrefix(trimmed, "task"):
		kind = "task"
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "task"))
	default:
		return "", ""
	}
	if strings.HasPrefix(trimmed, "automatic ") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "automatic "))
	} else if strings.HasPrefix(trimmed, "static ") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "static "))
	}
	cut := len(trimmed)
	for _, sep := range []string{"(", ";"} {
		if idx := strings.Index(trimmed, sep); idx >= 0 && idx < cut {
			cut = idx
		}
	}
	prefix := strings.TrimSpace(trimmed[:cut])
	fields := strings.Fields(prefix)
	if len(fields) == 0 {
		return "", ""
	}
	name := strings.Trim(fields[len(fields)-1], " ,;")
	if idx := strings.LastIndex(name, "::"); idx >= 0 {
		name = name[idx+2:]
	}
	if !isVerilogIdentifier(name) {
		return "", ""
	}
	return kind, name
}

func isVerilogIdentifier(name string) bool {
	if name == "" {
		return false
	}
	for i, ch := range name {
		if i == 0 {
			if ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
				continue
			}
			return false
		}
		if ch == '_' || ch == '$' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') {
			continue
		}
		return false
	}
	return true
}

func parseJuliaRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	scanText := maskJuliaNonCode(text)
	imports := regexCaptures(scanText, lightweightImportRules["julia"]...)
	symbols := make([]symbolDraft, 0)
	addNamed := func(kind string, re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(scanText, -1) {
			if len(match) < 4 {
				continue
			}
			line := lineForOffset(scanText, match[0])
			name := strings.TrimSpace(scanText[match[2]:match[3]])
			if name == "" {
				continue
			}
			symbols = append(symbols, symbolDraft{
				kind:      kind,
				name:      name,
				startLine: line,
				endLine:   blockEndLine(scanText, match[0], line),
				metadata:  graph.JSONBMap{"source": "regex_julia"},
			})
		}
	}
	ident := `[A-Za-z_][A-Za-z0-9_!]*`
	addNamed("module", regexp.MustCompile(`(?m)^[ \t]*(?:baremodule|module)[ \t]+(`+ident+`)`))
	juliaLeadingMacros := `(?:@[A-Za-z_][A-Za-z0-9_!]*(?:\([^)\r\n]*\))?[ \t]+)*`
	addNamed("type", regexp.MustCompile(`(?m)^[ \t]*`+juliaLeadingMacros+`(?:mutable[ \t]+)?struct[ \t]+(`+ident+`)`))
	addNamed("type", regexp.MustCompile(`(?m)^[ \t]*abstract[ \t]+type[ \t]+(`+ident+`)`))
	addNamed("type", regexp.MustCompile(`(?m)^[ \t]*primitive[ \t]+type[ \t]+(`+ident+`)`))
	addNamed("macro", regexp.MustCompile(`(?m)^[ \t]*macro[ \t]+(`+ident+`)\b`))
	addNamed("constant", regexp.MustCompile(`(?m)^[ \t]*const[ \t]+(`+ident+`)\b`))

	functionBlock := regexp.MustCompile(`^[ \t]*(?:@[A-Za-z_][A-Za-z0-9_!]*(?:\([^)\r\n]*\))?[ \t]+)*function[ \t]+(.+)$`)
	lines := strings.Split(scanText, "\n")
	pendingAssignment := ""
	pendingStartLine := 0
	for i, line := range lines {
		if match := functionBlock.FindStringSubmatch(line); len(match) == 2 {
			if name := juliaFunctionName(match[1]); name != "" {
				symbols = append(symbols, symbolDraft{
					kind:      "function",
					name:      name,
					startLine: i + 1,
					endLine:   i + 1,
					metadata:  graph.JSONBMap{"source": "regex_julia"},
				})
			}
			continue
		}
		if pendingAssignment != "" {
			pendingAssignment += " " + strings.TrimSpace(line)
			if name := juliaAssignmentFunctionName(pendingAssignment); name != "" {
				symbols = append(symbols, symbolDraft{
					kind:      "function",
					name:      name,
					startLine: pendingStartLine,
					endLine:   i + 1,
					metadata:  graph.JSONBMap{"source": "regex_julia"},
				})
				pendingAssignment = ""
				pendingStartLine = 0
				continue
			}
			if i+1-pendingStartLine >= 8 || juliaParenDepth(pendingAssignment) <= 0 && strings.TrimSpace(line) == "" {
				pendingAssignment = ""
				pendingStartLine = 0
			}
			continue
		}
		if name := juliaAssignmentFunctionName(line); name != "" {
			symbols = append(symbols, symbolDraft{
				kind:      "function",
				name:      name,
				startLine: i + 1,
				endLine:   i + 1,
				metadata:  graph.JSONBMap{"source": "regex_julia"},
			})
			continue
		}
		if juliaLooksLikeAssignmentSignatureStart(line) {
			pendingAssignment = strings.TrimSpace(line)
			pendingStartLine = i + 1
		}
	}
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("julia", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), imports
}

func maskJuliaNonCode(text string) string {
	buf := []byte(text)
	for i := 0; i < len(buf); {
		switch {
		case hasAt(buf, i, `"""`):
			i = blankThroughDelimiter(buf, i, `"""`)
		case hasAt(buf, i, `'''`):
			i = blankThroughDelimiter(buf, i, `'''`)
		case buf[i] == '#':
			for i < len(buf) && buf[i] != '\n' {
				buf[i] = ' '
				i++
			}
		case buf[i] == '"':
			i = blankJuliaQuoted(buf, i, '"')
		case buf[i] == '\'':
			i = blankJuliaQuoted(buf, i, '\'')
		default:
			i++
		}
	}
	return string(buf)
}

func blankJuliaQuoted(buf []byte, start int, quote byte) int {
	buf[start] = ' '
	i := start + 1
	escaped := false
	for i < len(buf) {
		ch := buf[i]
		if ch != '\n' {
			buf[i] = ' '
		}
		if escaped {
			escaped = false
		} else if ch == '\\' {
			escaped = true
		} else if ch == quote {
			i++
			break
		}
		i++
	}
	return i
}

func juliaAssignmentFunctionName(line string) string {
	trimmed := stripJuliaLeadingAnnotations(strings.TrimSpace(line))
	if trimmed == "" || strings.HasPrefix(trimmed, "const ") {
		return ""
	}
	if juliaStartsWithKeyword(trimmed) {
		return ""
	}
	eq := juliaAssignmentOperatorIndex(trimmed)
	if eq < 0 {
		return ""
	}
	lhs := strings.TrimSpace(trimmed[:eq])
	if !strings.Contains(lhs, "(") || !strings.Contains(lhs, ")") {
		return ""
	}
	return juliaFunctionName(lhs)
}

func juliaAssignmentOperatorIndex(line string) int {
	depth := 0
	for i := 0; i < len(line); i++ {
		switch line[i] {
		case '(', '[', '{':
			depth++
			continue
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
			continue
		}
		if line[i] != '=' {
			continue
		}
		if depth > 0 {
			continue
		}
		prev := byte(0)
		next := byte(0)
		if i > 0 {
			prev = line[i-1]
		}
		if i+1 < len(line) {
			next = line[i+1]
		}
		if prev == '=' || prev == '!' || prev == '<' || prev == '>' || next == '=' || next == '>' {
			continue
		}
		return i
	}
	return -1
}

func juliaLooksLikeAssignmentSignatureStart(line string) bool {
	trimmed := stripJuliaLeadingAnnotations(strings.TrimSpace(line))
	if trimmed == "" || strings.HasPrefix(trimmed, "const ") {
		return false
	}
	if juliaStartsWithKeyword(trimmed) || juliaAssignmentOperatorIndex(trimmed) >= 0 {
		return false
	}
	if !strings.Contains(trimmed, "(") {
		return false
	}
	if juliaParenDepth(trimmed) <= 0 && !strings.HasSuffix(trimmed, ";") && !strings.HasSuffix(trimmed, ",") {
		return false
	}
	first := trimmed[0]
	if first == '(' {
		return true
	}
	return (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') || first == '_'
}

func stripJuliaLeadingAnnotations(text string) string {
	trimmed := strings.TrimSpace(text)
	for strings.HasPrefix(trimmed, "@") {
		i := 1
		for i < len(trimmed) {
			ch := trimmed[i]
			if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '!' || ch == '.' {
				i++
				continue
			}
			break
		}
		if i >= len(trimmed) {
			return ""
		}
		trimmed = strings.TrimSpace(trimmed[i:])
		if strings.HasPrefix(trimmed, "(") {
			if end := matchingParenIndex(trimmed, 0); end >= 0 {
				trimmed = strings.TrimSpace(trimmed[end+1:])
			}
		}
	}
	return trimmed
}

func juliaParenDepth(text string) int {
	depth := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return depth
}

func juliaFunctionName(signature string) string {
	signature = strings.TrimSpace(signature)
	signature = strings.TrimSuffix(signature, " end")
	if signature == "" {
		return ""
	}
	if idx := strings.Index(signature, "#"); idx >= 0 {
		signature = strings.TrimSpace(signature[:idx])
	}
	open := juliaSignatureArgumentOpen(signature)
	if open < 0 {
		fields := strings.Fields(signature)
		if len(fields) == 1 {
			return juliaCleanFunctionName(fields[0])
		}
		return ""
	}
	name := strings.TrimSpace(signature[:open])
	if strings.HasPrefix(name, "(") {
		name = juliaCallableTypeName(name)
	}
	return juliaCleanFunctionName(name)
}

func juliaSignatureArgumentOpen(signature string) int {
	for i := len(signature) - 1; i >= 0; i-- {
		if signature[i] != '(' {
			continue
		}
		if end := matchingParenIndex(signature, i); end >= 0 {
			tail := strings.TrimSpace(signature[end+1:])
			if tail == "" || strings.HasPrefix(tail, "where") || strings.HasPrefix(tail, "::") {
				return i
			}
		}
	}
	return -1
}

func matchingParenIndex(text string, open int) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func juliaCallableTypeName(name string) string {
	if close := strings.Index(name, ")"); close >= 0 {
		inner := strings.TrimSpace(strings.TrimPrefix(name[:close], "("))
		if idx := strings.Index(inner, "::"); idx >= 0 {
			return inner[idx+2:]
		}
		return inner
	}
	return name
}

func juliaCleanFunctionName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if idx := strings.Index(name, " where "); idx >= 0 {
		name = strings.TrimSpace(name[:idx])
	}
	if idx := strings.Index(name, "::"); idx >= 0 {
		name = strings.TrimSpace(name[:idx])
	}
	name = strings.TrimPrefix(name, "Base.@")
	if idx := strings.Index(name, "{"); idx >= 0 {
		name = strings.TrimSpace(name[:idx])
	}
	if strings.HasPrefix(name, "(") && strings.HasSuffix(name, ")") && !strings.Contains(name, ".:(") {
		name = strings.Trim(name, "()")
	}
	if name == "" || juliaStartsWithKeyword(name) {
		return ""
	}
	for _, ch := range name {
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' || ch == '!' || ch == '.' || ch == ':' || ch == '=' || ch == '<' || ch == '>' || ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '(' || ch == ')' {
			continue
		}
		return ""
	}
	return name
}

func juliaStartsWithKeyword(text string) bool {
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return false
	}
	switch fields[0] {
	case "if", "elseif", "else", "for", "while", "try", "catch", "finally", "return", "throw", "break", "continue", "let", "begin", "quote", "do", "end", "struct", "module", "using", "import", "export":
		return true
	default:
		return false
	}
}

func hasAt(buf []byte, i int, needle string) bool {
	return i+len(needle) <= len(buf) && string(buf[i:i+len(needle)]) == needle
}

func blankThroughDelimiter(buf []byte, start int, delimiter string) int {
	i := start
	for i < len(buf) {
		if i > start && hasAt(buf, i, delimiter) {
			for j := 0; j < len(delimiter); j++ {
				buf[i+j] = ' '
			}
			return i + len(delimiter)
		}
		if buf[i] != '\n' {
			buf[i] = ' '
		}
		i++
	}
	return i
}

func regexRuleName(text string, match []int, rule regexSymbolRule) string {
	parts := make([]string, 0, len(rule.groups))
	for _, group := range rule.groups {
		if group == 0 {
			parts = append(parts, rule.kind)
			continue
		}
		idx := group * 2
		if idx+1 >= len(match) || match[idx] < 0 || match[idx+1] < 0 {
			continue
		}
		parts = append(parts, strings.TrimSpace(text[match[idx]:match[idx+1]]))
	}
	return strings.TrimSpace(strings.Join(parts, rule.sep))
}

func dedupeSymbolDrafts(symbols []symbolDraft) []symbolDraft {
	seen := map[string]bool{}
	out := make([]symbolDraft, 0, len(symbols))
	for _, symbol := range symbols {
		key := symbol.kind + "\x00" + symbol.name + "\x00" + itoa(symbol.startLine)
		if symbol.name == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, symbol)
	}
	return out
}

func parseCSharpRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	imports := regexCaptures(text, regexp.MustCompile(`using\s+([\w.]+);`))
	symbols := make([]symbolDraft, 0)
	addNamed := func(kind string, re *regexp.Regexp) {
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
	addTyped := func(re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(text, -1) {
			if len(match) < 6 || match[2] < 0 || match[4] < 0 {
				continue
			}
			line := lineForOffset(text, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      text[match[2]:match[3]],
				name:      text[match[4]:match[5]],
				startLine: line,
				endLine:   blockEndLine(text, match[0], line),
			})
		}
	}
	attr := `(?:\s*\[[^\]\r\n]+(?:\([^\)]*\))?\]\s*)*`
	mods := `(?:(?:public|private|protected|internal|abstract|sealed|static|partial|readonly|unsafe|new|record)\s+)*`
	ident := `[A-Za-z_][A-Za-z0-9_]*`
	typeDecl := regexp.MustCompile(`(?m)^` + attr + `\s*` + mods + `(class|interface|struct|enum|record)\s+(` + ident + `)`)
	methodDecl := regexp.MustCompile(`(?m)^` + attr + `\s*` +
		`(?:(?:public|private|protected|internal|static|virtual|override|async|extern|unsafe|sealed|abstract|partial|new|readonly)\s+)*` +
		`(?:[A-Za-z_][A-Za-z0-9_<>,.\[\]?@]*\s+)+(` + ident + `)\s*(?:<[^>\r\n]+>)?\s*\([^;\r\n{}]*\)\s*(?:where\b[^{;=>\r\n]+)?(?:\{|=>|;)`)
	ctorDecl := regexp.MustCompile(`(?m)^` + attr + `\s*` +
		`(?:(?:public|private|protected|internal|static|extern|unsafe)\s+)*~?(` + ident + `)\s*\([^;\r\n{}]*\)\s*(?:\{|=>|;)`)
	addTyped(typeDecl)
	addNamed("method", methodDecl)
	addNamed("method", ctorDecl)
	return dedupeSymbolDrafts(symbols), imports
}

func parseGroovyRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	scanText := maskGroovyNonCode(text)
	imports := regexCaptures(scanText, regexp.MustCompile(`(?m)^\s*import\s+(?:static\s+)?([\w.*]+)`))
	symbols := make([]symbolDraft, 0)
	addNamed := func(kind string, re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(scanText, -1) {
			if len(match) < 4 {
				continue
			}
			line := lineForOffset(scanText, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      kind,
				name:      scanText[match[2]:match[3]],
				startLine: line,
				endLine:   blockEndLine(scanText, match[0], line),
			})
		}
	}
	addTyped := func(re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(scanText, -1) {
			if len(match) < 6 || match[2] < 0 || match[4] < 0 {
				continue
			}
			line := lineForOffset(scanText, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      scanText[match[2]:match[3]],
				name:      scanText[match[4]:match[5]],
				startLine: line,
				endLine:   blockEndLine(scanText, match[0], line),
			})
		}
	}
	addTask := func(re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(scanText, -1) {
			if len(match) < 4 {
				continue
			}
			group := len(match)/2 - 1
			if group >= 2 && match[4] >= 0 {
				group = 2
			}
			idx := group * 2
			if idx+1 >= len(match) || match[idx] < 0 || match[idx+1] < 0 {
				continue
			}
			line := lineForOffset(scanText, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      "task",
				name:      strings.Trim(scanText[match[idx]:match[idx+1]], `"'`),
				startLine: line,
				endLine:   blockEndLine(scanText, match[0], line),
			})
		}
	}
	attr := `(?:\s*@[A-Za-z_][A-Za-z0-9_.]*(?:\([^)\r\n]*\))?\s*)*`
	mods := `(?:(?:public|private|protected|static|final|abstract|synchronized|transient|volatile|native|strictfp)\s+)*`
	ident := `[A-Za-z_][A-Za-z0-9_]*`
	ret := `(?:def|void|boolean|byte|short|int|long|float|double|char|String|File|Path|URI|URL|Map|List|Set|Collection|Object|Closure|GString|BigInteger|BigDecimal|[A-Z][A-Za-z0-9_.$]*(?:<[^>\r\n]+>)?(?:\[\])?)`
	addTyped(regexp.MustCompile(`(?m)^` + attr + `\s*` + mods + `(class|interface|trait|enum)\s+(` + ident + `)`))
	addNamed("method", regexp.MustCompile(`(?m)^`+attr+`\s*`+mods+ret+`\s+(`+ident+`)\s*\([^;\r\n{}]*\)\s*(?:\{|$|;)`))
	addNamed("method", regexp.MustCompile(`(?m)^`+attr+`\s*`+mods+`~?([A-Z][A-Za-z0-9_]*)\s*\([^;\r\n{}]*\)\s*\{`))
	addTask(regexp.MustCompile(`(?m)^\s*task\s+['"]?([A-Za-z_][A-Za-z0-9_-]*)['"]?\b`))
	addTask(regexp.MustCompile(`(?m)\btasks\.(?:register|named)\(\s*['"]([^'"]+)['"]`))
	return dedupeSymbolDrafts(symbols), imports
}

func maskGroovyNonCode(text string) string {
	buf := []byte(text)
	for i := 0; i < len(buf); {
		switch {
		case hasAt(buf, i, `"""`):
			i = blankThroughDelimiter(buf, i, `"""`)
		case hasAt(buf, i, `'''`):
			i = blankThroughDelimiter(buf, i, `'''`)
		case hasAt(buf, i, `/*`):
			i = blankThroughDelimiter(buf, i, `*/`)
		case hasAt(buf, i, `//`):
			for i < len(buf) && buf[i] != '\n' {
				buf[i] = ' '
				i++
			}
		default:
			i++
		}
	}
	return string(buf)
}

func parseObjCRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	scanText := maskCStyleNonCode(text)
	imports := regexCaptures(scanText, regexp.MustCompile(`(?m)^\s*#import\s+[<"]([^>"]+)[>"]`))
	symbols := make([]symbolDraft, 0)
	addNamed := func(kind string, re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(scanText, -1) {
			if len(match) < 4 {
				continue
			}
			line := lineForOffset(scanText, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      kind,
				name:      scanText[match[2]:match[3]],
				startLine: line,
				endLine:   blockEndLine(scanText, match[0], line),
			})
		}
	}
	addNamed("type", regexp.MustCompile(`(?m)^\s*@(?:interface|implementation|protocol)\s+([A-Za-z_][A-Za-z0-9_]*)`))
	selectorPieceRe := regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*:`)
	methodStartRe := regexp.MustCompile(`^\s*[+-]\s*\([^)\r\n]*\)\s*(.*)$`)
	lines := strings.Split(scanText, "\n")
	for i := 0; i < len(lines); i++ {
		match := methodStartRe.FindStringSubmatch(lines[i])
		if len(match) != 2 {
			continue
		}
		head := strings.TrimSpace(match[1])
		for j := i + 1; j < len(lines) && !strings.ContainsAny(head, "{;"); j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" {
				break
			}
			head += " " + next
		}
		if cut := strings.IndexAny(head, "{;"); cut >= 0 {
			head = head[:cut]
		}
		pieces := selectorPieceRe.FindAllStringSubmatch(head, -1)
		name := ""
		if len(pieces) > 0 {
			parts := make([]string, 0, len(pieces))
			for _, piece := range pieces {
				if piece[1] != "" {
					parts = append(parts, piece[1])
				}
			}
			if len(parts) > 0 {
				name = strings.Join(parts, ":") + ":"
			}
		} else if one := regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)`).FindStringSubmatch(head); len(one) == 2 {
			name = one[1]
		}
		if name == "" {
			continue
		}
		symbols = append(symbols, symbolDraft{
			kind:      "method",
			name:      name,
			startLine: i + 1,
			endLine:   i + 1,
		})
	}
	return dedupeSymbolDrafts(symbols), imports
}

func maskCStyleNonCode(text string) string {
	buf := []byte(text)
	for i := 0; i < len(buf); {
		switch {
		case hasAt(buf, i, `/*`):
			i = blankThroughDelimiter(buf, i, `*/`)
		case hasAt(buf, i, `//`):
			for i < len(buf) && buf[i] != '\n' {
				buf[i] = ' '
				i++
			}
		case buf[i] == '"' || buf[i] == '\'':
			quote := buf[i]
			buf[i] = ' '
			i++
			escaped := false
			for i < len(buf) {
				ch := buf[i]
				if ch != '\n' {
					buf[i] = ' '
				}
				if escaped {
					escaped = false
				} else if ch == '\\' {
					escaped = true
				} else if ch == quote {
					i++
					break
				}
				i++
			}
		default:
			i++
		}
	}
	return string(buf)
}

func parseDartRegex(path string, content []byte) ([]symbolDraft, []string) {
	text := string(content)
	imports := regexCaptures(text, regexp.MustCompile(`(?m)^\s*(?:import|export|part)\s+['"]([^'"]+)['"]`))
	scanText := maskCStyleComments(text)
	symbols := make([]symbolDraft, 0)
	addNamed := func(kind string, re *regexp.Regexp) {
		for _, match := range re.FindAllStringSubmatchIndex(scanText, -1) {
			if len(match) < 4 {
				continue
			}
			line := lineForOffset(scanText, match[0])
			symbols = append(symbols, symbolDraft{
				kind:      kind,
				name:      scanText[match[2]:match[3]],
				startLine: line,
				endLine:   blockEndLine(scanText, match[0], line),
				metadata:  graph.JSONBMap{"source": "regex_dart"},
			})
		}
	}
	ident := `[A-Za-z_][A-Za-z0-9_]*`
	dartType := `(?:void|bool|int|double|num|String|Object|dynamic|Never|Future|FutureOr|Stream|Iterable|List|Map|Set|Uri|Uint8List|ByteStream|[A-Z][A-Za-z0-9_]*)(?:\s*<[^\r\n]+>)?(?:\?)?`
	mods := `(?:(?:external|static|abstract|base|final|interface|sealed|mixin|covariant|late)\s+)*`
	body := `\s*(?:async\s*\*?|sync\s*\*?)?\s*(?:=>|\{|;)`
	typeRe := regexp.MustCompile(`(?m)^\s*` + mods + `(?:class|mixin|enum|extension(?:\s+type)?)\s+(` + ident + `)`)
	typeNames := map[string]bool{}
	for _, match := range typeRe.FindAllStringSubmatchIndex(scanText, -1) {
		if len(match) >= 4 && match[2] >= 0 {
			typeNames[scanText[match[2]:match[3]]] = true
		}
	}
	addNamed("type", typeRe)
	addNamed("typedef", regexp.MustCompile(`(?m)^\s*typedef\s+(`+ident+`)\b`))
	constructorStart := regexp.MustCompile(`^\s*(?:(?:external|const|factory)\s+)*(_?[A-Z][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)?)\s*(?:<[^>\r\n]+>)?\(`)
	lines := strings.Split(scanText, "\n")
	braceDepths := dartBraceDepths(scanText)
	for i := 0; i < len(lines); i++ {
		if i >= len(braceDepths) || braceDepths[i] != 1 {
			continue
		}
		if strings.HasSuffix(dartPreviousSignificantLine(lines, i), "=>") {
			continue
		}
		match := constructorStart.FindStringSubmatchIndex(lines[i])
		if len(match) < 4 || match[2] < 0 {
			continue
		}
		tail := lines[i][match[0]:]
		for j := i + 1; j < len(lines) && !dartSignatureTailOK(tail); j++ {
			tail += " " + strings.TrimSpace(lines[j])
		}
		if !dartSignatureTailOK(tail) {
			continue
		}
		name := lines[i][match[2]:match[3]]
		baseName := strings.SplitN(name, ".", 2)[0]
		if !typeNames[baseName] {
			continue
		}
		symbols = append(symbols, symbolDraft{
			kind:      "constructor",
			name:      name,
			startLine: i + 1,
			endLine:   i + 1,
			metadata:  graph.JSONBMap{"source": "regex_dart"},
		})
	}
	for i := 0; i < len(lines); i++ {
		if !strings.Contains(lines[i], "(") {
			continue
		}
		tail := strings.TrimSpace(lines[i])
		for j := i + 1; j < len(lines) && !dartSignatureTailOK(tail); j++ {
			tail += " " + strings.TrimSpace(lines[j])
		}
		if !dartSignatureTailOK(tail) {
			continue
		}
		name, ok := dartFunctionNameFromSignature(tail, typeNames)
		if !ok {
			continue
		}
		symbols = append(symbols, symbolDraft{
			kind:      "function",
			name:      name,
			startLine: i + 1,
			endLine:   i + 1,
			metadata:  graph.JSONBMap{"source": "regex_dart"},
		})
	}
	addNamed("getter", regexp.MustCompile(`(?m)^\s*(?:`+mods+`)?`+dartType+`\s+get\s+(`+ident+`)`+body))
	addNamed("setter", regexp.MustCompile(`(?m)^\s*(?:`+mods+`)?(?:`+dartType+`\s+)?set\s+(`+ident+`)\s*\([^;\r\n{}]*\)`+body))
	if len(symbols) == 0 {
		symbols = append(symbols, docDraft("dart", "document", filepath.Base(path), 1, len(strings.Split(text, "\n")), text))
	}
	return dedupeSymbolDrafts(symbols), imports
}

func dartFunctionNameFromSignature(signature string, typeNames map[string]bool) (string, bool) {
	open := strings.Index(signature, "(")
	if open < 0 {
		return "", false
	}
	prefix := strings.TrimSpace(signature[:open])
	fields := strings.Fields(prefix)
	for len(fields) > 0 && dartSkippableDeclarationPrefix(fields[0]) {
		fields = fields[1:]
	}
	if len(fields) < 2 {
		return "", false
	}
	if fields[len(fields)-2] == "get" || fields[len(fields)-2] == "set" {
		return "", false
	}
	name := dartCleanIdentifier(fields[len(fields)-1])
	if name == "" || name == "Function" || typeNames[name] || dartReservedWord(name) {
		return "", false
	}
	returnType := strings.Join(fields[:len(fields)-1], " ")
	if !dartReturnTypeOK(returnType) {
		return "", false
	}
	return name, true
}

func dartSkippableDeclarationPrefix(token string) bool {
	token = strings.TrimSpace(token)
	if strings.HasPrefix(token, "@") {
		return true
	}
	switch token {
	case "abstract", "external", "static", "base", "final", "interface", "sealed", "mixin", "covariant", "late":
		return true
	default:
		return false
	}
}

func dartCleanIdentifier(token string) string {
	token = strings.TrimSpace(token)
	if cut := strings.Index(token, "<"); cut >= 0 {
		token = token[:cut]
	}
	token = strings.TrimSuffix(token, "?")
	if token == "" {
		return ""
	}
	for i, r := range token {
		if i == 0 {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || r == '_' {
				continue
			}
			return ""
		}
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return ""
	}
	return token
}

func dartReturnTypeOK(returnType string) bool {
	returnType = strings.TrimSpace(returnType)
	if returnType == "" || strings.Contains(returnType, " Function") {
		return false
	}
	base := dartCleanIdentifier(returnType)
	if base == "" {
		return false
	}
	switch base {
	case "void", "bool", "int", "double", "num", "String", "Object", "dynamic", "Never", "Future", "FutureOr", "Stream", "Iterable", "List", "Map", "Set", "Uri", "Uint8List", "ByteStream":
		return true
	default:
		if dartReservedWord(base) {
			return false
		}
		return (base[0] >= 'A' && base[0] <= 'Z') || (strings.HasPrefix(base, "_") && len(base) > 1 && base[1] >= 'A' && base[1] <= 'Z')
	}
}

func dartReservedWord(token string) bool {
	switch token {
	case "assert", "await", "break", "case", "catch", "const", "continue", "default", "do", "else", "false", "for", "if", "in", "is", "new", "null", "rethrow", "return", "super", "switch", "this", "throw", "true", "try", "var", "void", "while", "with", "yield":
		return true
	default:
		return false
	}
}

func dartSignatureTailOK(text string) bool {
	start := strings.Index(text, "(")
	if start < 0 {
		return false
	}
	depth := 0
	for i := start; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				tail := strings.TrimSpace(text[i+1:])
				return strings.HasPrefix(tail, ";") ||
					strings.HasPrefix(tail, "{") ||
					strings.HasPrefix(tail, ":") ||
					strings.HasPrefix(tail, "=>") ||
					strings.HasPrefix(tail, "async") ||
					strings.HasPrefix(tail, "sync")
			}
		}
	}
	return false
}

func dartBraceDepths(text string) []int {
	lines := strings.Split(text, "\n")
	depths := make([]int, len(lines))
	depth := 0
	for i, line := range lines {
		depths[i] = depth
		for j := 0; j < len(line); j++ {
			switch line[j] {
			case '{':
				depth++
			case '}':
				if depth > 0 {
					depth--
				}
			}
		}
	}
	return depths
}

func dartPreviousSignificantLine(lines []string, index int) string {
	for i := index - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func maskCStyleComments(text string) string {
	buf := []byte(text)
	for i := 0; i < len(buf); {
		switch {
		case hasAt(buf, i, `/*`):
			i = blankThroughDelimiter(buf, i, `*/`)
		case hasAt(buf, i, `//`):
			for i < len(buf) && buf[i] != '\n' {
				buf[i] = ' '
				i++
			}
		default:
			i++
		}
	}
	return string(buf)
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
