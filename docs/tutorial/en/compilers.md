# Compilers

Compilers are the component responsible for transforming a `Query` instance into a SQL string that can be executed directly by the database engine.

## Supported compilers

sqlk supports natively the following dialects, one constructor each:

| Compiler | Constructor | Identifier | Engine code (`For`) |
| --- | --- | --- | --- |
| Sql Server | `compiler.NewSqlserver()` | `[Name]` | `sqlserver` |
| PostgreSql | `compiler.NewPostgres()` | `"Name"` | `postgres` |
| MySql | `compiler.NewMysql()` | `` `Name` `` | `mysql` |
| Oracle | `compiler.NewOracle()` / `compiler.NewOracleLegacy()` | `"Name"` | `oracle` |
| SQLite | `compiler.NewSqlite()` | `"Name"` | `sqlite` |
| Base | `compiler.New()` | `"Name"` | — |

## Some noticeable differences

Theoretically the output of the different compilers should be similar; this is true for about 80% of the cases. However in some edge cases the output can be very different. For instance, take a look at how the `Limit` and `Offset` clause get compiled by each compiler

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

Oracle (12c+)

```sql
SELECT * FROM "Posts" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY
```

In this documentation, we display the queries compiled by the Sql Server compiler only, except for the queries where the output is not the same.

## Supporting Legacy Oracle (pre-12c)

Use `compiler.NewOracleLegacy()` to target Oracle versions before 12c; pagination is expressed by wrapping the whole SELECT with `ROWNUM` conditions.

> **Note:** SqlKata's `UseLegacyPagination` switch for legacy Sql Server (< 2012, ROW_NUMBER wrapping) is deliberately not part of sqlk's capability surface; only Oracle keeps a legacy constructor. See [Limit and Offset](limit.md).

```go
comp := compiler.NewOracleLegacy()
```

With `Limit(10).Offset(20)` from the example above:

```sql
SELECT * FROM (SELECT "results_wrapper".*, ROWNUM "row_num" FROM (SELECT * FROM "Posts") "results_wrapper" WHERE ROWNUM <= ?) WHERE "row_num" > ?
```

args: `[30, 20]`

## Operator whitelist

Every operator used by `Where` / `Having` conditions is validated against a whitelist at compile time: unknown operators are rejected, which closes a classic SQL-injection door. Use `Whitelist` to extend a compiler instance with your own safe operators (the built-in set covers `=`, `<`, `>`, `<=`, `>=`, `<>`, `!=`, `<=>`, the `like`/`ilike`/`rlike`/`regexp` families and their negations):

```go
comp := compiler.NewPostgres().Whitelist("&&", "||")

sqlk.NewQuery().From("Trips").Where("Tags", "&&", []string{"family", "outdoor"})
```

```sql
SELECT * FROM "Trips" WHERE "Tags" && ?
```

An operator that is neither built-in nor whitelisted fails compilation with a distinguishable error:

```go
_, err := compiler.NewPostgres().Compile(
    sqlk.NewQuery().From("Trips").Where("Tags", "&&", []string{"family"}))
errors.Is(err, compiler.ErrOperatorNotAllowed) // true
```

> **Note:** `Whitelist` only affects the compiler instance it is called on; other compilers (including other instances of the same dialect) keep the built-in set.
