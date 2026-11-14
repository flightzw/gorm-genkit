package main

import "testing"

func TestTimeColumn(t *testing.T) {
	for _, tc := range []struct {
		db   DBType
		typ  string
		want bool
	}{
		{dbMySQL, "DATETIME(6)", true},
		{dbMySQL, "timestamp", true},
		{dbMySQL, "DATE", true},
		{dbMySQL, "TIME", false},
		{dbMySQL, "bigint", false},
		{dbMySQL, "varchar(255)", false},
		{dbPostgres, "timestamptz", true},
		{dbPostgres, "timestamp with time zone", true},
		{dbPostgres, "timestamp(6) without time zone", true},
		{dbPostgres, "timetz", false},
		{dbPostgres, "timestamp[]", false},
		{dbPostgres, "timestamp(6)[]", false},
		{dbSQLServer, "datetime2(7)", true},
		{dbSQLServer, "smalldatetime", true},
		{dbSQLServer, "datetimeoffset", true},
		{dbSQLServer, "timestamp", false},
		{dbSQLServer, "rowversion", false},
		{dbSQLite, "datetime", true},
		{dbSQLite, "integer", false},
		{dbSQLite, "text", false},
		{dbClickHouse, "Nullable(DateTime64(3, 'UTC'))", true},
		{dbClickHouse, "LowCardinality(Nullable(Date32))", true},
		{dbClickHouse, "UInt64", false},
		{dbClickHouse, "Array(DateTime)", false},
		{dbClickHouse, "Nullable(UInt64)", false},
		{dbMySQL, "unknown", false},
	} {
		t.Run(string(tc.db)+"/"+tc.typ, func(t *testing.T) {
			if got := isTimeColumn(tc.db, tc.typ); got != tc.want {
				t.Fatalf("isTimeColumn(%q, %q) = %v, want %v", tc.db, tc.typ, got, tc.want)
			}
		})
	}
}
