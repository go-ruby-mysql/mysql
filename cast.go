// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import (
	"strconv"
	"strings"
	"time"
)

// castValue turns one raw column value (the field's on-the-wire bytes) into the
// Go value mysql2 would cast it to, driven by the MySQL column type name and the
// active QueryOptions. It is the deterministic heart of the binding — no live
// server is needed to exercise it. NULL is handled by the caller (a NULL never
// reaches here); raw is the non-NULL field bytes.
//
// With cast:false, mysql2 returns every value as a String, so we return the raw
// text unchanged. With cast:true (the default) the column type selects the Ruby
// type per mysql2's cast table (see the package doc).
func castValue(typeName string, raw []byte, qo QueryOptions) Value {
	if !qo.Cast {
		return string(raw)
	}
	switch normalizeType(typeName) {
	case "TINYINT":
		if qo.CastBooleans {
			// mysql2 with :cast_booleans casts a TINYINT to true/false; a 0 is
			// false, anything else true (its check is `value != 0`).
			return string(raw) != "0"
		}
		return castInt(raw)
	case "SMALLINT", "MEDIUMINT", "INT", "INTEGER", "BIGINT", "YEAR":
		return castInt(raw)
	case "FLOAT", "DOUBLE":
		return castFloat(raw)
	case "DECIMAL":
		return Decimal(raw)
	case "DATE":
		return castDate(raw)
	case "DATETIME", "TIMESTAMP":
		return castDateTime(raw)
	case "TIME":
		// mysql2's Time cast for a TIME column is date-ambiguous (it borrows the
		// current date); to stay deterministic and timezone-safe we hand back the
		// canonical "HH:MM:SS" string for the host to interpret.
		return string(raw)
	case "BIT", "BLOB", "BINARY", "GEOMETRY":
		return copyBytes(raw)
	default:
		// CHAR / VARCHAR / TEXT / ENUM / SET / JSON and any unknown type: a
		// UTF-8 string.
		return string(raw)
	}
}

// castInt parses an integer field. Values that fit a signed 64-bit integer
// become int64; a larger unsigned BIGINT becomes uint64 (mysql2 returns an
// arbitrary-precision Integer either way). A value that parses as neither is
// returned as its raw string, matching mysql2's refusal to lose data.
func castInt(raw []byte) Value {
	s := string(raw)
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	if n, err := strconv.ParseUint(s, 10, 64); err == nil {
		return n
	}
	return s
}

// castFloat parses a floating-point field into a float64. An unparseable value
// falls back to its raw string.
func castFloat(raw []byte) Value {
	if f, err := strconv.ParseFloat(string(raw), 64); err == nil {
		return f
	}
	return string(raw)
}

// castDate parses a "YYYY-MM-DD" DATE field into a Date. A datetime-shaped value
// (from a prepared/binary result reformatted with a time part) is tolerated by
// reading only the leading date. An unparseable value falls back to its string.
func castDate(raw []byte) Value {
	s := string(raw)
	if len(s) >= 10 {
		s = s[:10]
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return string(raw)
	}
	return Date{Year: t.Year(), Month: int(t.Month()), Day: t.Day()}
}

// castDateTime parses a DATETIME / TIMESTAMP field into a naive time.Time built
// in UTC (no zone conversion, so assertions are timezone-independent). It
// accepts an optional fractional-seconds suffix. An unparseable value falls back
// to its raw string.
func castDateTime(raw []byte) Value {
	s := string(raw)
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return string(raw)
}

// copyBytes returns an independent copy of raw so a binary value stays valid
// after the driver reuses its scan buffer.
func copyBytes(raw []byte) []byte {
	b := make([]byte, len(raw))
	copy(b, raw)
	return b
}

// normalizeType canonicalises a database/sql DatabaseTypeName into the bare type
// keyword castValue switches on. It upper-cases, strips a leading "UNSIGNED "
// qualifier the driver may prepend, and collapses the width-tagged text/blob
// families (TINYTEXT, LONGBLOB, VARBINARY, …) onto their base keyword.
func normalizeType(name string) string {
	n := strings.ToUpper(strings.TrimSpace(name))
	n = strings.TrimPrefix(n, "UNSIGNED ")
	switch n {
	case "TINYTEXT", "MEDIUMTEXT", "LONGTEXT", "TEXT":
		return "TEXT"
	case "TINYBLOB", "MEDIUMBLOB", "LONGBLOB", "BLOB":
		return "BLOB"
	case "VARBINARY":
		return "BINARY"
	case "VARCHAR", "CHAR":
		return "CHAR"
	}
	return n
}

// rawCol is a sql.Scanner that captures a column's value as raw bytes plus a
// NULL flag, regardless of the concrete type the driver hands us. The text
// protocol delivers []byte; the binary (prepared-statement) protocol delivers
// typed values, which we render to their canonical MySQL text so castValue sees
// a uniform byte representation. This keeps the cast independent of which wire
// protocol produced the row.
type rawCol struct {
	val  []byte
	null bool
}

// Scan implements sql.Scanner.
func (r *rawCol) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		r.null = true
		r.val = nil
	case []byte:
		r.null = false
		r.val = append([]byte(nil), v...)
	case string:
		r.null = false
		r.val = []byte(v)
	case int64:
		r.null = false
		r.val = strconv.AppendInt(nil, v, 10)
	case uint64:
		r.null = false
		r.val = strconv.AppendUint(nil, v, 10)
	case float64:
		r.null = false
		r.val = strconv.AppendFloat(nil, v, 'g', -1, 64)
	case bool:
		r.null = false
		if v {
			r.val = []byte("1")
		} else {
			r.val = []byte("0")
		}
	case time.Time:
		r.null = false
		r.val = []byte(v.Format("2006-01-02 15:04:05.999999999"))
	default:
		// Anything else: defer to fmt-free stringification via the driver's own
		// []byte fallback is impossible here, so record the Go default text.
		r.null = false
		r.val = []byte(defaultString(v))
	}
	return nil
}

// defaultString renders an unexpected scan source without importing fmt into the
// hot cast path; it is only reached by an exotic driver value.
func defaultString(v any) string {
	if s, ok := v.(interface{ String() string }); ok {
		return s.String()
	}
	return ""
}
