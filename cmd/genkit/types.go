package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"gorm.io/gen"
	"gorm.io/gorm"
)

// SoftDeleteConfig overrides a database column's final generated Go type.
type SoftDeleteConfig struct {
	Column string `yaml:"column"`
	Type   string `yaml:"type"`
}

func (c *SoftDeleteConfig) column() string {
	if c.Column == "" {
		return "deleted_at"
	}
	return c.Column
}

// A custom type can implement integer/flag soft deletion, so only the built-in
// time-based type requires this check. Unresolved custom types stay explicit.
func isGormDeletedAt(goType string) bool {
	expr, err := parser.ParseExpr(goType)
	if err != nil {
		return false
	}
	for {
		switch node := expr.(type) {
		case *ast.ParenExpr:
			expr = node.X
		case *ast.StarExpr:
			expr = node.X
		default:
			selector, ok := expr.(*ast.SelectorExpr)
			if !ok {
				return false
			}
			pkg, ok := selector.X.(*ast.Ident)
			return ok && pkg.Name == "gorm" && selector.Sel.Name == "DeletedAt"
		}
	}
}

// Use the native database type, before any dataTypeMap conversion. SQL Server's
// timestamp is a binary rowversion, and SQL TIME lacks the date needed here.
func isTimeColumn(db DBType, databaseType string) bool {
	name := strings.ToLower(strings.TrimSpace(databaseType))
	if strings.Contains(name, "[") {
		return false
	}
	if db == dbClickHouse {
		for {
			if (strings.HasPrefix(name, "nullable(") || strings.HasPrefix(name, "lowcardinality(")) && strings.HasSuffix(name, ")") {
				name = strings.TrimSpace(name[strings.IndexByte(name, '(')+1 : len(name)-1])
			} else {
				break
			}
		}
	}
	if i := strings.IndexByte(name, '('); i >= 0 {
		name = strings.TrimSpace(name[:i])
	}
	switch db {
	case dbMySQL, dbSQLite:
		return name == "date" || name == "datetime" || name == "timestamp"
	case dbPostgres:
		return name == "date" || name == "timestamp" || name == "timestamptz" || name == "timestamp with time zone" || name == "timestamp without time zone"
	case dbSQLServer:
		return name == "date" || name == "datetime" || name == "datetime2" || name == "smalldatetime" || name == "datetimeoffset"
	case dbClickHouse:
		return name == "date" || name == "date32" || name == "datetime" || name == "datetime64"
	default:
		return false
	}
}

func (c *CmdParams) softDeleteField(field gen.Field) gen.Field {
	if c.SoftDelete == nil || field.ColumnName != c.SoftDelete.column() {
		return field
	}
	if isGormDeletedAt(c.SoftDelete.Type) && (field.Column == nil || !isTimeColumn(DBType(c.DB), field.Column.DatabaseTypeName())) {
		if field.Column != nil && field.Type == "gorm.DeletedAt" {
			// Gen itself promotes deleted_at + time.Time to gorm.DeletedAt
			// before field options run. Recompute only the fallback type with
			// that name-based special case disabled, retaining map and pointer rules.
			column := *field.Column
			column.ColumnType = regularColumn{column.ColumnType}
			field.Type = column.ToField(c.FieldNullable, c.FieldCoverable, c.FieldSignable, c.FieldWithDefaultTag).Type
		}
		return field
	}
	field.Type = c.SoftDelete.Type
	return field
}

type columnMetadata = gorm.ColumnType

type regularColumn struct{ columnMetadata }

func (regularColumn) Name() string { return "__genkit_regular_column" }

func validateGoType(name, goType string) error {
	// Parsing a declaration rejects calls and literals while allowing Go types.
	file, err := parser.ParseFile(token.NewFileSet(), "type.go", "package model\ntype mappedType "+goType, parser.AllErrors|parser.ParseComments)
	if err != nil {
		return fmt.Errorf("%s: invalid Go type %q: %w", name, goType, err)
	}
	if len(file.Decls) != 1 || len(file.Decls[0].(*ast.GenDecl).Specs) != 1 || len(file.Comments) != 0 {
		return fmt.Errorf("%s: expected a single Go type without comments", name)
	}
	if file.Decls[0].(*ast.GenDecl).Specs[0].(*ast.TypeSpec).Assign.IsValid() {
		return fmt.Errorf("%s: expected a Go type expression, not an alias declaration", name)
	}
	return nil
}

// validateTypes checks syntax without resolving packages in the target project.
func (c *CmdParams) validateTypes() error {
	for dbType, goType := range c.DataTypeMap {
		if strings.TrimSpace(dbType) == "" || dbType != strings.TrimSpace(dbType) {
			return fmt.Errorf("dataTypeMap: database type %q must be nonempty and have no surrounding whitespace", dbType)
		}
		if err := validateGoType(fmt.Sprintf("dataTypeMap[%q]", dbType), goType); err != nil {
			return err
		}
	}
	if c.SoftDelete != nil {
		if column := c.SoftDelete.column(); strings.TrimSpace(column) == "" || strings.TrimSpace(column) != column {
			return fmt.Errorf("softDelete.column: column must be nonempty and have no surrounding whitespace")
		}
		if err := validateGoType("softDelete.type", c.SoftDelete.Type); err != nil {
			return err
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
