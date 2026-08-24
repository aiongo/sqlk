# Having(分组后过滤)

`Having` 方法族在分组后过滤区段上完整镜像 `Where` 能力:条件形态相同、`Or` / `Not` 变体相同,只是区段关键字不同。

## Having

```go
commentsCount := sqlk.NewQuery().From("Comments").
    Select("PostId").
    SelectRaw("count(1) as Count").
    GroupBy("PostId")

query := sqlk.NewQuery().FromSub(commentsCount, "").Having("Count", ">", 100)
```

```sql
SELECT * FROM (SELECT [PostId], count(1) as Count FROM [Comments] GROUP BY [PostId]) HAVING [Count] > ?
```

args: `[100]`

## HavingRaw

```go
query := sqlk.NewQuery().From("Comments").
    Select("PostId").
    SelectRaw("count(1) as Count").
    GroupBy("PostId").
    HavingRaw("count(1) > 50")
```

```sql
SELECT [PostId], count(1) as Count FROM [Comments] GROUP BY [PostId] HAVING count(1) > 50
```

## 嵌套 Having
嵌套 having 条件用 `HavingGroup`:回调内以 `Where` 方法族累积条件,编译为带括号的 `HAVING (…)` 组。

```go
query := sqlk.NewQuery().From("Comments").
    Select("PostId").
    SelectRaw("count(1) as Count").
    GroupBy("PostId").
    HavingGroup(func(q *sqlk.Query) *sqlk.Query {
        return q.Where("Count", ">", 50).OrWhere("Count", "<", 20)
    })
```

```sql
SELECT [PostId], count(1) as Count FROM [Comments] GROUP BY [PostId] HAVING ([Count] > ? OR [Count] < ?)
```

args: `[50, 20]`
