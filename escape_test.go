// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import "testing"

func TestEscapeFastPath(t *testing.T) {
	if got := escapeString("plain text 123"); got != "plain text 123" {
		t.Fatalf("fast path changed string: %q", got)
	}
}

func TestEscapeAllSpecials(t *testing.T) {
	in := "a\x00b\nc\rd\\e'f\"g\x1ah"
	want := `a\0b\nc\rd\\e\'f\"g\Zh`
	if got := escapeString(in); got != want {
		t.Fatalf("escape = %q, want %q", got, want)
	}
}

func TestEscapePassthroughByte(t *testing.T) {
	// A non-special byte mixed with a special one exercises the default arm.
	if got := escapeString("x'"); got != `x\'` {
		t.Fatalf("escape = %q", got)
	}
}
