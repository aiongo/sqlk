# Join

## Basic Join

To apply an inner join use the `JoinEq` verb (the `Join` verb spells the operator out: `Join(table, first, op, second)`)

```go
query := sqlk.NewQuery().From("Posts").JoinEq("Authors", "Authors.Id", "Posts.AuthorId")
```

The verbs `LeftJoinEq`, `RightJoinEq` have the same signature; `CrossJoin` takes only the table (cross joins carry no ON condition).

```sql
SELECT * FROM [Posts]
INNER JOIN [Authors] ON [Authors].[Id] = [Posts].[AuthorId]
```

The 3rd parameter of `Join` is the join operator and defaults to `=` in the `…Eq` shorthands, pass any other operator to override it.

```go
query := sqlk.NewQuery().From("Posts").Join("Comments", "Comments.Date", ">", "Posts.Date")
```

```sql
SELECT * FROM [Posts]
INNER JOIN [Comments] ON [Comments].[Date] > [Posts].[Date]
```

## Join with a Sub Query

```go
topComments := sqlk.NewQuery().From("Comments").OrderByDesc("Likes").Limit(10)

posts := sqlk.NewQuery().From("Posts").LeftJoinSub(
    topComments.As("TopComments"), // Don't forget to alias the sub query
    func(j *sqlk.Join) *sqlk.Join { return j.On("TopComments.PostId", "=", "Posts.Id") },
)
```

```sql
SELECT * FROM [Posts]
LEFT JOIN (SELECT TOP (?) * FROM [Comments] ORDER BY [Likes] DESC) AS [TopComments] ON [TopComments].[PostId] = [Posts].[Id]
```

args: `[10]`

> **Warning:** Always alias your sub queries with the `As` method

## Advanced conditions

In some advanced cases you may need to apply some constraints on the join clause. The callback receives a `*sqlk.Join` scope: `On` / `OrOn` / `OnNot` append column-to-column conditions, and the whole `Where` family is available for anything else.

```go
comments := sqlk.NewQuery().From("Comments").LeftJoinOn("Posts", func(j *sqlk.Join) *sqlk.Join {
    return j.On("Posts.Id", "=", "Comments.Id").WhereNotNull("Comments.AuthorId")
})
```

```sql
SELECT * FROM [Comments]
LEFT JOIN [Posts] ON [Posts].[Id] = [Comments].[Id] AND [Comments].[AuthorId] IS NOT NULL
```

## Cross Join

```go
sqlk.NewQuery().From("Sizes").CrossJoin("Colors")
```

```sql
SELECT * FROM [Sizes]
CROSS JOIN [Colors]
```
