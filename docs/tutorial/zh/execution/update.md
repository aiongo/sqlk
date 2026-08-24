# 写入数据(Insert / Update / Delete)

执行层提供以下方法帮助对数据库做写操作:

- `Exec()` —— 执行根包写动词(insert / update / delete)构建的查询,返回影响行数
- `InsertGetId[T]()` —— 写入并取回自增 id

```go
db := exec.New(sqlxDB, compiler.NewSqlite())
```

## 写入单条

```go
affected, err := db.Exec(ctx, sqlk.NewQuery().From("Books").Insert(sqlk.Record{
    "Title":  "Introduction to C#",
    "Price":  18,
    "Status": "active",
}))
```

## 写入单条并取回 id

`InsertGetId` 在查询副本上构建 returnId 形态的 INSERT,执行并在同一往返取回 id(尾部 LastId 语句按方言——Sql Server 的 `scope_identity()`、PostgreSql 的 `lastval()`、SQLite 的 `last_insert_rowid()`):

```go
id, err := db.InsertGetId[int64](ctx, sqlk.NewQuery().From("Posts"), sqlk.Record{
    "Title": "New Post", "Likes": 0, "Lang": "en", "Date": "2024-02-01",
})
// id == 4
```

> **Note:** 当前该方法只支持单条插入语句取 id,多行插入暂不支持。

## 写入多条

```go
cols := []string{"Name", "Price"}

affected, err := db.Exec(ctx, sqlk.NewQuery().From("Products").InsertRows(cols,
    []any{"A", 1000},
    []any{"B", 2000},
    []any{"C", 3000},
))
```

## 从既有查询写入

```go
columns := []string{"Title", "Price", "Status"}
articlesQuery := sqlk.NewQuery().From("Articles").WhereEq("Type", "Book").Limit(100)

affected, err := db.Exec(ctx, sqlk.NewQuery().From("Books").InsertFrom(columns, articlesQuery))
```

## 更新数据

```go
affected, err := db.Exec(ctx, sqlk.NewQuery().From("Books").
    WhereEq("Id", 1).
    Update(sqlk.Record{
        "Price":  18,
        "Status": "active",
    }))
// affected == 1
```

`Increment` / `Decrement` 动词走同一条路径:

```go
_, err := db.Exec(ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 1).Increment("Likes"))
// 该行的 Likes 从 30 变为 31
```

## 删除

```go
affected, err := db.Exec(ctx, sqlk.NewQuery().From("Books").WhereEq("Status", "inactive").Delete())
```
