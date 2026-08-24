# Having

The `Having` family mirrors the full `Where` capability on the post-grouping section: same condition shapes, same `Or` / `Not` variants, only the section keyword differs.

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

## Nested Having
To nest having conditions, use `HavingGroup`: the callback accumulates conditions with the `Where` family, and the compiler renders them as a parenthesized `HAVING (…)` group.

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
