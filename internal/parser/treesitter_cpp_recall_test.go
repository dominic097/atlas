package parser

import "testing"

// cRecallSet captures parsed C/C++ symbols so tests can assert recall (a (name,
// kind) def appears) and precision (a name does NOT appear at all). A name may map
// to several kinds — e.g. a class and its same-named constructor — so kinds are
// kept per name.
type cRecallSet struct {
	byName map[string][]string // name -> kinds
	count  map[string]int      // name -> total emissions
}

func symbolIndex(t *testing.T, file, lang, src string) cRecallSet {
	t.Helper()
	res, err := Parse("repo", "owner/repo", file, lang, []byte(src))
	if err != nil {
		t.Fatalf("Parse(%s): %v", file, err)
	}
	s := cRecallSet{byName: map[string][]string{}, count: map[string]int{}}
	for _, sym := range res.Symbols {
		s.byName[sym.Name] = append(s.byName[sym.Name], sym.Kind)
		s.count[sym.Name]++
	}
	return s
}

// mustHave asserts a def with the given name AND kind was emitted (recall + kind).
func mustHave(t *testing.T, s cRecallSet, name, wantKind string) {
	t.Helper()
	kinds, ok := s.byName[name]
	if !ok {
		t.Errorf("missing symbol %q (recall): have %v", name, keysOf(s.byName))
		return
	}
	if wantKind == "" {
		return
	}
	for _, k := range kinds {
		if k == wantKind {
			return
		}
	}
	t.Errorf("symbol %q kinds = %v, want one to be %q", name, kinds, wantKind)
}

// mustNotHave asserts no def with the given name was emitted at all (precision).
func mustNotHave(t *testing.T, s cRecallSet, name string) {
	t.Helper()
	if k, ok := s.byName[name]; ok {
		t.Errorf("spurious symbol %q emitted as %v (precision violation)", name, k)
	}
}

// countOf returns how many times name was emitted across all kinds.
func countOf(s cRecallSet, name string) int { return s.count[name] }

