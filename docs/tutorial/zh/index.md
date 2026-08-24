# sqlk

<div class="tags-container">
  <span class="tag">Sql Server</span>
  <span class="tag">PostgreSql</span>
  <span class="tag">MySql</span>
  <span class="tag">Oracle</span>
  <span class="tag">SQLite</span>
</div>

## 介绍

一个优雅的查询构建器与执行器,帮你以可预期的方式处理 SQL 查询。

sqlk 是 [SqlKata](https://github.com/sqlkata/querybuilder) 的 Go 移植:单一 fluent 的 `Query` 类型承载全部动词(select / insert / update / delete),编译器按五种方言把它编译为参数化 SQL。英文版教程见 [`docs/tutorial/en/`](../en/index.md)。

库使用参数绑定技术保护应用免受 SQL 注入攻击,作为绑定传入的字符串无需清洗。

除注入防护外,这项技术还能让数据库引擎缓存并复用同一查询计划(即使参数变化),从而加速查询执行。

```go
posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts").
    Where("Likes", ">", 10).
    WhereIn("Lang", "en", "fr").
    WhereNotNull("AuthorId").
    OrderByDesc("Date").
    Select("Id", "Title"))
```

```sql
SELECT "Id", "Title" FROM "Posts" WHERE "Likes" > ? AND "Lang" IN (?, ?) AND "AuthorId" IS NOT NULL ORDER BY "Date" DESC
```

## 安装

sqlk 是单一 Go 模块,执行层已包含在内。

```
go get github.com/aiongo/sqlk
```

> **Note:** 库只依赖 `database/sql` 与 [sqlx](https://github.com/jmoiron/sqlx),不绑定任何数据库驱动,由你的应用注册所需驱动(教程测试使用 `modernc.org/sqlite`)。

## 快速上手

```go
import (
    "database/sql"

    "github.com/jmoiron/sqlx"
    _ "github.com/mattn/go-sqlite3" // 你的数据库驱动,由应用注册

    "github.com/aiongo/sqlk"
    "github.com/aiongo/sqlk/compiler"
    "github.com/aiongo/sqlk/exec"
)

// 建立连接并选择编译器
sqlDB, err := sql.Open("sqlite3", "mydatabase.db")
sqlxDB := sqlx.NewDb(sqlDB, "sqlite3")
db := exec.New(sqlxDB, compiler.NewSqlite())

// 此后即可构建并执行查询
post, err := db.First[Post](ctx, sqlk.NewQuery().From("Users").
    WhereEq("Id", 1).WhereEq("Status", "Active"))
```

SQL 输出

```sql
SELECT * FROM "Users" WHERE "Id" = ? AND "Status" = ? LIMIT ?
```

其中占位符分别绑定 `1`、`"Active"`、`1`(`First` 隐式附加 `Limit(1)`)。

## 仅编译示例

如果不需要执行查询,可以用 sqlk 把查询构建并编译为 SQL 字符串与有序参数列表,这里完全不需要连接实例。

最简单的起点是 `sqlk.NewQuery()`,随后链式调用 `From` 等动词。

```go
import (
    "github.com/aiongo/sqlk"
    "github.com/aiongo/sqlk/compiler"
)

// 创建 Sql Server 编译器
comp := compiler.NewSqlserver()

query := sqlk.NewQuery().From("Users").WhereEq("Id", 1).WhereEq("Status", "Active")

res, err := comp.Compile(query)

sql := res.SQL
args := res.Args // [1, "Active"]
```

它将生成如下 SQL 字符串

```sql
SELECT * FROM [Users] WHERE [Id] = ? AND [Status] = ?
```

## 本文档的约定

- 编译产物统一使用 `?` 占位符;绑定参数重要时,SQL 代码块之后附参数列表(`args: [...]`)。
- 除特别说明外,示例展示 **Sql Server** 编译器的输出;方言间输出不同的查询会分别列出各方言。
- 构建器可变,且每个动词都返回查询自身,因此调用可以自由串联;C# 的 `Where(column, value)` 重载式简写在 Go 侧变为命名变体(`WhereEq`、`JoinEq` 等,Go 没有缺省参数),`Or` / `Not` 前缀也逐一拼写(`OrWhereNull`、`WhereNotIn` 等)。
- 执行层示例的变量名按层次统一:`sqlDB` 为 `sql.Open` 产出的原生 `*sql.DB`,`sqlxDB` 为 `*sqlx.DB` 连接,`db` 为 `*exec.DB` 执行句柄。
- 本教程的每个示例都由 [`test/tutorial/`](../../../test/tutorial/) 中的测试验证;库行为变更时,那些测试先失败。
