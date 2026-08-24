# Advanced methods

## Conditional Statements

Sometimes you need to do some actions only when certain conditions are met. In these cases you can use the `When(condition, fn)` verb; the inverse branch is `WhenNot`.

```go
query := sqlk.NewQuery().From("Transactions")

amount := 100

query.When(amount > 0,
        func(q *sqlk.Query) *sqlk.Query { return q.Select("Debit as Amount") }).
    WhenNot(amount > 0,
        func(q *sqlk.Query) *sqlk.Query { return q.Select("Credit as Amount") })
```

is the same as

```go
query := sqlk.NewQuery().From("Transactions")

if amount > 0 {
    query.Select("Debit as Amount")
} else {
    query.Select("Credit as Amount")
}
```

Of course you can use it to build any part of the query.

## Clone

`Query` instances are mutable; chaining from a shared query mutates it. To derive independent variants from a base query, use the `Clone` verb: a deep copy of every clause (including embedded sub queries).

```go
baseQuery := sqlk.NewQuery().Select("Id", "Name").Limit(10).OrderBy("Date")

posts := baseQuery.Clone().From("Posts")
authors := baseQuery.Clone().From("Authors").Limit(100) // override the limit value
sites := baseQuery.Clone().From("Sites")
```

## Engine specific queries

sqlk allows you to tune your queries against specific engines by using the `For(engine, fn)` verb.

This is helpful when you want to apply some native functions that are available in some vendors and not in others. Everything built inside `fn` is visible only to the compiler of that engine; the engine code is the dialect name (`sqlserver`, `postgres`, `mysql`, `oracle`, `sqlite`).

### Casting Example

```go
query := sqlk.NewQuery().From("Posts").
    Select("Id", "Title").
    For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.SelectRaw("[Date]::date") }).
    For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.SelectRaw("CAST([Date] as DATE)") })
```

In Sql Server

```sql
SELECT [Id], [Title], CAST([Date] as DATE) FROM [Posts]
```

In PostgreSql

```sql
SELECT "Id", "Title", "Date"::date FROM "Posts"
```

In this example, MySql isn't affected

```sql
SELECT `Id`, `Title` FROM `Posts`
```

### Generating date series example

Another example is to generate a date series between two given dates: you can use `generate_series` in PostgreSql, and a recursive CTE in Sql Server.


```go
from, to := "2017-08-23", "2017-08-28"

query := sqlk.NewQuery().
    For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query {
        // everything written here is available to the postgres compiler only
        return q.FromRaw("generate_series ( ?::timestamp, ?::timestamp, '1 day'::interval) dates", from, to).
            SelectRaw("dates::date as date")
    }).
    For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query {
        // everything written here is available to the sqlserver compiler only
        return q.WithRaw("range",
            "SELECT CAST(? AS DATETIME) 'date' UNION ALL SELECT DATEADD(dd, 1, t.date) FROM range t WHERE DATEADD(dd, 1, t.date) <= ?",
            from, to).
            From("range")
    })
```

Although it's quite complicated, don't worry — just focus on the concept for now.

The following will output:

In Sql Server

```sql
WITH [range] AS (SELECT CAST(? AS DATETIME) 'date' UNION ALL SELECT DATEADD(dd, 1, t.date) FROM range t WHERE DATEADD(dd, 1, t.date) <= ?)
SELECT * FROM [range]
```

args: `["2017-08-23", "2017-08-28"]`

In PostgreSql

```sql
SELECT dates::date as date FROM generate_series ( ?::timestamp, ?::timestamp, '1 day'::interval) dates
```

Of course you can use any verb you want inside these callbacks.

## Comment

The `Comment` verb prefixes the statement with a database-side comment, useful to trace slow queries back to their origin.

```go
sqlk.NewQuery().From("Users").Comment("trace: load users").Limit(10)
```

```sql
/* trace: load users */ SELECT TOP (?) * FROM [Users]
```

## Query variables (Define / Variable)

`Define` declares a named value on the query; `sqlk.NewVariable(name)` references it from any value position. The compiler resolves the reference against the query's own definitions first, then walks up the parent query chain, and binds the resolved value as an ordinary parameter. An unresolved reference is rejected at compile time.

```go
since := time.Date(2017, 8, 1, 0, 0, 0, 0, time.UTC)

sqlk.NewQuery().From("Posts").
    Define("since", since).
    WhereDate("CreatedAt", ">=", sqlk.NewVariable("since"))
```

In PostgreSql

```sql
SELECT * FROM "Posts" WHERE "CreatedAt"::date >= ?
```

args: `[2017-08-01 00:00:00 +0000 UTC]`

## UnsafeLiteral

`sqlk.NewUnsafeLiteral(text)` inlines trusted text directly into the SQL instead of binding it as a parameter, the explicit escape hatch for things that cannot be parameterized (function calls, column name fragments). Never feed it user input.

```go
sqlk.NewQuery().From("Logs").Where("Host", "=", sqlk.NewUnsafeLiteral("HOST_NAME()"))
```

```sql
SELECT * FROM [Logs] WHERE [Host] = HOST_NAME()
```
