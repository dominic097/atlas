package parser

import (
	"unsafe"

	tree_sitter_swift "github.com/dominic097/atlas/internal/parser/tree_sitter_swift"
	tree_sitter_kotlin "github.com/tree-sitter-grammars/tree-sitter-kotlin/bindings/go"
	tree_sitter_lua "github.com/tree-sitter-grammars/tree-sitter-lua/bindings/go"
	tree_sitter_zig "github.com/tree-sitter-grammars/tree-sitter-zig/bindings/go"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_csharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_scala "github.com/tree-sitter/tree-sitter-scala/bindings/go"
)

// tagsregistry.go is the language→grammar registry for the GENERIC tags-query
// extractor (tagsquery.go). Adding a natively-parsed language is a two-line
// change here (a case returning the grammar pointer) plus an embedded
// queries/<lang>.scm — no new walker code. This is the scaling surface the
// later batches extend to the remaining regex-fallback languages.

// tagsLanguagePointer returns the tree-sitter grammar pointer for a language
// parsed via the generic tags-query extractor, or nil when the language has no
// registered tags grammar. PHP uses LanguagePHP() (the full PHP grammar that
// also models inline HTML), matching how .php files appear in the wild.
func tagsLanguagePointer(language string) unsafe.Pointer {
	switch language {
	case "rust":
		return tree_sitter_rust.Language()
	case "ruby":
		return tree_sitter_ruby.Language()
	case "csharp":
		return tree_sitter_csharp.Language()
	case "php":
		return tree_sitter_php.LanguagePHP()
	case "kotlin":
		return tree_sitter_kotlin.Language()
	case "scala":
		return tree_sitter_scala.Language()
	case "swift":
		return tree_sitter_swift.Language()
	case "lua":
		return tree_sitter_lua.Language()
	case "zig":
		return tree_sitter_zig.Language()
	default:
		return nil
	}
}

// tagsGrammar resolves a language to a live *tree_sitter.Language built from its
// registered grammar pointer, plus its embedded tags query. ok is false when the
// language is not registered for native tags parsing or the grammar/query is
// missing — the caller then leaves symbols empty rather than guessing.
func tagsGrammar(language string) (grammar *tree_sitter.Language, query string, ok bool) {
	ptr := tagsLanguagePointer(language)
	if ptr == nil {
		return nil, "", false
	}
	q := tagsQueryForLanguage(language)
	if q == "" {
		return nil, "", false
	}
	g := tree_sitter.NewLanguage(ptr)
	if g == nil {
		return nil, "", false
	}
	return g, q, true
}

// tagsParsedLanguage reports whether a language is routed through the generic
// native tags-query extractor (used by Parse to choose the native path over the
// regex fallback).
func tagsParsedLanguage(language string) bool {
	return tagsLanguagePointer(language) != nil
}
