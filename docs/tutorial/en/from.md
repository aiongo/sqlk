# From

## From a Table or View
The `From` verb sets the `from` clause; it is usually the first call after `NewQuery`.

```go
sqlk.NewQuery().From("Posts")
```

```sql
SELECT * FROM [Posts]
```

### Alias
To alias the table you should use the `as` syntax

```go
sqlk.NewQuery().From("Posts as p")
```

```sql
SELECT * FROM [Posts] AS [p]
```

## From a Sub Query

You can select from a sub query by passing a `*Query` to `FromSub` together with its alias (an empty alias keeps the alias the sub query set for itself with `As`).

```go
fewMonthsAgo := time.Date(2017, 6, 1, 6, 31, 26, 0, time.UTC)
oldPosts := sqlk.NewQuery().From("Posts").Where("Date", "<", fewMonthsAgo)

query := sqlk.NewQuery().FromSub(oldPosts, "old").OrderByDesc("Date")
```

```sql
SELECT * FROM (SELECT * FROM [Posts] WHERE [Date] < ?) AS [old] ORDER BY [Date] DESC
```

args: `[2017-06-01 06:31:26 +0000 UTC]`

## From a Raw expression

The `FromRaw` verb lets you write raw expressions.

For example in Sql Server you can use the `TABLESAMPLE` to get a 10% sample of the total rows in the comments table.

```go
query := sqlk.NewQuery().FromRaw("Comments TABLESAMPLE SYSTEM (10 PERCENT)")
```

```sql
SELECT * FROM Comments TABLESAMPLE SYSTEM (10 PERCENT)
```

> **Note:** Remember you can use the `[]` (or `{}`) markers to wrap identifier words in your string
