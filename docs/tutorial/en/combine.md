# Combining Multiple Queries

## Union / Except / Intersect

sqlk allows you to combine multiple queries using one of the following available operators `union`, `intersect` and `except` via the verbs `Union`, `UnionAll`, `Intersect`, `IntersectAll`, `Except` and `ExceptAll`.

The verbs accept a `*Query` directly, or a callback via the `…Func` variants (`UnionFunc`, `UnionAllFunc`, …).


```go
phones := sqlk.NewQuery().From("Phones")
laptops := sqlk.NewQuery().From("Laptops")

mobiles := laptops.Union(phones)
```

```sql
SELECT * FROM [Laptops] UNION SELECT * FROM [Phones]
```


Or by using the callback variant

```go
mobiles := sqlk.NewQuery().From("Laptops").
    ExceptAllFunc(func(q *sqlk.Query) *sqlk.Query { return q.From("OldLaptops") })
```

```sql
SELECT * FROM [Laptops] EXCEPT ALL SELECT * FROM [OldLaptops]
```

## Combining Raw Expressions

You can always use the `CombineRaw` verb to append raw expressions

```go
mobiles := sqlk.NewQuery().From("Laptops").CombineRaw("union all select * from OldLaptops")
```

```sql
SELECT * FROM [Laptops] union all select * from OldLaptops
```

Of course you can use the table identifier characters `[` and `]` to instruct sqlk to wrap the tables/columns keywords.


```go
mobiles := sqlk.NewQuery().From("Laptops").CombineRaw("union all select * from [OldLaptops]")
```

```sql
SELECT * FROM [Laptops] union all select * from [OldLaptops]
```

In PostgreSql

```sql
SELECT * FROM "Laptops" union all select * from "OldLaptops"
```
