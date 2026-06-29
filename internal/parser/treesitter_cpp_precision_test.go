package parser

import "testing"

// TestCppMacroAnnotationsNotMethods ensures Clang thread-safety annotation macros
// that trail a data-field declaration (mis-parsed by tree-sitter-cpp as a
// function declarator on the field) are NOT emitted as method definitions, while
// real members on the same class are still captured. This is the precision guard
// for the header-member recall fix.
func TestCppMacroAnnotationsNotMethods(t *testing.T) {
	src := []byte(`
class DB {
 public:
  void Get() const;            // real method — must be captured
 private:
  int counter_ GUARDED_BY(mu_);
  Cache* cache_ GUARDED_BY(mu_);
  bool busy_ LOCKS_EXCLUDED(mu_);
};
`)
	res, err := Parse("repo", "owner/repo", "db.h", "cpp", src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	kinds := map[string]string{}
	for _, s := range res.Symbols {
		kinds[s.Name] = s.Kind
	}
	for _, macro := range []string{"GUARDED_BY", "EXCLUSIVE_LOCKS_REQUIRED", "LOCKS_EXCLUDED"} {
		if k, ok := kinds[macro]; ok {
			t.Errorf("macro annotation %q was emitted as %q symbol (should be skipped)", macro, k)
		}
	}
	if _, ok := kinds["Get"]; !ok {
		t.Errorf("real method Get was not captured (recall must be preserved): got kinds %v", kinds)
	}
}

// TestIsMacroAnnotationName covers the ALL-CAPS-underscored predicate directly.
func TestIsMacroAnnotationName(t *testing.T) {
	yes := []string{"GUARDED_BY", "EXCLUSIVE_LOCKS_REQUIRED", "LOCKS_EXCLUDED", "ACQUIRED_BEFORE"}
	no := []string{"Get", "doThing", "snake_case", "Compact", "size", "X", "operator==", "MAX"} // MAX has no underscore
	for _, n := range yes {
		if !isMacroAnnotationName(n) {
			t.Errorf("isMacroAnnotationName(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if isMacroAnnotationName(n) {
			t.Errorf("isMacroAnnotationName(%q) = true, want false", n)
		}
	}
}
