// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
	"time"

	gsmysql "github.com/go-sql-driver/mysql"
)

// mustConnect opens a fake-backed client for a test, failing on error.
func mustConnect(t *testing.T, opts Options) *Client {
	t.Helper()
	c, err := NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestNewClientPingFailure(t *testing.T) {
	defer withFakeDriver(fakeState{pingErr: errors.New("no route")})()
	if _, err := NewClient(Options{}); err == nil {
		t.Fatal("expected ping failure")
	}
}

func TestNewClientOpenFailure(t *testing.T) {
	defer withFakeDriver(fakeState{})()
	saved := sqlOpen
	sqlOpen = func(string, string) (*sql.DB, error) { return nil, errors.New("bad dsn") }
	defer func() { sqlOpen = saved }()
	if _, err := NewClient(Options{}); err == nil {
		t.Fatal("expected open failure")
	}
}

func TestNewClientQueryDefaults(t *testing.T) {
	q := QueryOptions{Cast: false, As: asHash}
	defer withFakeDriver(fakeState{})()
	c := mustConnect(t, Options{QueryDefaults: &q})
	defer c.Close()
	if c.qdef.Cast {
		t.Fatal("QueryDefaults override not applied")
	}
}

func TestQuerySelect(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT id,name FROM t": {
			cols:  []string{"id", "name"},
			types: []string{"INT", "VARCHAR"},
			vals:  [][]driver.Value{{b("1"), b("web")}, {b("2"), nil}},
		},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	res, err := c.Query("SELECT id,name FROM t")
	if err != nil {
		t.Fatal(err)
	}
	if res.Count() != 2 {
		t.Fatalf("count = %d", res.Count())
	}
	if c.AffectedRows() != 2 {
		t.Fatalf("affected = %d", c.AffectedRows())
	}
	hs := res.Hashes()
	if hs[0]["id"] != int64(1) || hs[0]["name"] != "web" {
		t.Fatalf("row0 = %#v", hs[0])
	}
	if hs[1]["name"] != nil {
		t.Fatalf("row1 name should be nil, got %#v", hs[1]["name"])
	}
}

func TestQuerySelectExplicitOptions(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT n FROM t": {
			cols:  []string{"n"},
			types: []string{"INT"},
			vals:  [][]driver.Value{{b("5")}},
		},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	// cast:false via explicit per-query options -> value stays a string.
	res, err := c.Query("SELECT n FROM t", QueryOptions{Cast: false, As: asHash})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows()[0][0] != "5" {
		t.Fatalf("cast:false value = %#v", res.Rows()[0][0])
	}
}

func TestQuerySelectError(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT bad": {queryErr: mysqlErr(1146, "42S02", "no such table")},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	if _, err := c.Query("SELECT bad"); err == nil {
		t.Fatal("expected query error")
	}
}

func TestQueryRowsErr(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT x FROM t": {
			cols:    []string{"x"},
			types:   []string{"INT"},
			vals:    [][]driver.Value{{b("1")}},
			nextErr: errors.New("read reset by peer"),
		},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	if _, err := c.Query("SELECT x FROM t"); err == nil {
		t.Fatal("expected rows.Err propagation")
	}
}

