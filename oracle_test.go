// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import (
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	gsmysql "github.com/go-sql-driver/mysql"
)

// oracleOptions derives connection Options from the MYSQL2_TEST_DSN environment
// variable (a go-sql-driver DSN, e.g. "root:root@tcp(127.0.0.1:3306)/test").
// When it is unset the live oracle skips, so the deterministic, server-free
// suite alone keeps coverage at 100% and CI (which has no MySQL) stays green.
// Set MYSQL2_TEST_DSN to round-trip against a real server.
func oracleOptions(t *testing.T) Options {
	t.Helper()
	dsn := os.Getenv("MYSQL2_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL2_TEST_DSN unset; skipping live-mysql oracle")
	}
	cfg, err := gsmysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("bad MYSQL2_TEST_DSN: %v", err)
	}
	opts := Options{Username: cfg.User, Password: cfg.Passwd, Database: cfg.DBName}
	if cfg.Net == "unix" {
		opts.Socket = cfg.Addr
	} else if host, port, ok := strings.Cut(cfg.Addr, ":"); ok {
		opts.Host = host
		opts.Port, _ = strconv.Atoi(port)
	}
	return opts
}

// TestLiveOracle round-trips a representative typed row through a real MySQL
// server and checks this binding's casts. It is a no-op unless MYSQL2_TEST_DSN
// is set (so it never runs in the arch/qemu/Windows CI lanes).
func TestLiveOracle(t *testing.T) {
	c := mustConnect(t, oracleOptions(t))
	defer c.Close()

	si, err := c.ServerInfo()
	if err != nil {
		t.Fatal(err)
	}
	if si.VersionNumber == 0 {
		t.Fatalf("server_info parsed no version from %q", si.Version)
	}

	if _, err := c.Query("DROP TABLE IF EXISTS go_ruby_mysql_probe"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Query(`CREATE TABLE go_ruby_mysql_probe (
		id INT PRIMARY KEY AUTO_INCREMENT,
		n BIGINT, f DOUBLE, d DECIMAL(10,2),
		s VARCHAR(32), dt DATETIME, day DATE, blob_col BLOB, maybe INT NULL)`); err != nil {
		t.Fatal(err)
	}
	defer c.Query("DROP TABLE go_ruby_mysql_probe")

	st, err := c.Prepare(`INSERT INTO go_ruby_mysql_probe (n,f,d,s,dt,day,blob_col,maybe)
		VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Execute(int64(42), 3.5, "12.34", "hello",
		"2026-07-04 12:13:14", "2026-07-04", []byte{0x00, 0x01}, nil); err != nil {
		t.Fatal(err)
	}
	st.Close()
	if c.LastID() == 0 {
		t.Fatal("expected a non-zero last_id")
	}

	res, err := c.Query("SELECT n,f,d,s,dt,day,blob_col,maybe FROM go_ruby_mysql_probe")
	if err != nil {
		t.Fatal(err)
	}
	h := res.Hashes()[0]
	if h["n"] != int64(42) {
		t.Fatalf("n = %#v", h["n"])
	}
	if h["f"] != 3.5 {
		t.Fatalf("f = %#v", h["f"])
	}
	if h["d"] != Decimal("12.34") {
		t.Fatalf("d = %#v", h["d"])
	}
	if h["s"] != "hello" {
		t.Fatalf("s = %#v", h["s"])
	}
	if dt, ok := h["dt"].(time.Time); !ok || dt.Year() != 2026 {
		t.Fatalf("dt = %#v", h["dt"])
	}
	if day, ok := h["day"].(Date); !ok || day.String() != "2026-07-04" {
		t.Fatalf("day = %#v", h["day"])
	}
	if h["maybe"] != nil {
		t.Fatalf("maybe = %#v", h["maybe"])
	}
}
