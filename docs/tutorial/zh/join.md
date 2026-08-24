# Join(连接)

## 基础连接

用 `JoinEq` 动词做内连接(`Join` 动词显式给出操作符:`Join(table, first, op, second)`)

```go
query := sqlk.NewQuery().From("Posts").JoinEq("Authors", "Authors.Id", "Posts.AuthorId")
```

`LeftJoinEq`、`RightJoinEq` 签名相同;`CrossJoin` 只接收表名(交叉连接不携带 ON 条件)。

```sql
SELECT * FROM [Posts]
INNER JOIN [Authors] ON [Authors].[Id] = [Posts].[AuthorId]
```

`Join` 的第 3 参是连接操作符,在 `…Eq` 简写里缺省为 `=`;传其他操作符即可覆盖。

```go
query := sqlk.NewQuery().From("Posts").Join("Comments", "Comments.Date", ">", "Posts.Date")
```

```sql
SELECT * FROM [Posts]
INNER JOIN [Comments] ON [Comments].[Date] > [Posts].[Date]
```

## 连接子查询

```go
topComments := sqlk.NewQuery().From("Comments").OrderByDesc("Likes").Limit(10)

posts := sqlk.NewQuery().From("Posts").LeftJoinSub(
    topComments.As("TopComments"), // 别忘了给子查询起别名
    func(j *sqlk.Join) *sqlk.Join { return j.On("TopComments.PostId", "=", "Posts.Id") },
)
```

```sql
SELECT * FROM [Posts]
LEFT JOIN (SELECT TOP (?) * FROM [Comments] ORDER BY [Likes] DESC) AS [TopComments] ON [TopComments].[PostId] = [Posts].[Id]
```

args: `[10]`

> **Warning:** 永远用 `As` 方法给子查询起别名

## 高级条件

某些高级场景需要在连接子句上追加约束。回调收到一个 `*sqlk.Join` 作用域:`On` / `OrOn` / `OnNot` 追加列间条件,完整的 `Where` 方法族也可在其中使用。

```go
comments := sqlk.NewQuery().From("Comments").LeftJoinOn("Posts", func(j *sqlk.Join) *sqlk.Join {
    return j.On("Posts.Id", "=", "Comments.Id").WhereNotNull("Comments.AuthorId")
})
```

```sql
SELECT * FROM [Comments]
LEFT JOIN [Posts] ON [Posts].[Id] = [Comments].[Id] AND [Comments].[AuthorId] IS NOT NULL
```

## 交叉连接

```go
sqlk.NewQuery().From("Sizes").CrossJoin("Colors")
```

```sql
SELECT * FROM [Sizes]
CROSS JOIN [Colors]
```
