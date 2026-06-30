; Kotlin definition tags for Atlas's generic native tree-sitter extractor.
; tree-sitter-grammars/tree-sitter-kotlin v1.1.0 ships no official tags.scm, so
; these patterns cover the source definition surface used by the independent
; tree-sitter-kotlin benchmark: types, functions, properties, and enum entries.
; Keep Kotlin's existing Atlas kind surface (`type`, `function`, `variable`) so
; switching off the regex fallback does not break callers.

(class_declaration
  (identifier) @name) @definition.type

(object_declaration
  (identifier) @name) @definition.type

(type_alias
  type: (identifier) @name) @definition.type

(function_declaration
  (identifier) @name) @definition.function

(property_declaration
  (variable_declaration
    (identifier) @name)) @definition.variable

(enum_entry
  (identifier) @name) @definition.constant
