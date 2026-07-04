// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import (
	"errors"
	"testing"
)

// sampleResult builds a Result directly for the pure result-shape tests.
func sampleResult(opts QueryOptions) *Result {
	return &Result{
		fields: []string{"id", "name"},
		rows:   []Row{{int64(1), "a"}, {int64(2), "b"}},
		opts:   opts,
	}
}

func TestResultFieldsCount(t *testing.T) {
	r := sampleResult(DefaultQueryOptions())
	if len(r.Fields()) != 2 || r.Count() != 2 {
		t.Fatalf("fields=%v count=%d", r.Fields(), r.Count())
	}
}

func TestResultHashesAndRows(t *testing.T) {
	r := sampleResult(DefaultQueryOptions())
	hs := r.Hashes()
	if hs[0]["id"] != int64(1) || hs[1]["name"] != "b" {
		t.Fatalf("hashes = %#v", hs)
	}
	rows := r.Rows()
	if rows[0][0] != int64(1) {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestResultDuplicateColumns(t *testing.T) {
	r := &Result{fields: []string{"c", "c"}, rows: []Row{{int64(1), int64(2)}}, opts: DefaultQueryOptions()}
	h := r.Hashes()[0]
	if h["c"] != int64(2) {
		t.Fatalf("duplicate key should keep last, got %#v", h["c"])
	}
}

func TestResultAsAndSymbolize(t *testing.T) {
	if sampleResult(QueryOptions{As: asHash}).As() != asHash {
		t.Fatal("As hash")
	}
	if sampleResult(QueryOptions{As: asArray}).As() != asArray {
		t.Fatal("As array")
	}
	if sampleResult(QueryOptions{As: ""}).As() != asHash {
		t.Fatal("As default hash")
	}
	if !sampleResult(QueryOptions{SymbolizeKeys: true}).SymbolizeKeys() {
		t.Fatal("symbolize")
	}
}

func TestResultEachHash(t *testing.T) {
	r := sampleResult(QueryOptions{As: asHash})
	var n int
	if err := r.Each(func(row any) error {
		if _, ok := row.(HashRow); !ok {
			t.Fatalf("expected HashRow, got %T", row)
		}
		n++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("iterated %d", n)
	}
}

func TestResultEachArray(t *testing.T) {
	r := sampleResult(QueryOptions{As: asArray})
	if err := r.Each(func(row any) error {
		if _, ok := row.(Row); !ok {
			t.Fatalf("expected Row, got %T", row)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestResultEachErrors(t *testing.T) {
	r := sampleResult(DefaultQueryOptions())
	sentinel := errors.New("stop")
	if err := r.EachHash(func(HashRow) error { return sentinel }); err != sentinel {
		t.Fatalf("EachHash err = %v", err)
	}
	if err := r.EachArray(func(Row) error { return sentinel }); err != sentinel {
		t.Fatalf("EachArray err = %v", err)
	}
	if err := sampleResult(QueryOptions{As: asArray}).Each(func(any) error { return sentinel }); err != sentinel {
		t.Fatalf("Each array err = %v", err)
	}
}
