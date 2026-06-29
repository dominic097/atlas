package tree_sitter_objc

// #cgo CFLAGS: -std=c11 -fPIC -Isrc
// #include "src/parser.c"
import "C"

import "unsafe"

// Language returns the generated tree-sitter Objective-C grammar pointer.
func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_objc())
}
