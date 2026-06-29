package parser

import "testing"

// tagsquery_test.go verifies that languages registered in tagsregistry.go are
// extracted via the GENERIC native tree-sitter tags-query path (tagsquery.go),
// NOT the regex fallback. Each test drives the full Parse() entrypoint (so it
// exercises routing → grammar → compiled query → promotion to graph.CodeSymbol)
// and asserts the real AST definitions are recovered with their Atlas kinds.

// symbolPairs runs the full native Parse path and returns the set of
// (name,kind) pairs, asserting the language stamp / line / signature invariants
// on every symbol (proof the native path ran). A set (not a name→kind map) is
// used because a single name can legitimately carry two kinds — e.g. the rust
// tags.scm captures a fn inside a `mod` as BOTH function and method.
func symbolPairs(t *testing.T, lang, path string, src string) map[[2]string]bool {
	t.Helper()
	res, err := Parse("repo", "owner/repo", path, lang, []byte(src))
	if err != nil {
		t.Fatalf("%s Parse: %v", lang, err)
	}
	if len(res.Symbols) == 0 {
		t.Fatalf("%s: no symbols extracted (native tags path produced nothing)", lang)
	}
	pairs := make(map[[2]string]bool, len(res.Symbols))
	for _, s := range res.Symbols {
		if s.Language != lang {
			t.Errorf("%s symbol %q: Language=%q, want %q", lang, s.Name, s.Language, lang)
		}
		if s.StartLine <= 0 {
			t.Errorf("%s symbol %q: non-positive StartLine %d", lang, s.Name, s.StartLine)
		}
		if s.Signature == "" {
			t.Errorf("%s symbol %q: empty signature", lang, s.Name)
		}
		pairs[[2]string{s.Name, s.Kind}] = true
	}
	return pairs
}

// assertPairs requires every (name,kind) in want to be present in got.
func assertPairs(t *testing.T, lang string, got map[[2]string]bool, want map[string]string) {
	t.Helper()
	for name, kind := range want {
		if !got[[2]string{name, kind}] {
			t.Errorf("%s: missing expected definition %q/%q (got: %v)", lang, name, kind, pairKeys(got))
		}
	}
}

func pairKeys(m map[[2]string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k[0]+":"+k[1])
	}
	return out
}

// TestRustTagsSymbols: rust routes to the native tags extractor and yields its
// real def nodes (struct/enum→class, trait→interface, fn→function, impl method,
// mod→module, const→constant, macro_rules→macro).
func TestRustTagsSymbols(t *testing.T) {
	src := `// a geometry module
pub mod geometry {
    pub const PI: f64 = 3.14159;

    pub struct Point {
        x: f64,
        y: f64,
    }

    pub enum Shape {
        Circle,
        Square,
    }

    pub trait Drawable {
        fn draw(&self);
    }

    pub fn area(s: &Shape) -> f64 {
        compute(s)
    }

    impl Point {
        pub fn new(x: f64, y: f64) -> Point {
            Point { x, y }
        }
    }

    macro_rules! twice {
        ($x:expr) => { $x + $x };
    }
}
`
	got := symbolPairs(t, "rust", "geometry.rs", src)
	assertPairs(t, "rust", got, map[string]string{
		"geometry": "module",
		"PI":       "constant",
		"Point":    "class",     // struct_item → class (upstream tags convention)
		"Shape":    "class",     // enum_item   → class
		"Drawable": "interface", // trait_item  → interface
		"area":     "function",  // function_item at mod scope
		"new":      "method",    // fn inside impl's declaration_list
		"twice":    "macro",     // macro_definition → macro (kind passes through)
	})
	// The official rust tags.scm captures a fn inside a `mod` declaration_list as
	// BOTH function and method; both are retained (distinct kinds).
	if !got[[2]string{"area", "method"}] {
		t.Errorf("rust: expected area also captured as method via declaration_list")
	}
}

