// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import (
	"database/sql"
	"time"
)

// Statement is a prepared statement, mirroring Mysql2::Statement. Client#prepare
// compiles the SQL; Execute binds the given parameters and runs it, returning a
// *Result for a row-producing statement or a nil Result otherwise (after which
// the owning client's AffectedRows / LastID reflect the change).
type Statement struct {
	client *Client
	stmt   *sql.Stmt
	query  string
	closed bool
}

// Prepare compiles query into a *Statement (Mysql2::Client#prepare). Bind
// parameters positionally on Execute using MySQL's `?` placeholders.
func (c *Client) Prepare(query string) (*Statement, error) {
	stmt, err := c.db.Prepare(query)
	if err != nil {
		return nil, wrapError(err)
	}
	return &Statement{client: c, stmt: stmt, query: query}, nil
}

// Execute binds args to the statement's placeholders and runs it
// (Mysql2::Statement#execute). For a row-producing statement it returns a
// *Result using the client's default query options; for a changing statement it
// returns a nil Result and updates the client's AffectedRows / LastID. Use
// ExecuteOpts to override the query options.
func (s *Statement) Execute(args ...any) (*Result, error) {
	return s.ExecuteOpts(s.client.qdef, args...)
}

// ExecuteOpts is Execute with explicit per-execution query options.
func (s *Statement) ExecuteOpts(qo QueryOptions, args ...any) (*Result, error) {
	binds := normalizeBinds(args)
	if !producesRows(s.query) {
		res, err := s.stmt.Exec(binds...)
		if err != nil {
			return nil, wrapError(err)
		}
		s.client.recordExec(res)
		return nil, nil
	}
	rows, err := s.stmt.Query(binds...)
	if err != nil {
		return nil, wrapError(err)
	}
	defer rows.Close()
	result, err := buildResult(rows, qo)
	if err != nil {
		return nil, err
	}
	s.client.affectedRows = int64(result.Count())
	return result, nil
}

// SQL returns the statement's source text (Mysql2::Statement#sql-ish).
func (s *Statement) SQL() string { return s.query }

// Closed reports whether the statement has been closed.
func (s *Statement) Closed() bool { return s.closed }

// Close releases the prepared statement (Mysql2::Statement#close). Closing an
// already-closed statement is a no-op.
func (s *Statement) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return wrapError(s.stmt.Close())
}

// normalizeBinds maps Ruby-side bind values to the argument types the driver
// accepts. Integers widen to int64, float32 to float64, and time.Time / string
// / []byte / bool / nil pass through. A pre-built sql.NamedArg is forwarded as
// is. Any other type is passed through for the driver to reject.
func normalizeBinds(args []any) []any {
	if len(args) == 0 {
		return nil
	}
	out := make([]any, len(args))
	for i, a := range args {
		out[i] = normalizeBind(a)
	}
	return out
}

// normalizeBind maps one bind value (see normalizeBinds).
func normalizeBind(v any) any {
	switch n := v.(type) {
	case nil:
		return nil
	case int:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint64:
		return int64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	case bool:
		return n
	case string:
		return n
	case []byte:
		return n
	case time.Time:
		return n
	case Decimal:
		return string(n)
	case sql.NamedArg:
		return n
	default:
		return n
	}
}
