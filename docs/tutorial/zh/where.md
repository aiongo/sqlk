# Where(过滤)
sqlk 提供大量实用的方法,让编写 `Where` 条件变得容易。

所有方法都有 `Or` 与 `Not` 变体:`OrWhereNull` 以布尔 `OR` 连接条件,`WhereNotNull` / `OrWhereNotNull` 取反条件。Go 没有缺省参数,C# 的重载式简写在 Go 侧变为命名变体(`WhereEq` 即操作符为 `=` 的 `Where`)。

## 基础 Where

`WhereEq` 动词是等值比较的简写,以下两种写法完全等价。

```go
sqlk.NewQuery().From("Posts").WhereEq("Id", 10)

// 显式操作符形态
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

> **Note:** `WhereNot`、`OrWhere`、`OrWhereNot` 同理。操作符在编译期经白名单校验——见[编译器](compilers.md)。

## 多字段
想按多个字段过滤时,传入表达「列/值」的 map。列按字典序输出,保证编译产物确定(Go map 迭代顺序不定;AND 连接下列序不影响语义)。

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

## WhereNull、WhereTrue 与 WhereFalse
按 `NULL`、布尔 `true` 与布尔 `false` 过滤。

```go
sqlk.NewQuery().From("Users").WhereFalse("IsActive").OrWhereNull("LastActivityDate")
```

```sql
SELECT * FROM [Users] WHERE [IsActive] = cast(0 as bit) OR [LastActivityDate] IS NULL
```

> **Note:** 以上方法把值以字面量写进 SQL、不走参数绑定(布尔字面量按方言,如 Sql Server 的 `cast(0 as bit)`)。

## 子查询

`WhereSub` 让子查询作为整体与值比较:子查询在前,操作符与绑定值在后。

```go
// 可用库存(按出入库流水汇总)低于 10 的商品
sold := sqlk.NewQuery().From("OrderItems").
    WhereColumns("OrderItems.ProductId", "=", "Products.Id").
    Sum("Quantity")

query := sqlk.NewQuery().From("Products").WhereSub(sold, "<", 10)
```

```sql
SELECT * FROM [Products] WHERE (SELECT SUM([Quantity]) AS [sum] FROM [OrderItems] WHERE [OrderItems].[ProductId] = [Products].[Id]) < ?
```

args: `[10]`

> **Note:** 子查询应返回单个标量单元用于比较,必要时自行设置 `Limit(1)` 且只投影一列

## 嵌套条件与分组
把条件包进 `WhereGroup` 回调即可分组。

```go
sqlk.NewQuery().From("Posts").WhereGroup(func(q *sqlk.Query) *sqlk.Query {
    return q.WhereFalse("IsPublished").OrWhereEq("CommentsCount", 0)
})
```

```sql
SELECT * FROM [Posts] WHERE ([IsPublished] = cast(0 as bit) OR [CommentsCount] = ?)
```

args: `[0]`

`OrWhereGroup`、`WhereNotGroup`、`OrWhereNotGroup` 分别连接/取反条件组。

## 列与列比较
想比较两个列时使用此动词。

```go
sqlk.NewQuery().From("Posts").WhereColumns("Upvotes", ">", "Downvotes")
```

```sql
SELECT * FROM [Posts] WHERE [Upvotes] > [Downvotes]
```

## 区间(Between)

```go
sqlk.NewQuery().From("Posts").WhereBetween("Score", 10, 20)
```

```sql
SELECT * FROM [Posts] WHERE [Score] BETWEEN ? AND ?
```

args: `[10, 20]`

`WhereNotBetween` 取反区间,`Or…` 变体以 OR 连接。

## Where In
以变参列表传值,生成 SQL 的 `WHERE IN` 条件。
```go
sqlk.NewQuery().From("Posts").WhereNotIn("AuthorId", 1, 2, 3, 4, 5)
```

```sql
SELECT * FROM [Posts] WHERE [AuthorId] NOT IN (?, ?, ?, ?, ?)
```

args: `[1, 2, 3, 4, 5]`

用 `WhereNotInSub` 传入 `*Query`,以子查询为集合

```go
blocked := sqlk.NewQuery().From("Authors").WhereEq("Status", "blocked").Select("Id")

sqlk.NewQuery().From("Posts").WhereNotInSub("AuthorId", blocked)
```

```sql
SELECT * FROM [Posts] WHERE [AuthorId] NOT IN (SELECT [Id] FROM [Authors] WHERE [Status] = ?)
```

args: `["blocked"]`

> **Note:** 子查询应只返回一列

## Where Exists

选出至少有一条评论的文章。

```go
sqlk.NewQuery().From("Posts").WhereExists(
    sqlk.NewQuery().From("Comments").WhereColumns("Comments.PostId", "=", "Posts.Id"),
)
```

Sql Server 中
```sql
SELECT * FROM [Posts] WHERE EXISTS (SELECT 1 FROM [Comments] WHERE [Comments].[PostId] = [Posts].[Id])
```

PostgreSql 中

```sql
SELECT * FROM "Posts" WHERE EXISTS (SELECT 1 FROM "Comments" WHERE "Comments"."PostId" = "Posts"."Id")
```

sqlk 会省略 `EXISTS` 子查询的投影列、改为常量 `1`,以在所有方言上提供一致行为。

## Where Raw
`WhereRaw` 动词允许你写以上方法都不支持的内容,给你最大的灵活性。


```go
sqlk.NewQuery().From("Posts").WhereRaw("lower(Title) = ?", "sql")
```

```sql
SELECT * FROM [Posts] WHERE lower(Title) = ?
```

args: `["sql"]`

有时用引擎标识符包裹表/列是有用的,对 PostgreSql 这类大小写敏感的数据库尤其如此;把字符串包在 `[` 与 `]` 之间,sqlk 会替换为对应方言的标识符。

```go
sqlk.NewQuery().From("Posts").WhereRaw("lower([Title]) = ?", "sql")
```

Sql Server 中
```sql
SELECT * FROM [Posts] WHERE lower([Title]) = ?
```

PostgreSql 中
```sql
SELECT * FROM "Posts" WHERE lower("Title") = ?
```
