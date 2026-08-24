# 字符串操作

sqlk 提供 `WhereStarts`、`WhereEnds`、`WhereContains` 与 `WhereLike` 方法处理字符串类列。

缺省全部大小写不敏感:对列套用数据库的 `LOWER()` 并把传入值小写化。PostgreSql 方言改以 `ILIKE` 表达大小写不敏感(列不包 `LOWER`、值不小写化)。

传入 `sqlk.CaseSensitive()` 选项可覆盖该行为。


```go
sqlk.NewQuery().From("Posts").WhereEnds("Title", "Book")
```

```sql
SELECT * FROM [Posts] WHERE LOWER([Title]) like ?
```

args: `["%book"]`

使用大小写敏感选项

```go
sqlk.NewQuery().From("Posts").WhereStarts("Title", "Book", sqlk.CaseSensitive())
```

```sql
SELECT * FROM [Posts] WHERE [Title] like ?
```

args: `["Book%"]`

使用原生 `WhereLike` 方法

```go
sqlk.NewQuery().From("Posts").WhereLike("Title", "Book")
```

```sql
SELECT * FROM [Posts] WHERE LOWER([Title]) like ?
```

args: `["book"]`

PostgreSql 中

```sql
SELECT * FROM "Posts" WHERE "Title" ilike ?
```

args: `["Book"]`

> **Note:** `WhereLike` 方法里,通配符 `%` 由你自己书写

所有 LIKE 查询还可以用 `sqlk.EscapeLike` 选项追加可选的转义子句:

```go
sqlk.NewQuery().From("Posts").WhereLike("Title", `%The \% Sign%`, sqlk.EscapeLike(`\`))
```

PostgreSql 中
```sql
SELECT * FROM "Posts" WHERE "Title" ilike ? ESCAPE '\'
```

args: ``[`%The \% Sign%`]``

Sql Server 中(值与列一同小写化)

```sql
SELECT * FROM [Posts] WHERE LOWER([Title]) like ? ESCAPE '\'
```

args: ``[`%the \% sign%`]``
