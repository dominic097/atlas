; Atlas definition query for Lua. This is the OFFICIAL tags.scm shipped by
; tree-sitter-grammars/tree-sitter-lua v0.5.0 (queries/tags.scm), used verbatim
; so Lua's native definition surface matches the grammar authors' intent. Lua is
; sparse — definitions are functions: named function_declaration (incl.
; dotted M.fn), method_index_expression functions (M:fn), function values
; assigned to a name, and functions stored as table fields. The trailing
; @reference.call pattern is ignored by Atlas (only @definition.* yields
; symbols). Atlas keeps Lua colon definitions on the `function` kind because the
; benchmark and old regex surface count Lua definitions as functions.

(function_declaration
  name: [
    (identifier) @name
    (dot_index_expression) @name
  ]) @definition.function

(function_declaration
  name: (method_index_expression) @name) @definition.function

(assignment_statement
  (variable_list
    .
    name: [
      (identifier) @name
      (dot_index_expression) @name
      (method_index_expression) @name
      (bracket_index_expression) @name
    ])
  (expression_list
    .
    value: (function_definition))) @definition.function

(function_call
  name: [
    (identifier) @name
    (dot_index_expression
      field: (identifier) @name)
    (method_index_expression
      method: (identifier) @name)
  ]) @reference.call
