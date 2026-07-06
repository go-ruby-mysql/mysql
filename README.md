<p align="center"><img src="https://raw.githubusercontent.com/go-ruby-mysql/brand/main/social/go-ruby-mysql-mysql.png" alt="go-ruby-mysql/mysql" width="720"></p>

# mysql — go-ruby-mysql

[![Docs](https://img.shields.io/badge/docs-mkdocs--material-DC2626)](https://go-ruby-mysql.github.io/docs/)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.26.4%2B-00ADD8)](https://go.dev/dl/)
[![Coverage](https://img.shields.io/badge/coverage-100%25-1a7f37)](#tests--coverage)

**A pure-Go (no cgo) reimplementation of the Ruby
[`mysql2`](https://github.com/brianmario/mysql2) gem's `Mysql2` API.**
Upstream, `mysql2` is a C extension linking `libmysqlclient`; this module binds
[`github.com/go-sql-driver/mysql`](https://pkg.go.dev/github.com/go-sql-driver/mysql)
instead — a **pure-Go implementation of the MySQL client/server protocol** — over
`database/sql`, and exposes the gem's `Mysql2::Client` / `Mysql2::Result` /
`Mysql2::Statement` / `Mysql2::Error` surface on top. The whole stack links
statically with `CGO_ENABLED=0` on every 64-bit target the go-\* ecosystem
supports.

It completes the database set alongside
[go-ruby-sqlite3](https://github.com/go-ruby-sqlite3/sqlite3) (the SQLite engine)
and [go-ruby-pg](https://github.com/go-ruby-pg/pg) (the PostgreSQL wire
protocol), and is the MySQL backend for
[go-embedded-ruby](https://github.com/go-embedded-ruby/ruby) — a **standalone,
reusable** module.

> **What it is — and isn't.** The connection, the SQL, and the MySQL↔Ruby type
> coercions are fully deterministic and need **no interpreter**, so they live
> here as pure Go. Turning a returned row into live Ruby objects is the host's
> job; this library hands back a small, explicit value model (`int64`,
> `float64`, `Decimal`, `Date`, `time.Time`, `string`, `[]byte`, `nil`, `bool`),
> plus an `Error` carrying the exact `#error_number` and `#sql_state`.

## Features

Faithful port of the gem's `Mysql2` surface:

- **Client** — `NewClient` (`Mysql2::Client.new` with `host:`, `port:`,
  `username:`, `password:`, `database:`, `socket:`, `encoding:`, `flags:`, and
  the connect/read/write timeouts), `Query`, `Prepare`, `Escape`, `Ping`,
  `Close` (idempotent), `AffectedRows`, `LastID`, `ServerInfo`.
- **Result** — `Mysql2::Result` as Enumerable rows: `Fields`, `Count`, `Rows`
  (`as: :array`), `Hashes` (`as: :hash`), `Each` / `EachHash` / `EachArray`,
  and the `symbolize_keys` flag.
- **Statement** — `Prepare` → `Statement` with positional `?` binds,
  `Execute` / `ExecuteOpts`, `Close`.
- **Casting** — the gem's `cast` table (`cast`, `cast_booleans`,
  `symbolize_keys`, `as: :array`/`:hash`), turning each column into its Ruby
  type.
- **Errors** — `Mysql2::Error` with `#error_number` and `#sql_state`, mapped
  from the server error or a generic client-side failure (`HY000`).

CGO-free, **100% test coverage**, `gofmt` + `go vet` clean, and green across the
six 64-bit Go targets (amd64, arm64, riscv64, loong64, ppc64le, s390x) — the
whole stack, including the pure-Go MySQL protocol driver, builds and runs
pure-Go.

## Install

```sh
go get github.com/go-ruby-mysql/mysql
```

## Usage

```go
package main

import (
	"fmt"

	"github.com/go-ruby-mysql/mysql"
)

func main() {
	c, _ := mysql.NewClient(mysql.Options{
		Host: "127.0.0.1", Port: 3306,
		Username: "root", Password: "secret", Database: "app",
	}) // Mysql2::Client.new(host:, port:, username:, password:, database:)
	defer c.Close()

	res, _ := c.Query("SELECT id, name FROM hosts")
	for _, h := range res.Hashes() {
		fmt.Println(h["id"], h["name"]) // int64, string
	}

	// Prepared statement, like Mysql2::Statement.
	st, _ := c.Prepare("SELECT name FROM hosts WHERE id > ?")
	defer st.Close()
	r, _ := st.Execute(0)
	_ = r.EachHash(func(row mysql.HashRow) error {
		fmt.Println(row["name"])
		return nil
	})

	c.Query("INSERT INTO hosts (name) VALUES ('web')")
	fmt.Println(c.AffectedRows(), c.LastID())
}
```

## Type mapping

Result values crossing the boundary use mysql2's MySQL→Ruby cast table:

| MySQL column type          | Go (this package)      | Ruby (host binding)    |
| -------------------------- | ---------------------- | ---------------------- |
| `TINYINT`..`BIGINT`, `YEAR`| `int64` / `uint64`     | `Integer`              |
| `FLOAT`, `DOUBLE`          | `float64`              | `Float`                |
| `DECIMAL` / `NEWDECIMAL`   | `Decimal` (a string)   | `BigDecimal`           |
| `DATE`                     | `Date`                 | `Date`                 |
| `DATETIME`, `TIMESTAMP`    | `time.Time` (naive UTC)| `Time`                 |
| `TIME`                     | `string`               | `String`               |
| `CHAR`/`VARCHAR`/`TEXT`/…  | `string`               | `String` (UTF-8)       |
| `BLOB`/`BINARY`/`BIT`/…    | `[]byte`               | `String` (ASCII-8BIT)  |
| `NULL`                     | `nil`                  | `nil`                  |

The cast is governed by the same options mysql2 exposes, carried on
`QueryOptions`: `Cast` (`cast: false` returns every value as a `String`),
`CastBooleans` (a `TINYINT` → `bool`), `SymbolizeKeys`, and `As`
(`"hash"` / `"array"`).

## Errors

Errors are `*mysql.Error`, carrying the server error number and SQLSTATE:

```go
_, err := c.Query("INSERT INTO t (id) VALUES (1)") // duplicate key
var e *mysql.Error
if errors.As(err, &e) {
	fmt.Println(e.ErrorNumber()) // 1062
	fmt.Println(e.SQLState())    // 23000
}
```

A client-side failure (a dial error, a cancellation) surfaces as an `Error` with
number `0` and the general-error SQLSTATE `HY000`.

## Backend & architectures

The backend is `github.com/go-sql-driver/mysql` — the MySQL protocol in pure Go,
**no cgo**. Every arch below builds and tests with `CGO_ENABLED=0`:

| arch    | CGO=0 build | notes                          |
| ------- | ----------- | ------------------------------ |
| amd64   | ✅          | native CI lane                 |
| arm64   | ✅          | native CI lane                 |
| riscv64 | ✅          | qemu-user CI lane              |
| loong64 | ✅          | qemu-user CI lane              |
| ppc64le | ✅          | qemu-user CI lane              |
| s390x   | ✅          | qemu-user CI lane (big-endian) |

## Tests & coverage

The suite drives the whole `NewClient → Query → Result → cast` pipeline — every
MySQL type, NULLs, the option matrix, and the error taxonomy — through an
**in-process fake `database/sql` driver**, so it reaches **100% coverage with no
live MySQL server**. That is what keeps the qemu cross-arch and Windows lanes
green. An optional **live oracle** (`TestLiveOracle`) round-trips a typed row
against a real server when `MYSQL2_TEST_DSN` is set (a go-sql-driver DSN, e.g.
`root:root@tcp(127.0.0.1:3306)/test`), and skips otherwise — so it never runs in
CI.

```sh
COVERPKG=$(go list ./... | paste -sd, -)
go test -race -coverpkg="$COVERPKG" -coverprofile=cover.out ./...
go tool cover -func=cover.out | tail -1   # 100.0%

# Optional: round-trip against a real MySQL.
MYSQL2_TEST_DSN='root:root@tcp(127.0.0.1:3306)/test' go test -run TestLiveOracle ./...
```

## License

BSD-3-Clause — see [LICENSE](LICENSE). Copyright the go-ruby-mysql/mysql authors.

## WebAssembly

Being pure Go (CGO=0), this library also compiles to **WebAssembly** — both
`GOOS=js GOARCH=wasm` (browser / Node.js) and `GOOS=wasip1 GOARCH=wasm` (WASI).
CI builds both targets on every push, alongside the six 64-bit native/qemu arches.

```sh
GOOS=js     GOARCH=wasm go build ./...   # browser / Node
GOOS=wasip1 GOARCH=wasm go build ./...   # WASI (wasmtime, wasmer, wasmedge, …)
```
