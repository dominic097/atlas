package parser

import (
	"strings"

	"github.com/dominic097/atlas/internal/graph"
)

func parseBladeNative(path string, content []byte) ([]symbolDraft, []string, bool) {
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

	add("template", bladeViewName(path), 0, "blade_file")
	scanBladeDirectives(text, func(kind, name string, offset int, source string, isImport bool) {
		add(kind, name, offset, source)
		if isImport {
			addImport(name)
		}
	})
	scanBladeComponents(text, func(kind, name string, offset int, source string) {
		add(kind, name, offset, source)
	})
	scanBladeWireHandlers(text, func(kind, name string, offset int, source string) {
		add(kind, name, offset, source)
	})

	return dedupeSymbolDrafts(symbols), uniqueStrings(imports), true
}

func scanBladeDirectives(text string, add func(kind, name string, offset int, source string, isImport bool)) {
	type bladeDirective struct {
		name     string
		kind     string
		source   string
		isImport bool
	}
	directives := []bladeDirective{
		{name: "includeUnless", kind: "include", source: "blade_include", isImport: true},
		{name: "includeFirst", kind: "include", source: "blade_include", isImport: true},
		{name: "includeWhen", kind: "include", source: "blade_include", isImport: true},
		{name: "includeIf", kind: "include", source: "blade_include", isImport: true},
		{name: "include", kind: "include", source: "blade_include", isImport: true},
		{name: "component", kind: "component", source: "blade_component_directive", isImport: true},
		{name: "section", kind: "section", source: "blade_section"},
		{name: "extends", kind: "layout", source: "blade_extends", isImport: true},
		{name: "yield", kind: "slot", source: "blade_yield"},
	}

	for i := 0; i < len(text); i++ {
		if text[i] != '@' {
			continue
		}
		for _, directive := range directives {
			nameStart := i + 1
			nameEnd := nameStart + len(directive.name)
			if nameEnd > len(text) || text[nameStart:nameEnd] != directive.name {
				continue
			}
			if nameEnd < len(text) && isBladeIdentifierByte(text[nameEnd]) {
				continue
			}
			quoteStart, ok := bladeFirstQuotedArgumentStart(text, nameEnd)
			if !ok {
				continue
			}
			value, ok := readBladeQuotedValue(text, quoteStart)
			if !ok {
				continue
			}
			add(directive.kind, value, i, directive.source, directive.isImport)
			break
		}
	}
}

func bladeFirstQuotedArgumentStart(text string, offset int) (int, bool) {
	i := skipBladeSpace(text, offset)
	if i >= len(text) || text[i] != '(' {
		return 0, false
	}
	i = skipBladeSpace(text, i+1)
	if i >= len(text) || (text[i] != '\'' && text[i] != '"') {
		return 0, false
	}
	return i, true
}

func scanBladeComponents(text string, add func(kind, name string, offset int, source string)) {
	for i := 0; i < len(text); i++ {
		switch {
		case strings.HasPrefix(text[i:], "<livewire:"):
			start := i + len("<livewire:")
			end := start
			for end < len(text) && isBladeLivewireNameByte(text[end]) {
				end++
			}
			add("component", text[start:end], i, "blade_livewire_tag")
			i = end
		case strings.HasPrefix(text[i:], "<x-"):
			start := i + len("<x-")
			end := start
			for end < len(text) && isBladeAnonymousComponentNameByte(text[end]) {
				end++
			}
			add("component", text[start:end], i, "blade_anonymous_component")
			i = end
		}
	}
}

func scanBladeWireHandlers(text string, add func(kind, name string, offset int, source string)) {
	for i := 0; i < len(text); i++ {
		if !strings.HasPrefix(text[i:], "wire:") {
			continue
		}
		j := i + len("wire:")
		for j < len(text) && isBladeAnonymousComponentNameByte(text[j]) {
			j++
		}
		if j == i+len("wire:") {
			continue
		}
		j = skipBladeSpace(text, j)
		if j >= len(text) || text[j] != '=' {
			continue
		}
		j = skipBladeSpace(text, j+1)
		value, ok := readBladeQuotedValue(text, j)
		if !ok {
			continue
		}
		add("handler", value, i, "blade_wire_handler")
	}
}

func readBladeQuotedValue(text string, quoteStart int) (string, bool) {
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

func skipBladeSpace(text string, offset int) int {
	for offset < len(text) {
		switch text[offset] {
		case ' ', '\t', '\n', '\r':
			offset++
		default:
			return offset
		}
	}
	return offset
}

func isBladeIdentifierByte(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' ||
		ch >= 'a' && ch <= 'z' ||
		ch >= '0' && ch <= '9' ||
		ch == '_'
}

func isBladeLivewireNameByte(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' ||
		ch >= 'a' && ch <= 'z' ||
		ch >= '0' && ch <= '9' ||
		ch == '_' || ch == '.' || ch == '-'
}

func isBladeAnonymousComponentNameByte(ch byte) bool {
	return isBladeLivewireNameByte(ch) || ch == ':'
}
