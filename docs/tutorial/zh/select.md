# Select(投影)

## 列
选择单列或多列

```go
sqlk.NewQuery().From("Posts").Select("Id", "Title", "CreatedAt as Date")
```

```sql
SELECT [Id], [Title], [CreatedAt] AS [Date] FROM [Posts]
```

> **Note:** 投影列表里可以用 **as** 关键字给列起别名

## 子查询
把子查询作为投影列

```go
countQuery := sqlk.NewQuery().From("Comments").WhereColumns("Comments.PostId", "=", "Posts.Id").Count()

query := sqlk.NewQuery().From("Posts").Select("Id").SelectSub(countQuery, "CommentsCount")
```

```sql
SELECT [Id], (SELECT COUNT(*) AS [count] FROM [Comments] WHERE [Comments].[PostId] = [Posts].[Id]) AS [CommentsCount] FROM [Posts]
```

`Count` / `Sum` / `Avg` / `Min` / `Max` 把查询改写为聚合形态;`SelectSub` 把这类查询(或任意查询)作为带别名的投影列嵌入。

## 原生表达式
当你需要完全的自由时,它是你的朋友

```go
sqlk.NewQuery().From("Posts").Select("Id").SelectRaw("count(1) over(partition by AuthorId) as PostsByAuthor")
```

```sql
SELECT [Id], count(1) over(partition by AuthorId) as PostsByAuthor FROM [Posts]
```

## 识别原生表达式中的列与表
把标识符包在 `[` 与 `]` 之间,sqlk 就会把它识别为标识符,上面的例子可以改写为


```go
sqlk.NewQuery().From("Posts").Select("Id").SelectRaw("count(1) over(partition by [AuthorId]) as [PostsByAuthor]")
```

这样 `AuthorId` 与 `PostsByAuthor` 会被编译器的标识符包裹,对 PostgreSql 这类大小写敏感的方言尤其有用。

Sql Server 中

```sql
SELECT [Id], count(1) over(partition by [AuthorId]) as [PostsByAuthor] FROM [Posts]
```

PostgreSql 中

```sql
SELECT "Id", count(1) over(partition by "AuthorId") as "PostsByAuthor" FROM "Posts"
```

MySql 中

```sql
SELECT `Id`, count(1) over(partition by `AuthorId`) as `PostsByAuthor` FROM `Posts`
```

`{` 与 `}` 同样可以作为标识符标记。

## 选择多列
SqlKata 的花括号展开(`Users.{Id, Name, LastName}`)在 Go 侧没有对应物;直接以变参列表传列即可,可读性一致:

```go
sqlk.NewQuery().From("Users").
    JoinEq("Profiles", "Profiles.UserId", "Users.Id").
    Select(
        "Users.Id", "Users.Name", "Users.LastName",
        "Profiles.GithubUrl", "Profiles.Website", "Profiles.Stars",
    )
```

```sql
SELECT [Users].[Id], [Users].[Name], [Users].[LastName], [Profiles].[GithubUrl], [Profiles].[Website], [Profiles].[Stars] FROM [Users]
INNER JOIN [Profiles] ON [Profiles].[UserId] = [Users].[Id]
```

## 去重
调用 `Distinct` 消除投影中的重复行。

```go
sqlk.NewQuery().From("Posts").Distinct().Select("AuthorId")
```

```sql
SELECT DISTINCT [AuthorId] FROM [Posts]
```
