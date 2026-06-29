package parser

import (
	"testing"

	"github.com/dominic097/atlas/internal/graph"
)

// cppEdgeFor returns the first edge with the given bare callee (toRef), or nil.
func cppEdgeFor(edges []graph.DependencyEdge, toRef string) *graph.DependencyEdge {
	for i := range edges {
		if edges[i].ToRef == toRef {
			return &edges[i]
		}
	}
	return nil
}

func TestCppCallEdges(t *testing.T) {
	// compute() is defined INSIDE the struct body so the symbol walker attributes
	// it as a method named "compute" (out-of-class `Widget::compute` definitions
	// are a separate symbol-naming concern, orthogonal to call extraction).
	src := []byte(`
#include <string>

int helper(int x);

struct Widget {
  int compute() const {
    Helper h;
    Config* cfg = makeConfig();
    helper(1);          // free function -> bare, no qualifier
    h.run();            // method on typed local -> recv_type Helper
    cfg->reset();       // arrow method on typed local -> recv_type Config
    ns::doThing();      // qualified_identifier -> bare doThing, qualified ns::doThing
    Widget::staticOp(); // qualified_identifier -> bare staticOp
    return 0;
  }
};
`)

	syms, _, root, cleanup := parseTreeSitter("widget.cpp", "cpp", src)
	defer cleanup()
	if root == nil {
		t.Fatal("parseTreeSitter returned nil root")
	}

	cs := make([]graph.CodeSymbol, 0, len(syms))
	for _, s := range syms {
		cs = append(cs, graph.CodeSymbol{
			Name:      s.name,
			Kind:      s.kind,
			StartLine: s.startLine,
			EndLine:   s.endLine,
		})
	}

	edges := cppCallEdges(root, src, "repo", "owner/repo", "widget.cpp", cs)
	if len(edges) == 0 {
		t.Fatal("expected call edges, got none")
	}

	// 1. Qualified method call obj.method() -> qualified_ref "h.run".
	run := cppEdgeFor(edges, "run")
	if run == nil {
		t.Fatalf("missing edge for h.run(); edges=%+v", edges)
	}
	if got := run.Metadata["qualified_ref"]; got != "h.run" {
		t.Errorf("h.run qualified_ref = %q, want %q", got, "h.run")
	}
	if got := run.Metadata["recv_type"]; got != "Helper" {
		t.Errorf("h.run recv_type = %q, want %q", got, "Helper")
	}

	// 1b. Arrow method call obj->method() normalizes qualified_ref to "cfg.reset"
	//     and infers recv_type from the typed local (pointer decl).
	reset := cppEdgeFor(edges, "reset")
	if reset == nil {
		t.Fatalf("missing edge for cfg->reset()")
	}
	if got := reset.Metadata["qualified_ref"]; got != "cfg.reset" {
		t.Errorf("cfg->reset qualified_ref = %q, want %q", got, "cfg.reset")
	}
	if got := reset.Metadata["recv_type"]; got != "Config" {
		t.Errorf("cfg->reset recv_type = %q, want %q", got, "Config")
	}

	// 2. Bare external/free call resolves sanely: bare name, empty qualified_ref,
	//    and no recv_type.
	free := cppEdgeFor(edges, "helper")
	if free == nil {
		t.Fatalf("missing edge for free call helper()")
	}
	if got := free.Metadata["qualified_ref"]; got != "" {
		t.Errorf("helper() qualified_ref = %q, want empty", got)
	}
	if _, ok := free.Metadata["recv_type"]; ok {
		t.Errorf("helper() should not carry recv_type, got %v", free.Metadata["recv_type"])
	}

	// 3. qualified_identifier ns::doThing() -> bare last segment, full qualified.
	doThing := cppEdgeFor(edges, "doThing")
	if doThing == nil {
		t.Fatalf("missing edge for ns::doThing()")
	}
	if got := doThing.Metadata["qualified_ref"]; got != "ns::doThing" {
		t.Errorf("ns::doThing qualified_ref = %q, want %q", got, "ns::doThing")
	}

	// 4. All call edges attribute to the enclosing symbol and tag the cpp_ts source.
	for _, e := range edges {
		if e.Kind != graph.EdgeCalls {
			t.Errorf("edge %q has kind %q, want EdgeCalls", e.ToRef, e.Kind)
		}
		if got := e.Metadata["source"]; got != "cpp_ts" {
			t.Errorf("edge %q source = %q, want cpp_ts", e.ToRef, got)
		}
		if e.FromSymbol != "compute" {
			t.Errorf("edge %q from_symbol = %q, want compute", e.ToRef, e.FromSymbol)
		}
	}
}

func TestCppSymbols_NamespaceAndQualifiedDefinitions(t *testing.T) {
	src := []byte(`
namespace leveldb {

class DBImpl {
 public:
  Status Get();
};

Status DBImpl::Get() {
  return Status();
}

int FreeFn() {
  return 0;
}

}  // namespace leveldb
`)

	res, err := Parse("repo-cpp", "owner/repo", "db_impl.cc", "cpp", src)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}

	type key struct {
		name string
		kind string
	}
	symbols := map[key]map[string]any{}
	for _, sym := range res.Symbols {
		symbols[key{sym.Name, sym.Kind}] = sym.Metadata
	}

	for _, want := range []key{
		{"DBImpl", "class"},
		{"Get", "method"},
		{"FreeFn", "function"},
	} {
		if _, ok := symbols[want]; !ok {
			t.Errorf("missing C++ symbol %s/%s; got=%+v", want.name, want.kind, res.Symbols)
		}
	}
	if owner, _ := symbols[key{"Get", "method"}]["owner_type"].(string); owner != "DBImpl" {
		t.Errorf("DBImpl::Get owner_type = %q, want DBImpl", owner)
	}
	if qualified, _ := symbols[key{"Get", "method"}]["qualified_name"].(string); qualified != "DBImpl::Get" {
		t.Errorf("DBImpl::Get qualified_name = %q, want DBImpl::Get", qualified)
	}
}

