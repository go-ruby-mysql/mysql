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
)

func TestPrepareError(t *testing.T) {
	defer withFakeDriver(fakeState{})()
	c := mustConnect(t, Options{})
	defer c.Close()
	if _, err := c.Prepare("__prepfail__"); err == nil {
		t.Fatal("expected prepare error")
	}
}

func TestStatementExecuteSelect(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT name FROM t WHERE id > ?": {
			cols:  []string{"name"},
			types: []string{"VARCHAR"},
			vals:  [][]driver.Value{{b("web")}, {b("db")}},
		},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	st, err := c.Prepare("SELECT name FROM t WHERE id > ?")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if st.SQL() != "SELECT name FROM t WHERE id > ?" {
		t.Fatalf("sql = %q", st.SQL())
	}
	res, err := st.Execute(0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Count() != 2 || c.AffectedRows() != 2 {
		t.Fatalf("count=%d affected=%d", res.Count(), c.AffectedRows())
	}
}

func TestStatementExecuteOptsAndClose(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT n FROM t": {cols: []string{"n"}, types: []string{"INT"}, vals: [][]driver.Value{{b("9")}}},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	st, err := c.Prepare("SELECT n FROM t")
	if err != nil {
		t.Fatal(err)
	}
	res, err := st.ExecuteOpts(QueryOptions{Cast: false, As: asArray})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows()[0][0] != "9" {
		t.Fatalf("cast:false value = %#v", res.Rows()[0][0])
	}
	if st.Closed() {
		t.Fatal("not closed yet")
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	if !st.Closed() {
		t.Fatal("should be closed")
	}
	if err := st.Close(); err != nil { // idempotent
		t.Fatalf("second close: %v", err)
	}
}

func TestStatementExecuteBuildError(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT x FROM t": {
			cols:    []string{"x"},
			types:   []string{"INT"},
			vals:    [][]driver.Value{{b("1")}},
			nextErr: errors.New("boom"),
		},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	st, _ := c.Prepare("SELECT x FROM t")
	defer st.Close()
	if _, err := st.Execute(); err == nil {
		t.Fatal("expected build error")
	}
}

func TestStatementExecuteQueryError(t *testing.T) {
	defer withFakeDriver(fakeState{queries: map[string]fakeResultSet{
		"SELECT fail FROM t": {queryErr: mysqlErr(1142, "42000", "denied")},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	st, _ := c.Prepare("SELECT fail FROM t")
	defer st.Close()
	if _, err := st.Execute(); err == nil {
		t.Fatal("expected query error")
	}
}

func TestStatementExecuteExec(t *testing.T) {
	defer withFakeDriver(fakeState{execs: map[string]fakeExec{
		"INSERT INTO t VALUES (?)": {affected: 1, lastID: 42},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	st, err := c.Prepare("INSERT INTO t VALUES (?)")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	res, err := st.Execute("v")
	if err != nil {
		t.Fatal(err)
	}
	if res != nil || c.LastID() != 42 {
		t.Fatalf("res=%v last=%d", res, c.LastID())
	}
}

func TestStatementExecuteExecError(t *testing.T) {
	defer withFakeDriver(fakeState{execs: map[string]fakeExec{
		"DELETE FROM t WHERE id = ?": {err: mysqlErr(1205, "40001", "lock timeout")},
	}})()
	c := mustConnect(t, Options{})
	defer c.Close()
	st, _ := c.Prepare("DELETE FROM t WHERE id = ?")
	defer st.Close()
	if _, err := st.Execute(1); err == nil {
		t.Fatal("expected exec error")
	}
}

func TestNormalizeBinds(t *testing.T) {
	if normalizeBinds(nil) != nil {
		t.Fatal("empty binds should be nil")
	}
	ts := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	na := sql.Named("k", 1)
	type weird struct{ X int }
	in := []any{
		nil, int(1), int32(2), int64(3), uint(4), uint64(5),
		float32(1.5), float64(2.5), true, "s", []byte("b"), ts,
		Decimal("3.14"), na, weird{7},
	}
	out := normalizeBinds(in)
	checks := []struct {
		got  any
		want any
	}{
		{out[0], nil}, {out[1], int64(1)}, {out[2], int64(2)}, {out[3], int64(3)},
		{out[4], int64(4)}, {out[5], int64(5)}, {out[6], float64(1.5)}, {out[7], float64(2.5)},
		{out[8], true}, {out[9], "s"}, {out[11], ts}, {out[12], "3.14"}, {out[13], na},
	}
	for i, c := range checks {
		if c.got != c.want {
			t.Fatalf("bind[%d] = %#v, want %#v", i, c.got, c.want)
		}
	}
	if bs, ok := out[10].([]byte); !ok || string(bs) != "b" {
		t.Fatalf("bind bytes = %#v", out[10])
	}
	if _, ok := out[14].(weird); !ok {
		t.Fatalf("bind default passthrough = %#v", out[14])
	}
}
