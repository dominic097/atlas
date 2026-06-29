package tree_sitter_p4

// #cgo CFLAGS: -std=c11 -fPIC -Isrc
// #include "src/parser.c"
import "C"

import "unsafe"

func Language() unsafe.Pointer {
	return unsafe.Pointer(C.tree_sitter_p4())
}