func TestCppSymbols_CUDAKernelDefinitions(t *testing.T) {
	src := []byte(`
#include <cuda_runtime.h>

__global__ void testKernel(int *g_odata) {
  atomicAdd(&g_odata[0], 10);
}

int main() {
  testKernel<<<1, 1>>>(nullptr);
  return 0;
}
`)

	res, err := Parse("repo-cuda", "owner/repo", "simpleAtomicIntrinsics.cu", "", src)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got := LanguageForPath("simpleAtomicIntrinsics.cu"); got != "cpp" {
		t.Fatalf("LanguageForPath(.cu) = %q, want cpp", got)
	}

	type key struct {
		name string
		kind string
	}
	symbols := map[key]bool{}
	for _, sym := range res.Symbols {
		symbols[key{sym.Name, sym.Kind}] = true
	}
	for _, want := range []key{
		{"testKernel", "function"},
		{"main", "function"},
	} {
		if !symbols[want] {
			t.Errorf("missing CUDA symbol %s/%s; got=%+v", want.name, want.kind, res.Symbols)
		}
	}
}

type symKey struct{ name, kind string }

func cSymbolSet(t *testing.T, path, lang string, src string) (map[symKey]bool, []graph.CodeSymbol) {
	t.Helper()
	res, err := Parse("repo", "owner/repo", path, lang, []byte(src))
	if err != nil {
		t.Fatalf("Parse(%s) error: %v", path, err)
	}
	set := map[symKey]bool{}
	for _, s := range res.Symbols {
		set[symKey{s.Name, s.Kind}] = true
	}
	return set, res.Symbols
}

// TestCSymbols_RecallRootCauses pins the four C recall regressions the audit
// found on cJSON/leveldb: enums emitted at all, multi-line signatures, and
// pointer-return functions. Each must appear with the correct kind.
func TestCSymbols_RecallRootCauses(t *testing.T) {
	src := `
/* root cause 1: enum defs were never emitted */
enum Direction { kForward, kReverse };
typedef enum { kTypeDeletion = 0x0, kTypeValue = 0x1 } ValueType;

/* root cause 3: pointer-return functions were dropped */
static cJSON *cJSON_New_Item(const internal_hooks * const hooks)
{
    return NULL;
}
static unsigned char* ensure(printbuffer * const p, size_t needed)
{
    return NULL;
}

/* control: single-line non-pointer function (already worked) */
void cJSON_Delete(cJSON *item) { }

/* root cause 2: multi-line signature, pointer return */
leveldb_t* leveldb_open(
    const leveldb_options_t* options,
    const char* name,
    char** errptr) {
  return NULL;
}
`
	set, all := cSymbolSet(t, "fixture.c", "c", src)
	for _, want := range []symKey{
		{"Direction", "enum"},
		{"ValueType", "enum"},
		{"cJSON_New_Item", "function"},
		{"ensure", "function"},
		{"cJSON_Delete", "function"},
		{"leveldb_open", "function"},
	} {
		if !set[want] {
			t.Errorf("missing C symbol %s/%s; got=%+v", want.name, want.kind, all)
		}
	}
}

// TestCppSymbols_RecallRootCauses pins the C++ recall regressions: enum (incl.
// nested and enum-class) defs, and header-declared class members (methods,
// constructors, destructors) that have no inline body.
func TestCppSymbols_RecallRootCauses(t *testing.T) {
	src := `
enum class Color { Red, Green };
enum CompressionType { kNoCompression = 0x0, kSnappyCompression = 0x1 };

/* root cause 4: header-only class with ctor/dtor/method declarations */
class Iterator {
 public:
  Iterator();
  ~Iterator();
  bool Valid() const;
  void inlineSeek() { }     // inline-defined member still works
  enum Status { Ok, Err };  // nested enum
};

struct Slice {
  size_t size() const;
};
`
	set, all := cSymbolSet(t, "fixture.cpp", "cpp", src)
	for _, want := range []symKey{
		{"Color", "enum"},
		{"CompressionType", "enum"},
		{"Iterator", "class"},
		{"Iterator", "constructor"},
		{"~Iterator", "method"},
		{"Valid", "method"},
		{"inlineSeek", "method"},
		{"Status", "enum"},
		{"Slice", "struct"},
		{"size", "method"},
	} {
		if !set[want] {
			t.Errorf("missing C++ symbol %s/%s; got=%+v", want.name, want.kind, all)
		}
	}

	// Precision guard: a constructor must be exactly one symbol, and the class
	// type itself must not be double-emitted as a method.
	var ctorCount, classCount int
	for _, s := range all {
		if s.Name == "Iterator" && s.Kind == "constructor" {
			ctorCount++
		}
		if s.Name == "Iterator" && s.Kind == "class" {
			classCount++
		}
	}
	if ctorCount != 1 {
		t.Errorf("expected exactly 1 Iterator constructor, got %d", ctorCount)
	}
	if classCount != 1 {
		t.Errorf("expected exactly 1 Iterator class, got %d", classCount)
	}
}
