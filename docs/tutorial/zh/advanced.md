# 高级方法

## 条件语句

有时你只想在特定条件成立时执行部分构建,这时用 `When(condition, fn)` 动词;反相分支是 `WhenNot`。

```go
query := sqlk.NewQuery().From("Transactions")

amount := 100

query.When(amount > 0,
        func(q *sqlk.Query) *sqlk.Query { return q.Select("Debit as Amount") }).
    WhenNot(amount > 0,
        func(q *sqlk.Query) *sqlk.Query { return q.Select("Credit as Amount") })
```

等价于

```go
query := sqlk.NewQuery().From("Transactions")

if amount > 0 {
    query.Select("Debit as Amount")
} else {
    query.Select("Credit as Amount")
}
```

当然,它可以构建查询的任何部分。

## Clone

`Query` 实例是可变的;从共享查询继续链式调用会改动它自己。想从基础查询派生互不影响的变体,用 `Clone` 动词:对全部子句(含内嵌子查询)做深拷贝。

```go
baseQuery := sqlk.NewQuery().Select("Id", "Name").Limit(10).OrderBy("Date")

posts := baseQuery.Clone().From("Posts")
authors := baseQuery.Clone().From("Authors").Limit(100) // 覆盖 limit 值
sites := baseQuery.Clone().From("Sites")
```

## 引擎专属查询

用 `For(engine, fn)` 动词针对特定引擎调优查询。

当某些原生函数只在个别厂商提供时,这很有用。`fn` 期间构建的一切只对该引擎的编译器可见;engine 代码即方言名(`sqlserver`、`postgres`、`mysql`、`oracle`、`sqlite`)。

### 类型转换示例

```go
query := sqlk.NewQuery().From("Posts").
    Select("Id", "Title").
    For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.SelectRaw("[Date]::date") }).
    For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.SelectRaw("CAST([Date] as DATE)") })
```

Sql Server 中

```sql
SELECT [Id], [Title], CAST([Date] as DATE) FROM [Posts]
```

PostgreSql 中

```sql
SELECT "Id", "Title", "Date"::date FROM "Posts"
```

本例中 MySql 不受影响

```sql
SELECT `Id`, `Title` FROM `Posts`
```

### 生成日期序列示例

另一个例子是在两个日期之间生成日期序列:PostgreSql 用 `generate_series`,Sql Server 用递归 CTE。


```go
from, to := "2017-08-23", "2017-08-28"

query := sqlk.NewQuery().
    For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query {
        // 这里写的一切只对 postgres 编译器可见
        return q.FromRaw("generate_series ( ?::timestamp, ?::timestamp, '1 day'::interval) dates", from, to).
            SelectRaw("dates::date as date")
    }).
    For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query {
        // 这里写的一切只对 sqlserver 编译器可见
        return q.WithRaw("range",
            "SELECT CAST(? AS DATETIME) 'date' UNION ALL SELECT DATEADD(dd, 1, t.date) FROM range t WHERE DATEADD(dd, 1, t.date) <= ?",
            from, to).
            From("range")
    })
```

虽然相当复杂,别担心——暂时只关注概念即可。

输出如下:

Sql Server 中

```sql
WITH [range] AS (SELECT CAST(? AS DATETIME) 'date' UNION ALL SELECT DATEADD(dd, 1, t.date) FROM range t WHERE DATEADD(dd, 1, t.date) <= ?)
SELECT * FROM [range]
```

args: `["2017-08-23", "2017-08-28"]`

PostgreSql 中

```sql
SELECT dates::date as date FROM generate_series ( ?::timestamp, ?::timestamp, '1 day'::interval) dates
```

回调里当然可以使用任何动词。

## Comment

`Comment` 动词给语句冠以数据库侧注释,便于把慢查询追回来源。

```go
sqlk.NewQuery().From("Users").Comment("trace: load users").Limit(10)
```

```sql
/* trace: load users */ SELECT TOP (?) * FROM [Users]
```

## 查询变量(Define / Variable)

`Define` 在查询上声明一个命名值;`sqlk.NewVariable(name)` 在任意值位置引用它。编译器先查本查询的定义,再沿父查询链向上查找,把解析到的值按普通参数绑定;全链未定义的引用在编译期被拒绝。

```go
since := time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)

sqlk.NewQuery().From("Posts").
    Define("since", since).
    WhereDate("CreatedAt", ">=", sqlk.NewVariable("since"))
```

PostgreSql 中

```sql
SELECT * FROM "Posts" WHERE "CreatedAt"::date >= ?
```

args: `[2017-08-01 00:00:00 +0000 UTC]`

## UnsafeLiteral

`sqlk.NewUnsafeLiteral(text)` 把受信文本直接内联进 SQL 而非参数绑定,这是无法参数化场景(函数调用、列名片段)的显式逃生口。绝不可喂给它用户输入。

```go
sqlk.NewQuery().From("Logs").Where("Host", "=", sqlk.NewUnsafeLiteral("HOST_NAME()"))
```

```sql
SELECT * FROM [Logs] WHERE [Host] = HOST_NAME()
```
