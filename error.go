// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import (
	"errors"

	gsmysql "github.com/go-sql-driver/mysql"
)

// Error is a database error mirroring Mysql2::Error. It carries the MySQL error
// number and SQLSTATE the rbgo binding exposes as #error_number and #sql_state,
// plus the human-readable message (#message). A driver error that is not a
// server error (a dial failure, a context cancellation) surfaces as an Error
// with number 0 and the generic "HY000" SQLSTATE, matching how mysql2 wraps
// client-side failures.
type Error struct {
	number   int
	sqlState string
	message  string
}

// Error implements the error interface (Mysql2::Error#message).
func (e *Error) Error() string { return e.message }

// ErrorNumber returns the MySQL server error number (Mysql2::Error#error_number
// / #errno), e.g. 1062 for a duplicate-key violation. It is 0 for a client-side
// error with no server number.
func (e *Error) ErrorNumber() int { return e.number }

// SQLState returns the five-character SQLSTATE (Mysql2::Error#sql_state), e.g.
// "23000" for an integrity-constraint violation, or "HY000" (the general-error
// state) when the server supplied none.
func (e *Error) SQLState() string { return e.sqlState }

// wrapError converts an error from the go-sql-driver backend into a *mysql.Error
// with mysql2-faithful fields. A nil error passes through as nil. A server error
// (*mysql.MySQLError) contributes its Number and SQLState; any other error is
// wrapped as a client-side Error with number 0 and SQLSTATE "HY000".
func wrapError(err error) error {
	if err == nil {
		return nil
	}
	var me *gsmysql.MySQLError
	if errors.As(err, &me) {
		return &Error{
			number:   int(me.Number),
			sqlState: normalizeSQLState(me.SQLState),
			message:  me.Error(),
		}
	}
	return &Error{number: 0, sqlState: "HY000", message: err.Error()}
}

// normalizeSQLState renders the driver's fixed 5-byte SQLSTATE. The driver
// leaves it all-zero when the server sent no SQLSTATE marker; mysql2 reports the
// general-error state "HY000" there, so we do too.
func normalizeSQLState(b [5]byte) string {
	if b == [5]byte{} {
		return "HY000"
	}
	return string(b[:])
}
