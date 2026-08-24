# sqlk

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.27-00ADD8.svg)](go.mod)

**Go 的 SQL 查询构建器,灵感来自 [SqlKata](https://github.com/sqlkata/querybuilder)。**

[English](README.md) | **简体中文**

sqlk 提供单一 fluent `Query` 类型承载全部动词(select / insert / update / delete)、按方言把查询编译为参数化 SQL 的编译器、基于 [sqlx](https://github.com/jmoiron/sqlx) 的轻量执行层,以及面向不可信调用方的 JSON 查询线协议,全部在同一套风格下协同:

```go
posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts").
    Where("Likes", ">", 10).
    WhereIn("Lang", "en", "fr").
    WhereNotNull("AuthorId").
    OrderByDesc("Date").
    Select("Id", "Title"))
```

所有值都以参数绑定,没有字符串拼接,也没有 SQL 注入面。

## 安装

```
go get github.com/aiongo/sqlk
```

库本身只依赖 `database/sql` 与 [sqlx](https://github.com/jmoiron/sqlx),不绑定任何数据库驱动,由你的应用注册所需驱动(本项目的测试使用 `modernc.org/sqlite`)。

## 无连接的构建与编译

构建与执行严格分离。构建 `Query`,交给方言编译器,得到占位符 SQL 与有序参数列表:

```go
import (
    "github.com/aiongo/sqlk"
    "github.com/aiongo/sqlk/compiler"
)

query := sqlk.NewQuery().From("Posts").
    Where("Likes", ">", 10).
    WhereIn("Lang", "en", "fr").
    WhereNotNull("AuthorId").
    OrderByDesc("Date").
    Select("Id", "Title")

res, err := compiler.NewPostgres().Compile(query)

res.SQL  // SELECT "Id", "Title" FROM "Posts" WHERE "Likes" > ? AND "Lang" IN (?, ?) AND "AuthorId" IS NOT NULL ORDER BY "Date" DESC
res.Args // [10, "en", "fr"]
```

编译器覆盖 Sql Server、PostgreSQL、MySql、Oracle、SQLite 五种方言(`compiler.NewSqlserver`、`NewPostgres`、`NewMysql`、`NewOracle`、`NewSqlite`);`compiler.New()` 为 ANSI 风格的基础编译器。

## 执行查询

`exec` 包把(连接 + 编译器)封装为一个带泛型扫描方法的执行句柄。DB 与事务句柄暴露完全一致的 API,所有方法首参为 `context.Context`:

```go
import (
    "github.com/jmoiron/sqlx"
    _ "modernc.org/sqlite" // 你的数据库驱动,由应用注册

    "github.com/aiongo/sqlk/compiler"
    "github.com/aiongo/sqlk/exec"
)

sqlxDB := sqlx.NewDb(sqlDB, "sqlite")
db := exec.New(sqlxDB, compiler.NewSqlite())

// 扫描进你的类型 —— Get[T]、First[T]、FirstOrDefault[T]、
// Paginate[T]、Chunk[T]、Exists、Count[T]、Sum[T] 等
posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts").
    WhereEq("Lang", "en").
    OrderByDesc("Date").
    Limit(10))

// 写路径 —— Exec 与 InsertGetId;Increment / Decrement 是查询动词,走同一条 Exec 路径
id, err := db.InsertGetId[int64](ctx, sqlk.NewQuery().From("Posts"),
    sqlk.Record{"Title": "New Post", "Likes": 0, "Lang": "en", "Date": "2024-02-01"})
```

```go
tx, err := db.Begin(ctx) // 事务内 API 完全一致
```

## 来自不可信调用方的 JSON 查询(`qdata`)

`qdata` 包是 JSON 查询线协议的 Go 侧形态:外部调用方以 JSON 描述「要什么」,库把它转换为 `Query`,绝不直接产出 SQL。方言由你选择,`Hook` 让你在每个值边界做字段白名单检查:

```go
import (
    "encoding/json"

    "github.com/aiongo/sqlk/compiler"
    "github.com/aiongo/sqlk/qdata"
)

var q qdata.QData
if err := json.Unmarshal(payload, &q); err != nil {
    return err
}
query, err := q.ToQuery(nil) // nil hook = 不做拦截
if err != nil {
    return err // 校验问题聚合返回,可用 errors.Is 判别
}
res, err := compiler.NewPostgres().Compile(query)

res.SQL  // SELECT "Id", "Title" FROM "Posts" WHERE "Lang" = ? AND "Title" like ? ORDER BY Date DESC LIMIT ?
res.Args // ["en", "Go%", 10]
```

## 特性

- 单一 fluent 构建器:单一 `Query` 类型承载 select / insert / update / delete 与 Count/Sum/Avg/Min/Max 聚合形态;CTE(`With`)、集合运算(`Union` / `Intersect` / `Except`)、嵌套条件组、子查询、引擎作用域、查询变量、`When`/`Clone` 辅助。
- 五种方言:Sql Server、PostgreSQL、MySql、Oracle、SQLite。标识符包裹、分页、取自增 ID 等语义以 SqlKata 各方言编译器为基准。
- 注入安全是构造出来的:操作符经白名单校验、值全部参数绑定、标识符由编译器包裹;唯一的逃生口(`UnsafeLiteral`)显式且醒目。
- 基于 sqlx 的执行层:泛型扫描(`Get[T]`、`First[T]`、`Paginate[T]`、`Chunk[T]`、标量聚合)、`InsertGetId`、DB/Tx 同构句柄、`context.Context` 贯穿、可选编译日志。
- JSON 线协议:`qdata` 提供 16 个操作符码、约定关联 JOIN、聚合校验错误与 `Hook` 安全检查点。

## 文档

教程覆盖构建器、编译器、执行层与线协议的每一部分,所有示例均由测试验证:

- [中文教程](docs/tutorial/zh/index.md)
- [English tutorial](docs/tutorial/en/index.md)

全部测试离线可跑,无需数据库服务:

```
go test ./...
```

## 致谢

- [SqlKata](https://github.com/sqlkata/querybuilder)(MIT 协议):本项目灵感来源与移植基准的 C# 查询构建器。其能力面、fluent 风格与各方言编译语义是 sqlk 遵循的基线。
- [goqu](https://github.com/doug-martin/goqu)(MIT 协议):老牌 Go SQL 构建器。sqlk 刻意保留 SqlKata 的 fluent 单查询风格而非 goqu 的 Dataset 分体风格,但其方言断言测试影响了本项目的测试套件。
- [sqlx](https://github.com/jmoiron/sqlx)(MIT 协议):执行层的基础。

## 许可证

[MIT](LICENSE) — Copyright (c) 2026 AIOnGo
