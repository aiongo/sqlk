# Fetching Records

The execution layer provides the following methods to help execute your queries:

- `Get[T]`
- `First[T]` / `FirstOrDefault[T]`
- `Exists` / `NotExist`
- `Paginate[T]`
- `Chunk[T]`

All of them take `context.Context` as their first argument, so timeouts and cancellation reach the driver.

## Retrieving Records

`Get[T]` executes the query and scans all rows into a slice of your type. The type is yours — no dynamic rows; map columns with the usual sqlx `db` tags.

```go
db := exec.New(sqlxDB, compiler.NewSqlite())

posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts").
    Where("Likes", ">", 10).
    WhereIn("Lang", "en", "fr").
    WhereNotNull("AuthorId").
    OrderByDesc("Date").
    Select("Id", "Title"))
```

The example above is the one from the [introduction](../index.md) and runs against a seeded SQLite table in the docs test suite.

## Getting One Record

Use `First[T]` or `FirstOrDefault[T]` to get the first record of the query.

```go
post, err := db.First[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 1))
```

> **Note:** `First` and `FirstOrDefault` add the `Limit(1)` clause implicitly to the query, so there is no need to add it by yourself.

When no row matches, `First` returns a distinguishable error — it is `sql.ErrNoRows`, also exposed as `exec.ErrNoRows`:

```go
_, err := db.First[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 99))
errors.Is(err, exec.ErrNoRows) // true
```

`FirstOrDefault` treats the missing row as a non-error and returns the zero value:

```go
missing, err := db.FirstOrDefault[Post](ctx, sqlk.NewQuery().From("Posts").WhereEq("Id", 99))
// missing == Post{} , err == nil
```

## Lightweight existence checks

```go
exists, err := db.Exists(ctx, sqlk.NewQuery().From("Posts").WhereEq("Lang", "fr"))
missing, err := db.NotExist(ctx, sqlk.NewQuery().From("Posts").WhereEq("Lang", "de"))
```

## Data Pagination

To paginate your data, use `Paginate[T](page, perPage)` instead of `Get`. It returns a `PaginationResult[T]` carrying the total count, the current page number, the per-page size, and the page's `List`.

```go
page1, err := db.Paginate[Post](ctx, sqlk.NewQuery().From("Posts").OrderBy("Id"), 1, 2)
// page1.Total == 3, len(page1.List) == 2, page1.HasMore() == true

page2, err := db.Paginate[Post](ctx, sqlk.NewQuery().From("Posts").OrderBy("Id"), 2, 2)
// len(page2.List) == 1, page2.HasMore() == false
```

`HasMore` reports whether more data follows the current page (SqlKata's `HasNext`); fetching the next page is simply another `Paginate` call — the result carries no execution-time cursors. SqlKata's `Next` / `Previous` / `NextQuery` cursor members are deliberately not ported: re-paginate with new parameters (or add constraints to a fresh query) instead.

## Data Chunks

Sometimes you may want to retrieve data in chunks to prevent loading the whole table into memory at once; for this you can use the `Chunk` iterator.

```go
for rows, err := range db.Chunk[Post](ctx, sqlk.NewQuery().From("Posts").OrderBy("Id"), 2) {
    if err != nil {
        break // the iterator also terminates after yielding an error
    }
    for _, post := range rows {
        // do something with post
    }
}
```

Chunks are fetched lazily, page by page; break out of the loop to stop early.

## Scalar aggregates

The generic aggregate methods execute an aggregate form of the query and scan the scalar straight into your number:

```go
count, err := db.Count[int64](ctx, sqlk.NewQuery().From("Posts")) // 3
sum, err := db.Sum[int64](ctx, sqlk.NewQuery().From("Posts"), "Likes") // 75
```

`Avg[T]`, `Min[T]` and `Max[T]` work the same way.

## Execute Raw Statements

For free-form statements keep using your sqlx handle directly — the execution layer deliberately does not duplicate it:

```go
var users []User
err := sqlxDB.SelectContext(ctx, &users, "exec sp_get_users_by_date @date", sql.Named("date", time.Now()))

_, err = sqlxDB.ExecContext(ctx, "truncate table Users")
```
