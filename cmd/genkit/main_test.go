package main

import (
	"bytes"
	"errors"
	"flag"
	"go/ast"
	"go/build"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigAndFlags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gen.yml")
	writeFile(t, path, "database:\n  db: sqlite\n  dsn: config.db\n  tables: [' users ', '']\n  dataTypeMap:\n    text: custom.Label\n  importPkgPaths: [example.com/custom]\n")
	params, err := parseArgs(flag.NewFlagSet("test", flag.ContinueOnError), []string{"-c", path, "-db", "mysql", "-dsn", "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	params.revise()
	if params.DB != "sqlite" || params.DSN != "config.db" || params.OutPath != defaultQueryPath || len(params.Tables) != 1 || params.Tables[0] != "users" || params.DataTypeMap["text"] != "custom.Label" {
		t.Fatalf("unexpected config: %+v", params)
	}
	params, err = parseArgs(flag.NewFlagSet("test", flag.ContinueOnError), []string{"-tables", " users, orders, ,", "-onlyModel"})
	if err != nil {
		t.Fatal(err)
	}
	params.revise()
	if params.DB != "mysql" || !params.OnlyModel || strings.Join(params.Tables, ",") != "users,orders" {
		t.Fatalf("unexpected flags: %+v", params)
	}
	for _, name := range []string{"dataTypeMap", "importPkgPaths"} {
		fs := flag.NewFlagSet("test", flag.ContinueOnError)
		fs.SetOutput(&bytes.Buffer{})
		if _, err := parseArgs(fs, []string{"-" + name, "text:string"}); err == nil {
			t.Fatalf("%s must only be configurable through a file", name)
		}
	}
	if err := run([]string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("help: %v", err)
	}
}

func TestInvalidConfig(t *testing.T) {
	for name, content := range map[string]string{
		"missing database":  "version: '1'",
		"invalid yaml":      "database: [",
		"unknown field":     "database: {dataTypeMaps: {text: string}}",
		"empty db type":     "database: {dataTypeMap: {'': string}}",
		"empty go type":     "database: {dataTypeMap: {text: ''}}",
		"invalid go type":   "database: {dataTypeMap: {text: 'foo()'}}",
		"alias declaration": "database: {dataTypeMap: {text: '= string'}}",
		"extra declaration": "database:\n  dataTypeMap:\n    text: |\n      string\n      var x int\n",
		"empty import":      "database: {importPkgPaths: ['']}",
		"invalid import":    "database: {importPkgPaths: ['bad\"path']}",
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "gen.yml")
			writeFile(t, path, content)
			if _, err := loadConfig(path); err == nil {
				t.Fatal("expected config error")
			}
		})
	}
	if _, err := loadConfig(filepath.Join(t.TempDir(), "missing.yml")); err == nil {
		t.Fatal("expected missing file error")
	}
	if err := run(nil); err == nil || !strings.Contains(err.Error(), "dsn cannot be empty") {
		t.Fatalf("empty DSN: %v", err)
	}
	for _, db := range []DBType{dbMySQL, dbPostgres, dbSQLite, dbSQLServer, dbClickHouse} {
		if _, err := dialectorFor(db, "test"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := dialectorFor("unknown", "test"); err == nil {
		t.Fatal("expected unsupported driver error")
	}
}

// Exercise the entire file-config -> schema -> generated model/query flow, then
// compile the generated code in a separate module, including a custom package.
func TestGenerateWithTypeMapping(t *testing.T) {
	if !build.Default.CgoEnabled {
		t.Skip("SQLite integration requires CGO_ENABLED=1 and a C compiler")
	}
	for _, onlyModel := range []bool{true, false} {
		name := "query"
		if onlyModel {
			name = "model"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			dsn := filepath.Join(dir, "schema.db")
			db, err := gorm.Open(sqlite.Open(dsn))
			if err != nil {
				t.Fatal(err)
			}
			for _, statement := range []string{
				"CREATE TABLE users (id integer PRIMARY KEY NOT NULL, name text NOT NULL, age integer NOT NULL, active numeric NOT NULL, optional text, note blob)",
				"CREATE TABLE ignored (id integer PRIMARY KEY)",
			} {
				if err := db.Exec(statement).Error; err != nil {
					t.Fatal(err)
				}
			}
			sqlDB, _ := db.DB()
			if err := sqlDB.Close(); err != nil {
				t.Fatal(err)
			}
			// Reuse pinned dependencies so compilation requires no network access.
			for _, file := range []string{"go.mod", "go.sum"} {
				content, err := os.ReadFile(filepath.Join("..", "..", file))
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(dir, file), strings.Replace(string(content), "module github.com/flightzw/gorm-genkit", "module example.com/generated", 1))
			}
			writeFile(t, filepath.Join(dir, "custom", "label.go"), "package custom\ntype Label string\n")
			params := &CmdParams{
				DB: "sqlite", DSN: dsn, OnlyModel: onlyModel,
				OutPath: filepath.Join(dir, "query"), ModelPkgName: filepath.Join(dir, "model"),
				WithDefaultQuery: true, WithQueryInterface: true,
				FieldNullable:  true,
				DataTypeMap:    map[string]string{"text": "ct.Label", "integer": "uint32"},
				ImportPkgPaths: []string{`ct "example.com/generated/custom"`},
			}
			// Cover all-table discovery in model mode and selection in query mode.
			if !onlyModel {
				params.Tables = []string{" users ", ""}
			}
			content, err := yaml.Marshal(YamlConfig{Version: "1", Database: params})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "gen.yml")
			writeFile(t, path, string(content))
			if err := run([]string{"-c", path}); err != nil {
				t.Fatal(err)
			}
			file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, "model", "users.gen.go"), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			fields := map[string]string{}
			ast.Inspect(file, func(node ast.Node) bool {
				if field, ok := node.(*ast.Field); ok && len(field.Names) == 1 {
					var value bytes.Buffer
					if err := format.Node(&value, token.NewFileSet(), field.Type); err != nil {
						t.Fatal(err)
					}
					fields[field.Names[0].Name] = value.String()
				}
				return true
			})
			if fields["ID"] != "uint32" || fields["Age"] != "uint32" || fields["Name"] != "ct.Label" || fields["Active"] != "int32" || fields["Optional"] != "*ct.Label" || fields["Note"] != "*[]byte" {
				t.Fatalf("unexpected generated fields: %v", fields)
			}
			_, queryErr := os.Stat(filepath.Join(dir, "query", "gen.go"))
			_, ignoredErr := os.Stat(filepath.Join(dir, "model", "ignored.gen.go"))
			if onlyModel && !os.IsNotExist(queryErr) || !onlyModel && queryErr != nil {
				t.Fatalf("query file: %v", queryErr)
			}
			if onlyModel && ignoredErr != nil || !onlyModel && !os.IsNotExist(ignoredErr) {
				t.Fatalf("table selection: %v", ignoredErr)
			}
			cmd := exec.Command("go", "test", "-mod=readonly", "./...")
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "GOPROXY=off", "GOWORK=off")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("compile generated code: %v\n%s", err, output)
			}
		})
	}
}