// TestRubyTagsSymbols: ruby native tags path yields module, class, instance and
// singleton methods, and a top-level method.
func TestRubyTagsSymbols(t *testing.T) {
	src := `# Geometry helpers
module Geometry
  class Shape
    def area
      compute
    end

    def self.describe
      "a shape"
    end
  end

  def helper
    42
  end
end
`
	got := symbolPairs(t, "ruby", "geometry.rb", src)
	assertPairs(t, "ruby", got, map[string]string{
		"Geometry": "module",
		"Shape":    "class",
		"area":     "method",
		"describe": "method", // singleton_method
		"helper":   "method",
	})
}

// TestCSharpTagsSymbols: csharp native tags path yields namespace→module,
// interface, class, method, plus the Atlas-augmented struct/enum/enum-member/
// constructor/property/record def nodes.
func TestCSharpTagsSymbols(t *testing.T) {
	src := `namespace Geometry
{
    public interface IDrawable
    {
        void Draw();
    }

    public struct Point
    {
        public int X;
    }

    public enum Suit
    {
        Hearts,
        Spades,
    }

    public class Circle : IDrawable
    {
        public Circle()
        {
        }

        public void Draw()
        {
        }

        public double Radius { get; set; }
    }

    public record Vec(int X, int Y);
}
`
	got := symbolPairs(t, "csharp", "geometry.cs", src)
	assertPairs(t, "csharp", got, map[string]string{
		"Geometry":  "module",
		"IDrawable": "interface",
		"Point":     "struct",
		"Suit":      "enum",
		"Hearts":    "constant", // enum_member_declaration
		"Spades":    "constant",
		"Circle":    "class",  // class_declaration
		"Draw":      "method", // method_declaration (not a regex line scan)
		"Radius":    "field",  // property_declaration → field
		"Vec":       "class",  // record_declaration → class
	})
	// The constructor and the class share the name "Circle"; both are emitted
	// (distinct kinds) via the augmented constructor_declaration pattern.
	if !got[[2]string{"Circle", "constructor"}] {
		t.Errorf("csharp: expected Circle constructor captured via constructor_declaration")
	}
}

// TestPHPTagsSymbols: php native tags path yields namespace→module, interface,
// trait→interface, class, property→field, function, methods, plus the
// Atlas-augmented enum/enum-case def nodes.
func TestPHPTagsSymbols(t *testing.T) {
	src := `<?php
namespace Geometry;

interface Drawable
{
    public function draw();
}

trait Loggable
{
    public function log() {}
}

class Circle implements Drawable
{
    public $radius;

    public function __construct()
    {
    }

    public function draw()
    {
    }
}

function area($shape)
{
    return 0;
}

enum Suit
{
    case Hearts;
    case Spades;
}
`
	got := symbolPairs(t, "php", "geometry.php", src)
	assertPairs(t, "php", got, map[string]string{
		"Geometry":    "module",
		"Drawable":    "interface",
		"Loggable":    "interface", // trait_declaration → interface (upstream)
		"Circle":      "class",
		"radius":      "field",
		"__construct": "function", // method_declaration → function (upstream)
		"area":        "function",
		"Suit":        "enum",     // Atlas augmentation
		"Hearts":      "constant", // enum_case
		"Spades":      "constant",
	})
}

// TestTagsLanguagesRouteNative asserts each tags-query language is registered
// for the native path (and the registry/embedded query resolve), guarding
// against a regression that silently drops one back to the regex fallback.
func TestTagsLanguagesRouteNative(t *testing.T) {
	for _, lang := range []string{"rust", "ruby", "csharp", "php", "kotlin", "scala", "swift", "lua", "zig"} {
		if !tagsParsedLanguage(lang) {
			t.Errorf("%s: not routed to native tags path", lang)
		}
		grammar, query, ok := tagsGrammar(lang)
		if !ok || grammar == nil || query == "" {
			t.Errorf("%s: tagsGrammar resolve failed (ok=%v grammar=%v queryLen=%d)", lang, ok, grammar != nil, len(query))
		}
	}
}
