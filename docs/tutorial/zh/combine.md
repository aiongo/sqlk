# 组合多个查询

## Union / Except / Intersect

sqlk 允许用 `union`、`intersect`、`except` 三种集合运算组合多个查询,对应 `Union`、`UnionAll`、`Intersect`、`IntersectAll`、`Except`、`ExceptAll` 动词。

动词直接接收 `*Query`,或经 `…Func` 变体(`UnionFunc`、`UnionAllFunc` 等)以回调构建成员。


```go
phones := sqlk.NewQuery().From("Phones")
laptops := sqlk.NewQuery().From("Laptops")

mobiles := laptops.Union(phones)
```

```sql
SELECT * FROM [Laptops] UNION SELECT * FROM [Phones]
```


或使用回调变体

```go
mobiles := sqlk.NewQuery().From("Laptops").
    ExceptAllFunc(func(q *sqlk.Query) *sqlk.Query { return q.From("OldLaptops") })
```

```sql
SELECT * FROM [Laptops] EXCEPT ALL SELECT * FROM [OldLaptops]
```

## 组合原生表达式

随时可以用 `CombineRaw` 动词追加原生表达式

```go
mobiles := sqlk.NewQuery().From("Laptops").CombineRaw("union all select * from OldLaptops")
```

```sql
SELECT * FROM [Laptops] union all select * from OldLaptops
```

当然,可以用表标识符字符 `[` 与 `]` 指示 sqlk 包裹表/列关键字。


```go
mobiles := sqlk.NewQuery().From("Laptops").CombineRaw("union all select * from [OldLaptops]")
```

```sql
SELECT * FROM [Laptops] union all select * from [OldLaptops]
```

PostgreSql 中

```sql
SELECT * FROM "Laptops" union all select * from "OldLaptops"
```
