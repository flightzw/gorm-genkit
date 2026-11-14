package main

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func unsetTestEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		// Register restoration before removing a potentially inherited value.
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

func TestConnectionEnv(t *testing.T) {
	for _, tc := range []struct {
		name    string
		file    string
		content string
		config  string
		args    []string
		env     map[string]string
		db      string
		dsn     string
		wantErr string
	}{
		{name: "default dotenv", file: ".env", content: "GENKIT_DB=sqlite\nGENKIT_DSN='password$literal'\n", db: "sqlite", dsn: "password$literal"},
		{name: "system environment wins", file: ".env", content: "GENKIT_DB=mysql\nGENKIT_DSN=file\n", env: map[string]string{"GENKIT_DB": "postgres", "GENKIT_DSN": "system"}, db: "postgres", dsn: "system"},
		{name: "explicit file", file: "custom.env", content: "GENKIT_DB=sqlite\nGENKIT_DSN=custom.db\n", args: []string{"-envFile", "custom.env"}, db: "sqlite", dsn: "custom.db"},
		{name: "missing default is optional", db: "mysql"},
		{name: "missing explicit file", args: []string{"-envFile", "missing.env"}, wantErr: "load env file"},
		{name: "malformed file", file: ".env", content: "bad!key=secret-password", wantErr: "cannot read or parse"},
		{name: "disabled loading", file: ".env", content: "GENKIT_DB=sqlite\nGENKIT_DSN=file\n", args: []string{"-envFile", ""}, db: "mysql"},
		{name: "CLI overrides environment", file: ".env", content: "GENKIT_DB=mysql\nGENKIT_DSN=file\n", args: []string{"-db", "sqlite", "-dsn", "explicit.db"}, db: "sqlite", dsn: "explicit.db"},
		{name: "YAML overrides environment and CLI", file: ".env", content: "GENKIT_DB=mysql\nGENKIT_DSN=file\n", config: "database: {db: sqlite, dsn: config.db}", args: []string{"-db", "postgres", "-dsn", "ignored"}, db: "sqlite", dsn: "config.db"},
		{name: "YAML environment fallback", file: ".env", content: "GENKIT_DB=sqlite\nGENKIT_DSN=fallback.db\n", config: "database: {onlyModel: true}", db: "sqlite", dsn: "fallback.db"},
		{name: "YAML placeholders after decoding", file: ".env", content: "GENKIT_TEST_DB=postgres\nGENKIT_TEST_DSN='password$literal: # quote\"'\n", config: "database: {db: '${GENKIT_TEST_DB}', dsn: '${GENKIT_TEST_DSN}'}", db: "postgres", dsn: "password$literal: # quote\""},
		{name: "CLI placeholders", env: map[string]string{"GENKIT_TEST_DB": "sqlite", "GENKIT_TEST_DSN": "test.db"}, args: []string{"-db", "${GENKIT_TEST_DB}", "-dsn", "${GENKIT_TEST_DSN}"}, db: "sqlite", dsn: "test.db"},
		{name: "missing placeholder", args: []string{"-dsn", "secret:${GENKIT_TEST_DSN}"}, wantErr: "undefined environment variable"},
		{name: "literal dollars", args: []string{"-dsn", "password$literal"}, db: "mysql", dsn: "password$literal"},
		{name: "help skips dotenv", file: ".env", content: "bad!key=secret-password", args: []string{"-h"}, wantErr: "flag: help requested"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			unsetTestEnv(t, "GENKIT_DB", "GENKIT_DSN", "GENKIT_TEST_DB", "GENKIT_TEST_DSN")
			for name, value := range tc.env {
				t.Setenv(name, value)
			}
			if tc.file != "" {
				writeFile(t, tc.file, tc.content)
			}
			args := append([]string{}, tc.args...)
			if tc.config != "" {
				path := filepath.Join("config", "gen.yml")
				writeFile(t, path, tc.config)
				args = append(args, "-c", path)
			}
			params, err := parseArgs(flag.NewFlagSet("test", flag.ContinueOnError), args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected %q, got %v", tc.wantErr, err)
				}
				if strings.Contains(err.Error(), "secret") {
					t.Fatalf("error leaked credentials: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			params.revise()
			if params.DB != tc.db || params.DSN != tc.dsn {
				t.Fatalf("unexpected connection configuration for %s", tc.name)
			}
		})
	}
}
