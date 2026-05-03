package codeindex

import (
	"path/filepath"
	"strings"

	treesitter "github.com/tree-sitter/go-tree-sitter"
	sittergo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	sitterjavascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	sitterpython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	sittertypescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type LanguageAdapter struct {
	ID              string
	Extensions      []string
	Language        *treesitter.Language
	DefinitionQuery string
	ImportQuery     string
	ReferenceQuery  string
}

func BuiltinLanguages() []LanguageAdapter {
	return []LanguageAdapter{
		{
			ID:         "go",
			Extensions: []string{".go"},
			Language:   treesitter.NewLanguage(sittergo.Language()),
			DefinitionQuery: `
(function_declaration name: (identifier) @name) @function
(method_declaration name: (field_identifier) @name) @method
(type_spec name: (type_identifier) @name type: (struct_type)) @struct
(type_spec name: (type_identifier) @name type: (interface_type)) @interface
(type_spec name: (type_identifier) @name) @type
(var_spec name: (identifier) @name) @variable
(const_spec name: (identifier) @name) @constant
`,
			ImportQuery: `
(import_spec path: (interpreted_string_literal) @source)
(import_spec path: (raw_string_literal) @source)
(import_spec name: (package_identifier) @alias path: (interpreted_string_literal) @source)
(import_spec name: (package_identifier) @alias path: (raw_string_literal) @source)
`,
			ReferenceQuery: `
(call_expression function: (identifier) @call)
(call_expression function: (selector_expression) @call)
(identifier) @reference
(type_identifier) @reference
`,
		},
		{
			ID:         "javascript",
			Extensions: []string{".js", ".jsx", ".mjs", ".cjs"},
			Language:   treesitter.NewLanguage(sitterjavascript.Language()),
			DefinitionQuery: `
(function_declaration name: (identifier) @name) @function
(generator_function_declaration name: (identifier) @name) @function
(class_declaration name: (identifier) @name) @class
(method_definition name: (property_identifier) @name) @method
(variable_declarator name: (identifier) @name value: [(arrow_function) (function_expression)]) @function
(variable_declarator name: (identifier) @name) @variable
`,
			ImportQuery: `
(import_statement source: (string) @source)
(call_expression function: (identifier) @require (#eq? @require "require") arguments: (arguments (string) @source))
`,
			ReferenceQuery: `
(call_expression function: (identifier) @call)
(call_expression function: (member_expression) @call)
(identifier) @reference
`,
		},
		{
			ID:         "typescript",
			Extensions: []string{".ts", ".tsx"},
			Language:   treesitter.NewLanguage(sittertypescript.LanguageTypescript()),
			DefinitionQuery: `
(function_declaration name: (identifier) @name) @function
(generator_function_declaration name: (identifier) @name) @function
(class_declaration name: (type_identifier) @name) @class
(interface_declaration name: (type_identifier) @name) @interface
(type_alias_declaration name: (type_identifier) @name) @type
(method_definition name: (property_identifier) @name) @method
(public_field_definition name: (property_identifier) @name) @variable
(variable_declarator name: (identifier) @name value: [(arrow_function) (function_expression)]) @function
(variable_declarator name: (identifier) @name) @variable
`,
			ImportQuery: `
(import_statement source: (string) @source)
(call_expression function: (identifier) @require (#eq? @require "require") arguments: (arguments (string) @source))
`,
			ReferenceQuery: `
(call_expression function: (identifier) @call)
(call_expression function: (member_expression) @call)
(identifier) @reference
(type_identifier) @reference
`,
		},
		{
			ID:         "python",
			Extensions: []string{".py"},
			Language:   treesitter.NewLanguage(sitterpython.Language()),
			DefinitionQuery: `
(function_definition name: (identifier) @name) @function
(class_definition name: (identifier) @name) @class
`,
			ImportQuery: `
(import_statement name: (dotted_name) @source)
(import_from_statement module_name: (dotted_name) @source)
`,
			ReferenceQuery: `
(call function: (identifier) @call)
(call function: (attribute) @call)
(identifier) @reference
`,
		},
	}
}

func AdapterForPath(path string, adapters []LanguageAdapter) (LanguageAdapter, bool) {
	ext := strings.ToLower(filepath.Ext(path))
	for _, a := range adapters {
		for _, candidate := range a.Extensions {
			if ext == candidate {
				return a, true
			}
		}
	}
	return LanguageAdapter{}, false
}
