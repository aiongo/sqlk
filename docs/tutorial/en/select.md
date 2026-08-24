# Select

## Column
Select a single or many columns

```go
sqlk.NewQuery().From("Posts").Select("Id", "Title", "CreatedAt as Date")
```

```sql
SELECT [Id], [Title], [CreatedAt] AS [Date] FROM [Posts]
```

> **Note:** You can use the **as** keyword to alias a column in the select list

## Sub query
Select from a sub query

```go
countQuery := sqlk.NewQuery().From("Comments").WhereColumns("Comments.PostId", "=", "Posts.Id").Count()

query := sqlk.NewQuery().From("Posts").Select("Id").SelectSub(countQuery, "CommentsCount")
```

```sql
SELECT [Id], (SELECT COUNT(*) AS [count] FROM [Comments] WHERE [Comments].[PostId] = [Posts].[Id]) AS [CommentsCount] FROM [Posts]
```

`Count` / `Sum` / `Avg` / `Min` / `Max` rewrite the query into an aggregate form; `SelectSub` embeds such a query (or any query) as a projection column with an alias.

## Raw
Your friend when you need the full freedom

```go
sqlk.NewQuery().From("Posts").Select("Id").SelectRaw("count(1) over(partition by AuthorId) as PostsByAuthor")
```

```sql
SELECT [Id], count(1) over(partition by AuthorId) as PostsByAuthor FROM [Posts]
```

## Identify columns and tables inside Raw
You can wrap your identifier inside `[` and `]` so they get recognized by sqlk as an identifier, so we can rewrite the same example above as


```go
sqlk.NewQuery().From("Posts").Select("Id").SelectRaw("count(1) over(partition by [AuthorId]) as [PostsByAuthor]")
```

Now `AuthorId` and `PostsByAuthor` get wrapped with the compiler identifiers, this is helpful especially for case sensitive engines like PostgreSql.

In Sql Server

```sql
SELECT [Id], count(1) over(partition by [AuthorId]) as [PostsByAuthor] FROM [Posts]
```

In PostgreSql

```sql
SELECT "Id", count(1) over(partition by "AuthorId") as "PostsByAuthor" FROM "Posts"
```

In MySql

```sql
SELECT `Id`, count(1) over(partition by `AuthorId`) as `PostsByAuthor` FROM `Posts`
```

`{` and `}` work as identifier markers too.

## Selecting many columns
SqlKata's braces expansion (`Users.{Id, Name, LastName}`) has no Go equivalent here; pass the columns as a plain variadic list instead, which reads the same:

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

## Distinct
Call `Distinct` to eliminate duplicate rows from the projection.

```go
sqlk.NewQuery().From("Posts").Distinct().Select("AuthorId")
```

```sql
SELECT DISTINCT [AuthorId] FROM [Posts]
```