func TestQueryExec(t *testing.T) {
	defer withFakeDriver(fakeState{execs: map[string]fakeExec{
		"INSERT INTO t VALUES (1)": {affected: 1, lastID: 7},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	res, err := c.Query("INSERT INTO t VALUES (1)")
	if err != nil {
		t.Fatal(err)
	}
	if res != nil {
		t.Fatal("exec should return nil Result")
	}
	if c.AffectedRows() != 1 || c.LastID() != 7 {
		t.Fatalf("affected=%d last=%d", c.AffectedRows(), c.LastID())
	}
}

func TestQueryExecError(t *testing.T) {
	defer withFakeDriver(fakeState{execs: map[string]fakeExec{
		"DELETE FROM t": {err: mysqlErr(1451, "23000", "fk constraint")},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	if _, err := c.Query("DELETE FROM t"); err == nil {
		t.Fatal("expected exec error")
	}
}

func TestBuildResultColumnSeams(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT a FROM t": {cols: []string{"a"}, types: []string{"INT"}, vals: [][]driver.Value{{b("1")}}},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()

	t.Run("columns", func(t *testing.T) {
		saved := rowColumns
		rowColumns = func(*sql.Rows) ([]string, error) { return nil, errors.New("cols fail") }
		defer func() { rowColumns = saved }()
		if _, err := c.Query("SELECT a FROM t"); err == nil {
			t.Fatal("expected columns error")
		}
	})
	t.Run("columntypes", func(t *testing.T) {
		saved := rowColumnTypes
		rowColumnTypes = func(*sql.Rows) ([]*sql.ColumnType, error) { return nil, errors.New("ct fail") }
		defer func() { rowColumnTypes = saved }()
		if _, err := c.Query("SELECT a FROM t"); err == nil {
			t.Fatal("expected column-types error")
		}
	})
	t.Run("scan", func(t *testing.T) {
		saved := scanRow
		scanRow = func(*sql.Rows, []any) error { return errors.New("scan fail") }
		defer func() { scanRow = saved }()
		if _, err := c.Query("SELECT a FROM t"); err == nil {
			t.Fatal("expected scan error")
		}
	})
}

func TestServerInfo(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT VERSION()": {cols: []string{"VERSION()"}, types: []string{"VARCHAR"}, vals: [][]driver.Value{{b("8.0.34-log")}}},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	si, err := c.ServerInfo()
	if err != nil {
		t.Fatal(err)
	}
	if si.Version != "8.0.34-log" || si.VersionNumber != 80034 {
		t.Fatalf("server info = %#v", si)
	}
}

func TestServerInfoError(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT VERSION()": {queryErr: errors.New("gone")},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	if _, err := c.ServerInfo(); err == nil {
		t.Fatal("expected server info error")
	}
}

func TestParseVersionNumber(t *testing.T) {
	cases := map[string]int{
		"8.0.34-log": 80034,
		"5.7.42":     50742,
		"10.11":      101100,
		"9":          90000,
		"":           0,
	}
	for in, want := range cases {
		if got := parseVersionNumber(in); got != want {
			t.Fatalf("parseVersionNumber(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestPingAndEscapeAndClose(t *testing.T) {
	defer withFakeDriver(fakeState{})()
	c := mustConnect(t, Options{})
	if !c.Ping() {
		t.Fatal("ping should succeed")
	}
	// Force a ping failure through the live fake state.
	fake.pingErr = errors.New("down")
	if c.Ping() {
		t.Fatal("ping should fail")
	}
	fake.pingErr = nil
	if c.Escape("x'") != `x\'` {
		t.Fatal("escape delegation broken")
	}
	if c.Closed() {
		t.Fatal("should not be closed yet")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if !c.Closed() {
		t.Fatal("should be closed")
	}
	if err := c.Close(); err != nil { // idempotent
		t.Fatalf("second close: %v", err)
	}
}

func TestBuildDSNTCP(t *testing.T) {
	dsn := buildDSN(Options{
		Host: "db.example", Port: 3307, Username: "u", Password: "p",
		Database: "app", Encoding: "utf8mb4_general_ci",
		Flags:          []string{"MULTI_STATEMENTS", "FOUND_ROWS", "IGNORE_ME"},
		ConnectTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: time.Second,
	})
	cfg, err := gsmysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse dsn %q: %v", dsn, err)
	}
	if cfg.Net != "tcp" || cfg.Addr != "db.example:3307" {
		t.Fatalf("addr = %s %s", cfg.Net, cfg.Addr)
	}
	if cfg.User != "u" || cfg.Passwd != "p" || cfg.DBName != "app" {
		t.Fatalf("creds = %#v", cfg)
	}
	if cfg.Collation != "utf8mb4_general_ci" {
		t.Fatalf("collation = %s", cfg.Collation)
	}
	if !cfg.MultiStatements || !cfg.ClientFoundRows {
		t.Fatalf("flags not applied: %#v", cfg)
	}
}

func TestBuildDSNDefaultsAndSocket(t *testing.T) {
	tcp, _ := gsmysql.ParseDSN(buildDSN(Options{}))
	if tcp.Addr != "127.0.0.1:3306" {
		t.Fatalf("default addr = %s", tcp.Addr)
	}
	unix, _ := gsmysql.ParseDSN(buildDSN(Options{Socket: "/var/run/mysqld/mysqld.sock"}))
	if unix.Net != "unix" || unix.Addr != "/var/run/mysqld/mysqld.sock" {
		t.Fatalf("socket dsn = %s %s", unix.Net, unix.Addr)
	}
}

func TestProducesRows(t *testing.T) {
	yes := []string{
		"SELECT 1", "  select * from t", "SHOW TABLES", "DESCRIBE t", "DESC t",
		"EXPLAIN SELECT 1", "WITH x AS (SELECT 1) SELECT * FROM x", "VALUES ROW(1)",
		"CALL proc()", "TABLE t", "(SELECT 1)", "-- c\nSELECT 1", "/* c */ SELECT 1",
	}
	for _, q := range yes {
		if !producesRows(q) {
			t.Fatalf("producesRows(%q) = false, want true", q)
		}
	}
	no := []string{
		"INSERT INTO t VALUES (1)", "UPDATE t SET a=1", "DELETE FROM t", "SET @x=1",
		"CREATE TABLE t (a INT)", "-- only a comment", "/* unclosed", "",
	}
	for _, q := range no {
		if producesRows(q) {
			t.Fatalf("producesRows(%q) = true, want false", q)
		}
	}
}
