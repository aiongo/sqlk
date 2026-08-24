# 查询执行

`exec` 包在 [sqlx](https://github.com/jmoiron/sqlx) 之上提供轻量的查询执行:泛型扫描到结构体、分页、分块遍历与写动词,`context.Context` 全程贯穿。

## 安装数据库提供者

库不绑定任何数据库驱动——注册你的应用所需的驱动,把得到的 `*sql.DB` 交给 sqlx 即可。

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

测试用途的内存库则是

```go
sqlDB, err := sql.Open("sqlite", ":memory:")
```
