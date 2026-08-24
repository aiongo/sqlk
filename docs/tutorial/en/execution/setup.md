# Setup Your Project

To execute queries, create an execution handle from (connection + compiler): `exec.New(sqlxDB, compiler)` binds a `*sqlx.DB`, `exec.NewTx(tx, compiler)` binds a `*sqlx.Tx`. Both produce the exact same API.

## The DB handle

```go
import (
    "database/sql"

    "github.com/jmoiron/sqlx"
    _ "github.com/go-sql-driver/mysql"

    "github.com/aiongo/sqlk"
    "github.com/aiongo/sqlk/compiler"
    "github.com/aiongo/sqlk/exec"
)

sqlDB, err := sql.Open("mysql", "user:secret@tcp(localhost:3306)/Users")
sqlxDB := sqlx.NewDb(sqlDB, "mysql")
db := exec.New(sqlxDB, compiler.NewMysql())

users, err := db.Get[User](ctx, sqlk.NewQuery().From("Users").Limit(10))
```

Queries are built with the root package's `sqlk.NewQuery()` exactly as in the builder chapters — building and execution are strictly separated, there is no executable-query subclass.

## Transactions

`Begin` opens a transaction and returns a handle with the same API as the DB handle (compiler and logger are inherited):

```go
tx, err := db.Begin(ctx)
if err != nil {
    return err
}
defer tx.Rollback()

if _, err := tx.Exec(ctx, q1); err != nil {
    return err
}
if _, err := tx.Exec(ctx, q2); err != nil {
    return err
}
return tx.Commit()
```

## The Executor: writing code that works inside and outside a transaction

Both handles embed the same `*exec.Executor`. Take it as a parameter and your data-access functions switch between the two without changes:

```go
func loadCars(ctx context.Context, x *exec.Executor) ([]Car, error) {
    return x.Get[Car](ctx, sqlk.NewQuery().From("Cars"))
}

cars, err := loadCars(ctx, db.Executor) // db *exec.DB
cars, err = loadCars(ctx, tx.Executor)  // tx *exec.Tx
```

If you manage transactions yourself (e.g. with specific isolation options), open them with sqlx's `BeginTxx` and wrap with `exec.NewTx`.

## Wiring into your dependency container

There is nothing framework-specific to register: `exec.New` is a plain constructor. Register the resulting `*exec.DB` as a singleton — it is safe for concurrent use — and hand it the compiler of your database.

```go
func NewDB() (*exec.DB, func(), error) {
    sqlDB, err := sql.Open("mysql", "user:secret@tcp(localhost:3306)/Users")
    if err != nil {
        return nil, nil, err
    }
    sqlxDB := sqlx.NewDb(sqlDB, "mysql")
    db := exec.New(sqlxDB, compiler.NewMysql())
    return db, func() { sqlDB.Close() }, nil
}
```
