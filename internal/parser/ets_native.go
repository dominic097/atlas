package parser

func parseETSNative(path string, content []byte) ([]symbolDraft, []string, bool) {
	symbols, imports := parseETSRegex(path, content)
	return symbols, imports, true
}
