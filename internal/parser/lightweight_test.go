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

func TestLanguageForPathGraphifyDispatchFormats(t *testing.T) {
	cases := map[string]string{
		"src/lib.rs":            "rust",
		"app/models/user.rb":    "ruby",
		"src/Main.kt":           "kotlin",
		"build.gradle.kts":      "kotlin",
		"src/Main.scala":        "scala",
		"src/index.php":         "php",
		"views/home.blade.php":  "blade",
		"Sources/App.swift":     "swift",
		"plugin/init.lua":       "lua",
		"plugin/init.luau":      "lua",
		"addon/MyAddon.toc":     "lua",
		"src/main.zig":          "zig",
		"lib/app.ex":            "elixir",
		"lib/app.exs":           "elixir",
		"AppDelegate.m":         "objc",
		"ViewController.mm":     "objc",
		"src/main.jl":           "julia",
		"solver.F90":            "fortran",
		"solver.f03":            "fortran",
		"lib/main.dart":         "dart",
		"rtl/core.v":            "verilog",
		"rtl/core.sv":           "verilog",
		"rtl/core.svh":          "verilog",
		"src/unit.pas":          "pascal",
		"src/unit.pp":           "pascal",
		"src/project.dpr":       "pascal",
		"src/package.dpk":       "pascal",
		"src/main.lpr":          "pascal",
		"src/include.inc":       "pascal",
		"forms/main.dfm":        "delphi",
		"forms/main.lfm":        "delphi",
		"forms/pkg.lpk":         "delphi",
		"infra/main.tf":         "terraform",
		"infra/vars.tfvars":     "terraform",
		"infra/module.hcl":      "terraform",
		"code/game.dm":          "byond",
		"code/project.dme":      "byond",
		"code/icon.dmi":         "byond",
		"code/map.dmm":          "byond",
		"code/ui.dmf":           "byond",
		"App.sln":               "dotnet",
		"App.slnx":              "dotnet",
		"App/App.csproj":        "dotnet",
		"App/App.fsproj":        "dotnet",
		"App/App.vbproj":        "dotnet",
		"Pages/Index.razor":     "razor",
		"Pages/Index.cshtml":    "razor",
		"force-app/Foo.cls":     "apex",
		"force-app/Foo.trigger": "apex",
		"web/App.vue":           "vue",
		"web/App.svelte":        "svelte",
		"web/App.astro":         "astro",
		"native/kernel.cu":      "cpp",
		"native/kernel.cuh":     "cpp",
		"docs/design.qmd":       "markdown",
		"scripts/profile.psm1":  "powershell",
		"scripts/manifest.psd1": "powershell",
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

func TestParseAdditionalGraphifyLanguageSymbols(t *testing.T) {
	cases := []struct {
		name       string
		path       string
		content    string
		wantSymbol string
		wantKind   string
		wantImport string
	}{
		{
			name:       "rust",
			path:       "src/lib.rs",
			content:    "use std::fmt;\npub struct Worker {}\nfn run() { helper(); }\nfn helper() {}\n",
			wantSymbol: "run",
			wantKind:   "function",
			wantImport: "std::fmt",
		},
		{
			name:       "ruby",
			path:       "app/user.rb",
			content:    "require 'json'\nmodule ::Admin; def audit; end; end\nclass User\n  def save!\n  end\n  def [](key)\n  end\n  def @@sink.flush(*) end\nend\n",
			wantSymbol: "[]",
			wantKind:   "method",
			wantImport: "json",
		},
		{
			name:       "kotlin",
			path:       "src/main.kt",
			content:    "import okhttp3.Request\nactual open class PlatformClient\nfun interface EventListener\ninternal actual fun Request.prepare() = this\noverride val timeoutMillis: Int = 0\n",
			wantSymbol: "PlatformClient",
			wantKind:   "type",
			wantImport: "okhttp3.Request",
		},
		{
			name:       "groovy",
			path:       "src/main/groovy/demo/App.groovy",
			content:    "import java.nio.file.Path\nclass App {\n  static String getVersion() { '1' }\n}\ninterface Job { void start() }\ntask hello { doLast { println 'hi' } }\n",
			wantSymbol: "getVersion",
			wantKind:   "method",
			wantImport: "java.nio.file.Path",
		},
		{
			name:       "zig",
			path:       "src/main.zig",
			content:    "const std = @import(\"std\");\npub const Server = struct {};\npub const Data = packed union {};\nconst Mode = enum { fast, slow };\nconst source_line_range_utf8: lsp.types.Range = .{};\nconst lhs, const rhs = pair;\nconst @\"*i32\" = value;\npub inline fn fmt() void {}\nnoinline fn walkContainerDecl() void {}\npub fn main(init: std.process.Init) !u8 { _ = init; return 0; }\n",
			wantSymbol: "Server",
			wantKind:   "type",
			wantImport: "std",
		},
		{
			name:       "terraform",
			path:       "infra/main.tf",
			content:    "resource \"aws_instance\" \"web\" {}\nmodule \"vpc\" {}\n",
			wantSymbol: "aws_instance.web",
			wantKind:   "resource",
		},
		{
			name:       "swift",
			path:       "Sources/App.swift",
			content:    "import Foundation\nstruct App {\n  func run() {}\n}\n",
			wantSymbol: "run",
			wantKind:   "function",
			wantImport: "Foundation",
		},
		{
			name:       "elixir",
			path:       "lib/app.ex",
			content:    "defmodule App do\n  import Enum\n  defmacro __using__(_opts), do: :ok\n  defdelegate reload!(endpoint, opts), to: App.Server\n  defguard is_ready(term) when is_atom(term)\n  def run(), do: count([])\nend\n",
			wantSymbol: "reload!",
			wantKind:   "delegate",
			wantImport: "Enum",
		},
		{
			name:       "fortran",
			path:       "src/stdlib_hashmaps.f90",
			content:    "module stdlib_hashmaps\nuse iso_fortran_env\ninterface append\n  module procedure append_int\nend interface\n  type, abstract :: hashmap_type\n  end type hashmap_type\ncontains\n  pure module function loading(map) result(load)\n  end function loading\n  module subroutine free_chaining_map(map)\n  end subroutine free_chaining_map\nend module stdlib_hashmaps\n",
			wantSymbol: "loading",
			wantKind:   "function",
			wantImport: "iso_fortran_env",
		},
		{
			name:       "verilog",
			path:       "rtl/ibex_core.sv",
			content:    "package ibex_pkg;\nendpackage\nmodule ibex_core import ibex_pkg::*; #(parameter int Width = 32);\n  function automatic logic [6:0] cm_stack_adj_base(input logic [3:0] rlist);\n  endfunction\nendmodule\n",
			wantSymbol: "cm_stack_adj_base",
			wantKind:   "function",
		},
		{
			name:       "objective-c",
			path:       "Sources/SDImageCache.m",
			content:    "#import <Foundation/Foundation.h>\n@implementation SDImageCache\n- (void)storeImage:(UIImage *)image forKey:(NSString *)key completion:(void (^)(void))completion { }\n@end\n",
			wantSymbol: "storeImage:forKey:completion:",
			wantKind:   "method",
			wantImport: "Foundation/Foundation.h",
		},
		{
			name:       "dart",
			path:       "lib/src/client.dart",
			content:    "import 'dart:async';\nabstract class BaseClient {\n  Future<Response> send(BaseRequest request);\n}\n",
			wantSymbol: "send",
			wantKind:   "function",
			wantImport: "dart:async",
		},
		{
			name:       "razor",
			path:       "Pages/Index.razor",
			content:    "@page \"/home\"\n@using Demo.Core\n@code {\n  public void Save() {}\n}\n",
			wantSymbol: "/home",
			wantKind:   "route",
			wantImport: "Demo.Core",
		},
		{
			name:       "powershell",
			path:       "scripts/install.ps1",
			content:    "Import-Module Pester\nfunction Invoke-Install {\n  Invoke-Verify\n}\nfunction Invoke-Verify {}\n",
			wantSymbol: "Invoke-Install",
			wantKind:   "function",
			wantImport: "Pester",
		},
		{
			name:       "sql",
			path:       "schema.sql",
			content:    "CREATE TABLE reviews (id text);\nCREATE VIEW review_counts AS SELECT 1;\n",
			wantSymbol: "reviews",
			wantKind:   "table",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Parse("repo", "org/repo", tc.path, "", []byte(tc.content))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			sym := findSymbol(res.Symbols, tc.wantSymbol)
			if sym == nil {
				t.Fatalf("missing symbol %q; symbols=%+v", tc.wantSymbol, res.Symbols)
			}
			if sym.Kind != tc.wantKind {
				t.Fatalf("%s kind = %q, want %q", tc.wantSymbol, sym.Kind, tc.wantKind)
			}
			if tc.wantImport != "" && !containsString(res.Imports, tc.wantImport) {
				t.Fatalf("imports = %#v, want %q", res.Imports, tc.wantImport)
			}
		})
	}
}

func TestParseFortranLightweightDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "src/stdlib_hashmaps.f90", "", []byte(`
module stdlib_hashmaps
use iso_fortran_env
interface append
  module procedure append_int
end interface
type, abstract :: hashmap_type
end type hashmap_type
type chaining_map_entry_ptr
end type chaining_map_entry_ptr
contains
pure module function loading(map) result(load)
end function loading
pure logical function eq_stringlist(lhs, rhs)
end function eq_stringlist
type(c_ptr) function stdlib_get_cwd(len, stat) bind(C, name='stdlib_get_cwd')
end function stdlib_get_cwd
module subroutine free_chaining_map(map)
end subroutine free_chaining_map
end module stdlib_hashmaps
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for name, kind := range map[string]string{
		"stdlib_hashmaps":        "module",
		"hashmap_type":           "type",
		"chaining_map_entry_ptr": "type",
		"loading":                "function",
		"eq_stringlist":          "function",
		"stdlib_get_cwd":         "function",
		"free_chaining_map":      "function",
	} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Fortran symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if sym := findSymbol(res.Symbols, "procedure"); sym != nil {
		t.Fatalf("unexpected module-procedure false positive: %+v", sym)
	}
	if symbols := symbolsNamedKind(res.Symbols, "loading", "function"); len(symbols) != 1 {
		t.Fatalf("loading symbols = %+v, want exactly one definition", symbols)
	}
	if !containsString(res.Imports, "iso_fortran_env") {
		t.Fatalf("imports = %#v, want iso_fortran_env", res.Imports)
	}
}

func TestParseVerilogSystemVerilogDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "rtl/ibex_core.sv", "", []byte(`
package ibex_pkg;
  class config_obj;
  endclass
  function automatic logic [6:0] cm_stack_adj_base(input logic [3:0] rlist);
  endfunction
endpackage

interface bus_if(input logic clk);
endinterface

module ibex_core import ibex_pkg::*; #(
  parameter int Width = 32
);
  task automatic reset();
  endtask
  function automatic void decode_i_insn(input string mnemonic);
  endfunction
endmodule
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for name, kind := range map[string]string{
		"ibex_pkg":          "package",
		"config_obj":        "class",
		"cm_stack_adj_base": "function",
		"bus_if":            "interface",
		"ibex_core":         "module",
		"reset":             "task",
		"decode_i_insn":     "function",
	} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing SystemVerilog symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if symbols := symbolsNamedKind(res.Symbols, "decode_i_insn", "function"); len(symbols) != 1 {
		t.Fatalf("decode_i_insn symbols = %+v, want exactly one definition", symbols)
	}
}

