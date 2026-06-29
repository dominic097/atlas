(class_declaration name: (identifier) @name) @definition.class

(class_declaration (base_list (_) @name)) @reference.class

(interface_declaration name: (identifier) @name) @definition.interface

(interface_declaration (base_list (_) @name)) @reference.interface

(method_declaration name: (identifier) @name) @definition.method

(object_creation_expression type: (identifier) @name) @reference.class

(type_parameter_constraints_clause (identifier) @name) @reference.class

(type_parameter_constraint (type type: (identifier) @name)) @reference.class

(variable_declaration type: (identifier) @name) @reference.class

(invocation_expression function: (member_access_expression name: (identifier) @name)) @reference.send

(namespace_declaration name: (identifier) @name) @definition.module

(namespace_declaration name: (identifier) @name) @module

; --- Atlas augmentation (real def nodes the upstream tags.scm omits) ---------
; The official C# tags.scm only models class/interface/method/namespace. These
; patterns add the remaining first-class C# definition sites so Atlas indexes
; structs, enums (and their members), constructors, properties and records via
; the same native AST path. Node names verified against tree-sitter-c-sharp
; v0.23.5 (struct_declaration / enum_declaration / enum_member_declaration /
; constructor_declaration / property_declaration / record_declaration).

(struct_declaration name: (identifier) @name) @definition.struct

(enum_declaration name: (identifier) @name) @definition.enum

(enum_member_declaration name: (identifier) @name) @definition.constant

(constructor_declaration name: (identifier) @name) @definition.constructor

(property_declaration name: (identifier) @name) @definition.field

(record_declaration name: (identifier) @name) @definition.class
