// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

// Value is a datum crossing the Go<->MySQL boundary. On a cast result it is one
// of: nil, int64, uint64, float64, Decimal, Date, time.Time, string, []byte, or
// bool. On binds this package accepts int / int32 / int64 / uint / float32 /
// float64 / string / []byte / bool / time.Time / nil.
type Value = any

// Row is a single result row as a slice of column Values in column order — the
// shape Mysql2::Result yields with `as: :array`.
type Row = []Value

// HashRow is a single result row keyed by column name — the shape
// Mysql2::Result yields by default (`as: :hash`).
type HashRow = map[string]Value

// Decimal is a fixed-point value from a DECIMAL / NEWDECIMAL column, carried as
// its exact decimal string so no precision is lost. The rbgo binding turns it
// into a Ruby BigDecimal (mysql2's cast for DECIMAL). Its String is the raw
// database text (e.g. "3.14").
type Decimal string

// String returns the decimal text.
func (d Decimal) String() string { return string(d) }

// Date is a calendar date from a DATE column, with no time-of-day or zone —
// mirroring the Ruby Date mysql2 returns for DATE. The rbgo binding maps it to a
// Ruby Date; keeping it distinct from time.Time lets the host pick Date vs Time.
type Date struct {
	Year  int
	Month int
	Day   int
}

// String renders the date as "YYYY-MM-DD", the canonical MySQL DATE literal.
func (d Date) String() string {
	return pad4(d.Year) + "-" + pad2(d.Month) + "-" + pad2(d.Day)
}

// pad2 formats n as a zero-padded two-digit string (00..99, wider if larger).
func pad2(n int) string {
	if n < 0 {
		return "-" + pad2(-n)
	}
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

// pad4 formats n as a zero-padded four-digit year.
func pad4(n int) string {
	if n < 0 {
		return "-" + pad4(-n)
	}
	s := itoa(n)
	for len(s) < 4 {
		s = "0" + s
	}
	return s
}

// itoa is a tiny base-10 formatter for a non-negative int, used by the Date
// stringer (sign is handled by pad2/pad4, so n is always >= 0 here).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
