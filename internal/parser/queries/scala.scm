; Atlas definition query for Scala (tree-sitter/tree-sitter-scala v0.26.0).
; The grammar ships no tags.scm; node names and fields were verified against the
; grammar's node-types.json and by parsing real Scala (Scala 3 enums and operator
; names included). Every definition node exposes a `name:` field (covering both
; identifier and operator_identifier, so def `<*>` / object `==:` are captured),
; except val/var which carry the bound name in a `pattern:` field. Anchoring on
; the field (rather than a positional child) keeps the query precise — it does
; not over-capture right-hand-side type_identifiers or parameter identifiers.

; classes, objects, traits, enums keep Scala's existing Atlas `type` surface.
(class_definition
    name: (_) @name) @definition.type

(object_definition
    name: (_) @name) @definition.type

(trait_definition
    name: (_) @name) @definition.type

(enum_definition
    name: (_) @name) @definition.type

; methods / functions (concrete and abstract defs; name may be an operator)
(function_definition
    name: (_) @name) @definition.function

(function_declaration
    name: (_) @name) @definition.function

; values and variables keep Scala's existing Atlas `variable` surface.
(val_definition
    pattern: (identifier) @name) @definition.variable

(var_definition
    pattern: (identifier) @name) @definition.variable

; type aliases
(type_definition
    name: (type_identifier) @name) @definition.type

; Scala 3 enum members
(simple_enum_case
    name: (_) @name) @definition.constant
