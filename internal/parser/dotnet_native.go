package parser

func parseDotnetNative(path string, content []byte) ([]symbolDraft, []string, bool) {
	symbols, imports := parseDotnetRegex(path, content)
	return symbols, imports, true
}
