; Atlas definition query for Zig (tree-sitter-grammars/tree-sitter-zig v1.1.2).
; The grammar ships no tags.scm; node names/fields were verified against the
; grammar's node-types.json and by parsing real Zig. Zig has no dedicated
; type-declaration node: a type is a `const Name = struct/enum/union/opaque {...}`
; (an error set is `const Name = error {...}`), i.e. a variable_declaration whose
; value child is the corresponding *_declaration. Functions are
; function_declaration, whose name is either a `name:` identifier field or — for
; @"quoted" identifiers — a `string` child.
;
; Value constants are captured from variable_declaration as constants. Atlas's
; tags extractor filters duplicate constant captures when the same node is a
; struct/enum/union/opaque/error-set type declaration, preserving precision while
; still handling normal and compound const declarations.

; const Name = struct { ... }  → type
(variable_declaration
    (identifier) @name
    (struct_declaration)) @definition.type

; const Name = enum { ... }  → type
(variable_declaration
    (identifier) @name
    (enum_declaration)) @definition.type

; const Name = union { ... }  → type
(variable_declaration
    (identifier) @name
    (union_declaration)) @definition.type

; const Name = opaque { ... }  → type
(variable_declaration
    (identifier) @name
    (opaque_declaration)) @definition.type

; const Name = error { ... }  → type
(variable_declaration
    (identifier) @name
    (error_set_declaration)) @definition.type

; const name = value; const lhs, const rhs = pair;  → constants
(variable_declaration
    (identifier) @name) @definition.constant

; functions and methods (named with a plain identifier)
(function_declaration
    name: (identifier) @name) @definition.function

; functions and methods named with a @"quoted" identifier
(function_declaration
    (string) @name) @definition.function
