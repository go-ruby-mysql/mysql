// Copyright (c) the go-ruby-mysql/mysql authors
//
// SPDX-License-Identifier: BSD-3-Clause

package mysql

import (
	"database/sql"
	"strconv"
	"strings"
	"time"

	gsmysql "github.com/go-sql-driver/mysql"
)

// asHash and asArray are the two row shapes mysql2's `as:` option selects.
const (
	asHash  = "hash"
	asArray = "array"
)

// QueryOptions carries the per-query options mysql2 accepts on Client#query and
// merges from Client.new's default_query_options. The zero value is not the
// gem's default (mysql2 defaults cast:true); build one with DefaultQueryOptions
// and adjust, or set Client.QueryDefaults.
type QueryOptions struct {
	// Cast toggles type casting (mysql2 :cast). When false every value is
	// returned as a String. Default true.
	Cast bool
	// CastBooleans casts TINYINT columns to bool (mysql2 :cast_booleans).
	CastBooleans bool
	// SymbolizeKeys requests Symbol hash keys (mysql2 :symbolize_keys); carried
	// for the host to honour, as Go has no Symbol.
	SymbolizeKeys bool
	// As selects the row shape: asHash ("hash", default) or asArray ("array").
	As string
}

// DefaultQueryOptions returns the gem's default query options: cast on, no
// boolean cast, string keys, hash rows.
func DefaultQueryOptions() QueryOptions {
	return QueryOptions{Cast: true, As: asHash}
}

// Options configures a Client, mirroring the keyword arguments of
// Mysql2::Client.new (host:, port:, username:, password:, database:, socket:,
// encoding:, flags:, connect_timeout:, read_timeout:, write_timeout:, …).
type Options struct {
	Host           string
	Port           int
	Username       string
	Password       string
	Database       string
	Socket         string        // unix-socket path; when set, Net becomes "unix"
	Encoding       string        // connection charset (mysql2 :encoding), e.g. "utf8mb4"
	Flags          []string      // capability flags, e.g. "MULTI_STATEMENTS", "FOUND_ROWS"
	ConnectTimeout time.Duration // dial timeout (mysql2 :connect_timeout)
	ReadTimeout    time.Duration // I/O read timeout (mysql2 :read_timeout)
	WriteTimeout   time.Duration // I/O write timeout (mysql2 :write_timeout)
	// QueryDefaults overrides the per-client default query options. When nil the
	// client uses DefaultQueryOptions().
	QueryDefaults *QueryOptions
}

// Client is a handle to a MySQL server, mirroring Mysql2::Client. It wraps a
// *sql.DB over the go-sql-driver backend.
type Client struct {
	db           *sql.DB
	qdef         QueryOptions
	affectedRows int64
	lastID       int64
	closed       bool
}

// sqlOpen is the seam onto database/sql.Open, overridable in tests to exercise
// the open-error branch without a live server.
var sqlOpen = sql.Open

// driverName is the registered database/sql driver the client opens. It is the
// pure-Go go-sql-driver "mysql" driver in production; the deterministic test
// suite points it at an in-process fake so no server is needed.
var driverName = "mysql"

