package parser

import (
	"strings"

	"github.com/dominic097/atlas/internal/graph"
)

type sqlToken struct {
	text      string
	offset    int
	line      int
	lineStart bool
}

func parseSQLNative(content []byte) ([]symbolDraft, bool) {
	text := string(content)
	tokens := sqlDDLTokens(text)
	symbols := make([]symbolDraft, 0)
	for i := 0; i < len(tokens); i++ {
		if !tokens[i].lineStart || !strings.EqualFold(tokens[i].text, "CREATE") {
			continue
		}
		kind, name, ok := sqlDefinitionFromTokens(tokens, i)
		if !ok {
			continue
		}
		symbols = append(symbols, symbolDraft{
			kind:      kind,
			name:      name,
			startLine: tokens[i].line,
			endLine:   tokens[i].line,
			signature: sqlLineAt(text, tokens[i].offset),
			metadata:  graph.JSONBMap{"source": "sql_source_parser"},
		})
	}
	return sortDedupDrafts(symbols), true
}

func sqlDefinitionFromTokens(tokens []sqlToken, start int) (kind, name string, ok bool) {
	i := start + 1
	if i+1 < len(tokens) && strings.EqualFold(tokens[i].text, "OR") && strings.EqualFold(tokens[i+1].text, "REPLACE") {
		i += 2
	}
	if i >= len(tokens) {
		return "", "", false
	}
	switch strings.ToUpper(tokens[i].text) {
	case "TABLE":
		kind = "table"
	case "VIEW":
		kind = "view"
	case "FUNCTION":
		kind = "function"
	case "PROCEDURE":
		kind = "procedure"
	case "TRIGGER":
		kind = "trigger"
	default:
		return "", "", false
	}
	i++
	if i+2 < len(tokens) && strings.EqualFold(tokens[i].text, "IF") && strings.EqualFold(tokens[i+1].text, "NOT") && strings.EqualFold(tokens[i+2].text, "EXISTS") {
		i += 3
	}
	if i >= len(tokens) {
		return "", "", false
	}
	name = sqlTrimDDLName(tokens[i].text)
	if !sqlValidDDLName(name) {
		return "", "", false
	}
	return kind, name, true
}

func sqlDDLTokens(text string) []sqlToken {
	tokens := make([]sqlToken, 0)
	line := 1
	atLineStart := true
	for i := 0; i < len(text); {
		switch {
		case text[i] == '\n':
			line++
			atLineStart = true
			i++
		case text[i] == ' ' || text[i] == '\t' || text[i] == '\r':
			i++
		case i+1 < len(text) && text[i] == '-' && text[i+1] == '-':
			for i < len(text) && text[i] != '\n' {
				i++
			}
		case i+1 < len(text) && text[i] == '/' && text[i+1] == '*':
			i += 2
			for i+1 < len(text) && !(text[i] == '*' && text[i+1] == '/') {
				if text[i] == '\n' {
					line++
					atLineStart = true
				}
				i++
			}
			if i+1 < len(text) {
				i += 2
			}
		case text[i] == '\'':
			i, line = skipSQLSingleQuoted(text, i, line)
			atLineStart = false
		case text[i] == '$':
			next, nextLine, ok := skipSQLDollarQuoted(text, i, line)
			if ok {
				i = next
				line = nextLine
				atLineStart = false
			} else {
				tok, next := readSQLToken(text, i)
				tokens = append(tokens, sqlToken{text: tok, offset: i, line: line, lineStart: atLineStart})
				i = next
				atLineStart = false
			}
		default:
			tok, next := readSQLToken(text, i)
			if tok == "" {
				i++
				atLineStart = false
				continue
			}
			tokens = append(tokens, sqlToken{text: tok, offset: i, line: line, lineStart: atLineStart})
			i = next
			atLineStart = false
		}
	}
	return tokens
}

func readSQLToken(text string, offset int) (string, int) {
	if offset >= len(text) {
		return "", offset
	}
	if text[offset] == '"' {
		i := offset + 1
		for i < len(text) {
			if text[i] == '"' {
				i++
				break
			}
			i++
		}
		return text[offset:i], i
	}
	if strings.ContainsRune("();,", rune(text[offset])) {
		return string(text[offset]), offset + 1
	}
	i := offset
	for i < len(text) {
		if text[i] == '"' {
			i++
			for i < len(text) && text[i] != '"' {
				i++
			}
			if i < len(text) {
				i++
			}
			continue
		}
		if text[i] == '\n' || text[i] == ' ' || text[i] == '\t' || text[i] == '\r' || strings.ContainsRune("();,", rune(text[i])) {
			break
		}
		i++
	}
	return text[offset:i], i
}

func skipSQLSingleQuoted(text string, offset, line int) (int, int) {
	i := offset + 1
	for i < len(text) {
		if text[i] == '\n' {
			line++
		}
		if text[i] == '\'' {
			if i+1 < len(text) && text[i+1] == '\'' {
				i += 2
				continue
			}
			return i + 1, line
		}
		i++
	}
	return i, line
}

func skipSQLDollarQuoted(text string, offset, line int) (int, int, bool) {
	end := offset + 1
	for end < len(text) && (text[end] == '_' || text[end] >= 'A' && text[end] <= 'Z' || text[end] >= 'a' && text[end] <= 'z' || text[end] >= '0' && text[end] <= '9') {
		end++
	}
	if end >= len(text) || text[end] != '$' {
		return offset, line, false
	}
	tag := text[offset : end+1]
	i := end + 1
	for i < len(text) {
		if text[i] == '\n' {
			line++
		}
		if strings.HasPrefix(text[i:], tag) {
			return i + len(tag), line, true
		}
		i++
	}
	return len(text), line, true
}

func sqlTrimDDLName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.TrimRight(name, "();")
	return name
}

func sqlValidDDLName(name string) bool {
	if name == "" {
		return false
	}
	first := name[0]
	if !(first == '"' || first == '_' || first >= 'A' && first <= 'Z' || first >= 'a' && first <= 'z') {
		return false
	}
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch == '"' || ch == '_' || ch == '.' || ch == '$' || ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
			continue
		}
		return false
	}
	return true
}

func sqlLineAt(text string, offset int) string {
	if offset < 0 {
		offset = 0
	}
	if offset > len(text) {
		offset = len(text)
	}
	start := strings.LastIndexByte(text[:offset], '\n') + 1
	end := strings.IndexByte(text[offset:], '\n')
	if end < 0 {
		end = len(text)
	} else {
		end += offset
	}
	return strings.TrimSpace(text[start:end])
}
