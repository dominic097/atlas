package tree_sitter_powershell

// #cgo CFLAGS: -std=c11 -fPIC -Isrc
// #include "src/parser.c"
// #include "src/scanner.c"
import "C"

import "unsafe"

// Language returns the generated tree-sitter PowerShell grammar pointer.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_powershell())
}
