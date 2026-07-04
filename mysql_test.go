// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import "testing"

func TestDecimalString(t *testing.T) {
	if Decimal("3.14").String() != "3.14" {
		t.Fatal("Decimal.String")
	}
}

func TestDateString(t *testing.T) {
	cases := map[Date]string{
		{2026, 7, 4}:  "2026-07-04",
		{0, 0, 0}:     "0000-00-00",
		{999, 12, 31}: "0999-12-31",
		{-44, -3, -4}: "-0044--03--04",
		{10000, 1, 1}: "10000-01-01",
	}
	for d, want := range cases {
		if got := d.String(); got != want {
			t.Fatalf("Date%v.String() = %q, want %q", d, got, want)
		}
	}
}
