package parser

func parseDelphiNative(path string, content []byte) ([]symbolDraft, []string, bool) {
	symbols, imports := parseDelphiRegex(path, content)
	return symbols, imports, true
}
