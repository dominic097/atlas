package parser

import "testing"

const goNavigationSymbolSource = `package sample

const DefaultLimit = 10

type Worker struct {
	Name string
	Size int
}

type Runner interface {
	Run(ctx Context) error
	Stop() error
}
`

func TestGoNavigationSymbols(t *testing.T) {
	res, err := Parse("repo-1", "owner/repo", "sample.go", "go", []byte(goNavigationSymbolSource))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	seen := map[string]map[string]graphMeta{}
	for _, sym := range res.Symbols {
		if seen[sym.Kind] == nil {
			seen[sym.Kind] = map[string]graphMeta{}
		}
		seen[sym.Kind][sym.Name] = graphMeta(sym.Metadata)
	}

	if _, ok := seen["constant"]["DefaultLimit"]; !ok {
		t.Fatalf("constant DefaultLimit not indexed; symbols=%+v", res.Symbols)
	}
	for _, name := range []string{"Name", "Size"} {
		meta, ok := seen["field"][name]
		if !ok {
			t.Fatalf("field %s not indexed; symbols=%+v", name, res.Symbols)
		}
		if meta["owner_type"] != "Worker" {
			t.Fatalf("field %s owner_type = %v, want Worker", name, meta["owner_type"])
		}
	}
	for _, name := range []string{"Run", "Stop"} {
		meta, ok := seen["method_spec"][name]
		if !ok {
			t.Fatalf("method spec %s not indexed; symbols=%+v", name, res.Symbols)
		}
		if meta["owner_type"] != "Runner" {
			t.Fatalf("method spec %s owner_type = %v, want Runner", name, meta["owner_type"])
		}
	}
}

type graphMeta map[string]any
