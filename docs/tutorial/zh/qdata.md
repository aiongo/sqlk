# QData 查询(JSON 线协议)

`qdata` 包是 JSON 查询线协议的 Go 侧形态:不受信任(或单纯外部)的调用方以 JSON 描述「要什么」,库把它转换为根包的 `*sqlk.Query`,绝不直接产出 SQL。方言编译器仍由你选择。

`select` / `filter` / `orderby` / `top` / `skip` / `count` 键名取自 OData 查询选项;`from` 为本协议自有的资源列表;filter 子键(`group_op` / `field` / `op` / `data`)沿用旧键。旧协议顶层键(`entity`、`limit`、`sort`、`selects`、`sorts`、`includes`)被忽略。相对旧的 goqu 实现,操作符语义修正了四处(见[操作符语义](#操作符语义))。

## 载荷

```json
{
    "from": ["Posts"],
    "select": ["Id", "Title", "count(1) as Count"],
    "filter": {
        "group_op": "and",
        "rules": [
            {"field": "Status", "op": "eq", "data": "active"},
            {"field": "Score", "op": "ge", "data": 10}
        ],
        "groups": [{
            "group_op": "or",
            "rules": [
                {"field": "Title", "op": "bw", "data": "Go"},
                {"field": "Title", "op": "ew", "data": "kata"}
            ]
        }]
    },
    "orderby": [{"by": "CreatedAt", "xsc": "desc"}],
    "top": 20,
    "skip": 0
}
```

- `from` —— 取数目标列表;首元素为主表,其余每个元素按约定追加一个 INNER JOIN(见[约定关联](#约定关联))。必填:列表为空(或含空元素)被拒绝。
- `select` —— 投影列;含 `(` 的项按原生 SQL 表达式处理,其余按标识符。空列表投影 `*`。
- `filter` —— 条件树;`rules` 是「列-操作符-值」三元组,`groups` 任意深度嵌套,每层以自己的 `group_op`(`and` / `or`,缺省 `and`)连接。
- `orderby` —— 排序;`by` 按原生表达式编译,`xsc` 为 `asc`(缺省)或 `desc`。
- `top` / `skip` —— 分页;无缺省:缺 `top`(或 `top: 0`)不生成 LIMIT 子句,`skip` 仅在 `top > 0` 时生效。
- `count` —— 为 `true` 时生成 COUNT 聚合查询(WHERE 与约定关联保留;投影/排序/分页不施加)。

## 反序列化、校验、转换、编译

```go
import (
    "encoding/json"

    "github.com/aiongo/sqlk/compiler"
    "github.com/aiongo/sqlk/qdata"
)

var q qdata.QData
if err := json.Unmarshal(payload, &q); err != nil { // 无缺省值需要补齐
    return err
}
if err := q.Validate(); err != nil { // 可选的前置检查;ToQuery 也会执行它
    return err
}
query, err := q.ToQuery(nil) // nil hook = 不拦截
if err != nil {
    return err
}
res, err := compiler.NewSqlite().Compile(query) // 方言由你选择
```

上面的载荷编译为

```sql
SELECT "Id", "Title", count(1) as Count FROM "Posts" WHERE "Status" = ? AND "Score" >= ? AND ("Title" like ? OR "Title" like ?) ORDER BY CreatedAt DESC LIMIT ?
```

args: `["active", 10, "Go%", "%kata", 20]`(JSON 数值以 `float64` 到达)

完全不带 `filter` / `orderby` / `top` 键时,`{"from": ["Posts"]}` 编译为缺省形态

```sql
SELECT * FROM "Posts"
```

无参数。不主动要求就不限制行数。

## 约定关联

`from` 首元素之后的每个元素按 `<主表>.<x>_id = <x>.<x>_id` 的外键约定追加一个 INNER JOIN,列名由编译器按标识符包裹:

```json
{"from": ["Posts", "Authors"], "top": 5}
```

```sql
SELECT * FROM "Posts"
INNER JOIN "Authors" ON "Posts"."Authors_id" = "Authors"."Authors_id" LIMIT ?
```

args: `[5]`

(旧协议的 `includes` 即归宿于此:关联表直接进入 `from` 列表。更细粒度的连接仍走根构建器的 `Join` 族。)

## 操作符语义

16 个操作符码:

| 码 | 含义 | 编译为 | `data` 说明 |
| --- | --- | --- | --- |
| `eq` / `ne` | 等于 / 不等于 | `= ?` / `!= ?` | 标量 |
| `lt` / `le` / `gt` / `ge` | 比较 | `< ?` `<= ?` `> ?` `>= ?` | 标量 |
| `in` / `ni` | 在 / 不在集合内 | `IN (…)` / `NOT IN (…)` | 数组**或**单标量 |
| `is` / `ns` | 为 / 不为 NULL | `IS NULL` / `IS NOT NULL` | 忽略 |
| `bw` / `bn` | 前缀匹配 / 非 | `like 'data%'` | 字符串 |
| `ew` / `en` | 后缀匹配 / 非 | `like '%data'` | 字符串 |
| `cn` / `nc` | 包含匹配 / 非 | `like '%data%'` | 字符串 |

LIKE 族编译为普通 `LIKE`(不包 `LOWER`、值不小写化);大小写敏感性交由数据库排序规则决定。

相对旧实现,这里修正了四处语义:

1. `bw` / `ew` / `cn` 分别生成前缀(`data%`)、后缀(`%data`)、包含(`%data%`)模式;旧实现一律生成 `%data%`,`bn` / `en` / `nc` 为其否定。
2. `is` / `ns` 编译为 `IS NULL` / `IS NOT NULL`,不再借用值参数。
3. `in` / `ni` 的 data 既接受数组也接受单标量(单值枚举不必再包一层)。
4. `count: true` 生成真正的 COUNT 聚合查询;旧实现的该分支未完成。

```json
{
    "from": ["Posts"],
    "filter": {"rules": [
        {"field": "AuthorId", "op": "ns"},
        {"field": "Lang", "op": "in", "data": "en"},
        {"field": "Title", "op": "cn", "data": "go"}
    ]},
    "top": 0
}
```

```sql
SELECT * FROM "Posts" WHERE "AuthorId" IS NOT NULL AND "Lang" IN (?) AND "Title" like ?
```

args: `["en", "%go%"]`

同一份 filter 配上 `count: true` 则产出聚合形态;WHERE 保留,投影/排序/分页不施加:

```json
{
    "from": ["Posts"],
    "filter": {"rules": [{"field": "Status", "op": "eq", "data": "active"}]},
    "count": true
}
```

```sql
SELECT COUNT(*) AS "count" FROM "Posts" WHERE "Status" = ?
```

args: `["active"]`

两条旧行为出于兼容保留:data 为空(空串、空数组、`null`)的规则在编译时跳过但仍参与校验;未知 JSON 键被忽略。

## 校验

`Validate`(以及会先执行它的 `ToQuery`)把全部问题聚合为一次错误返回,调用方一轮就能修正请求。每个问题都可用 `errors.Is` / `errors.As` 判别:

| 问题 | sentinel |
| --- | --- |
| `from` 为空(列表或元素) | `qdata.ErrFromRequired` |
| 非法 `group_op` | `qdata.ErrInvalidGroupOp`(`*qdata.GroupOpError`) |
| 规则 `field` 为空 | `qdata.ErrRuleFieldRequired` |
| 规则 `op` 不在 16 码之列 | `qdata.ErrInvalidOp`(`*qdata.OpError`) |
| `orderby.by` 为空 | `qdata.ErrOrderByByRequired` |
| 非法 `xsc` | `qdata.ErrInvalidOrderByDirection` |
| `top` / `skip` 为负 | `qdata.ErrInvalidPagination`(`*qdata.PaginationError`) |

```go
err := q.Validate()
errors.Is(err, qdata.ErrInvalidOp) // 任一规则带非法 op 时为 true
```

## Hook:安全切点

`ToQuery(hook)` 在每个值边界回调 `Hook`:这是做字段白名单或字段改写的地方。返回错误即中止转换并原样传播;Hook 只能收紧校验,不能放宽。

```go
type allowHook struct{ columns map[string]bool }

func (h allowHook) From(from []string) ([]string, error) { return from, nil }
func (h allowHook) Select(column string) (string, error) {
    if !h.columns[column] {
        return "", errors.New("column not allowed: " + column)
    }
    return column, nil
}
func (h allowHook) OrderBy(by string) (string, error) { return by, nil }
func (h allowHook) Rule(rule qdata.Rule) (qdata.Rule, error) {
    if !h.columns[rule.Field] {
        return rule, errors.New("field not allowed: " + rule.Field)
    }
    return rule, nil
}

hook := allowHook{columns: map[string]bool{"Id": true, "Title": true, "Status": true}}
query, err := q.ToQuery(hook)
```

以上白名单下,投影 `Password` 的载荷以 `column not allowed: Password` 失败。`From` 切点看到整个目标列表;在其中增删关联表,正是你管控调用方可触达哪些表的地方。

## 编程构造

服务端代码不经 JSON 也能构造同一结构:`qdata.New()` 起一个空查询,`With*` 动词像根构建器一样链式拼装。

```go
q := qdata.New().
    WithFrom("Posts").
    WithSelect("Id", "Title").
    WithFilter(*qdata.NewFilter().
        WithRule(*qdata.NewRule("Status", qdata.OpEq, "active")).
        WithRule(*qdata.NewRule("Score", qdata.OpGt, 10))).
    WithOrderBy(*qdata.NewOrderBy("CreatedAt", "desc")).
    WithTop(20)

query, err := q.ToQuery(nil)
```

```sql
SELECT "Id", "Title" FROM "Posts" WHERE "Status" = ? AND "Score" > ? ORDER BY CreatedAt DESC LIMIT ?
```

args: `["active", 10, 20]`
