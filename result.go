// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

// Result is a buffered result set, mirroring Mysql2::Result. mysql2 returns one
// from Client#query / Statement#execute for a statement that yields rows. It is
// Enumerable over its rows; each row is a HashRow (keyed by column name, the
// default) or a Row (a positional array, the `as: :array` shape), selected by
// the QueryOptions the query ran with. Fields and Count mirror #fields and
// #count / #size.
type Result struct {
	fields []string
	rows   []Row
	opts   QueryOptions
}

// Fields returns the column names in column order (Mysql2::Result#fields).
func (r *Result) Fields() []string { return r.fields }

// Count returns the number of rows (Mysql2::Result#count / #size).
func (r *Result) Count() int { return len(r.rows) }

// SymbolizeKeys reports whether the query requested symbol keys
// (Mysql2::Result built with symbolize_keys: true). Go has no Symbol type, so
// the flag is carried for the rbgo binding to intern the string keys into Ruby
// Symbols; the maps here are always string-keyed.
func (r *Result) SymbolizeKeys() bool { return r.opts.SymbolizeKeys }

// As returns the row shape the result was built for ("hash" or "array"),
// mirroring the query's `as:` option.
func (r *Result) As() string {
	if r.opts.As == asArray {
		return asArray
	}
	return asHash
}

// Rows returns every row as a positional array in column order — the
// `as: :array` shape (Mysql2::Result#each with as: :array).
func (r *Result) Rows() []Row { return r.rows }

// Hashes returns every row as a name-keyed map — the default `as: :hash` shape.
// A duplicate column name keeps the last column's value, matching how a Ruby
// Hash overwrites a repeated key.
func (r *Result) Hashes() []HashRow {
	out := make([]HashRow, len(r.rows))
	for i, row := range r.rows {
		out[i] = r.hashRow(row)
	}
	return out
}

// hashRow builds one name-keyed row from a positional row.
func (r *Result) hashRow(row Row) HashRow {
	h := make(HashRow, len(r.fields))
	for i, name := range r.fields {
		h[name] = row[i]
	}
	return h
}

// EachArray calls fn once per row as a positional array, stopping at and
// returning the first error fn returns (the block form of #each with
// as: :array).
func (r *Result) EachArray(fn func(Row) error) error {
	for _, row := range r.rows {
		if err := fn(row); err != nil {
			return err
		}
	}
	return nil
}

// EachHash calls fn once per row as a name-keyed map, stopping at and returning
// the first error fn returns (the block form of #each).
func (r *Result) EachHash(fn func(HashRow) error) error {
	for _, row := range r.rows {
		if err := fn(r.hashRow(row)); err != nil {
			return err
		}
	}
	return nil
}

// Each iterates the result in its configured shape (Mysql2::Result#each): it
// yields a HashRow by default, or a Row when the query used as: :array. fn
// receives an any it type-switches; iteration stops at the first error.
func (r *Result) Each(fn func(any) error) error {
	if r.opts.As == asArray {
		return r.EachArray(func(row Row) error { return fn(row) })
	}
	return r.EachHash(func(h HashRow) error { return fn(h) })
}
