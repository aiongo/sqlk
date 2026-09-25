# Insert、Update 与 Delete

> **Note:** 当前以下子句对 **Insert**、**Update** 与 **Delete** 语句完全被忽略:`order by`、`group by`、`having`、`limit`、`offset` 与 `distinct`。`join` 在支持带连接删除的方言中参与 Delete。

## Insert

`AsInsert` 动词接收表达「列/值」的 `sqlk.Record`(`map[string]any` 的别名)。列按字典序输出,保证编译产物确定(Go map 迭代顺序不定;列序不影响写入语义)。

```go
query := sqlk.NewQuery("Books").AsInsert(sqlk.Record{
    "Title":     "Toyota Kata",
    "CreatedAt": time.Date(2009, 8, 4, 0, 0, 0, 0, time.UTC),
    "Author":    "Mike Rother",
})
```

```sql
INSERT INTO [Books] ([Author], [CreatedAt], [Title]) VALUES (?, ?, ?)
```

args: `["Mike Rother", 2009-08-04 00:00:00 +0000 UTC, "Toyota Kata"]`

> **Note:** 执行查询取回插入 **id** 用执行层的 `InsertGetId`(见[执行 / Insert、Update、Delete](execution/update.md));构建侧的对应动词是 `AsInsertReturnId`:

```go
query := sqlk.NewQuery("Books").AsInsertReturnId(sqlk.Record{
    "Title":  "Introduction to Dart",
    "Price":  0,
    "Status": "active",
})
```

Sql Server 中

```sql
INSERT INTO [Books] ([Price], [Status], [Title]) VALUES (?, ?, ?);SELECT scope_identity() as Id
```

PostgreSql 中

```sql
INSERT INTO "Books" ("Price", "Status", "Title") VALUES (?, ?, ?);SELECT lastval() AS id
```

### 多行插入
用 `AsInsertRows` 动词插入多条记录

```go
cols := []string{"Name", "Price"}

query := sqlk.NewQuery("Products").AsInsertRows(cols,
    []any{"A", 1000},
    []any{"B", 2000},
    []any{"C", 3000},
)
```

```sql
INSERT INTO [Products] ([Name], [Price]) VALUES (?, ?), (?, ?), (?, ?)
```

### 从查询插入

也可以把另一个 select 查询的结果写入。

```go
cols := []string{"Id", "Name", "Address"}
sqlk.NewQuery("ActiveUsers").AsInsertFrom(cols,
    sqlk.NewQuery("Users").WhereEq("Active", 1))
```

```sql
INSERT INTO [ActiveUsers] ([Id], [Name], [Address]) SELECT * FROM [Users] WHERE [Active] = ?
```

args: `[1]`

### 冲突时更新(Upsert)
`OnConflict` 把 INSERT 标记为 upsert:命中唯一键冲突时,指定列回写为本次插入的值。各插入形态(单行、多行、从查询插入)均适用;目前仅 MySQL 方言支持,其余方言在编译期报错。

```go
query := sqlk.NewQuery("Products").AsInsertRows([]string{"Name", "Price"},
    []any{"A", 1000},
    []any{"B", 2000},
).OnConflict("Price")
```

MySQL 下编译为

```sql
INSERT IGNORE INTO `Products` (`Name`, `Price`) VALUES (?, ?), (?, ?) ON DUPLICATE KEY UPDATE `Price` = VALUES(`Price`)
```

## Update

```go
query := sqlk.NewQuery("Posts").WhereNull("AuthorId").
    AsUpdate(sqlk.Record{"AuthorId": 10})
```

```sql
UPDATE [Posts] SET [AuthorId] = ? WHERE [AuthorId] IS NULL
```

args: `[10]`

## Increment 与 Decrement

数值调整用 `AsIncrement` / `AsDecrement`,编译为 `SET 列 = 列 ± ?`;增量缺省 1。

```go
sqlk.NewQuery("Posts").WhereEq("Id", 1).AsIncrement("Views")
sqlk.NewQuery("Products").WhereEq("Id", 1).AsDecrement("Stock", 2)
```

```sql
UPDATE [Posts] SET [Views] = [Views] + ? WHERE [Id] = ?
UPDATE [Products] SET [Stock] = [Stock] - ? WHERE [Id] = ?
```

args: 分别为 `[1, 1]` 与 `[2, 1]`

## Delete

```go
query := sqlk.NewQuery("Posts").Where("Date", ">", thirtyDaysAgo).AsDelete()
```

```sql
DELETE FROM [Posts] WHERE [Date] > ?
```
