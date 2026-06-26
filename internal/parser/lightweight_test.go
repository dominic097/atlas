package parser

import (
	"strings"
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

func TestLanguageForPathReviewContextTextFormats(t *testing.T) {
	cases := map[string]string{
		"go.mod":                "gomod",
		"go.sum":                "gosum",
		"flow.json":             "json",
		"Atlas.code-workspace":  "json",
		"service.proto":         "proto",
		"README.mdx":            "mdx",
		"Makefile":              "makefile",
		"Jenkinsfile.server":    "groovy",
		"scripts/postinstall":   "bash",
		".env.example":          "config",
		".dockerignore":         "config",
		"systemd/atlas.service": "config",
		"config.alloy":          "config",
		"uv.lock":               "config",
		"scripts/install.bat":   "batch",
		"scripts/install.ps1":   "powershell",
		"provider.go.disabled":  "go",
		"registry.go.backup":    "go",
		"dump.sql.bak":          "sql",
		"settings.json.orig":    "json",
		"data/employees.csv":    "csv",
	}

	for path, want := range cases {
		if got := LanguageForPath(path); got != want {
			t.Fatalf("LanguageForPath(%q) = %q, want %q", path, got, want)
		}
		if !Supported(want) {
			t.Fatalf("Supported(%q) = false", want)
		}
	}
}

func TestParseProtoSymbols(t *testing.T) {
	content := []byte(`syntax = "proto3";
import "google/protobuf/timestamp.proto";

message ReviewRequest {
  string id = 1;
}

service ReviewService {
  rpc BuildContext (ReviewRequest) returns (ReviewRequest);
}
`)

	res, err := Parse("repo", "org/repo", "review.proto", "", content)
	if err != nil {
		t.Fatalf("Parse proto: %v", err)
	}
	if !containsString(res.Imports, "google/protobuf/timestamp.proto") {
		t.Fatalf("imports = %#v, want protobuf timestamp import", res.Imports)
	}
	want := map[string]string{
		"ReviewRequest": "message",
		"ReviewService": "service",
		"BuildContext":  "rpc",
	}
	for name, kind := range want {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing proto symbol %s", name)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
		if sym.Metadata["body_excerpt"] == "" {
			t.Fatalf("%s missing body excerpt", name)
		}
	}
}

func TestParseMakefileTargetBody(t *testing.T) {
	content := []byte(`.PHONY: build
build:
	go test ./...

lint:
	golangci-lint run
`)

	res, err := Parse("repo", "org/repo", "Makefile", "", content)
	if err != nil {
		t.Fatalf("Parse Makefile: %v", err)
	}
	build := findSymbol(res.Symbols, "build")
	if build == nil {
		t.Fatal("missing build target")
	}
	body, _ := build.Metadata["body_excerpt"].(string)
	if !strings.Contains(body, "go test ./...") {
		t.Fatalf("build body excerpt = %q, want command body", body)
	}
	if findSymbol(res.Symbols, ".PHONY") != nil {
		t.Fatal(".PHONY should not be indexed as a target symbol")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func findSymbol(symbols []graph.CodeSymbol, name string) *graph.CodeSymbol {
	for i := range symbols {
		if symbols[i].Name == name {
			return &symbols[i]
		}
	}
	return nil
}
