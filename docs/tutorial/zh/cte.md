# 公共表表达式(CTE)
公共表表达式(CTE)可以看作一个临时结果集。

## 关于 CTE 方案的一些说明

- CTE 更易读
嵌套查询难以审视:你得先理解所有内层查询,再回到主查询;CTE 方案的阅读过程更自然,自上而下。

- 可以重载既有表
重载既有表对测试很有帮助。

在 SQL 里,CTE 以 `with` 子句表达。

## With
用 `With` 动词给查询加一个 CTE。

```go
activePosts := sqlk.NewQuery().From("Comments").
    Select("PostId").
    SelectRaw("count(1) as Count").
    GroupBy("PostId").
    HavingRaw("count(1) > 100")

query := sqlk.NewQuery().From("Posts").
    With("ActivePosts", activePosts). // 此后可以把 ActivePosts 当作库里的常规表
    JoinEq("ActivePosts", "ActivePosts.PostId", "Posts.Id").
    Select("Posts.*", "ActivePosts.Count")
```

```sql
WITH [ActivePosts] AS (SELECT [PostId], count(1) as Count FROM [Comments] GROUP BY [PostId] HAVING count(1) > 100)
SELECT [Posts].*, [ActivePosts].[Count] FROM [Posts]
INNER JOIN [ActivePosts] ON [ActivePosts].[PostId] = [Posts].[Id]
```

## WithRaw
想传原生 SQL 表达式时用 `WithRaw` 动词。

```go
query := sqlk.NewQuery().From("Posts").
    WithRaw("ActivePosts", "select PostId, count(1) as count from Comments having count(1) > ?", 50). // 此后可以把 ActivePosts 当作库里的常规表
    JoinEq("ActivePosts", "ActivePosts.PostId", "Posts.Id").
    Select("Posts.*", "ActivePosts.Count")
```

```sql
WITH [ActivePosts] AS (select PostId, count(1) as count from Comments having count(1) > ?)
SELECT [Posts].*, [ActivePosts].[Count] FROM [Posts]
INNER JOIN [ActivePosts] ON [ActivePosts].[PostId] = [Posts].[Id]
```

args: `[50]`

如上例所示,绑定参数以额外参数传入。

## WithFunc 与 WithTable

`WithFunc` 以回调(而非预构建查询)定义 CTE 体;`WithTable` 以列名与值行定义 ad-hoc 值表,编译为以 `UNION ALL` 连接的常量投影。

```go
query := sqlk.NewQuery().
    WithFunc("recent", func(q *sqlk.Query) *sqlk.Query {
        return q.From("Logs").WhereEq("Level", "error")
    }).
    WithTable("dates", []string{"day"}, []any{"2024-01-01"}, []any{"2024-01-02"}).
    From("recent")
```

PostgreSql 中

```sql
WITH "recent" AS (SELECT * FROM "Logs" WHERE "Level" = ?),
"dates" AS (SELECT ? AS "day" UNION ALL SELECT ? AS "day")
SELECT * FROM "recent"
```

args: `["error", "2024-01-01", "2024-01-02"]`

查询树中任意位置定义的 CTE(包括嵌套子查询里的)都会被收集、去重并前置到最外层语句。
