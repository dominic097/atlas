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
		"src/lib.rs":                         "rust",
		"app/models/user.rb":                 "ruby",
		"src/Main.kt":                        "kotlin",
		"build.gradle.kts":                   "kotlin",
		"src/Main.scala":                     "scala",
		"src/index.php":                      "php",
		"views/home.blade.php":               "blade",
		"Sources/App.swift":                  "swift",
		"plugin/init.lua":                    "lua",
		"plugin/init.luau":                   "lua",
		"addon/MyAddon.toc":                  "lua",
		"src/main.zig":                       "zig",
		"lib/app.ex":                         "elixir",
		"lib/app.exs":                        "elixir",
		"AppDelegate.m":                      "objc",
		"ViewController.mm":                  "objc",
		"src/main.jl":                        "julia",
		"solver.F90":                         "fortran",
		"solver.f03":                         "fortran",
		"lib/main.dart":                      "dart",
		"rtl/core.v":                         "verilog",
		"rtl/core.sv":                        "verilog",
		"rtl/core.svh":                       "verilog",
		"src/unit.pas":                       "pascal",
		"src/unit.pp":                        "pascal",
		"src/project.dpr":                    "pascal",
		"src/package.dpk":                    "pascal",
		"src/main.lpr":                       "pascal",
		"src/include.inc":                    "pascal",
		"forms/main.dfm":                     "delphi",
		"forms/main.lfm":                     "delphi",
		"forms/pkg.lpk":                      "delphi",
		"infra/main.tf":                      "terraform",
		"infra/vars.tfvars":                  "terraform",
		"infra/module.hcl":                   "terraform",
		"code/game.dm":                       "byond",
		"code/project.dme":                   "byond",
		"code/icon.dmi":                      "byond",
		"code/map.dmm":                       "byond",
		"code/ui.dmf":                        "byond",
		"App.sln":                            "dotnet",
		"App.slnx":                           "dotnet",
		"App/App.csproj":                     "dotnet",
		"App/App.fsproj":                     "dotnet",
		"App/App.vbproj":                     "dotnet",
		"Pages/Index.razor":                  "razor",
		"Pages/Index.cshtml":                 "razor",
		"force-app/Foo.cls":                  "apex",
		"force-app/Foo.trigger":              "apex",
		"web/App.vue":                        "vue",
		"web/App.svelte":                     "svelte",
		"web/App.astro":                      "astro",
		"views/index.ejs":                    "ejs",
		"entry/src/main/ets/MainAbility.ets": "ets",
		"R/plot-build.R":                     "r",
		"native/kernel.cu":                   "cpp",
		"native/kernel.cuh":                  "cpp",
		"docs/design.qmd":                    "markdown",
		"scripts/profile.psm1":               "powershell",
		"scripts/manifest.psd1":              "powershell",
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
			// rust now routes to the NATIVE tags-query path; symbols are recovered
			// from the AST (run → function). Imports are not modeled by the tags
			// query, so wantImport is intentionally empty for the native languages.
			name:       "rust",
			path:       "src/lib.rs",
			content:    "use std::fmt;\npub struct Worker {}\nfn run() { helper(); }\nfn helper() {}\n",
			wantSymbol: "run",
			wantKind:   "function",
		},
		{
			// ruby now routes to the NATIVE tags-query path: the `[]` operator
			// method is recovered as a method from the AST. Imports are not modeled
			// by the tags query (wantImport empty).
			name:       "ruby",
			path:       "app/user.rb",
			content:    "require 'json'\nmodule ::Admin; def audit; end; end\nclass User\n  def save!\n  end\n  def [](key)\n  end\n  def @@sink.flush(*) end\nend\n",
			wantSymbol: "[]",
			wantKind:   "method",
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

func TestParseRustNestedGenericAndRawIdentifierFunctions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "src/lib.rs", "", []byte(`
pub fn patterns_from_path<P: AsRef<Path>>(path: P) -> io::Result<Vec<String>> {
    todo!()
}

fn r#async(mut stderr: process::ChildStderr) -> StderrReader {
    todo!()
}
`))
	if err != nil {
		t.Fatalf("Parse rust: %v", err)
	}
	for _, want := range []string{"patterns_from_path", "r#async"} {
		sym := findSymbol(res.Symbols, want)
		if sym == nil {
			t.Fatalf("missing rust symbol %q; symbols=%+v", want, res.Symbols)
		}
		if sym.Kind != "function" {
			t.Fatalf("%s kind = %q, want function", want, sym.Kind)
		}
	}
}

func TestParsePowerShellNativeDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "scripts/install.ps1", "", []byte(`
using module './lib/common.psm1'
Import-Module -Name Pester

function Invoke-Install {
  Invoke-Verify
}

filter Select-Install {
  process { $_ }
}

workflow Start-Install { }

class Installer {
  [void]Run() { }
  Installer() { }
}

# function Ignored-Install { }
`))
	if err != nil {
		t.Fatalf("Parse powershell: %v", err)
	}
	want := map[string]string{
		"Invoke-Install": "function",
		"Select-Install": "function",
		"Start-Install":  "function",
		"Installer":      "class",
		"Run":            "method",
	}
	for name, kind := range want {
		if len(symbolsNamedKind(res.Symbols, name, kind)) == 0 {
			t.Fatalf("missing PowerShell %s %q; symbols=%+v", kind, name, res.Symbols)
		}
	}
	if findSymbol(res.Symbols, "Ignored-Install") != nil {
		t.Fatalf("commented PowerShell function was indexed: %+v", res.Symbols)
	}
	for _, wantImport := range []string{"./lib/common.psm1", "Pester"} {
		if !containsString(res.Imports, wantImport) {
			t.Fatalf("PowerShell imports = %#v, want %q", res.Imports, wantImport)
		}
	}
}

func TestParseVueNativeScriptDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "web/App.vue", "", []byte(`<template>
  <main>{{ title }}</main>
</template>

<script setup lang="ts">
import { ref } from 'vue'
const title = ref('Atlas')
let count = 0
// const ignoredComment = 1
</script>

<script>
import service from './service'
function loadArticles() {
  const nested = true
  return service.all()
}
</script>
`))
	if err != nil {
		t.Fatalf("Parse vue: %v", err)
	}
	want := map[string]int{
		"title":        7,
		"count":        8,
		"loadArticles": 14,
		"nested":       15,
	}
	for name, line := range want {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Vue symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != "function" {
			t.Fatalf("Vue symbol %q kind = %q, want function", name, sym.Kind)
		}
		if sym.StartLine != line {
			t.Fatalf("Vue symbol %q line = %d, want %d", name, sym.StartLine, line)
		}
	}
	if findSymbol(res.Symbols, "ignoredComment") != nil {
		t.Fatalf("commented Vue declaration was indexed: %+v", res.Symbols)
	}
	for _, wantImport := range []string{"vue", "./service"} {
		if !containsString(res.Imports, wantImport) {
			t.Fatalf("Vue imports = %#v, want %q", res.Imports, wantImport)
		}
	}
}

func TestParsePascalDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "Source/uPSCompiler.pas", "", []byte(`
unit uPSCompiler;

interface

type
  TPSPascalCompiler = class
  end;

procedure RegisterClassLibraryRuntime;
function MakeHash(const Value: string): Longint;
constructor TPSPascalCompiler.Create;
destructor TPSPascalCompiler.Destroy;
class procedure TPSPascalCompiler.RegisterStandardLibrary;
class function TPSPascalCompiler.CompileUnit: Boolean;

implementation

end.
`))
	if err != nil {
		t.Fatalf("Parse pascal: %v", err)
	}
	want := map[string]string{
		"uPSCompiler":                               "unit",
		"TPSPascalCompiler":                         "type",
		"RegisterClassLibraryRuntime":               "function",
		"MakeHash":                                  "function",
		"TPSPascalCompiler.Create":                  "function",
		"TPSPascalCompiler.Destroy":                 "function",
		"TPSPascalCompiler.RegisterStandardLibrary": "function",
		"TPSPascalCompiler.CompileUnit":             "function",
	}
	for name, kind := range want {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing pascal symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
}

func TestParseBladeDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "resources/views/settings/layout.blade.php", "", []byte(`
@extends('layouts.simple')
@section('body')
  @include('settings.parts.navbar')
  @includeIf('form.user-select')
  @component('components.alert')
    @yield('after-content')
  @endcomponent
  <livewire:user-picker />
  <x-dropdown.item wire:click="saveRole('admin')" />
@endsection
`))
	if err != nil {
		t.Fatalf("Parse blade: %v", err)
	}
	want := map[string]string{
		"settings.layout":       "template",
		"layouts.simple":        "layout",
		"body":                  "section",
		"settings.parts.navbar": "include",
		"form.user-select":      "include",
		"components.alert":      "component",
		"after-content":         "slot",
		"user-picker":           "component",
		"dropdown.item":         "component",
		"saveRole('admin')":     "handler",
	}
	for name, kind := range want {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing blade symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	for _, wantImport := range []string{"layouts.simple", "settings.parts.navbar", "form.user-select", "components.alert"} {
		if !containsString(res.Imports, wantImport) {
			t.Fatalf("imports = %#v, want %q", res.Imports, wantImport)
		}
	}
}

func TestParseRazorDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "src/BlazorAdmin/Pages/CatalogItemPage/Create.razor", "", []byte(`
@page "/catalog-items/create"
@using Microsoft.AspNetCore.Components.Forms
@inject IJSRuntime JSRuntime
@inherits BlazorAdmin.Helpers.BlazorComponent
@model ConfirmEmailModel

<EditForm Model="@Item">
  <DataAnnotationsValidator />
  <InputText @bind-Value="Item.Name" />
  <div>plain html should not be a component</div>
</EditForm>

@code {
  public void CreateClick() {}
  protected override async Task OnInitializedAsync() {}
}
`))
	if err != nil {
		t.Fatalf("Parse razor: %v", err)
	}
	want := map[string]string{
		"BlazorAdmin.Pages.CatalogItemPage.Create": "component",
		"/catalog-items/create":                    "route",
		"Microsoft.AspNetCore.Components.Forms":    "import",
		"IJSRuntime":                               "service",
		"BlazorAdmin.Helpers.BlazorComponent":      "base",
		"ConfirmEmailModel":                        "model",
		"EditForm":                                 "component",
		"DataAnnotationsValidator":                 "component",
		"InputText":                                "component",
		"CreateClick":                              "method",
		"OnInitializedAsync":                       "method",
	}
	for name, kind := range want {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing razor symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if findSymbol(res.Symbols, "Div") != nil {
		t.Fatalf("html Div tag should not be indexed as a Razor component: %+v", res.Symbols)
	}
	for _, wantImport := range []string{"Microsoft.AspNetCore.Components.Forms", "IJSRuntime"} {
		if !containsString(res.Imports, wantImport) {
			t.Fatalf("imports = %#v, want %q", res.Imports, wantImport)
		}
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
program stdlib_driver
end program stdlib_driver
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
		"stdlib_driver":          "program",
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
  def +(left, right), do: left + right
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
		"+":                 "function",
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

func TestParseBashNativeDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "scripts/install.sh", "", []byte(`
#!/usr/bin/env bash
source ./lib/common.sh
. ./lib/colors.sh

function install_node {
  echo installing
}

nvm_use() {
  echo using
}

# comment_only() {
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, name := range []string{"install_node", "nvm_use"} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing Bash symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != "function" {
			t.Fatalf("%s kind = %q, want function", name, sym.Kind)
		}
	}
	if !containsString(res.Imports, "./lib/common.sh") {
		t.Fatalf("imports = %#v, want ./lib/common.sh", res.Imports)
	}
	if !containsString(res.Imports, "./lib/colors.sh") {
		t.Fatalf("imports = %#v, want ./lib/colors.sh", res.Imports)
	}
	if sym := findSymbol(res.Symbols, "comment_only"); sym != nil {
		t.Fatalf("unexpected Bash comment symbol: %+v", sym)
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
	// C# is now parsed via the NATIVE tree-sitter tags-query path (tagsquery.go),
	// not the regex fallback. The native AST recovers the same set of definitions
	// (and more — the struct's constructor too). A `record` declaration maps to
	// @definition.class under the cross-grammar tags convention, so
	// DynamicParameters is a class here (the old regex path reported "record").
	for _, want := range []struct {
		name string
		kind string
	}{
		{"SqlMapper", "class"},
		{"CommandDefinition", "struct"},
		{"DynamicParameters", "class"},
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
	// The struct's constructor is recovered by the native path (the regex path
	// did not emit it) — a strict gain in recall.
	if findSymbol(res.Symbols, "CommandDefinition") == nil {
		t.Fatal("missing CommandDefinition")
	}
	// Imports are not modeled by the tags query, so res.Imports is empty for the
	// native languages; `using System.Data;` is intentionally not surfaced here.
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

func TestParseMarkdownSectionsIgnoreFencedCode(t *testing.T) {
	content := []byte(`# Guide

intro

` + "```rust" + `
# fn main() {
#     println!("hidden line");
# }
` + "```" + `

#not-a-heading

## Install ##

~~~sh
### not a shell heading
~~~

### Usage
`)

	res, err := Parse("repo", "org/repo", "guide.md", "", content)
	if err != nil {
		t.Fatalf("Parse markdown: %v", err)
	}
	want := []string{"Guide", "Install", "Usage"}
	for _, name := range want {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing markdown section %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != "section" {
			t.Fatalf("%s kind = %q, want section", name, sym.Kind)
		}
	}
	for _, name := range []string{"fn main() {", "not a shell heading", "not-a-heading"} {
		if findSymbol(res.Symbols, name) != nil {
			t.Fatalf("markdown false-positive heading %q was indexed", name)
		}
	}
}

func TestParseJSONConfigKeys(t *testing.T) {
	content := []byte(`{
  "scripts": {
    "test": "go test ./...",
    "lint": "golangci-lint run"
  },
  "compilerOptions": {
    "outDir": "dist"
  },
  "contributes": {
    "commands": [
      {"command": "atlas.review", "title": "Review"}
    ]
  }
}`)

	res, err := Parse("repo", "org/repo", "package.json", "", content)
	if err != nil {
		t.Fatalf("Parse json: %v", err)
	}
	for _, name := range []string{"scripts", "scripts.test", "compilerOptions.outDir", "contributes.commands[].command"} {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing json key %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != "key" {
			t.Fatalf("%s kind = %q, want key", name, sym.Kind)
		}
		if sym.Metadata["source"] != "json_parser" {
			t.Fatalf("%s metadata source = %#v, want json_parser", name, sym.Metadata["source"])
		}
	}
	if findSymbol(res.Symbols, "package.json") != nil {
		t.Fatal("valid json should not fall back to a file-level document symbol")
	}
}

func TestParseInvalidJSONFallsBackToDocument(t *testing.T) {
	res, err := Parse("repo", "org/repo", "broken.json", "", []byte(`{"scripts":`))
	if err != nil {
		t.Fatalf("Parse invalid json: %v", err)
	}
	sym := findSymbol(res.Symbols, "broken.json")
	if sym == nil {
		t.Fatalf("missing fallback document; symbols=%+v", res.Symbols)
	}
	if sym.Kind != "document" {
		t.Fatalf("fallback kind = %q, want document", sym.Kind)
	}
}

func TestParseDotnetProjectFiles(t *testing.T) {
	csproj, err := Parse("repo", "org/repo", "Dapper/Dapper.csproj", "", []byte(`<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFrameworks>net461;netstandard2.0;net8.0</TargetFrameworks>
  </PropertyGroup>
  <ItemGroup>
    <!--<PackageReference Include="Commented.Package" />-->
    <PackageReference Include="Microsoft.Bcl.AsyncInterfaces" />
    <ProjectReference Include="..\Dapper.ProviderTools\Dapper.ProviderTools.csproj" />
  </ItemGroup>
</Project>`))
	if err != nil {
		t.Fatalf("Parse csproj: %v", err)
	}
	wantCSProj := map[string]string{
		"Dapper":                        "project",
		"Microsoft.NET.Sdk":             "sdk",
		"Microsoft.Bcl.AsyncInterfaces": "package",
		"Dapper.ProviderTools":          "project_reference",
		"netstandard2.0":                "target_framework",
	}
	for name, kind := range wantCSProj {
		sym := findSymbol(csproj.Symbols, name)
		if sym == nil {
			t.Fatalf("missing csproj symbol %q; symbols=%+v", name, csproj.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if !containsString(csproj.Imports, `..\Dapper.ProviderTools\Dapper.ProviderTools.csproj`) {
		t.Fatalf("csproj imports = %#v, want ProjectReference path", csproj.Imports)
	}
	if findSymbol(csproj.Symbols, "Commented.Package") != nil {
		t.Fatal("commented PackageReference should not be indexed")
	}

	slnx, err := Parse("repo", "org/repo", "Dapper.slnx", "", []byte(`<Solution>
  <Project Path="Dapper/Dapper.csproj" />
  <Project Path="tests/Dapper.Tests/Dapper.Tests.csproj" />
</Solution>`))
	if err != nil {
		t.Fatalf("Parse slnx: %v", err)
	}
	for _, name := range []string{"Dapper", "Dapper.Tests"} {
		sym := findSymbol(slnx.Symbols, name)
		if sym == nil {
			t.Fatalf("missing slnx project %q; symbols=%+v", name, slnx.Symbols)
		}
		if sym.Kind != "project" {
			t.Fatalf("%s kind = %q, want project", name, sym.Kind)
		}
	}

	sln, err := Parse("repo", "org/repo", "Dapper.sln", "", []byte(`Project("{GUID}") = "Dapper", "Dapper\Dapper.csproj", "{PROJECT-GUID}"
EndProject
`))
	if err != nil {
		t.Fatalf("Parse sln: %v", err)
	}
	if sym := findSymbol(sln.Symbols, "Dapper"); sym == nil || sym.Kind != "project" {
		t.Fatalf("missing sln project Dapper; symbols=%+v", sln.Symbols)
	}
}

func TestParseAstroDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "src/pages/tags/[tag].astro", "", []byte(`---
import BaseLayout from "../../layouts/BaseLayout.astro";
import BlogPost from "../../components/BlogPost.astro";

export async function getStaticPaths() {
  return [];
}

const { tag } = Astro.params;
const { posts } = Astro.props;
const pageTitle = "Tag Index";
---

<BaseLayout pageTitle={tag}>
  <BlogPost url="/" title="Hello" />
</BaseLayout>
`))
	if err != nil {
		t.Fatalf("Parse astro: %v", err)
	}
	want := map[string]string{
		"[tag]":          "component",
		"getStaticPaths": "function",
		"tag":            "variable",
		"posts":          "variable",
		"pageTitle":      "variable",
		"BaseLayout":     "component",
		"BlogPost":       "component",
	}
	for name, kind := range want {
		sym := findSymbol(res.Symbols, name)
		if sym == nil {
			t.Fatalf("missing astro symbol %q; symbols=%+v", name, res.Symbols)
		}
		if sym.Kind != kind {
			t.Fatalf("%s kind = %q, want %q", name, sym.Kind, kind)
		}
	}
	if !containsString(res.Imports, "../../layouts/BaseLayout.astro") {
		t.Fatalf("astro imports = %#v, want BaseLayout import", res.Imports)
	}
}

func TestParseApexDefinitions(t *testing.T) {
	classRes, err := Parse("repo", "org/repo", "force-app/main/default/classes/AccountService.cls", "", []byte(`
public inherited sharing class AccountService extends BaseService implements Queueable {
  public enum SortOrder { ASC, DESC }
  public interface ServiceShape {
    void execute();
  }

  public AccountService() {}

  @AuraEnabled(cacheable=true)
  public static List<Account> listAccounts() {
    List<Account> accounts = [SELECT Id, Name FROM Account LIMIT 10];
    // DELETE request text in a comment must not become DML context.
    insert accounts;
    Database.update(accounts);
    return accounts;
  }

  testMethod static void legacyTest() {}
  public void execute(QueueableContext context) {}
}
`))
	if err != nil {
		t.Fatalf("Parse apex class: %v", err)
	}
	wantClass := []struct {
		name string
		kind string
	}{
		{"AccountService", "type"},
		{"SortOrder", "type"},
		{"ServiceShape", "type"},
		{"AccountService", "constructor"},
		{"listAccounts", "method"},
		{"legacyTest", "method"},
		{"execute", "method"},
		{"Account", "sobject"},
		{"insert", "dml"},
		{"update", "dml"},
	}
	for _, want := range wantClass {
		matches := symbolsNamedKind(classRes.Symbols, want.name, want.kind)
		if len(matches) == 0 {
			t.Fatalf("missing apex %s %q; symbols=%+v", want.kind, want.name, classRes.Symbols)
		}
	}
	if sym := findSymbol(classRes.Symbols, "listAccounts"); sym == nil || sym.Metadata["annotations"] != "auraenabled" {
		t.Fatalf("listAccounts annotation metadata = %#v", sym)
	}
	if len(symbolsNamedKind(classRes.Symbols, "delete", "dml")) > 0 {
		t.Fatalf("comment-only delete DML should not be indexed; symbols=%+v", classRes.Symbols)
	}

	triggerRes, err := Parse("repo", "org/repo", "force-app/main/default/triggers/AccountTrigger.trigger", "", []byte(`
trigger AccountTrigger on Account (before insert, after update) {
  AccountService.listAccounts();
}
`))
	if err != nil {
		t.Fatalf("Parse apex trigger: %v", err)
	}
	for name, kind := range map[string]string{
		"AccountTrigger": "trigger",
		"Account":        "sobject",
	} {
		if len(symbolsNamedKind(triggerRes.Symbols, name, kind)) == 0 {
			t.Fatalf("missing apex trigger %s %q; symbols=%+v", kind, name, triggerRes.Symbols)
		}
	}
}

func TestParseDetectorOnlyGraphifyFormats(t *testing.T) {
	ejs, err := Parse("repo", "org/repo", "views/users/index.ejs", "", []byte(`
<%- include('../partials/header') %>
<% function formatUser(user) { return user.name } %>
<% const pageTitle = "Users"; %>
<ul>
  <% users.forEach(function(user) { %>
    <li><%= formatUser(user) %></li>
  <% }) %>
</ul>
`))
	if err != nil {
		t.Fatalf("Parse ejs: %v", err)
	}
	for name, kind := range map[string]string{
		"users.index":        "template",
		"../partials/header": "include",
		"formatUser":         "function",
		"pageTitle":          "variable",
	} {
		if len(symbolsNamedKind(ejs.Symbols, name, kind)) == 0 {
			t.Fatalf("missing EJS %s %q; symbols=%+v", kind, name, ejs.Symbols)
		}
	}
	if !containsString(ejs.Imports, "../partials/header") {
		t.Fatalf("EJS imports = %#v, want partial include", ejs.Imports)
	}

	ets, err := Parse("repo", "org/repo", "entry/src/main/ets/pages/Index.ets", "", []byte(`
import router from '@ohos.router';
import Logger from '../common/Logger';

@Entry
@Component
struct Index {
  @State message: string = 'Hello';
  private count: number = 0;

  aboutToAppear() {
    Logger.info('show');
  }

  build() {
    Column() {
      Text(this.message)
      if (this.count > 0) {
        Text('count')
      }
    }
  }

  constructor() {}
}

export function openDetail(id: string): void {
  router.pushUrl({ url: 'pages/Detail' });
}
`))
	if err != nil {
		t.Fatalf("Parse ets: %v", err)
	}
	for name, kind := range map[string]string{
		"Index":         "type",
		"message":       "variable",
		"count":         "variable",
		"aboutToAppear": "method",
		"build":         "method",
		"constructor":   "constructor",
		"openDetail":    "function",
	} {
		if len(symbolsNamedKind(ets.Symbols, name, kind)) == 0 {
			t.Fatalf("missing ETS %s %q; symbols=%+v", kind, name, ets.Symbols)
		}
	}
	if findSymbol(ets.Symbols, "Column") != nil {
		t.Fatalf("ArkUI component call Column should not be indexed as a method; symbols=%+v", ets.Symbols)
	}
	if findSymbol(ets.Symbols, "if") != nil {
		t.Fatalf("control-flow keyword if should not be indexed as a method; symbols=%+v", ets.Symbols)
	}
	for _, want := range []string{"@ohos.router", "../common/Logger"} {
		if !containsString(ets.Imports, want) {
			t.Fatalf("ETS imports = %#v, want %q", ets.Imports, want)
		}
	}

	r, err := Parse("repo", "org/repo", "R/build_plot.R", "", []byte(`
library(ggplot2)
source("R/helpers.R")

build_plot <- function(data) {
  ggplot(data)
}

summarise.data = function(data) {
  data
}

ggplot.default <-
  function(data, mapping = aes(), ...) {
    data
  }

setClass("PlotSpec", slots = list(title = "character"))
GeomPoint <- ggproto("GeomPoint", Geom)
plot_cache <- new.env()
`))
	if err != nil {
		t.Fatalf("Parse r: %v", err)
	}
	for name, kind := range map[string]string{
		"build_plot":     "function",
		"summarise.data": "function",
		"ggplot.default": "function",
		"PlotSpec":       "type",
		"GeomPoint":      "type",
		"plot_cache":     "variable",
	} {
		if len(symbolsNamedKind(r.Symbols, name, kind)) == 0 {
			t.Fatalf("missing R %s %q; symbols=%+v", kind, name, r.Symbols)
		}
	}
	for _, want := range []string{"ggplot2", "R/helpers.R"} {
		if !containsString(r.Imports, want) {
			t.Fatalf("R imports = %#v, want %q", r.Imports, want)
		}
	}
	for _, fake := range []string{"title"} {
		if sym := findSymbol(r.Symbols, fake); sym != nil {
			t.Fatalf("unexpected R argument symbol %q: %+v", fake, sym)
		}
	}
}

func TestParseByondDefinitions(t *testing.T) {
	res, err := Parse("repo", "org/repo", "code/modules/mob/living/living.dm", "", []byte(`#include "helpers.dm"

/mob/living
	name = "living mob"
	Initialize(mapload)
		if(mapload)
			update_transform()
	proc/prepare_data_huds()
		med_hud_set_health()

/mob/living/Destroy()
	return ..()

/mob/living/proc/ZImpactDamage(turf/impacted_turf, levels)
	return levels

/// Modifier for mobs landing on their feet after a fall
/datum/movespeed_modifier/landed_on_feet
	multiplicative_slowdown = 0.5

/obj/item/proc/equip_to_slot(slot)
	return TRUE

proc/global_helper()
	return TRUE

// /mob/living/proc/commented_out()
`))
	if err != nil {
		t.Fatalf("Parse byond dm: %v", err)
	}
	for name, kind := range map[string]string{
		"/mob/living":                              "type",
		"/mob/living/Initialize":                   "method",
		"/mob/living/prepare_data_huds":            "method",
		"/mob/living/Destroy":                      "method",
		"/mob/living/ZImpactDamage":                "method",
		"/datum/movespeed_modifier/landed_on_feet": "type",
		"/obj/item":                                "type",
		"/obj/item/equip_to_slot":                  "method",
		"global_helper":                            "proc",
	} {
		if len(symbolsNamedKind(res.Symbols, name, kind)) == 0 {
			t.Fatalf("missing BYOND %s %q; symbols=%+v", kind, name, res.Symbols)
		}
	}
	for _, unexpected := range []string{"if", "update_transform", "/mob/living/commented_out"} {
		if findSymbol(res.Symbols, unexpected) != nil {
			t.Fatalf("unexpected BYOND false-positive %q; symbols=%+v", unexpected, res.Symbols)
		}
	}
	if !containsString(res.Imports, "helpers.dm") {
		t.Fatalf("BYOND imports = %#v, want helpers.dm", res.Imports)
	}
}

func TestParseByondResourceDefinitions(t *testing.T) {
	dmf, err := Parse("repo", "org/repo", "code/ui.dmf", "", []byte(`window "main"
	elem "chat"
	type = output
	elem "input"
	type = input
`))
	if err != nil {
		t.Fatalf("Parse byond dmf: %v", err)
	}
	for name, kind := range map[string]string{
		"main":             "window",
		"main/chat":        "element",
		"main/chat:output": "element_type",
		"main/input":       "element",
		"main/input:input": "element_type",
	} {
		if len(symbolsNamedKind(dmf.Symbols, name, kind)) == 0 {
			t.Fatalf("missing BYOND DMF %s %q; symbols=%+v", kind, name, dmf.Symbols)
		}
	}

	dmi, err := Parse("repo", "org/repo", "icons/mob.dmi", "", []byte(`# BEGIN DMI
state = "standing"
state = dead
# END DMI
`))
	if err != nil {
		t.Fatalf("Parse byond dmi: %v", err)
	}
	for _, state := range []string{"standing", "dead"} {
		if len(symbolsNamedKind(dmi.Symbols, state, "state")) == 0 {
			t.Fatalf("missing BYOND DMI state %q; symbols=%+v", state, dmi.Symbols)
		}
	}

	dmm, err := Parse("repo", "org/repo", "maps/station.dmm", "", []byte(`"aa" = (/turf/open/floor,/area/station/hallway)
(1,1,1) = {"aa"}
`))
	if err != nil {
		t.Fatalf("Parse byond dmm: %v", err)
	}
	for _, ref := range []string{"/turf/open/floor", "/area/station/hallway"} {
		if len(symbolsNamedKind(dmm.Symbols, ref, "map_reference")) == 0 {
			t.Fatalf("missing BYOND DMM reference %q; symbols=%+v", ref, dmm.Symbols)
		}
	}
}

func TestParseDelphiLazarusDefinitions(t *testing.T) {
	form, err := Parse("repo", "org/repo", "ide/aboutfrm.lfm", "", []byte(`object AboutForm: TAboutForm
  OnClose = FormClose
  OnCreate = AboutFormCreate
  object Notebook: TPageControl
    OnChange = NotebookPageChanged
    object VersionPage: TTabSheet
    end
  end
end
`))
	if err != nil {
		t.Fatalf("Parse delphi form: %v", err)
	}
	for name, kind := range map[string]string{
		"AboutForm":           "component",
		"TAboutForm":          "component_type",
		"Notebook":            "component",
		"TPageControl":        "component_type",
		"VersionPage":         "component",
		"TTabSheet":           "component_type",
		"FormClose":           "event",
		"AboutFormCreate":     "event",
		"NotebookPageChanged": "event",
	} {
		if len(symbolsNamedKind(form.Symbols, name, kind)) == 0 {
			t.Fatalf("missing Delphi form %s %q; symbols=%+v", kind, name, form.Symbols)
		}
	}

	pkg, err := Parse("repo", "org/repo", "ide/packages/ideproject/ideproject.lpk", "", []byte(`<?xml version="1.0" encoding="UTF-8"?>
<CONFIG>
  <Package Version="5">
    <Name Value="IdeProject"/>
    <Files>
      <Item>
        <Filename Value="buildmanager.pas"/>
        <UnitName Value="BuildManager"/>
      </Item>
    </Files>
    <RequiredPkgs>
      <Item>
        <PackageName Value="IdePackager"/>
      </Item>
    </RequiredPkgs>
  </Package>
</CONFIG>
`))
	if err != nil {
		t.Fatalf("Parse lazarus package: %v", err)
	}
	for name, kind := range map[string]string{
		"IdeProject":   "package",
		"BuildManager": "unit",
		"IdePackager":  "dependency",
	} {
		if len(symbolsNamedKind(pkg.Symbols, name, kind)) == 0 {
			t.Fatalf("missing Lazarus package %s %q; symbols=%+v", kind, name, pkg.Symbols)
		}
	}
	if !containsString(pkg.Imports, "IdePackager") {
		t.Fatalf("Lazarus package imports = %#v, want IdePackager", pkg.Imports)
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
