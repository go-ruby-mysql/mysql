// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import "strings"

// escapeString escapes a string for safe interpolation into a MySQL statement,
// mirroring Mysql2::Client#escape (which calls mysql_real_escape_string under
// the default, non-NO_BACKSLASH_ESCAPES SQL mode). The escaped characters match
// libmysqlclient's set: NUL, newline, carriage return, backslash, single quote,
// double quote, and Ctrl-Z. The result is returned without surrounding quotes,
// exactly as the gem's #escape does.
func escapeString(s string) string {
	// Fast path: nothing to escape.
	if strings.IndexAny(s, "\x00\n\r\\'\"\x1a") < 0 {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case 0:
			b.WriteString(`\0`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '"':
			b.WriteString(`\"`)
		case 0x1a:
			b.WriteString(`\Z`)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}
