// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

// Package mysql is a pure-Go (CGO=0) reimplementation of the Ruby mysql2 gem's
// Mysql2 API. Upstream, mysql2 is a C extension linking libmysqlclient; this
// package instead binds github.com/go-sql-driver/mysql — a pure-Go, CGO-free
// implementation of the MySQL client/server protocol — through database/sql, so
// the whole stack links statically with CGO_ENABLED=0 on every 64-bit target the
// go-* ecosystem supports (amd64, arm64, riscv64, loong64, ppc64le, s390x).
//
// The API mirrors Mysql2::Client, Mysql2::Result, Mysql2::Statement and
// Mysql2::Error:
//
//	c, _ := mysql.NewClient(mysql.Options{Host: "127.0.0.1", Username: "root", Database: "test"})
//	defer c.Close()
//	res, _ := c.Query("SELECT id, name FROM hosts")
//	for _, h := range res.Hashes() {
//		fmt.Println(h["id"], h["name"]) // int64, string
//	}
//
// Result values crossing the boundary use mysql2's MySQL->Ruby cast table:
//
//	MySQL column type        Go (this package)     Ruby (rbgo binding)
//	TINYINT..BIGINT, YEAR    int64 / uint64        Integer
//	FLOAT, DOUBLE            float64               Float
//	DECIMAL / NEWDECIMAL     Decimal (a string)    BigDecimal
//	DATE                     Date                  Date
//	DATETIME, TIMESTAMP      time.Time (naive UTC) Time
//	TIME                     string                String
//	CHAR/VARCHAR/TEXT/…      string                String (UTF-8)
//	BLOB/BINARY/BIT/GEOMETRY []byte                String (ASCII-8BIT)
//	NULL                     nil                   nil
//
// The cast is governed by the same options mysql2 exposes — cast, cast_booleans,
// symbolize_keys, and as: :array vs :hash — carried on QueryOptions. Errors map
// to Mysql2::Error via the Error type, exposing #error_number and #sql_state.
//
// The connection is a host seam: the deterministic test suite drives the whole
// query -> result -> cast pipeline through an in-process fake database/sql
// driver, so it reaches 100% coverage without a live server, while an optional
// env-gated oracle round-trips against a real MySQL when MYSQL2_TEST_DSN is set.
package mysql
