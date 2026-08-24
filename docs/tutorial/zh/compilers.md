# 编译器

编译器负责把 `Query` 实例转换为可由数据库引擎直接执行的 SQL 字符串。

## 支持的编译器

sqlk 原生支持以下方言,各方言一个构造函数:

| 编译器 | 构造函数 | 标识符 | 引擎代码(`For`) |
| --- | --- | --- | --- |
| Sql Server | `compiler.NewSqlserver()` | `[Name]` | `sqlserver` |
| PostgreSql | `compiler.NewPostgres()` | `"Name"` | `postgres` |
| MySql | `compiler.NewMysql()` | `` `Name` `` | `mysql` |
| Oracle | `compiler.NewOracle()` / `compiler.NewOracleLegacy()` | `"Name"` | `oracle` |
| SQLite | `compiler.NewSqlite()` | `"Name"` | `sqlite` |
| 基础 | `compiler.New()` | `"Name"` | — |

## 值得注意的差异

理论上不同编译器的输出应当相似,80% 的场景确实如此;但在一些边界场景,输出可能差异很大。比如看看各方言如何编译 `Limit` 与 `Offset` 子句

```go
sqlk.NewQuery().From("Posts").Limit(10).Offset(20)
```

Sql Server
```sql
SELECT * FROM [Posts] ORDER BY (SELECT 0) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY
```

args: `[20, 10]`

MySql
```sql
SELECT * FROM `Posts` LIMIT ? OFFSET ?
```

args: `[10, 20]`

PostgreSql / SQLite

```sql
SELECT * FROM "Posts" LIMIT ? OFFSET ?
```

Oracle(12c+)

```sql
SELECT * FROM "Posts" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY
```

本文档默认只展示 Sql Server 编译器编译的查询,输出不同的场景另行说明。

## 支持旧版 Oracle(pre-12c)

用 `compiler.NewOracleLegacy()` 面向 12c 之前的 Oracle;分页以 `ROWNUM` 条件包装整条 SELECT 表达。

> **Note:** SqlKata 面向旧版 Sql Server 的 `UseLegacyPagination` 开关(< 2012 的 ROW_NUMBER 包装)刻意不在 sqlk 能力面内,legacy 构造函数仅 Oracle 提供。见 [Limit 与 Offset](limit.md)。

```go
comp := compiler.NewOracleLegacy()
```

对上例的 `Limit(10).Offset(20)`:

```sql
SELECT * FROM (SELECT "results_wrapper".*, ROWNUM "row_num" FROM (SELECT * FROM "Posts") "results_wrapper" WHERE ROWNUM <= ?) WHERE "row_num" > ?
```

args: `[30, 20]`

## 操作符白名单

`Where` / `Having` 条件使用的每个操作符都在编译期经白名单校验:未知操作符被拒绝,关上一扇经典的 SQL 注入门。用 `Whitelist` 给编译器实例扩展你自己的安全操作符(内置集合覆盖 `=`、`<`、`>`、`<=`、`>=`、`<>`、`!=`、`<=>`、`like`/`ilike`/`rlike`/`regexp` 族及其否定形态):

```go
comp := compiler.NewPostgres().Whitelist("&&", "||")

sqlk.NewQuery().From("Trips").Where("Tags", "&&", []string{"family", "outdoor"})
```

```sql
SELECT * FROM "Trips" WHERE "Tags" && ?
```

既非内置也未白名单的操作符使编译失败,返回可判别的错误:

```go
_, err := compiler.NewPostgres().Compile(
    sqlk.NewQuery().From("Trips").Where("Tags", "&&", []string{"family"}))
errors.Is(err, compiler.ErrOperatorNotAllowed) // true
```

> **Note:** `Whitelist` 只作用于它所在的编译器实例;其他编译器(包括同方言的其他实例)保持内置集合。
