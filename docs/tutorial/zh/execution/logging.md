# 查询日志

构造执行句柄时传入 `WithLogger` 选项即可记录查询。回调在编译成功后、执行前收到编译产物——SQL 文本与有序参数;编译失败不触发回调。

```go
db := exec.New(sqlxDB, compiler.NewSqlite(),
    exec.WithLogger(func(res compiler.Result) {
        log.Println(res.SQL, res.Args)
    }))

posts, err := db.Get[Post](ctx, sqlk.NewQuery().From("Posts"))
```

将打印到日志

```sh
SELECT * FROM "Posts" []
```

以 `Begin` 开启的事务句柄自动继承日志回调。
