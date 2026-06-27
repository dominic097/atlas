package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"
)

type stats struct {
	Documents       int            `json:"documents"`
	TotalDocuments  int            `json:"total_documents,omitempty"`
	FilterPrefixes  []string       `json:"filter_prefixes,omitempty"`
	ExternalSymbols int            `json:"external_symbols"`
	Symbols         int            `json:"symbols"`
	Occurrences     int            `json:"occurrences"`
	Definitions     int            `json:"definitions"`
	References      int            `json:"references"`
	Imports         int            `json:"imports"`
	Relationships   int            `json:"relationships"`
	Kinds           map[string]int `json:"kinds"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: scipstats <index.scip> [document-prefix ...]")
		os.Exit(2)
	}

	body, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}

	var index scip.Index
	if err := proto.Unmarshal(body, &index); err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}

	out := stats{
		TotalDocuments:  len(index.GetDocuments()),
		FilterPrefixes:  normalizePrefixes(os.Args[2:]),
		ExternalSymbols: len(index.GetExternalSymbols()),
		Kinds:           map[string]int{},
	}
	for _, doc := range index.GetDocuments() {
		if !includeDocument(doc.GetRelativePath(), out.FilterPrefixes) {
			continue
		}
		out.Documents++
		out.Symbols += len(doc.GetSymbols())
		out.Occurrences += len(doc.GetOccurrences())
		for _, symbol := range doc.GetSymbols() {
			out.Kinds[symbol.GetKind().String()]++
			out.Relationships += len(symbol.GetRelationships())
		}
		for _, occurrence := range doc.GetOccurrences() {
			roles := occurrence.GetSymbolRoles()
			if roles&int32(scip.SymbolRole_Definition) != 0 {
				out.Definitions++
			} else if occurrence.GetSymbol() != "" {
				out.References++
			}
			if roles&int32(scip.SymbolRole_Import) != 0 {
				out.Imports++
			}
		}
	}

	encoded, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode stats: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(encoded))
}

func normalizePrefixes(prefixes []string) []string {
	out := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		p := strings.Trim(strings.ReplaceAll(prefix, "\\", "/"), "/")
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func includeDocument(path string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}
	p := strings.Trim(strings.ReplaceAll(path, "\\", "/"), "/")
	for _, prefix := range prefixes {
		if p == prefix || strings.HasPrefix(p, prefix+"/") {
			return true
		}
	}
	return false
}
