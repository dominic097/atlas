package parser

func parseSvelteNative(content []byte) ([]symbolDraft, []string, bool) {
	symbols, imports, ok := parseVueNative(content)
	if !ok {
		return nil, nil, false
	}
	for i := range symbols {
		if symbols[i].metadata != nil {
			symbols[i].metadata["source"] = "tree_sitter_svelte_script"
		}
	}
	return symbols, imports, true
}
