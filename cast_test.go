// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import (
	"reflect"
	"testing"
	"time"
)

func TestCastValueCastOff(t *testing.T) {
	qo := QueryOptions{Cast: false}
	if got := castValue("INT", []byte("42"), qo); got != "42" {
		t.Fatalf("cast:false = %#v, want string \"42\"", got)
	}
}

func TestCastValueMatrix(t *testing.T) {
	on := DefaultQueryOptions()
	cases := []struct {
		name string
		typ  string
		raw  string
		want Value
	}{
		{"tinyint", "TINYINT", "7", int64(7)},
		{"smallint", "SMALLINT", "-3", int64(-3)},
		{"mediumint", "MEDIUMINT", "100", int64(100)},
		{"int", "INT", "2147483647", int64(2147483647)},
		{"integer", "INTEGER", "5", int64(5)},
		{"bigint", "BIGINT", "9223372036854775807", int64(9223372036854775807)},
		{"year", "YEAR", "2026", int64(2026)},
		{"ubig", "BIGINT", "18446744073709551615", uint64(18446744073709551615)},
		{"int-bad", "INT", "notanint", "notanint"},
		{"float", "FLOAT", "1.5", float64(1.5)},
		{"double", "DOUBLE", "3.25", float64(3.25)},
		{"float-bad", "DOUBLE", "NaNsense", "NaNsense"},
		{"decimal", "DECIMAL", "3.14", Decimal("3.14")},
		{"date", "DATE", "2026-07-04", Date{2026, 7, 4}},
		{"date-datetimeish", "DATE", "2026-07-04 12:00:00", Date{2026, 7, 4}},
		{"date-bad", "DATE", "not-a-date", "not-a-date"},
		{"datetime", "DATETIME", "2026-07-04 12:13:14", time.Date(2026, 7, 4, 12, 13, 14, 0, time.UTC)},
		{"datetime-frac", "DATETIME", "2026-07-04 12:13:14.5", time.Date(2026, 7, 4, 12, 13, 14, 500000000, time.UTC)},
		{"timestamp", "TIMESTAMP", "2026-01-02 03:04:05", time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)},
		{"datetime-bad", "DATETIME", "nope", "nope"},
		{"time", "TIME", "12:13:14", "12:13:14"},
		{"char", "CHAR", "x", "x"},
		{"varchar", "VARCHAR", "hi", "hi"},
		{"text", "TEXT", "body", "body"},
		{"enum", "ENUM", "a", "a"},
		{"json", "JSON", "{}", "{}"},
		{"unknown", "GYARBAGE", "z", "z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := castValue(c.typ, []byte(c.raw), on)
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("castValue(%q,%q) = %#v, want %#v", c.typ, c.raw, got, c.want)
			}
		})
	}
}

func TestCastValueBinaryFamilies(t *testing.T) {
	on := DefaultQueryOptions()
	for _, typ := range []string{"BIT", "BLOB", "BINARY", "GEOMETRY", "VARBINARY", "LONGBLOB"} {
		got := castValue(typ, []byte{0x01, 0x02}, on)
		bs, ok := got.([]byte)
		if !ok || !reflect.DeepEqual(bs, []byte{0x01, 0x02}) {
			t.Fatalf("castValue(%q) = %#v, want []byte{1,2}", typ, got)
		}
	}
}

func TestCastBooleans(t *testing.T) {
	qo := QueryOptions{Cast: true, CastBooleans: true}
	if v := castValue("TINYINT", []byte("0"), qo); v != false {
		t.Fatalf("TINYINT 0 with cast_booleans = %#v, want false", v)
	}
	if v := castValue("TINYINT", []byte("1"), qo); v != true {
		t.Fatalf("TINYINT 1 with cast_booleans = %#v, want true", v)
	}
	if v := castValue("TINYINT", []byte("2"), qo); v != true {
		t.Fatalf("TINYINT 2 with cast_booleans = %#v, want true", v)
	}
}

func TestNormalizeType(t *testing.T) {
	cases := map[string]string{
		"int":             "INT",
		"UNSIGNED BIGINT": "BIGINT",
		"tinytext":        "TEXT",
		"MEDIUMTEXT":      "TEXT",
		"longtext":        "TEXT",
		"text":            "TEXT",
		"tinyblob":        "BLOB",
		"MEDIUMBLOB":      "BLOB",
		"longblob":        "BLOB",
		"blob":            "BLOB",
		"varbinary":       "BINARY",
		"varchar":         "CHAR",
		"char":            "CHAR",
		"  datetime ":     "DATETIME",
	}
	for in, want := range cases {
		if got := normalizeType(in); got != want {
			t.Fatalf("normalizeType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRawColScan(t *testing.T) {
	var r rawCol
	// nil -> NULL
	_ = r.Scan(nil)
	if !r.null || r.val != nil {
		t.Fatalf("nil scan: null=%v val=%v", r.null, r.val)
	}
	cases := []struct {
		src  any
		want string
	}{
		{[]byte("bytes"), "bytes"},
		{"str", "str"},
		{int64(-9), "-9"},
		{uint64(18446744073709551615), "18446744073709551615"},
		{float64(2.5), "2.5"},
		{true, "1"},
		{false, "0"},
		{time.Date(2026, 7, 4, 1, 2, 3, 0, time.UTC), "2026-07-04 01:02:03"},
	}
	for _, c := range cases {
		var rc rawCol
		if err := rc.Scan(c.src); err != nil {
			t.Fatalf("scan %#v: %v", c.src, err)
		}
		if rc.null || string(rc.val) != c.want {
			t.Fatalf("scan %#v = %q (null=%v), want %q", c.src, rc.val, rc.null, c.want)
		}
	}
}

type stringerT struct{}

func (stringerT) String() string { return "S!" }

type plainT struct{}

func TestRawColScanDefault(t *testing.T) {
	var r rawCol
	if err := r.Scan(stringerT{}); err != nil || string(r.val) != "S!" {
		t.Fatalf("stringer default = %q, %v", r.val, err)
	}
	var r2 rawCol
	if err := r2.Scan(plainT{}); err != nil || string(r2.val) != "" {
		t.Fatalf("plain default = %q, %v", r2.val, err)
	}
}

func TestCopyBytesIndependent(t *testing.T) {
	src := []byte("abc")
	got := copyBytes(src)
	got[0] = 'z'
	if src[0] != 'a' {
		t.Fatal("copyBytes did not copy")
	}
}