func keysOf(m map[string][]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// ── [C #1] Unity parse-cascade: unbalanced function-like macros (EXPECT_ABORT_
// BEGIN expands to an open brace, VERIFY_FAILS_END to a close brace) bury plain
// `void testXxx(void){...}` defs in ERROR nodes. They must be recovered. ────────
func TestRecoverUnityParseCascade(t *testing.T) {
	// EXPECT_ABORT_BEGIN expands (in the real Unity headers) to `... if (...) {`
	// and VERIFY_FAILS_END to `}`. tree-sitter has no preprocessor, so it sees the
	// bare identifiers and the body braces are balanced ONLY because the test text
	// itself balances — BUT the macro-call statements with embedded `"` and `;`
	// shred the bodies into ERROR cascades that nest the following functions. This
	// mirrors testunity.c exactly (no #define here, just the call sites).
	src := `void testNotEqualString3(void)
{
    EXPECT_ABORT_BEGIN
    TEST_ASSERT_EQUAL_STRING("", "bar");
    VERIFY_FAILS_END
}

void testNotEqualStringArray1(void)
{
    const char *a[] = { "foo" };
    EXPECT_ABORT_BEGIN
    TEST_ASSERT_EQUAL_STRING_ARRAY(a, a, 1);
    VERIFY_FAILS_END
}

void testPlainOne(void)
{
    EXPECT_ABORT_BEGIN
    do_thing();
    VERIFY_FAILS_END
}
`
	kinds := symbolIndex(t, "testunity.c", "c", src)
	for _, fn := range []string{"testNotEqualString3", "testNotEqualStringArray1", "testPlainOne"} {
		mustHave(t, kinds, fn, "function")
		if c := countOf(kinds, fn); c != 1 {
			t.Errorf("function %q emitted %d times, want 1 (precision/dedup)", fn, c)
		}
	}
	// Precision: the assertion / control macros must NOT become function defs.
	for _, macro := range []string{"EXPECT_ABORT_BEGIN", "VERIFY_FAILS_END",
		"TEST_ASSERT_EQUAL_STRING", "TEST_ASSERT_EQUAL_STRING_ARRAY", "TEST_PROTECT"} {
		mustNotHave(t, kinds, macro)
	}
}

// ── [C #2a] macro calling-convention modifier between return type and name:
// `int CJSON_CDECL main(void) { ... }`. ────────────────────────────────────────
func TestRecoverMacroModifierFunction(t *testing.T) {
	src := `
int CJSON_CDECL main(void)
{
    return 0;
}

int plain_main(void)
{
    return 1;
}

void WINAPI handler(int x)
{
    use(x);
}
`
	kinds := symbolIndex(t, "test.c", "c", src)
	mustHave(t, kinds, "main", "function")
	mustHave(t, kinds, "plain_main", "function")
	mustHave(t, kinds, "handler", "function")
	if c := countOf(kinds, "main"); c != 1 {
		t.Errorf("main emitted %d times, want 1", c)
	}
	// Precision: the calling-convention macros are never symbols.
	mustNotHave(t, kinds, "CJSON_CDECL")
	mustNotHave(t, kinds, "WINAPI")
}

// ── [C #2b] typedef struct / typedef enum definitions, incl. tag≠alias. ─────────
func TestRecoverTypedefAggregates(t *testing.T) {
	src := `
typedef struct cJSON {
    struct cJSON *next;
    int type;
} cJSON;

typedef struct {
    int a;
} parse_buffer;

typedef struct GuardBytes {
    int size;
} Guard;

typedef enum {
    A, B, C
} ValueType;

typedef enum UNITY_FLOAT_TRAIT {
    IS_INF, IS_NAN
} UNITY_FLOAT_TRAIT_T;

typedef int cJSON_bool;
`
	kinds := symbolIndex(t, "cJSON.h", "c", src)
	// Same-name typedef struct: one struct named cJSON.
	mustHave(t, kinds, "cJSON", "struct")
	// Anonymous struct → typedef name.
	mustHave(t, kinds, "parse_buffer", "struct")
	// Named-tag struct with differing alias: BOTH reported (matches clangd).
	mustHave(t, kinds, "GuardBytes", "struct")
	mustHave(t, kinds, "Guard", "struct")
	// Anonymous enum → typedef name.
	mustHave(t, kinds, "ValueType", "enum")
	// Named-tag enum with differing alias: BOTH reported.
	mustHave(t, kinds, "UNITY_FLOAT_TRAIT", "enum")
	mustHave(t, kinds, "UNITY_FLOAT_TRAIT_T", "enum")
	// Precision: a scalar typedef alias is NOT a definition.
	mustNotHave(t, kinds, "cJSON_bool")
}

// ── [C #2c] opaque forward typedef must NOT be emitted (precision). ─────────────
func TestOpaqueForwardTypedefNotEmitted(t *testing.T) {
	src := `
typedef struct internal_Impl Handle;

typedef struct realDef {
    int x;
} realDef;
`
	kinds := symbolIndex(t, "h.h", "c", src)
	// The bodyless forward typedef alias is a declaration, not a def.
	mustNotHave(t, kinds, "Handle")
	// The real definition is still captured.
	mustHave(t, kinds, "realDef", "struct")
}

// ── [C++ #1] #if-interleaved member-init list buries an entire anonymous-
// namespace block. The classes/methods after the cascade must be recovered. ─────
func TestRecoverPreprocErrorCascade(t *testing.T) {
	// Mirrors leveldb util/env_posix.cc: a #if inside the Limiter ctor's member-
	// initializer list makes tree-sitter ERROR-swallow the rest of the namespace.
	src := `
namespace {

class Limiter {
 public:
  Limiter(int max_acquires)
      :
#if !defined(NDEBUG)
        max_acquires_(max_acquires),
#endif
        acquires_allowed_(max_acquires) {}

  bool Acquire() { return true; }

 private:
  int acquires_allowed_;
};

class PosixSequentialFile {
 public:
  PosixSequentialFile(int fd) : fd_(fd) {}
  ~PosixSequentialFile() { close(fd_); }
  int Read(size_t n) { return n; }

 private:
  int fd_;
};

class PosixEnv {
 public:
  PosixEnv();
  void Schedule() {}
};

}
`
	kinds := symbolIndex(t, "env_posix.cc", "cpp", src)
	// The class that triggers the cascade is still captured.
	mustHave(t, kinds, "Limiter", "class")
	// The classes buried AFTER the cascade are recovered.
	mustHave(t, kinds, "PosixSequentialFile", "class")
	mustHave(t, kinds, "PosixEnv", "class")
	// Their members are recovered. Members of classes that survive structurally
	// (PosixSequentialFile/PosixEnv) keep the method kind; a member of the
	// cascade-triggering Limiter (Acquire) may be recovered via the flat token
	// scanner as a bare function — recall is what matters there.
	mustHave(t, kinds, "Acquire", "")
	mustHave(t, kinds, "Read", "method")
	mustHave(t, kinds, "~PosixSequentialFile", "method")
	mustHave(t, kinds, "Schedule", "method")
	// The same-named constructor is also recovered (distinct from the class def).
	mustHave(t, kinds, "PosixSequentialFile", "constructor")
	// Precision: a class + its same-named constructor are the ONLY legitimate
	// name collision; a method recovered both structurally and via the flat shred
	// scanner must collapse to one. Acquire/Read/Schedule are method-only here.
	for _, m := range []string{"Acquire", "Read", "Schedule", "~PosixSequentialFile"} {
		if c := countOf(kinds, m); c != 1 {
			t.Errorf("method %q emitted %d times, want 1 (dedup/precision)", m, c)
		}
	}
}

// ── [C++ #2] nested class/struct defined inside another class body, with inline
// override members. ────────────────────────────────────────────────────────────
func TestRecoverNestedClass(t *testing.T) {
	src := `
class ModelDB {
 public:
  class ModelIter : public Iterator {
   public:
    ModelIter(const KVMap* map, bool owned) {}
    bool Valid() const override { return true; }
    void Next() override {}
  };

  struct Inner {
    int v;
    int get() const { return v; }
  };

  Iterator* NewIterator() { return nullptr; }
};
`
	kinds := symbolIndex(t, "db_test.cc", "cpp", src)
	mustHave(t, kinds, "ModelDB", "class")
	mustHave(t, kinds, "ModelIter", "class")
	mustHave(t, kinds, "Inner", "struct")
	// Inline members of the nested types.
	mustHave(t, kinds, "Valid", "method")
	mustHave(t, kinds, "Next", "method")
	mustHave(t, kinds, "get", "method")
	mustHave(t, kinds, "NewIterator", "method")
	// The nested type's constructor is also captured (distinct from its class def).
	mustHave(t, kinds, "ModelIter", "constructor")
}

// ── [C++ #3] method with a trailing thread-safety annotation BEFORE its body:
// `int Read() LOCKS_EXCLUDED(mu_) { ... }`. The method is real; the annotation
// must not be emitted. Contrast with a data field that merely TRAILS an
// annotation (no body), which must stay suppressed. ────────────────────────────
func TestRecoverLockAnnotatedMethod(t *testing.T) {
	src := `
class AtomicCounter {
 public:
  int Read() LOCKS_EXCLUDED(mu_) {
    return count_;
  }
  void Reset() LOCKS_EXCLUDED(mu_) {
    count_ = 0;
  }
  void Increment() {
    count_++;
  }
 private:
  int count_ GUARDED_BY(mu_);
};
`
	kinds := symbolIndex(t, "db_test.cc", "cpp", src)
	mustHave(t, kinds, "Read", "method")
	mustHave(t, kinds, "Reset", "method")
	mustHave(t, kinds, "Increment", "method")
	// Precision: annotation macros are never methods, and the guarded data field
	// is not a callable.
	mustNotHave(t, kinds, "LOCKS_EXCLUDED")
	mustNotHave(t, kinds, "GUARDED_BY")
}

// ── Precision guard for ERROR recovery: an ERROR region holding only PROTOTYPES
// (no body) must not yield function defs. ───────────────────────────────────────
func TestErrorRecoveryRejectsPrototypes(t *testing.T) {
	// A stray unmatched brace forces an ERROR cascade over the following
	// declarations, which are all prototypes (end in `;`, no body).
	src := `
void opener(void) {

int proto_a(int x);
int proto_b(void);
struct OnlyFwd;
`
	kinds := symbolIndex(t, "broken.c", "c", src)
	// Prototypes inside the ERROR must NOT be emitted as definitions.
	mustNotHave(t, kinds, "proto_a")
	mustNotHave(t, kinds, "proto_b")
}
