package parser

import (
	"path/filepath"
	"strings"

	"github.com/dominic097/atlas/internal/graph"
)

func parseRazorNative(path string, content []byte) ([]symbolDraft, []string, bool) {
	text := string(content)
	var symbols []symbolDraft
	var imports []string
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

	forEachLineWithOffset(text, func(line string, offset int) {
		trimmed := strings.TrimLeft(line, " \t")
		leading := len(line) - len(trimmed)
		directiveOffset := offset + leading
		if rest, ok := razorCutDirective(trimmed, "@page"); ok {
			add("route", razorQuotedValue(rest), directiveOffset, "razor_page")
			return
		}
		if rest, ok := razorCutDirective(trimmed, "@using"); ok {
			name, _ := razorReadTypeToken(rest)
			add("import", name, directiveOffset, "razor_using")
			addImport(name)
			return
		}
		if rest, ok := razorCutDirective(trimmed, "@inject"); ok {
			name, _ := razorReadTypeToken(rest)
			add("service", name, directiveOffset, "razor_inject")
			addImport(name)
			return
		}
		if rest, ok := razorCutDirective(trimmed, "@inherits"); ok {
			name, _ := razorReadTypeToken(rest)
			add("base", name, directiveOffset, "razor_inherits")
			return
		}
		if rest, ok := razorCutDirective(trimmed, "@model"); ok {
			name, _ := razorReadTypeToken(rest)
			add("model", name, directiveOffset, "razor_model")
		}
	})

	for _, tag := range razorComponentTags(text) {
		add("component", tag.name, tag.offset, "razor_component_tag")
	}
	for _, block := range razorNativeCodeBlocks(text) {
		forEachLineWithOffset(block.text, func(line string, offset int) {
			if name := razorMethodName(line); name != "" {
				add("method", name, block.offset+offset, "razor_code")
			}
		})
	}
	return sortDedupDrafts(symbols), uniqueStrings(imports), true
}

type razorTag struct {
	name   string
	offset int
}

func razorComponentTags(text string) []razorTag {
	var tags []razorTag
	for i := 0; i < len(text); i++ {
		if text[i] != '<' || i+1 >= len(text) || text[i+1] < 'A' || text[i+1] > 'Z' {
			continue
		}
		start := i + 1
		end := start
		for end < len(text) && ((text[end] >= 'A' && text[end] <= 'Z') || (text[end] >= 'a' && text[end] <= 'z') || (text[end] >= '0' && text[end] <= '9')) {
			end++
		}
		if end == start {
			continue
		}
		if end < len(text) && text[end] != ' ' && text[end] != '\t' && text[end] != '\r' && text[end] != '\n' && text[end] != '/' && text[end] != '>' {
			continue
		}
		tags = append(tags, razorTag{name: text[start:end], offset: i})
	}
	return tags
}

func razorNativeCodeBlocks(text string) []razorCodeBlock {
	var blocks []razorCodeBlock
	for i := 0; i < len(text); i++ {
		if text[i] != '@' {
			continue
		}
		rest := text[i+1:]
		var after string
		if value, ok := razorCutWord(rest, "code"); ok {
			after = value
		} else if value, ok := razorCutWord(rest, "functions"); ok {
			after = value
		} else {
			continue
		}
		space := len(after) - len(strings.TrimLeft(after, " \t\r\n"))
		brace := i + 1 + (len(rest) - len(after)) + space
		if brace >= len(text) || text[brace] != '{' {
			continue
		}
		start := brace + 1
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
			i = pos - 1
		}
	}
	return blocks
}

func razorMethodName(line string) string {
	line = strings.TrimSpace(line)
	open := strings.IndexByte(line, '(')
	if open < 0 {
		return ""
	}
	head := strings.TrimSpace(line[:open])
	if strings.Contains(head, "=") {
		return ""
	}
	fields := strings.Fields(head)
	if len(fields) < 3 || !razorMethodModifier(fields[0]) {
		return ""
	}
	name := fields[len(fields)-1]
	if !razorIdentifier(name) {
		return ""
	}
	return name
}

func razorCutDirective(line, directive string) (string, bool) {
	if len(line) < len(directive) || line[:len(directive)] != directive {
		return "", false
	}
	if len(line) > len(directive) && razorIdentByte(line[len(directive)]) {
		return "", false
	}
	return strings.TrimLeft(line[len(directive):], " \t"), true
}

func razorCutWord(line, word string) (string, bool) {
	if len(line) < len(word) || line[:len(word)] != word {
		return "", false
	}
	if len(line) > len(word) && razorIdentByte(line[len(word)]) {
		return "", false
	}
	return line[len(word):], true
}

func razorQuotedValue(s string) string {
	s = strings.TrimLeft(s, " \t")
	if s == "" || (s[0] != '\'' && s[0] != '"') {
		return ""
	}
	quote := s[0]
	for i := 1; i < len(s); i++ {
		if s[i] == quote {
			return s[1:i]
		}
	}
	return ""
}

func razorReadTypeToken(s string) (string, string) {
	s = strings.TrimLeft(s, " \t")
	i := 0
	for i < len(s) {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '_' || c == '.' || c == '<' || c == '>' || c == '[' || c == ']' || c == '?' {
			i++
			continue
		}
		break
	}
	return s[:i], s[i:]
}

func razorMethodModifier(s string) bool {
	switch s {
	case "public", "private", "protected", "internal", "static", "async", "override", "virtual", "abstract":
		return true
	default:
		return false
	}
}

func razorIdentifier(s string) bool {
	if s == "" || !razorIdentStartByte(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if !razorIdentByte(s[i]) {
			return false
		}
	}
	return true
}

func razorIdentStartByte(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_'
}

func razorIdentByte(c byte) bool {
	return razorIdentStartByte(c) || (c >= '0' && c <= '9')
}

func forEachLineWithOffset(text string, fn func(line string, offset int)) {
	offset := 0
	for offset <= len(text) {
		next := offset
		for next < len(text) && text[next] != '\n' && text[next] != '\r' {
			next++
		}
		fn(text[offset:next], offset)
		if next >= len(text) {
			break
		}
		if text[next] == '\r' && next+1 < len(text) && text[next+1] == '\n' {
			next++
		}
		offset = next + 1
	}
}
