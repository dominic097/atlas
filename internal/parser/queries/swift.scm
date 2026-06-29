; Swift definition tags for Atlas's generic native tree-sitter extractor.
; Based on alex-pinkus/tree-sitter-swift v0.7.3 node shapes. Atlas keeps its
; existing Swift kind surface: declarations become type/function/variable plus
; enum cases as constants.

(class_declaration
  name: (_) @name) @definition.type

(protocol_declaration
  name: (_) @name) @definition.type

(typealias_declaration
  name: (_) @name) @definition.type

(associatedtype_declaration
  name: (_) @name) @definition.type

(function_declaration
  name: (simple_identifier) @name) @definition.function

(protocol_function_declaration
  name: (simple_identifier) @name) @definition.function

(init_declaration
  "init" @name) @definition.function

(deinit_declaration
  "deinit" @name) @definition.function

(property_declaration
  name: (pattern
    (simple_identifier) @name)) @definition.variable

(protocol_property_declaration
  name: (pattern
    (simple_identifier) @name)) @definition.variable

(enum_entry
  name: (simple_identifier) @name) @definition.constant
