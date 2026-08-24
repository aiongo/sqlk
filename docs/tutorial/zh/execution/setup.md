# 项目搭建

执行查询前,先用(连接 + 编译器)构造执行句柄:`exec.New(sqlxDB, compiler)` 绑定 `*sqlx.DB`,`exec.NewTx(tx, compiler)` 绑定 `*sqlx.Tx`。两者产出完全一致的 API。

## DB 句柄

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

查询用根包的 `sqlk.NewQuery()` 构建,与构建器各章完全一致——构建与执行严格分离,没有「可执行查询」子类。

## 事务

`Begin` 开启事务并返回与 DB 句柄同构的句柄(编译器与日志回调随行继承):

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

## Executor:书写事务内外通用的代码

两种句柄都嵌入同一个 `*exec.Executor`。以它为参数,你的取数函数在事务内外无感切换:

```go
func loadCars(ctx context.Context, x *exec.Executor) ([]Car, error) {
    return x.Get[Car](ctx, sqlk.NewQuery().From("Cars"))
}

cars, err := loadCars(ctx, db.Executor) // db *exec.DB
cars, err = loadCars(ctx, tx.Executor)  // tx *exec.Tx
```

如果事务生命周期自行管理(如指定隔离级别),用 sqlx 的 `BeginTxx` 开启,再以 `exec.NewTx` 包裹。

## 接入依赖注入容器

没有框架相关的注册项:`exec.New` 就是普通构造函数。把产出的 `*exec.DB` 注册为单例——它并发安全——并为其配备你数据库的编译器即可。

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
