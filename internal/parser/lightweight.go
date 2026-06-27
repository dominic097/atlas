package parser

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/dominic097/atlas/internal/graph"
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
		symbolRule("function", `(?m)^\s*(?:pub(?:\([^)]*\))?\s+)?(?:async\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)\s*(?:<[^>{}]*>)?\s*\(`),
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
		symbolRule("function", `(?mi)^\s*(?:procedure|function)\s+([A-Za-z_][A-Za-z0-9_.]*)`),
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
