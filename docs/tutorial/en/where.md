# Where
sqlk offers many useful methods to make it easy writing `Where` conditions.

All these methods come in `Or` and `Not` variants: `OrWhereNull` connects the condition with a boolean `OR`, and `WhereNotNull` / `OrWhereNotNull` negate the condition. Since Go has no optional or default parameters, the C# overload shorthands become named variants (`WhereEq` is `Where` with the `=` operator).

## Basic Where

The `WhereEq` verb is the shorthand for the equality operator, so these two statements are totally the same.

```go
sqlk.NewQuery().From("Posts").WhereEq("Id", 10)

// the explicit operator form
sqlk.NewQuery().From("Posts").Where("Id", "=", 10)
```

```sql
SELECT * FROM [Posts] WHERE [Id] = ?
```

args: `[10]`

```go
sqlk.NewQuery().From("Posts").WhereFalse("IsPublished").Where("Score", ">", 10)
```

```sql
SELECT * FROM [Posts] WHERE [IsPublished] = cast(0 as bit) AND [Score] > ?
```

args: `[10]`

> **Note:** The same applies to `WhereNot`, `OrWhere` and `OrWhereNot`. Operators are validated against a whitelist at compile time — see [Compilers](compilers.md).

## Multiple fields
If you want to filter your query against multiple fields, pass a map that represents col/values. Columns are emitted in sorted order so the compiled output is deterministic (Go map iteration order is undefined; under AND the order does not change the semantics).

```go
query := sqlk.NewQuery().From("Posts").WhereMap(sqlk.Record{
    "Year":         2017,
    "CategoryId":   198,
    "IsPublished":  true,
})
```

```sql
SELECT * FROM [Posts] WHERE [CategoryId] = ? AND [IsPublished] = ? AND [Year] = ?
```

args: `[198, true, 2017]`

## WhereNull, WhereTrue and WhereFalse
To filter against `NULL`, boolean `true` and boolean `false` values.

```go
sqlk.NewQuery().From("Users").WhereFalse("IsActive").OrWhereNull("LastActivityDate")
```

```sql
SELECT * FROM [Users] WHERE [IsActive] = cast(0 as bit) OR [LastActivityDate] IS NULL
```

> **Note:** the above methods put the values literally in the generated sql and do not use parameter binding (the boolean literal is dialect specific, e.g. `cast(0 as bit)` on Sql Server).

## Sub Query

`WhereSub` compares a sub query as a whole against a value: the sub query goes first, the operator and the bound value follow.

```go
// products whose available stock (summed over movements) is below 10
sold := sqlk.NewQuery().From("OrderItems").
    WhereColumns("OrderItems.ProductId", "=", "Products.Id").
    Sum("Quantity")

query := sqlk.NewQuery().From("Products").WhereSub(sold, "<", 10)
```

```sql
SELECT * FROM [Products] WHERE (SELECT SUM([Quantity]) AS [sum] FROM [OrderItems] WHERE [OrderItems].[ProductId] = [Products].[Id]) < ?
```

args: `[10]`

> **Note:** the sub query should return one scalar cell to compare with, so you may need to set `Limit(1)` and select one column if needed

## Nested conditions and Grouping
To group your conditions, wrap them inside a `WhereGroup` callback.

```go
sqlk.NewQuery().From("Posts").WhereGroup(func(q *sqlk.Query) *sqlk.Query {
    return q.WhereFalse("IsPublished").OrWhereEq("CommentsCount", 0)
})
```

```sql
SELECT * FROM [Posts] WHERE ([IsPublished] = cast(0 as bit) OR [CommentsCount] = ?)
```

args: `[0]`

`OrWhereGroup`, `WhereNotGroup` and `OrWhereNotGroup` connect / negate the group.

## Comparing two columns
Use this verb when you want to compare two columns together.

```go
sqlk.NewQuery().From("Posts").WhereColumns("Upvotes", ">", "Downvotes")
```

```sql
SELECT * FROM [Posts] WHERE [Upvotes] > [Downvotes]
```

## Between

```go
sqlk.NewQuery().From("Posts").WhereBetween("Score", 10, 20)
```

```sql
SELECT * FROM [Posts] WHERE [Score] BETWEEN ? AND ?
```

args: `[10, 20]`

`WhereNotBetween` negates the interval, the `Or…` variants connect with OR.

## Where In
Pass values as a variadic list to apply the SQL `WHERE IN` condition.
```go
sqlk.NewQuery().From("Posts").WhereNotIn("AuthorId", 1, 2, 3, 4, 5)
```

```sql
SELECT * FROM [Posts] WHERE [AuthorId] NOT IN (?, ?, ?, ?, ?)
```

args: `[1, 2, 3, 4, 5]`

You can pass a `*Query` to filter against a sub query with `WhereNotInSub`

```go
blocked := sqlk.NewQuery().From("Authors").WhereEq("Status", "blocked").Select("Id")

sqlk.NewQuery().From("Posts").WhereNotInSub("AuthorId", blocked)
```

```sql
SELECT * FROM [Posts] WHERE [AuthorId] NOT IN (SELECT [Id] FROM [Authors] WHERE [Status] = ?)
```

args: `["blocked"]`

> **Note:** The sub query should return one column

## Where Exists

To select all posts that have at least one comment.

```go
sqlk.NewQuery().From("Posts").WhereExists(
    sqlk.NewQuery().From("Comments").WhereColumns("Comments.PostId", "=", "Posts.Id"),
)
```

In Sql Server
```sql
SELECT * FROM [Posts] WHERE EXISTS (SELECT 1 FROM [Comments] WHERE [Comments].[PostId] = [Posts].[Id])
```

In PostgreSql

```sql
SELECT * FROM "Posts" WHERE EXISTS (SELECT 1 FROM "Comments" WHERE "Comments"."PostId" = "Posts"."Id")
```

sqlk optimizes the `EXISTS` query by disregarding the selected columns and projecting the constant `1` in order to provide a consistent behavior across all database engines.

## Where Raw
The `WhereRaw` verb allows you to write anything not supported by the methods above, so it will give you the maximum flexibility.


```go
sqlk.NewQuery().From("Posts").WhereRaw("lower(Title) = ?", "sql")
```

```sql
SELECT * FROM [Posts] WHERE lower(Title) = ?
```

args: `["sql"]`

Sometimes it's useful to wrap your table/columns by the engine identifier, this is helpful when the database is case sensitive like in PostgreSql, to do so just wrap your string with `[` and `]` and sqlk will put the correspondent identifiers.

```go
sqlk.NewQuery().From("Posts").WhereRaw("lower([Title]) = ?", "sql")
```

In Sql Server
```sql
SELECT * FROM [Posts] WHERE lower([Title]) = ?
```

In PostgreSql
```sql
SELECT * FROM "Posts" WHERE lower("Title") = ?
```
