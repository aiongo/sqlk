# Query Execution

The `exec` package provides an easy way to execute your queries on top of [sqlx](https://github.com/jmoiron/sqlx): scanning into structs (generically), pagination, chunked iteration and write verbs, with `context.Context` throughout.

## Installing Database Providers

The library does not bind any database driver — register the one your application needs and hand the resulting `*sql.DB` to sqlx.

### Sql Server

```sh
go get github.com/microsoft/go-mssqldb
```

### PostgreSql

```sh
go get github.com/lib/pq
```

### MySql

```sh
go get github.com/go-sql-driver/mysql
```

### SQLite

```sh
go get modernc.org/sqlite
```

```go
import (
    "database/sql"

    "github.com/jmoiron/sqlx"
    _ "modernc.org/sqlite"
)

sqlDB, err := sql.Open("sqlite", "file:mydatabase.db")
sqlxDB := sqlx.NewDb(sqlDB, "sqlite")
db := exec.New(sqlxDB, compiler.NewSqlite())
```

Or if you want an in-memory database for testing purposes

```go
sqlDB, err := sql.Open("sqlite", ":memory:")
```
