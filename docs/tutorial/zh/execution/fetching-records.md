# 取数

执行层提供以下方法帮助执行查询:

- `Get[T]`
- `First[T]` / `FirstOrDefault[T]`
- `Exists` / `NotExist`
- `Paginate[T]`
- `Chunk[T]`

全部方法首参为 `context.Context`,超时与取消贯穿到驱动。

## 取回记录

`Get[T]` 执行查询并把全部行扫描进你的类型切片。类型由你给定——没有动态行;列映射用 sqlx 惯用的 `db` 标签。

```go
db := exec.New(sqlxDB, compiler.NewSqlite())

posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts").
    Where("Likes", ">", 10).
    WhereIn("Lang", "en", "fr").
    WhereNotNull("AuthorId").
    OrderByDesc("Date").
    Select("Id", "Title"))
```

上面的例子就是[介绍页](../index.md)中的那个,在文档测试套件里对预置数据的 SQLite 表真实执行。

## 取单条记录

用 `First[T]` 或 `FirstOrDefault[T]` 取查询的首条记录。

```go
post, err := db.First[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 1))
```

> **Note:** `First` 与 `FirstOrDefault` 会隐式给查询附加 `Limit(1)`,无需自己添加。

无匹配行时 `First` 返回可判别的错误——即 `sql.ErrNoRows`,也以 `exec.ErrNoRows` 导出:

```go
_, err := db.First[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 99))
errors.Is(err, exec.ErrNoRows) // true
```

`FirstOrDefault` 把缺行视为非错误,返回零值:

```go
missing, err := db.FirstOrDefault[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 99))
// missing == Post{} , err == nil
```

## 轻量存在性判断

```go
exists, err := db.Exists(ctx, sqlk.NewQuery().From("Posts").WhereEq("Lang", "fr"))
missing, err := db.NotExist(ctx, sqlk.NewQuery().From("Posts").WhereEq("Lang", "de"))
```

## 数据分页

分页取数用 `Paginate[T](page, perPage)` 代替 `Get`。它返回 `PaginationResult[T]`:总数、当前页号、每页数与当页 `List`。

```go
page1, err := db.Paginate[Post](ctx, sqlk.NewQuery().From("Posts").OrderBy("Id"), 1, 2)
// page1.Total == 3, len(page1.List) == 2, page1.HasMore() == true

page2, err := db.Paginate[Post](ctx, sqlk.NewQuery().From("Posts").OrderBy("Id"), 2, 2)
// len(page2.List) == 1, page2.HasMore() == false
```

`HasMore` 报告当前页之后是否还有数据(即 SqlKata 的 `HasNext`);取下一页就是再调一次 `Paginate`——结果值不携带执行期游标。SqlKata 的 `Next` / `Previous` / `NextQuery` 游标成员刻意不移植:以新参数重新分页(或给新查询追加条件)即可。

## 数据分块

有时你想分块取数,避免整表一次性载入内存;这时用 `Chunk` 迭代器。

```go
for rows, err := range db.Chunk[Post](ctx, sqlk.NewQuery().From("Posts").OrderBy("Id"), 2) {
    if err != nil {
        break // 迭代器产出一次错误后即终止
    }
    for _, post := range rows {
        // 处理 post
    }
}
```

分块按页懒取;跳出循环即提前停止。

## 标量聚合

泛型聚合方法执行查询的聚合形态,把标量直接扫进你的数字:

```go
count, err := db.Count[int64](ctx, sqlk.NewQuery().From("Posts")) // 3
sum, err := db.Sum[int64](ctx, sqlk.NewQuery().From("Posts"), "Likes") // 75
```

`Avg[T]`、`Min[T]`、`Max[T]` 同理。

## 执行原生语句

自由格式的语句直接继续使用你的 sqlx 句柄——执行层刻意不重复它:

```go
var users []User
err := sqlxDB.SelectContext(ctx, &users, "exec sp_get_users_by_date @date", sql.Named("date", time.Now()))

_, err = sqlxDB.ExecContext(ctx, "truncate table Users")
```
