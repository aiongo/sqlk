# QData Query (JSON wire protocol)

The `qdata` package is the Go side of a JSON query wire protocol: untrusted (or simply external) callers describe *what* they want in JSON, and the library turns it into a root-package `*sqlk.Query`, never SQL directly. The dialect compiler stays your choice.

The keys `select` / `filter` / `orderby` / `top` / `skip` / `count` take their names from OData query options; `from` is this protocol's own resource list; the filter sub-keys (`group_op` / `field` / `op` / `data`) keep their legacy shapes. Legacy top-level keys (`entity`, `limit`, `sort`, `selects`, `sorts`, `includes`) are ignored. Relative to the legacy goqu-based implementation the operator semantics carry four fixes (see [Operator semantics](#operator-semantics)).

## The payload

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

- `from` — the target list; the first element is the main table, every further element adds a convention INNER JOIN (see [Convention joins](#convention-joins)). Required: an empty list (or an empty element) is rejected.
- `select` — projection columns; items containing `(` are treated as raw SQL expressions, everything else as identifiers. Empty list projects `*`.
- `filter` — the condition tree; `rules` are column-operator-value triples, `groups` nest to any depth, each level connects with its own `group_op` (`and` / `or`, default `and`).
- `orderby` — orderings; `by` is compiled as a raw expression, `xsc` is `asc` (default) or `desc`.
- `top` / `skip` — pagination; there is no default: a missing `top` (or `top: 0`) emits no LIMIT clause, `skip` only takes effect when `top > 0`.
- `count` — when `true`, produces a COUNT aggregate query instead (WHERE and convention joins are kept; projection / ordering / pagination are not applied).

## Unmarshal, validate, convert, compile

```go
import (
    "encoding/json"

    "github.com/aiongo/sqlk/compiler"
    "github.com/aiongo/sqlk/qdata"
)

var q qdata.QData
if err := json.Unmarshal(payload, &q); err != nil { // no defaults to fill
    return err
}
if err := q.Validate(); err != nil { // optional upfront check; ToQuery runs it too
    return err
}
query, err := q.ToQuery() // no hooks = no interception
if err != nil {
    return err
}
res, err := compiler.NewSqlite().Compile(query) // your dialect choice
```

The payload above compiles to

```sql
SELECT "Id", "Title", count(1) as Count FROM "Posts" WHERE "Status" = ? AND "Score" >= ? AND ("Title" like ? OR "Title" like ?) ORDER BY CreatedAt DESC LIMIT ?
```

args: `["active", 10, "Go%", "%kata", 20]` (JSON numbers arrive as `float64`)

With no `filter` / `orderby` / `top` keys at all, `{"from": ["Posts"]}` compiles to the default shape

```sql
SELECT * FROM "Posts"
```

no args. Nothing limits the result unless you ask for it.

## Convention joins

Every element after the first in `from` adds a convention INNER JOIN on the `<main>.<x>_id = <x>.<x>_id` foreign-key convention, with all names wrapped as identifiers by the compiler:

```json
{"from": ["Posts", "Authors"], "top": 5}
```

```sql
SELECT * FROM "Posts"
INNER JOIN "Authors" ON "Posts"."Authors_id" = "Authors"."Authors_id" LIMIT ?
```

args: `[5]`

(This is where the legacy protocol's `includes` went: the join list now lives in `from` itself. Finer-grained joins stay available through the root builder's `Join` family.)

## Operator semantics

The 16 operator codes:

| Code | Meaning | Compiled as | Notes on `data` |
| --- | --- | --- | --- |
| `eq` / `ne` | equals / not equals | `= ?` / `!= ?` | scalar |
| `lt` / `le` / `gt` / `ge` | comparison | `< ?` `<= ?` `> ?` `>= ?` | scalar |
| `in` / `ni` | in / not in set | `IN (…)` / `NOT IN (…)` | array **or** single scalar |
| `is` / `ns` | is null / is not null | `IS NULL` / `IS NOT NULL` | ignored |
| `bw` / `bn` | begins with / not | `like 'data%'` | string |
| `ew` / `en` | ends with / not | `like '%data'` | string |
| `cn` / `nc` | contains / not | `like '%data%'` | string |

The LIKE family compiles to plain `LIKE` (no `LOWER`, no lowercasing); case sensitivity is left to the database collation.

Relative to the legacy implementation, four semantics are fixed here:

1. `bw` / `ew` / `cn` generate prefix (`data%`), suffix (`%data`) and contains (`%data%`) patterns respectively; the legacy version produced `%data%` for all of them, and `bn` / `en` / `nc` are their negations.
2. `is` / `ns` compile to `IS NULL` / `IS NOT NULL` instead of borrowing the value parameter.
3. `in` / `ni` accept an array **or** a single scalar (a single enum value no longer needs wrapping).
4. `count: true` produces a real COUNT aggregate query; the legacy branch was unimplemented.

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

With `count: true` the same filter produces the aggregate instead; WHERE is kept, projection / ordering / pagination are not applied:

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

Two legacy behaviors are kept for compatibility: rules with empty `data` (empty string, empty array, `null`) are skipped at compile time but still validated; unknown JSON keys are ignored.

## Validation

`Validate` (and `ToQuery`, which runs it first) aggregates every problem into one error, so callers can fix a request in a single round trip. Each problem is distinguishable with `errors.Is` / `errors.As`:

| Problem | Sentinel |
| --- | --- |
| `from` empty (list or element) | `qdata.ErrFromRequired` |
| illegal `group_op` | `qdata.ErrInvalidGroupOp` (`*qdata.GroupOpError`) |
| rule `field` empty | `qdata.ErrRuleFieldRequired` |
| rule `op` not one of the 16 | `qdata.ErrInvalidOp` (`*qdata.OpError`) |
| `orderby.by` empty | `qdata.ErrOrderByByRequired` |
| illegal `xsc` | `qdata.ErrInvalidOrderByDirection` |
| negative `top` / `skip` | `qdata.ErrInvalidPagination` (`*qdata.PaginationError`) |

```go
err := q.Validate()
errors.Is(err, qdata.ErrInvalidOp) // true when any rule carries a bad op
```

## The Hook: a security checkpoint

`ToQuery(hooks...)` invokes your `Hook`s at every value boundary: the place to whitelist fields or rewrite them. Several hooks run in argument order, each seeing the previous one's rewrite. Returning an error aborts the conversion and propagates as-is; a hook can only *tighten* validation, never loosen it.

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

With the whitelist above, a payload selecting `Password` fails with `column not allowed: Password`. The `From` checkpoint sees the whole target list; dropping or adding convention joins there is exactly how you gate which tables a caller may touch.

## Building programmatically

Server-side code can construct the same structure without JSON: `qdata.New()` starts an empty query, the `With*` verbs chain like the root builder.

```go
q := qdata.New().
    WithFrom("Posts").
    WithSelect("Id", "Title").
    WithFilter(*qdata.NewFilter().
        WithRule(*qdata.NewRule("Status", qdata.OpEq, "active")).
        WithRule(*qdata.NewRule("Score", qdata.OpGt, 10))).
    WithOrderBy(*qdata.NewOrderBy("CreatedAt", "desc")).
    WithTop(20)

query, err := q.ToQuery()
```

```sql
SELECT "Id", "Title" FROM "Posts" WHERE "Status" = ? AND "Score" > ? ORDER BY CreatedAt DESC LIMIT ?
```

args: `["active", 10, 20]`