// NewClient connects to a MySQL server described by opts and returns a Client,
// mirroring Mysql2::Client.new. It opens the pool and verifies the connection
// with a ping (mysql2 connects eagerly), wrapping any failure as a *mysql.Error.
func NewClient(opts Options) (*Client, error) {
	db, err := sqlOpen(driverName, buildDSN(opts))
	if err != nil {
		return nil, wrapError(err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, wrapError(err)
	}
	qdef := DefaultQueryOptions()
	if opts.QueryDefaults != nil {
		qdef = *opts.QueryDefaults
	}
	return &Client{db: db, qdef: qdef}, nil
}

// buildDSN renders Options into a go-sql-driver DSN. It mirrors the mysql2
// connection keywords onto the driver's Config: a Socket switches the network to
// a unix socket, Encoding becomes the connection collation base, and the
// timeouts and capability flags map onto their Config counterparts.
func buildDSN(opts Options) string {
	cfg := gsmysql.NewConfig()
	cfg.User = opts.Username
	cfg.Passwd = opts.Password
	cfg.DBName = opts.Database
	if opts.Socket != "" {
		cfg.Net = "unix"
		cfg.Addr = opts.Socket
	} else {
		cfg.Net = "tcp"
		host := opts.Host
		if host == "" {
			host = "127.0.0.1"
		}
		port := opts.Port
		if port == 0 {
			port = 3306
		}
		cfg.Addr = host + ":" + strconv.Itoa(port)
	}
	if opts.Encoding != "" {
		cfg.Collation = opts.Encoding
	}
	cfg.Timeout = opts.ConnectTimeout
	cfg.ReadTimeout = opts.ReadTimeout
	cfg.WriteTimeout = opts.WriteTimeout
	applyFlags(cfg, opts.Flags)
	return cfg.FormatDSN()
}

// applyFlags maps mysql2-style capability flag names onto the driver Config
// booleans. Unrecognised flags are ignored, matching mysql2's tolerance of
// flags libmysqlclient does not know.
func applyFlags(cfg *gsmysql.Config, flags []string) {
	for _, f := range flags {
		switch strings.ToUpper(strings.TrimSpace(f)) {
		case "MULTI_STATEMENTS":
			cfg.MultiStatements = true
		case "FOUND_ROWS":
			cfg.ClientFoundRows = true
		}
	}
}

// queryOptions resolves the effective options for a call: the client default,
// overridden wholesale by an explicit QueryOptions if the caller passed one.
func (c *Client) queryOptions(opts []QueryOptions) QueryOptions {
	if len(opts) > 0 {
		return opts[0]
	}
	return c.qdef
}

// Query runs sql and returns a *Result for a row-producing statement, or a nil
// Result for a statement that changes rows (INSERT / UPDATE / DELETE / DDL),
// mirroring Mysql2::Client#query (which returns a Result or nil). After a
// non-row statement AffectedRows and LastID reflect it. An optional QueryOptions
// overrides the client defaults for this call.
func (c *Client) Query(query string, opts ...QueryOptions) (*Result, error) {
	qo := c.queryOptions(opts)
	if !producesRows(query) {
		res, err := c.db.Exec(query)
		if err != nil {
			return nil, wrapError(err)
		}
		c.recordExec(res)
		return nil, nil
	}
	rows, err := c.db.Query(query)
	if err != nil {
		return nil, wrapError(err)
	}
	defer rows.Close()
	result, err := buildResult(rows, qo)
	if err != nil {
		return nil, err
	}
	c.affectedRows = int64(result.Count())
	return result, nil
}

// recordExec captures the affected-row count and last insert id from an Exec
// result so AffectedRows / LastID can report them (mysql2's #affected_rows /
// #last_id). A driver that cannot report either leaves the prior value.
func (c *Client) recordExec(res sql.Result) {
	if n, err := res.RowsAffected(); err == nil {
		c.affectedRows = n
	}
	if id, err := res.LastInsertId(); err == nil {
		c.lastID = id
	}
}

// AffectedRows returns the number of rows changed by the most recent statement
// (Mysql2::Client#affected_rows).
func (c *Client) AffectedRows() int64 { return c.affectedRows }

// LastID returns the AUTO_INCREMENT value generated by the most recent INSERT
// (Mysql2::Client#last_id / #insert_id).
func (c *Client) LastID() int64 { return c.lastID }

// Escape escapes a string for safe interpolation into a statement
// (Mysql2::Client#escape).
func (c *Client) Escape(s string) string { return escapeString(s) }

// Ping checks the connection is alive (Mysql2::Client#ping), returning false on
// failure like the gem rather than an error.
func (c *Client) Ping() bool { return c.db.Ping() == nil }

// ServerInfo describes the connected server, mirroring the Hash
// Mysql2::Client#server_info returns: the version string and a packed numeric id
// (major*10000 + minor*100 + patch).
type ServerInfo struct {
	Version       string
	VersionNumber int
}

// ServerInfo queries the server version (Mysql2::Client#server_info) via
// SELECT VERSION() and returns it parsed.
func (c *Client) ServerInfo() (ServerInfo, error) {
	var v string
	if err := c.db.QueryRow("SELECT VERSION()").Scan(&v); err != nil {
		return ServerInfo{}, wrapError(err)
	}
	return ServerInfo{Version: v, VersionNumber: parseVersionNumber(v)}, nil
}

// parseVersionNumber packs a "major.minor.patch…" version string into mysql2's
// integer form major*10000 + minor*100 + patch. Missing or non-numeric
// components count as zero.
func parseVersionNumber(v string) int {
	// Trim any suffix after the numeric core (e.g. "8.0.34-log").
	core := v
	if i := strings.IndexByte(core, '-'); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	get := func(i int) int {
		if i < len(parts) {
			n, _ := strconv.Atoi(parts[i])
			return n
		}
		return 0
	}
	return get(0)*10000 + get(1)*100 + get(2)
}

// Close closes the client and its connection pool (Mysql2::Client#close).
// Closing an already-closed client is a no-op, matching the gem.
func (c *Client) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	return wrapError(c.db.Close())
}

