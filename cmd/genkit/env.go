package main

import (
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/joho/godotenv"
)

func loadEnvFile(path string, explicit bool) error {
	if path == "" {
		return nil
	}
	if err := godotenv.Load(path); err != nil {
		if !explicit && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		// Parser errors may contain source lines, including credentials.
		return fmt.Errorf("load env file %q: cannot read or parse dotenv file", path)
	}
	return nil
}

var connectionEnvVar = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func (c *CmdParams) resolveConnectionEnv() error {
	for _, field := range []struct {
		name     string
		value    *string
		fallback string
	}{
		{"db", &c.DB, "GENKIT_DB"},
		{"dsn", &c.DSN, "GENKIT_DSN"},
	} {
		if *field.value == "" {
			// Environment values are already resolved by godotenv; do not
			// expand them again, as a password may contain a literal dollar.
			*field.value = os.Getenv(field.fallback)
			continue
		}
		var missing string
		value := connectionEnvVar.ReplaceAllStringFunc(*field.value, func(match string) string {
			name := match[2 : len(match)-1]
			value, ok := os.LookupEnv(name)
			if !ok {
				missing = name
			}
			return value
		})
		if missing != "" {
			return fmt.Errorf("%s references undefined environment variable %q", field.name, missing)
		}
		*field.value = value
	}
	return nil
}
