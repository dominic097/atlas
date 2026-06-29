package parser

func parseByondNative(path string, content []byte) ([]symbolDraft, []string, bool) {
	symbols, imports := parseByondRegex(path, content)
	return symbols, imports, true
}
