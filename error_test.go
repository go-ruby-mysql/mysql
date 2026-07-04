// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import (
	"errors"
	"testing"
)

func TestWrapErrorNil(t *testing.T) {
	if wrapError(nil) != nil {
		t.Fatal("wrapError(nil) should be nil")
	}
}

func TestWrapErrorServer(t *testing.T) {
	err := wrapError(mysqlErr(1062, "23000", "Duplicate entry"))
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("not a *mysql.Error: %T", err)
	}
	if e.ErrorNumber() != 1062 {
		t.Fatalf("error_number = %d, want 1062", e.ErrorNumber())
	}
	if e.SQLState() != "23000" {
		t.Fatalf("sql_state = %q, want 23000", e.SQLState())
	}
	if e.Error() == "" {
		t.Fatal("empty message")
	}
}

func TestWrapErrorServerNoSQLState(t *testing.T) {
	err := wrapError(mysqlErr(1105, "", "Unknown error"))
	var e *Error
	errors.As(err, &e)
	if e.SQLState() != "HY000" {
		t.Fatalf("empty sql_state should normalize to HY000, got %q", e.SQLState())
	}
}

func TestWrapErrorGeneric(t *testing.T) {
	err := wrapError(errors.New("dial tcp: connection refused"))
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("not a *mysql.Error: %T", err)
	}
	if e.ErrorNumber() != 0 || e.SQLState() != "HY000" {
		t.Fatalf("generic error should be (0, HY000), got (%d, %q)", e.ErrorNumber(), e.SQLState())
	}
}
