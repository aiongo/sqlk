# Common Table Expression
A common table expression (CTE) can be thought of as a temporary result set.

## Some notes about the CTE approach

- CTEs are easier to read
Nested queries are hard to inspect: you have to understand all the nested queries first, then go up to the main query, while in the CTE approach the read process is more natural, from top to bottom.

- Ability to overload existing tables
You can overload existing tables; this may help for testing purposes.

In SQL, a CTE is represented as a `with` clause.

## With
To add a CTE to your query simply use the `With` verb.

```go
activePosts := sqlk.NewQuery().From("Comments").
    Select("PostId").
    SelectRaw("count(1) as Count").
    GroupBy("PostId").
    HavingRaw("count(1) > 100")

query := sqlk.NewQuery().From("Posts").
    With("ActivePosts", activePosts). // now you can consider ActivePosts as a regular table in the database
    JoinEq("ActivePosts", "ActivePosts.PostId", "Posts.Id").
    Select("Posts.*", "ActivePosts.Count")
```

```sql
WITH [ActivePosts] AS (SELECT [PostId], count(1) as Count FROM [Comments] GROUP BY [PostId] HAVING count(1) > 100)
SELECT [Posts].*, [ActivePosts].[Count] FROM [Posts]
INNER JOIN [ActivePosts] ON [ActivePosts].[PostId] = [Posts].[Id]
```

## WithRaw
You can use the `WithRaw` verb if you want to pass a raw SQL expression.

```go
query := sqlk.NewQuery().From("Posts").
    WithRaw("ActivePosts", "select PostId, count(1) as count from Comments having count(1) > ?", 50). // now you can consider ActivePosts as a regular table in the database
    JoinEq("ActivePosts", "ActivePosts.PostId", "Posts.Id").
    Select("Posts.*", "ActivePosts.Count")
```

```sql
WITH [ActivePosts] AS (select PostId, count(1) as count from Comments having count(1) > ?)
SELECT [Posts].*, [ActivePosts].[Count] FROM [Posts]
INNER JOIN [ActivePosts] ON [ActivePosts].[PostId] = [Posts].[Id]
```

args: `[50]`

As in the example above, you can pass bindings as extra arguments.

## WithFunc and WithTable

`WithFunc` defines the CTE body with a callback instead of a prebuilt query; `WithTable` defines an ad-hoc value table from column names and value rows, compiled as constant projections joined by `UNION ALL`.

```go
query := sqlk.NewQuery().
    WithFunc("recent", func(q *sqlk.Query) *sqlk.Query {
        return q.From("Logs").WhereEq("Level", "error")
    }).
    WithTable("dates", []string{"day"}, []any{"2024-01-01"}, []any{"2024-01-02"}).
    From("recent")
```

In PostgreSql

```sql
WITH "recent" AS (SELECT * FROM "Logs" WHERE "Level" = ?),
"dates" AS (SELECT ? AS "day" UNION ALL SELECT ? AS "day")
SELECT * FROM "recent"
```

args: `["error", "2024-01-01", "2024-01-02"]`

CTEs defined anywhere in the query tree (including nested sub queries) are collected, de-duplicated and hoisted to the front of the outermost statement.
