package tree_sitter_swift

// #cgo CFLAGS: -std=c11 -fPIC
// #if defined(__clang__)
// #pragma clang diagnostic ignored "-Wmacro-redefined"
// #endif
// #include "src/parser.c"
// #include "src/scanner.c"
import "C"

import "unsafe"

// Language returns the generated tree-sitter Swift grammar pointer.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_swift())
}
