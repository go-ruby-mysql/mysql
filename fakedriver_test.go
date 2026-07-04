// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync"

	gsmysql "github.com/go-sql-driver/mysql"
)

// This file registers an in-process database/sql driver ("mysqlfake") that
// returns scripted result sets and exec outcomes. It lets the deterministic
// suite drive the whole NewClient -> Query -> buildResult -> cast pipeline —
// including NULLs, every MySQL type, error taxonomy, and the option matrix —
// with no live MySQL server, holding coverage at 100%. Tests point driverName at
// it via withFakeDriver.

func init() { sql.Register("mysqlfake", fakeDriver{}) }

// fakeState is the scripted behaviour a test installs before running.
type fakeState struct {
	openErr    error                    // Open fails with this
	pingErr    error                    // Ping fails with this
	queries    map[string]fakeResultSet // SELECT-like results by exact SQL
	execs      map[string]fakeExec      // exec outcomes by exact SQL
	defaultErr error                    // returned for an unmatched statement
}

// fakeResultSet is one scripted result set.
type fakeResultSet struct {
	cols     []string
	types    []string
	vals     [][]driver.Value
	queryErr error // Query returns this instead of rows
	nextErr  error // Next returns this after emitting every row (drives rows.Err)
}

// fakeExec is one scripted exec outcome.
type fakeExec struct {
	affected int64
	lastID   int64
	err      error
}

var (
	fakeMu    sync.Mutex
	fake      fakeState
	fakeSaved string
)

// withFakeDriver installs state and points the client at the fake driver,
// returning a restore func the caller defers.
func withFakeDriver(state fakeState) func() {
	fakeMu.Lock()
	fake = state
	fakeSaved = driverName
	driverName = "mysqlfake"
	return func() {
		driverName = fakeSaved
		fake = fakeState{}
		fakeMu.Unlock()
	}
}

type fakeDriver struct{}

func (fakeDriver) Open(string) (driver.Conn, error) {
	if fake.openErr != nil {
		return nil, fake.openErr
	}
	return &fakeConn{}, nil
}

type fakeConn struct{}

func (c *fakeConn) Prepare(query string) (driver.Stmt, error) {
	if query == "__prepfail__" {
		return nil, errors.New("fake: prepare failed")
	}
	return &fakeStmt{query: query}, nil
}
func (c *fakeConn) Close() error              { return nil }
func (c *fakeConn) Begin() (driver.Tx, error) { return nil, errors.New("fake: no transactions") }

func (c *fakeConn) Ping(context.Context) error { return fake.pingErr }

func (c *fakeConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	return runQuery(query)
}

func (c *fakeConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	return runExec(query)
}

type fakeStmt struct{ query string }

func (s *fakeStmt) Close() error  { return nil }
func (s *fakeStmt) NumInput() int { return -1 }
func (s *fakeStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return runExec(s.query)
}
func (s *fakeStmt) Query(_ []driver.Value) (driver.Rows, error) {
	return runQuery(s.query)
}

// runExec resolves a scripted exec outcome for query.
func runExec(query string) (driver.Result, error) {
	if e, ok := fake.execs[query]; ok {
		if e.err != nil {
			return nil, e.err
		}
		return fakeDriverResult{affected: e.affected, lastID: e.lastID}, nil
	}
	if fake.defaultErr != nil {
		return nil, fake.defaultErr
	}
	return fakeDriverResult{}, nil
}

// runQuery resolves a scripted result set for query.
func runQuery(query string) (driver.Rows, error) {
	rs, ok := fake.queries[query]
	if !ok {
		if fake.defaultErr != nil {
			return nil, fake.defaultErr
		}
		return &fakeRows{cols: []string{"x"}, types: []string{"INT"}}, nil
	}
	if rs.queryErr != nil {
		return nil, rs.queryErr
	}
	return &fakeRows{cols: rs.cols, types: rs.types, vals: rs.vals, nextErr: rs.nextErr}, nil
}

type fakeDriverResult struct {
	affected int64
	lastID   int64
}

func (r fakeDriverResult) LastInsertId() (int64, error) { return r.lastID, nil }
func (r fakeDriverResult) RowsAffected() (int64, error) { return r.affected, nil }

type fakeRows struct {
	cols    []string
	types   []string
	vals    [][]driver.Value
	pos     int
	nextErr error
}

func (r *fakeRows) Columns() []string { return r.cols }
func (r *fakeRows) Close() error      { return nil }
func (r *fakeRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.vals) {
		if r.nextErr != nil {
			return r.nextErr
		}
		return io.EOF
	}
	copy(dest, r.vals[r.pos])
	r.pos++
	return nil
}

// ColumnTypeDatabaseTypeName reports each column's MySQL type name, the hook
// database/sql exposes as ColumnType.DatabaseTypeName so buildResult can pick
// the cast.
func (r *fakeRows) ColumnTypeDatabaseTypeName(i int) string {
	if i < len(r.types) {
		return r.types[i]
	}
	return "TEXT"
}

// b is a shorthand for a []byte column value in scripted rows.
func b(s string) driver.Value { return []byte(s) }

// mysqlErr builds a go-sql-driver server error for the error-taxonomy tests.
func mysqlErr(number uint16, sqlState, msg string) error {
	me := &gsmysql.MySQLError{Number: number, Message: msg}
	copy(me.SQLState[:], sqlState)
	return me
}