// Closed reports whether the client has been closed.
func (c *Client) Closed() bool { return c.closed }

// producesRows reports whether query returns a result set (SELECT / SHOW /
// DESCRIBE / EXPLAIN / WITH … SELECT / VALUES / CALL / TABLE), so it goes through
// Query; other statements go through Exec so affected-rows / last-id are
// meaningful. Leading whitespace and -- / /* */ comments are skipped, matching
// how a server dispatches the statement.
func producesRows(query string) bool {
	s := strings.TrimLeft(query, " \t\r\n(")
	for {
		if strings.HasPrefix(s, "--") {
			i := strings.IndexByte(s, '\n')
			if i < 0 {
				return false
			}
			s = strings.TrimLeft(s[i+1:], " \t\r\n(")
			continue
		}
		if strings.HasPrefix(s, "/*") {
			i := strings.Index(s, "*/")
			if i < 0 {
				return false
			}
			s = strings.TrimLeft(s[i+2:], " \t\r\n(")
			continue
		}
		break
	}
	up := strings.ToUpper(s)
	for _, kw := range []string{"SELECT", "SHOW", "DESCRIBE", "DESC ", "EXPLAIN", "WITH", "VALUES", "CALL", "TABLE"} {
		if strings.HasPrefix(up, kw) {
			return true
		}
	}
	return false
}

// buildResult drains a *sql.Rows into a Result, casting every value per qo. It
// reads the column names and their MySQL type names, then scans each row through
// rawCol holders (so NULLs and both wire protocols are handled uniformly) and
// casts each field. The column/scan steps go through package vars so tests can
// inject the driver failures a live backend almost never produces.
func buildResult(rows *sql.Rows, qo QueryOptions) (*Result, error) {
	cols, err := rowColumns(rows)
	if err != nil {
		return nil, wrapError(err)
	}
	cts, err := rowColumnTypes(rows)
	if err != nil {
		return nil, wrapError(err)
	}
	types := make([]string, len(cts))
	for i, ct := range cts {
		types[i] = ct.DatabaseTypeName()
	}
	var data []Row
	for rows.Next() {
		holders := make([]rawCol, len(cols))
		ptrs := make([]any, len(cols))
		for i := range holders {
			ptrs[i] = &holders[i]
		}
		if err := scanRow(rows, ptrs); err != nil {
			return nil, wrapError(err)
		}
		row := make(Row, len(cols))
		for i := range cols {
			if holders[i].null {
				row[i] = nil
			} else {
				row[i] = castValue(types[i], holders[i].val, qo)
			}
		}
		data = append(data, row)
	}
	if err := rows.Err(); err != nil {
		return nil, wrapError(err)
	}
	return &Result{fields: cols, rows: data, opts: qo}, nil
}

// rowColumns, rowColumnTypes, and scanRow are seams onto the *sql.Rows methods,
// overridable in tests to exercise buildResult's error branches (the live
// go-sql-driver backend does not fail these mid-result).
var (
	rowColumns     = func(rows *sql.Rows) ([]string, error) { return rows.Columns() }
	rowColumnTypes = func(rows *sql.Rows) ([]*sql.ColumnType, error) { return rows.ColumnTypes() }
	scanRow        = func(rows *sql.Rows, ptrs []any) error { return rows.Scan(ptrs...) }
)