func TestParseKotlinLightweightModifiers(t *testing.T) {
	res, err := Parse("repo", "org/repo", "src/main.kt", "", []byte(`
import okhttp3.Request
actual open class PlatformClient
fun interface EventListener
internal actual fun Request.prepare() = this
override val timeoutMillis: Int = 0
const val DEFAULT_PORT = 443
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for name, kind := range map[string]string{
		"PlatformClient": "type",
		"EventListener":  "type",
		"prepare":        "function",
		"timeoutMillis":  "variable",
		"DEFAULT_PORT":   "variable",
	} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Kotlin symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
}

func TestParseScalaLightweightModifiers(t *testing.T) {
	res, err := Parse("repo", "org/repo", "src/main.scala", "", []byte(`
import cats.Functor
sealed abstract class Eval[A]
private[cats] trait Traverse[F[_]]
object Functor
opaque type Id[A] = A
override type Representation = Unit
type :<:[F[_], G[_]] = InjectK[F, G]
object ==:
override def map[A, B](fa: F[A])(f: A => B): F[B]
def functor: Functor[F]
@inline final def <*>[A, B](ff: F[A => B])(fa: F[A]): F[B] = ???
val inj = new FunctionK[F, F] { def apply[A](fa: F[A]): F[A] = fa }
lazy val defaultFunctor: Functor[List] = ???
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for name, kind := range map[string]string{
		"Eval":           "type",
		"Traverse":       "type",
		"Functor":        "type",
		"Id":             "type",
		"Representation": "type",
		":<:":            "type",
		"==:":            "type",
		"map":            "function",
		"functor":        "function",
		"<*>":            "function",
		"apply":          "function",
		"defaultFunctor": "variable",
	} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Scala symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if !containsString(res.Imports, "cats.Functor") {
		t.Fatalf("imports = %#v, want cats.Functor", res.Imports)
	}
}

func TestParseZigLightweightDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "src/main.zig", "", []byte(`
const std = @import("std");
pub const Server = struct {};
pub const Data = packed union {};
const Header = extern struct {};
const Mode = enum { fast, slow };
const source_line_range_utf8: lsp.types.Range = .{};
const lhs, const rhs = pair;
const @"*i32" = value;
pub inline fn fmt() void {}
noinline fn walkContainerDecl() void {}
fn @"zls env"() void {}
pub fn main(init: std.process.Init) !u8 { _ = init; return 0; }
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for name, kind := range map[string]string{
		"Server":                 "type",
		"Data":                   "type",
		"Header":                 "type",
		"Mode":                   "type",
		"source_line_range_utf8": "constant",
		"lhs":                    "constant",
		"rhs":                    "constant",
		`@"*i32"`:                "constant",
		"fmt":                    "function",
		"walkContainerDecl":      "function",
		`@"zls env"`:             "function",
		"main":                   "function",
	} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Zig symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if !containsString(res.Imports, "std") {
		t.Fatalf("imports = %#v, want std", res.Imports)
	}
}

func TestParseElixirLightweightDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "lib/router.ex", "", []byte(`
defmodule Phoenix.Router do
  use Plug.Router
  alias Phoenix.Router.Route

  @doc """
  defmodule DocOnly do
    def fake(), do: :ignored
  end
  """
  # def comment_only(), do: :ignored

  defmacro __using__(opts), do: quote(do: opts)
  defmacrop define_method(method), do: method
  defdelegate reload!(endpoint, opts), to: Phoenix.CodeReloader.Server
  defguard is_ready(term) when is_atom(term)
  defguardp has_private(term) when is_binary(term)
  def socket(path, module, opts \\ []) do
    {path, module, opts}
  end
  defp private_route(conn), do: conn
end

defprotocol Phoenix.Param do
  def to_param(term)
end

defimpl Phoenix.Param, for: Integer do
  def to_param(integer), do: Integer.to_string(integer)
end

defimpl Phoenix.HTML.Safe, for: URI do
  def to_iodata(uri), do: URI.to_string(uri)
end
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for name, kind := range map[string]string{
		"Phoenix.Router":    "module",
		"__using__":         "macro",
		"define_method":     "macro",
		"reload!":           "delegate",
		"is_ready":          "guard",
		"has_private":       "guard",
		"socket":            "function",
		"private_route":     "function",
		"Phoenix.Param":     "protocol",
		"Phoenix.HTML.Safe": "implementation",
		"to_param":          "function",
	} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Elixir symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if !containsString(res.Imports, "Plug.Router") {
		t.Fatalf("imports = %#v, want Plug.Router", res.Imports)
	}
	for _, fake := range []string{"DocOnly", "fake", "comment_only"} {
		if sym := findSymbol(res.Symbols, fake); sym != nil {
			t.Fatalf("unexpected Elixir doc/comment symbol %q: %+v", fake, sym)
		}
	}
}

func TestParseGroovyLightweightDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "modules/nf-commons/src/main/nextflow/SysEnv.groovy", "", []byte(`
package nextflow

import java.nio.file.Path
import static java.util.concurrent.TimeUnit.SECONDS

/**
 * class DocOnly {
 *   def fake() { }
 * }
 */
class SysEnv {
  static boolean containsKey(String key) { true }
  static Map<String,String> get() { [:] }
  private static Path getHomeDir(String appname) { null }
  SysEnv(Map<String,String> values) { }
}

interface Job {
  void start()
}

trait Worker {
  abstract void apply()
}

enum Mode {
  FAST, SLOW
}

def topLevel(arg) { arg }

task hello {
  doLast { println 'hi' }
}

tasks.register('cleanGenerated') {
}

// def commentOnly() { }
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for name, kind := range map[string]string{
		"SysEnv":         "class",
		"containsKey":    "method",
		"get":            "method",
		"getHomeDir":     "method",
		"Job":            "interface",
		"Worker":         "trait",
		"Mode":           "enum",
		"topLevel":       "method",
		"hello":          "task",
		"cleanGenerated": "task",
	} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Groovy symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if !containsString(res.Imports, "java.nio.file.Path") {
		t.Fatalf("imports = %#v, want java.nio.file.Path", res.Imports)
	}
	if !containsString(res.Imports, "java.util.concurrent.TimeUnit.SECONDS") {
		t.Fatalf("imports = %#v, want static java.util.concurrent.TimeUnit.SECONDS", res.Imports)
	}
	for _, fake := range []string{"DocOnly", "fake", "commentOnly"} {
		if sym := findSymbol(res.Symbols, fake); sym != nil {
			t.Fatalf("unexpected Groovy doc/comment symbol %q: %+v", fake, sym)
		}
	}
}

func TestParseObjCLightweightSelectors(t *testing.T) {
	res, err := Parse("repo", "org/repo", "SDWebImage/Core/SDImageCache.m", "", []byte(`
#import <Foundation/Foundation.h>

// - (void)commentOnly:(id)value;
/* @interface CommentOnly : NSObject */
@interface SDImageCache : NSObject
- (void)storeImage:(UIImage *)image forKey:(NSString *)key completion:(void (^)(void))completion;
+ (instancetype)sharedImageCache;
@end

@implementation SDImageCache
- (void)storeImage:(UIImage *)image forKey:(NSString *)key completion:(void (^)(void))completion { }
+ (instancetype)sharedImageCache { return nil; }
- (void)storeImage:(UIImage *)image imageData:(NSData *)imageData
            forKey:(NSString *)key
         cacheType:(SDImageCacheType)cacheType
        completion:(void (^)(void))completionBlock {
}
@end

@protocol SDCache <NSObject>
- (id)objectForKey:(NSString *)key;
@end
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for name, kind := range map[string]string{
		"SDImageCache":                  "type",
		"SDCache":                       "type",
		"storeImage:forKey:completion:": "method",
		"storeImage:imageData:forKey:cacheType:completion:": "method",
		"sharedImageCache": "method",
		"objectForKey:":    "method",
	} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Objective-C symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if !containsString(res.Imports, "Foundation/Foundation.h") {
		t.Fatalf("imports = %#v, want Foundation/Foundation.h", res.Imports)
	}
	for _, fake := range []string{"CommentOnly", "commentOnly:"} {
		if sym := findSymbol(res.Symbols, fake); sym != nil {
			t.Fatalf("unexpected Objective-C doc/comment symbol %q: %+v", fake, sym)
		}
	}
}

func TestParseDartLightweightDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "pkgs/http/lib/src/base_client.dart", "", []byte(`
import 'dart:async';
export 'src/request.dart';

abstract class BaseClient {
  Future<Response> get(Uri url);
  Future<StreamedResponse> send(BaseRequest request) async => StreamedResponse();
  String get name => 'client';
  set contentLength(int? value) {
    throw UnsupportedError('read only');
  }
}

abstract mixin class Abortable implements BaseRequest {}
final class RetryClient extends BaseClient {
  RetryClient(this._inner);
  RetryClient.withDelays(this._inner);
  factory RetryClient.fromClient(BaseClient inner) =>
      RetryClient(inner);
  void close() {}
  final FutureOr<bool> Function(BaseResponse) _when;
  Duration _defaultDelay(int retryCount) =>
      const Duration(milliseconds: 500) * math.pow(1.5, retryCount);
}

class _ClientSocketException extends ClientException {
  _ClientSocketException(SocketException e, Uri uri) : super(e.message, uri);
}

class Response {}
Future<Response> makeResponse() => Response();

typedef ClientFactory = Future<Response> Function(BaseRequest request);
extension HeadersWithSplitValues on BaseResponse {
  List<String> get splitValues => [];
}
enum Method { get, post }
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for name, kind := range map[string]string{
		"BaseClient":             "type",
		"Abortable":              "type",
		"RetryClient":            "type",
		"HeadersWithSplitValues": "type",
		"Method":                 "type",
		"get":                    "function",
		"send":                   "function",
		"name":                   "getter",
		"contentLength":          "setter",
		"RetryClient.withDelays": "constructor",
		"RetryClient.fromClient": "constructor",
		"close":                  "function",
		"_defaultDelay":          "function",
		"makeResponse":           "function",
		"ClientFactory":          "typedef",
		"splitValues":            "getter",
	} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Dart symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if !containsString(res.Imports, "dart:async") {
		t.Fatalf("imports = %#v, want dart:async", res.Imports)
	}
	if constructors := symbolsNamedKind(res.Symbols, "_ClientSocketException", "constructor"); len(constructors) == 0 {
		t.Fatalf("missing Dart private constructor _ClientSocketException; symbols=%+v", res.Symbols)
	}
	if constructors := symbolsNamedKind(res.Symbols, "RetryClient", "constructor"); len(constructors) != 1 {
		t.Fatalf("RetryClient constructors = %+v, want only the declaration, not the arrow-body call", constructors)
	}
	for _, fake := range []string{"Function", "ArgumentError", "Duration", "UnsupportedError"} {
		if sym := findSymbol(res.Symbols, fake); sym != nil {
			t.Fatalf("unexpected Dart false-positive symbol %q: %+v", fake, sym)
		}
	}
	if constructors := symbolsNamedKind(res.Symbols, "Response", "constructor"); len(constructors) > 0 {
		t.Fatalf("unexpected Dart constructor-call symbol Response: %+v", constructors)
	}
}

func TestParseJuliaLightweightDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "src/JSON.jl", "", []byte(`
module JSON
using Dates, StructUtils

"""
struct DocOnly
end
function fake_doc()
end
"""

abstract type JSONStyle <: StructStyle end
primitive type T 8 end
mutable struct Object{K,V} end
@kwdef struct LazyOptions
  allownan::Bool = false
end
@noarg mutable struct MutableOptions
end
struct JSONText
  value::String
end
const DEFAULT_OBJECT_TYPE = Object{String, Any}

macro omit_empty(expr)
  expr
end

function json end
@noinline function invalid(error, buf, pos::Int, T)
end
Base.getindex(x::Object, key) = get(x, key)
lazyfile(file; jsonlines::Union{Bool, Nothing}=nothing, kw...) = open(io -> json(io; kw...), file)
parse(buf::Union{AbstractVector{UInt8},AbstractString}, ::Type{T}=Any;
  dicttype::Type{O}=DEFAULT_OBJECT_TYPE,
) where {T,O} = _parse(buf, T, O)
invalid_escape(src, n) = throw(ArgumentError("encountered invalid escape: $(n)"))
@noinline unknownfielderror(::Type{T}, key) where {T} =
  ArgumentError("encountered unknown JSON member $(repr(key)) while parsing T")
@noinline float_style_throw(fs) = throw(ArgumentError("Invalid float style: $fs"))
Base.:(==)(x::Object, y::Object) = true
Object{K,V}() where {K,V} = Object{K,V}()
_k(obj) !== notset && (count += 1)
Base.isempty(obj::Object) = _k(obj) === notset && _ch(obj) === notset
function (f::WriteClosure)(key, val)
end

_ch(obj) === notset && break
end
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for name, kind := range map[string]string{
		"JSON":                "module",
		"JSONStyle":           "type",
		"T":                   "type",
		"Object":              "type",
		"LazyOptions":         "type",
		"MutableOptions":      "type",
		"JSONText":            "type",
		"DEFAULT_OBJECT_TYPE": "constant",
		"omit_empty":          "macro",
		"json":                "function",
		"invalid":             "function",
		"Base.getindex":       "function",
		"lazyfile":            "function",
		"parse":               "function",
		"invalid_escape":      "function",
		"unknownfielderror":   "function",
		"float_style_throw":   "function",
		"Base.:(==)":          "function",
		"Base.isempty":        "function",
		"WriteClosure":        "function",
	} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Julia symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if !containsString(res.Imports, "Dates") || !containsString(res.Imports, "StructUtils") {
		t.Fatalf("imports = %#v, want Dates and StructUtils", res.Imports)
	}
	for _, fake := range []string{"DocOnly", "fake_doc", "_ch"} {
		if sym := findSymbol(res.Symbols, fake); sym != nil {
			t.Fatalf("unexpected Julia false-positive symbol %q: %+v", fake, sym)
		}
	}
	if constructors := symbolsNamedKind(res.Symbols, "Object", "function"); len(constructors) == 0 {
		t.Fatalf("missing Julia constructor-like Object function; symbols=%+v", res.Symbols)
	}
}

func TestParseRubyLightweightOperatorsAndQualifiedModules(t *testing.T) {
	res, err := Parse("repo", "org/repo", "app/user.rb", "", []byte(`
module ::Admin; def audit; end; end
class User
  def <=>(other)
  end
  def []=(key, value)
  end
  def @@sink.flush(*) end
end
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, name := range []string{"Admin", "audit", "<=>", "[]=", "flush"} {
		if findSymbol(res.Symbols, name) == nil {
			t.Fatalf("missing Ruby symbol %q; symbols=%+v", name, res.Symbols)
		}
	}
}

func TestParseCSharpLightweightMultipleModifiers(t *testing.T) {
	res, err := Parse("repo", "org/repo", "Dapper/SqlMapper.cs", "", []byte(`
using System.Data;

namespace Dapper;

[Obsolete]
public static partial class SqlMapper
{
    public readonly struct CommandDefinition
    {
        public CommandDefinition(string commandText) {}
        public ValueTask<int> ExecuteAsync(IDbConnection cnn) => default;
    }

    internal sealed record DynamicParameters;

    public static Task<IEnumerable<T>> QueryAsync<T>(this IDbConnection cnn, string sql)
        where T : class => default;
}
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, want := range []struct {
		name string
		kind string
	}{
		{"SqlMapper", "class"},
		{"CommandDefinition", "struct"},
		{"DynamicParameters", "record"},
		{"ExecuteAsync", "method"},
		{"QueryAsync", "method"},
	} {
		sym := findSymbol(res.Symbols, want.name)
		if sym == nil {
			t.Fatalf("missing symbol %q; symbols=%+v", want.name, res.Symbols)
		}
		if sym.Kind != want.kind {
			t.Fatalf("%s kind = %q, want %q", want.name, sym.Kind, want.kind)
		}
	}
	if !containsString(res.Imports, "System.Data") {
		t.Fatalf("imports = %#v, want System.Data", res.Imports)
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

func symbolsNamedKind(symbols []graph.CodeSymbol, name, kind string) []graph.CodeSymbol {
	out := make([]graph.CodeSymbol, 0)
	for _, symbol := range symbols {
		if symbol.Name == name && symbol.Kind == kind {
			out = append(out, symbol)
		}
	}
	return out
}
