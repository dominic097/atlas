package parser

import (
	"encoding/xml"
	"io"
	"path/filepath"
	"strings"

	"github.com/dominic097/atlas/internal/graph"
)

func parseDotnetNative(path string, content []byte) ([]symbolDraft, []string, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".sln":
		symbols := parseDotnetSolution(content)
		if len(symbols) == 0 {
			return dotnetDocumentFallback(path, content), nil, true
		}
		return dedupeSymbolDrafts(symbols), nil, true
	case ".slnx":
		symbols, imports, ok := parseDotnetSolutionXML(content)
		if !ok {
			return dotnetDocumentFallback(path, content), nil, true
		}
		if len(symbols) == 0 {
			return dotnetDocumentFallback(path, content), uniqueStrings(imports), true
		}
		return dedupeSymbolDrafts(symbols), uniqueStrings(imports), true
	default:
		symbols, imports, ok := parseDotnetProjectXML(path, content)
		if !ok {
			return dotnetDocumentFallback(path, content), nil, true
		}
		if len(symbols) == 0 {
			return dotnetDocumentFallback(path, content), uniqueStrings(imports), true
		}
		return dedupeSymbolDrafts(symbols), uniqueStrings(imports), true
	}
}

func parseDotnetProjectXML(path string, content []byte) ([]symbolDraft, []string, bool) {
	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	symbols := []symbolDraft{dotnetDraft("project", dotnetBaseProjectName(path), 1, "dotnet_project_xml")}
	var imports []string
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return symbols, imports, true
			}
			return nil, nil, false
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		line := lineForOffset(string(content), int(decoder.InputOffset()))
		switch start.Name.Local {
		case "Project":
			if sdk := dotnetAttr(start, "Sdk"); sdk != "" {
				symbols = append(symbols, dotnetDraft("sdk", sdk, line, "dotnet_project_xml"))
			}
		case "PackageReference":
			if name := dotnetAttr(start, "Include"); name != "" {
				symbols = append(symbols, dotnetDraft("package", name, line, "dotnet_project_xml"))
				imports = append(imports, name)
			} else if name := dotnetAttr(start, "Update"); name != "" {
				symbols = append(symbols, dotnetDraft("package", name, line, "dotnet_project_xml"))
				imports = append(imports, name)
			}
		case "ProjectReference":
			if ref := dotnetAttr(start, "Include"); ref != "" {
				symbols = append(symbols, dotnetDraft("project_reference", dotnetBaseProjectName(ref), line, "dotnet_project_xml"))
				imports = append(imports, ref)
			}
		case "TargetFramework", "TargetFrameworks":
			text, ok := dotnetElementText(decoder)
			if !ok {
				return nil, nil, false
			}
			for _, framework := range strings.FieldsFunc(text, func(r rune) bool { return r == ';' || r == ',' }) {
				if framework = strings.TrimSpace(framework); framework != "" {
					symbols = append(symbols, dotnetDraft("target_framework", framework, line, "dotnet_project_xml"))
				}
			}
		}
	}
}

func parseDotnetSolutionXML(content []byte) ([]symbolDraft, []string, bool) {
	decoder := xml.NewDecoder(strings.NewReader(string(content)))
	var symbols []symbolDraft
	var imports []string
	for {
		token, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				return symbols, imports, true
			}
			return nil, nil, false
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "Project" {
			continue
		}
		path := dotnetAttr(start, "Path")
		if path == "" {
			continue
		}
		line := lineForOffset(string(content), int(decoder.InputOffset()))
		symbols = append(symbols, dotnetDraft("project", dotnetBaseProjectName(path), line, "dotnet_solution_xml"))
		imports = append(imports, path)
	}
}

func parseDotnetSolution(content []byte) []symbolDraft {
	var symbols []symbolDraft
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		name, ok := dotnetSolutionProjectName(line)
		if !ok {
			continue
		}
		symbols = append(symbols, dotnetDraft("project", name, i+1, "dotnet_solution"))
	}
	return symbols
}

func dotnetElementText(decoder *xml.Decoder) (string, bool) {
	var b strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return "", false
		}
		switch t := token.(type) {
		case xml.CharData:
			b.Write([]byte(t))
		case xml.EndElement:
			return b.String(), true
		}
	}
}

func dotnetAttr(start xml.StartElement, name string) string {
	for _, attr := range start.Attr {
		if strings.EqualFold(attr.Name.Local, name) {
			return strings.TrimSpace(attr.Value)
		}
	}
	return ""
}

func dotnetSolutionProjectName(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "Project(") {
		return "", false
	}
	closeIdx := strings.Index(trimmed, ")")
	if closeIdx < 0 {
		return "", false
	}
	rest := strings.TrimSpace(trimmed[closeIdx+1:])
	if !strings.HasPrefix(rest, "=") {
		return "", false
	}
	rest = strings.TrimSpace(strings.TrimPrefix(rest, "="))
	if !strings.HasPrefix(rest, `"`) {
		return "", false
	}
	rest = rest[1:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return "", false
	}
	name := strings.TrimSpace(rest[:end])
	return name, name != ""
}

func dotnetBaseProjectName(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), `\`, `/`)
	base := filepath.Base(value)
	ext := filepath.Ext(base)
	if ext != "" {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

func dotnetDraft(kind, name string, line int, source string) symbolDraft {
	return symbolDraft{
		kind:      kind,
		name:      strings.TrimSpace(name),
		startLine: line,
		endLine:   line,
		metadata:  graph.JSONBMap{"source": source},
	}
}

func dotnetDocumentFallback(path string, content []byte) []symbolDraft {
	return []symbolDraft{docDraft("dotnet", "document", filepath.Base(path), 1, len(strings.Split(string(content), "\n")), string(content))}
}
