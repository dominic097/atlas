package parser

import (
	"strings"

	"github.com/dominic097/atlas/internal/graph"
)

func parseEJSNative(path string, content []byte) ([]symbolDraft, []string, bool) {
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
	addImport := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" {
			imports = append(imports, name)
		}
	}

	add("template", ejsViewName(path), 0, "ejs_file")
	scanEJSTags(text, func(tag string, offset int) {
		if include, ok := ejsIncludeName(tag); ok {
			add("include", include, offset, "ejs_include")
			addImport(include)
		}
		if name, ok := firstEJSFunctionName(tag); ok {
			add("function", name, offset, "ejs_function")
		}
		if name, ok := firstEJSVariableName(tag); ok {
			add("variable", name, offset, "ejs_variable")
		}
	})

	return dedupeSymbolDrafts(symbols), uniqueStrings(imports), true
}

func scanEJSTags(text string, visit func(tag string, offset int)) {
	for offset := 0; offset < len(text); {
		start := strings.Index(text[offset:], "<%")
		if start < 0 {
			return
		}
		start += offset
		bodyStart := start + len("<%")
		closeRel := strings.Index(text[bodyStart:], "%>")
		bodyEnd := len(text)
		if closeRel >= 0 {
			bodyEnd = bodyStart + closeRel
		}
		visit(text[bodyStart:bodyEnd], start)
		if closeRel < 0 {
			return
		}
		offset = bodyEnd + len("%>")
	}
}

func ejsIncludeName(tag string) (string, bool) {
	i := skipEJSSpace(tag, 0)
	if i < len(tag) && (tag[i] == '-' || tag[i] == '=' || tag[i] == '_') {
		i = skipEJSSpace(tag, i+1)
	}

	switch {
	case strings.HasPrefix(tag[i:], "await"):
		end := i + len("await")
		if end < len(tag) && !isEJSSpace(tag[end]) {
			return "", false
		}
		i = skipEJSSpace(tag, end)
		if !strings.HasPrefix(tag[i:], "include") {
			return "", false
		}
		i += len("include")
	case strings.HasPrefix(tag[i:], "include"):
		i += len("include")
	default:
		return "", false
	}
	if i < len(tag) && isEJSIdentifierByte(tag[i]) {
		return "", false
	}

	i = skipEJSSpace(tag, i)
	if i < len(tag) && tag[i] == '(' {
		i = skipEJSSpace(tag, i+1)
	}
	value, ok := readEJSQuotedValue(tag, i)
	return value, ok
}

func firstEJSFunctionName(tag string) (string, bool) {
	for i := 0; i < len(tag); i++ {
		if !hasEJSWordAt(tag, i, "function") {
			continue
		}
		j := skipEJSSpace(tag, i+len("function"))
		if j >= len(tag) || !isEJSIdentifierStart(tag[j]) {
			continue
		}
		start := j
		for j < len(tag) && isEJSIdentifierByte(tag[j]) {
			j++
		}
		k := skipEJSSpace(tag, j)
		if k < len(tag) && tag[k] == '(' {
			return tag[start:j], true
		}
	}
	return "", false
}

func firstEJSVariableName(tag string) (string, bool) {
	for i := 0; i < len(tag); i++ {
		keyword, ok := ejsVariableKeywordAt(tag, i)
		if !ok {
			continue
		}
		j := skipEJSSpace(tag, i+len(keyword))
		if j >= len(tag) || !isEJSIdentifierStart(tag[j]) {
			continue
		}
		start := j
		for j < len(tag) && isEJSIdentifierByte(tag[j]) {
			j++
		}
		k := skipEJSSpace(tag, j)
		if k < len(tag) && tag[k] == '=' {
			return tag[start:j], true
		}
	}
	return "", false
}

func ejsVariableKeywordAt(tag string, offset int) (string, bool) {
	for _, keyword := range []string{"const", "let", "var"} {
		if hasEJSWordAt(tag, offset, keyword) {
			return keyword, true
		}
	}
	return "", false
}

func hasEJSWordAt(text string, offset int, word string) bool {
	end := offset + len(word)
	if offset < 0 || end > len(text) || text[offset:end] != word {
		return false
	}
	if offset > 0 && isEJSIdentifierByte(text[offset-1]) {
		return false
	}
	if end < len(text) && isEJSIdentifierByte(text[end]) {
		return false
	}
	return true
}

func readEJSQuotedValue(text string, quoteStart int) (string, bool) {
	if quoteStart >= len(text) || (text[quoteStart] != '\'' && text[quoteStart] != '"') {
		return "", false
	}
	quote := text[quoteStart]
	for i := quoteStart + 1; i < len(text); i++ {
		if text[i] == quote {
			return text[quoteStart+1 : i], true
		}
	}
	return "", false
}

func skipEJSSpace(text string, offset int) int {
	for offset < len(text) && isEJSSpace(text[offset]) {
		offset++
	}
	return offset
}

func isEJSSpace(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func isEJSIdentifierStart(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' ||
		ch >= 'a' && ch <= 'z' ||
		ch == '_'
}

func isEJSIdentifierByte(ch byte) bool {
	return isEJSIdentifierStart(ch) || ch >= '0' && ch <= '9'
}
