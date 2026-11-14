package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"gorm.io/gorm"
)

// validateTypes checks syntax without resolving packages in the target project.
func (c *CmdParams) validateTypes() error {
	for dbType, goType := range c.DataTypeMap {
		if strings.TrimSpace(dbType) == "" || dbType != strings.TrimSpace(dbType) {
			return fmt.Errorf("dataTypeMap: database type %q must be nonempty and have no surrounding whitespace", dbType)
		}
		// Parsing a declaration rejects expressions such as calls or literals while
		// allowing pointers, slices, maps, qualified names, and generic types.
		file, err := parser.ParseFile(token.NewFileSet(), "type.go", "package model\ntype mappedType "+goType, parser.AllErrors|parser.ParseComments)
		if err != nil {
			return fmt.Errorf("dataTypeMap[%q]: invalid Go type %q: %w", dbType, goType, err)
		}
		if len(file.Decls) != 1 || len(file.Decls[0].(*ast.GenDecl).Specs) != 1 || len(file.Comments) != 0 {
			return fmt.Errorf("dataTypeMap[%q]: expected a single Go type without comments", dbType)
		}
		if file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Assign.IsValid() {
			return fmt.Errorf("dataTypeMap[%q]: expected a Go type expression, not an alias declaration", dbType)
		}
	}
	for _, path := range c.ImportPkgPaths {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("importPkgPaths: package path cannot be empty")
		}
		file, err := parser.ParseFile(token.NewFileSet(), "import.go", "package model\nimport "+quoteImport(path), parser.AllErrors|parser.ParseComments)
		if err != nil {
			return fmt.Errorf("importPkgPaths: invalid import %q: %w", path, err)
		}
		if len(file.Decls) != 1 || len(file.Imports) != 1 || len(file.Comments) != 0 {
			return fmt.Errorf("importPkgPaths: expected a single package import in %q", path)
		}
	}
	return nil
}

// Match gen.Config.WithImportPkgPath, which accepts paths or quoted imports
// with aliases, e.g. money "example.com/project/money".
func quoteImport(path string) string {
	path = strings.TrimSpace(path)
	if path[0] != '"' && path[len(path)-1] != '"' {
		return `"` + path + `"`
	}
	return path
}

func (c *CmdParams) dataTypeMapping() map[string]func(gorm.ColumnType) string {
	mapping := make(map[string]func(gorm.ColumnType) string, len(c.DataTypeMap))
	for dbType, goType := range c.DataTypeMap {
		mapping[dbType] = func(gorm.ColumnType) string { return goType }
	}
	return mapping
}
