# Query Logging

You can log your queries by passing the `WithLogger` option when constructing the execution handle. The callback receives the compiled result — SQL text and ordered arguments — after successful compilation and before execution; failed compilations do not trigger it.

```go
db := exec.New(sqlxDB, compiler.NewSqlite(),
    exec.WithLogger(func(res compiler.Result) {
        log.Println(res.SQL, res.Args)
    }))

posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts"))
```

Will print to the log

```sh
SELECT * FROM "Posts" []
```

Transaction handles opened with `Begin` inherit the logger automatically.
