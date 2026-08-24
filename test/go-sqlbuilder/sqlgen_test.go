// Package gosqlbuilder migrates the scenario coverage of
// github.com/huandu/go-sqlbuilder's *_test.go files onto the sqlk builder.
//
// Each case pairs build code with the expected SQL text and argument
// sequence, asserting at the library's main seam —
// `compiler.Compile(Query) -> (SQL, args)`. The scenarios are translated
// from go-sqlbuilder's API and expected output, but the asserted SQL is
// sqlk's own. Where sqlk diverges from go-sqlbuilder the difference is noted
// in the case comment rather than hidden: sqlk quotes identifiers
// (go-sqlbuilder does not), normalizes operators to lowercase, wraps
// case-insensitive LIKE in LOWER(...) on the base/mysql/oracle/sqlite
// dialects and renders it as ILIKE on postgres, drops ORDER BY/LIMIT on
// UPDATE and DELETE, attaches ORDER BY/LIMIT to the main query before a
// combine (rather than after the union), and wraps NOT LIKE as
// NOT (col like ?) rather than col NOT LIKE ?.
//
// go-sqlbuilder covers more flavors than sqlk (CQL, ClickHouse, Presto,
// Informix, Doris); only the five sqlk supports — sqlite, postgres, mysql,
// sqlserver, oracle — are migrated here, and go-sqlbuilder-only engines are
// ignored. go-sqlbuilder features with no sqlk analog are out of scope and
// not migrated: statement-level interpolation (flavor_test, args_test — sqlk
// emits placeholder SQL and leaves rebind to sqlx), INSERT IGNORE / REPLACE
// INTO, UPDATE ... FROM, DELETE ... USING, LATERAL derived tables, row-value
// Tuple IN, the Flatten slice helper, and arbitrary SQL fragments injected
// between clauses (the SQL/Build/Buildf/BuildNamed composition framework).
// Source scenarios: the go-sqlbuilder test suite
// (https://github.com/huandu/go-sqlbuilder).
package gosqlbuilder

import (
	"errors"
	"reflect"
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// compileCase is a table-driven case mapping a builder function to the
// expected SQL text and argument sequence, run against one compiler.
type compileCase struct {
	name  string
	build func(*sqlk.Query) *sqlk.Query
	sql   string
	args  []any
}

// runCompileCases compiles each case and asserts the SQL text and argument
// sequence.
func runCompileCases(t *testing.T, comp *compiler.Compiler, cases []compileCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			res, err := comp.Compile(tt.build(sqlk.NewQuery()))
			if err != nil {
				t.Fatalf("Compile(...) error = %v, want nil", err)
			}
			if res.SQL != tt.sql {
				t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, tt.sql)
			}
			if !reflect.DeepEqual(res.Args, tt.args) {
				t.Errorf("Compile(...) Args = %#v, want %#v", res.Args, tt.args)
			}
		})
	}
}

// errCase is a case expected to fail compilation; wantErr is matched with
// errors.Is. Migrated from go-sqlbuilder's invalid-argument scenarios.
type errCase struct {
	name    string
	build   func(*sqlk.Query) *sqlk.Query
	wantErr error
}

func runErrCases(t *testing.T, comp *compiler.Compiler, cases []errCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			_, err := comp.Compile(tt.build(sqlk.NewQuery()))
			if err == nil {
				t.Fatalf("Compile(...) error = nil, want %v", tt.wantErr)
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Compile(...) error = %v, want errors.Is(..., %v)", err, tt.wantErr)
			}
		})
	}
}

// wantResult is the expected output for one dialect.
type wantResult struct {
	sql  string
	args []any
}

// dialectCase runs one build against all five sqlk dialect compilers and
// asserts each dialect's expected output. Dialects whose entry is absent are
// skipped, so a case can opt in to only the dialects where go-sqlbuilder's
// flavor loop produced distinct output.
type dialectCase struct {
	name  string
	build func(*sqlk.Query) *sqlk.Query
	want  map[string]wantResult
}

// dialectCompilers returns the five dialect compilers keyed by engine code.
func dialectCompilers() map[string]*compiler.Compiler {
	return map[string]*compiler.Compiler{
		sqlk.EngineSqlite:    compiler.NewSqlite(),
		sqlk.EnginePostgres:  compiler.NewPostgres(),
		sqlk.EngineMysql:     compiler.NewMysql(),
		sqlk.EngineSqlserver: compiler.NewSqlserver(),
		sqlk.EngineOracle:    compiler.NewOracle(),
	}
}

func runDialectCases(t *testing.T, cases []dialectCase) {
	t.Helper()
	comps := dialectCompilers()
	for _, tt := range cases {
		for engine, comp := range comps {
			want, ok := tt.want[engine]
			if !ok {
				continue
			}
			t.Run(tt.name+"/"+engine, func(t *testing.T) {
				res, err := comp.Compile(tt.build(sqlk.NewQuery()))
				if err != nil {
					t.Fatalf("Compile(%s) error = %v, want nil", engine, err)
				}
				if res.SQL != want.sql {
					t.Errorf("Compile(%s) SQL = %q, want %q", engine, res.SQL, want.sql)
				}
				if !reflect.DeepEqual(res.Args, want.args) {
					t.Errorf("Compile(%s) Args = %#v, want %#v", engine, res.Args, want.args)
				}
			})
		}
	}
}

// TestSelectScenarios migrates ExampleSelect / ExampleSelectBuilder_varInCols
// and the basic projection shapes. The base compiler is the sqlk default;
// dialect-specific quoting is covered by TestIdentifierQuotingMatrix.
func TestSelectScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// ExampleSelect: projection, qualified from, limit.
			name:  "select columns from qualified table with limit",
			build: func(q *sqlk.Query) *sqlk.Query { return q.Select("id", "name", "year").From("demo.user").Limit(100) },
			sql:   `SELECT "id", "name", "year" FROM "demo"."user" LIMIT ?`,
			args:  []any{100},
		},
		{
			// ExampleSelectBuilder_varInCols: a raw expression projection
			// (go-sqlbuilder's Var) maps to sqlk's parameterized raw column;
			// the placeholder text and its binding are both supplied to
			// SelectRaw.
			name: "raw expression column keeps a placeholder",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.Select("colHasA$Sign").SelectRaw("?", "foo").From("table")
			},
			sql: `SELECT "colHasA$Sign", ? FROM "table"`,
			// sqlk wraps unknown identifier characters verbatim; the $ sign
			// is not a marker and survives inside the quoted identifier.
			args: []any{"foo"},
		},
		{
			name:  "distinct projection",
			build: func(q *sqlk.Query) *sqlk.Query { return q.Distinct().Select("id", "name").From("user") },
			sql:   `SELECT DISTINCT "id", "name" FROM "user"`,
		},
		{
			// sb.As("COUNT(*)", "t") -> a raw aggregate column with an alias.
			name: "raw aggregate column with alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.Distinct().Select("id", "name").SelectRaw("COUNT(*) AS t").From("user")
			},
			sql: `SELECT DISTINCT "id", "name", COUNT(*) AS t FROM "user"`,
		},
		{
			name:  "table alias via as",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("user as u").Select("u.id", "u.name") },
			sql:   `SELECT "u"."id", "u"."name" FROM "user" AS "u"`,
		},
		{
			name:  "column alias via as",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("user").Select("name as FullName") },
			sql:   `SELECT "name" AS "FullName" FROM "user"`,
		},
		{
			// ExampleSelectBuilder_ForUpdate: sqlk has no FOR UPDATE clause
			// (out of scope per spec), so the scenario reduces to a plain
			// filtered select — included to confirm the where shape.
			name:  "filtered select (for-update scenario without the clause)",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("user").WhereEq("id", 1234) },
			sql:   `SELECT * FROM "user" WHERE "id" = ?`,
			args:  []any{1234},
		},
	})
}

// TestConditionScenarios migrates cond_test.go's operator coverage onto
// sqlk's Where family. go-sqlbuilder's Like is case-sensitive plain LIKE;
// sqlk's WhereLike is case-insensitive by default, so CaseSensitive() is
// supplied to match the case-sensitive shape. IsDistinctFrom / Any / All /
// Some have no sqlk verb and are expressed through WhereRaw where useful.
func TestConditionScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			name:  "equal",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereEq("a", 123) },
			sql:   `SELECT * FROM "t" WHERE "a" = ?`,
			args:  []any{123},
		},
		{
			name:  "not equal",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "<>", 123) },
			sql:   `SELECT * FROM "t" WHERE "a" <> ?`,
			args:  []any{123},
		},
		{
			name:  "greater than",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", ">", 123) },
			sql:   `SELECT * FROM "t" WHERE "a" > ?`,
			args:  []any{123},
		},
		{
			name:  "greater equal",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", ">=", 123) },
			sql:   `SELECT * FROM "t" WHERE "a" >= ?`,
			args:  []any{123},
		},
		{
			name:  "less than",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "<", 123) },
			sql:   `SELECT * FROM "t" WHERE "a" < ?`,
			args:  []any{123},
		},
		{
			name:  "less equal",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "<=", 123) },
			sql:   `SELECT * FROM "t" WHERE "a" <= ?`,
			args:  []any{123},
		},
		{
			name:  "in value list",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereIn("a", 1, 2, 3) },
			sql:   `SELECT * FROM "t" WHERE "a" IN (?, ?, ?)`,
			args:  []any{1, 2, 3},
		},
		{
			// go-sqlbuilder's In("$a") compiles to "0 = 1" — a false
			// tautology. sqlk expresses the same intent as "1 = 0" with an
			// explanatory comment (reversed operands, same semantics).
			name:  "in empty list is a false tautology",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereIn("a") },
			sql:   `SELECT * FROM "t" WHERE 1 = 0 /* IN [empty list] */`,
		},
		{
			name:  "not in value list",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotIn("a", 1, 2, 3) },
			sql:   `SELECT * FROM "t" WHERE "a" NOT IN (?, ?, ?)`,
			args:  []any{1, 2, 3},
		},
		{
			// go-sqlbuilder's NotIn("$a") compiles to "0 = 0" — a true
			// tautology. sqlk expresses the same intent as "1 = 1" with an
			// explanatory comment (reversed operands, same semantics).
			name:  "not in empty list is a true tautology",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotIn("a") },
			sql:   `SELECT * FROM "t" WHERE 1 = 1 /* NOT IN [empty list] */`,
		},
		{
			// go-sqlbuilder Like is case-sensitive plain LIKE.
			name:  "like is case sensitive plain like",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereLike("a", "%Huan%", sqlk.CaseSensitive()) },
			sql:   `SELECT * FROM "t" WHERE "a" like ?`,
			args:  []any{"%Huan%"},
		},
		{
			// go-sqlbuilder NotLike emits "a NOT LIKE ?". sqlk has no
			// NOT LIKE operator; it wraps the whole comparison as
			// NOT (col like ?) instead.
			name:  "not like wraps the comparison in not",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotLike("a", "%Huan%", sqlk.CaseSensitive()) },
			sql:   `SELECT * FROM "t" WHERE NOT ("a" like ?)`,
			args:  []any{"%Huan%"},
		},
		{
			name:  "is null",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNull("a") },
			sql:   `SELECT * FROM "t" WHERE "a" IS NULL`,
		},
		{
			name:  "is not null",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotNull("a") },
			sql:   `SELECT * FROM "t" WHERE "a" IS NOT NULL`,
		},
		{
			name:  "between",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereBetween("a", 123, 456) },
			sql:   `SELECT * FROM "t" WHERE "a" BETWEEN ? AND ?`,
			args:  []any{123, 456},
		},
		{
			name:  "not between",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotBetween("a", 123, 456) },
			sql:   `SELECT * FROM "t" WHERE "a" NOT BETWEEN ? AND ?`,
			args:  []any{123, 456},
		},
		{
			// go-sqlbuilder Or("1 = 1", "2 = 2", "3 = 3") -> a parenthesized
			// OR group. As the sole condition, the leading OR connector is
			// dropped (sqlk omits the connector on the first condition).
			name: "or group",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").WhereGroup(func(n *sqlk.Query) *sqlk.Query {
					return n.WhereRaw("1 = 1").OrWhereRaw("2 = 2").OrWhereRaw("3 = 3")
				})
			},
			sql: `SELECT * FROM "t" WHERE (1 = 1 OR 2 = 2 OR 3 = 3)`,
		},
		{
			// cond.Not("1 = 1") -> NOT (...). sqlk has no NotRaw verb, so the
			// negation is written as a raw NOT (...) condition.
			name:  "not wraps a raw condition",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereRaw("NOT (1 = 1)") },
			sql:   `SELECT * FROM "t" WHERE NOT (1 = 1)`,
		},
		{
			// cond.Exists / NotExists take a subquery in sqlk; the correlated
			// predicate is a column-to-column comparison (WhereColumns).
			name: "exists subquery",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").WhereExists(sqlk.NewQuery().From("c").WhereColumns("c.t_id", "=", "t.id"))
			},
			sql: `SELECT * FROM "t" WHERE EXISTS (SELECT 1 FROM "c" WHERE "c"."t_id" = "t"."id")`,
		},
		{
			name: "not exists subquery",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").WhereNotExists(sqlk.NewQuery().From("c"))
			},
			sql: `SELECT * FROM "t" WHERE NOT EXISTS (SELECT 1 FROM "c")`,
		},
		{
			// builder_test's BuildNamed scenario: a named argument referenced
			// by name. sqlk models this as Define + Variable: the definition
			// binds as a plain parameter at compile time.
			name: "named variable resolves to its definition",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("products").Define("@name", "Anto").Where("ProductName", "=", sqlk.NewVariable("@name"))
			},
			sql:  `SELECT * FROM "products" WHERE "ProductName" = ?`,
			args: []any{"Anto"},
		},
	})
}

// TestWhereComposition migrates ExampleSelectBuilder's full where section:
// AND-chain, a LIKE, an OR-group with IS NULL and IN, a NOT IN over a
// subquery, and a raw arithmetic condition — followed by GROUP BY, HAVING
// NOT IN, ORDER BY, LIMIT and OFFSET.
func TestWhereComposition(t *testing.T) {
	build := func(q *sqlk.Query) *sqlk.Query {
		return q.Distinct().Select("id", "name").SelectRaw("COUNT(*) AS t").From("demo.user").
			Where("id", ">", 1234).
			WhereLike("name", "%Du", sqlk.CaseSensitive()).
			WhereGroup(func(n *sqlk.Query) *sqlk.Query {
				return n.WhereNull("id_card").OrWhereIn("status", 1, 2, 5)
			}).
			WhereNotInSub("id", sqlk.NewQuery().Select("id").From("banned")).
			WhereRaw("modified_at > created_at + ?", 86400).
			GroupBy("status").
			HavingNotIn("status", 4, 5).
			OrderBy("modified_at").
			Limit(10).Offset(5)
	}
	want := `SELECT DISTINCT "id", "name", COUNT(*) AS t FROM "demo"."user" ` +
		`WHERE "id" > ? AND "name" like ? AND ("id_card" IS NULL OR "status" IN (?, ?, ?)) ` +
		`AND "id" NOT IN (SELECT "id" FROM "banned") AND modified_at > created_at + ? ` +
		`GROUP BY "status" HAVING "status" NOT IN (?, ?) ORDER BY "modified_at" LIMIT ? OFFSET ?`
	res, err := compiler.New().Compile(build(sqlk.NewQuery()))
	if err != nil {
		t.Fatalf("Compile(...) error = %v, want nil", err)
	}
	if res.SQL != want {
		t.Errorf("Compile(...) SQL = %q\nwant %q", res.SQL, want)
	}
	wantArgs := []any{1234, "%Du", 1, 2, 5, 86400, 4, 5, 10, int64(5)}
	if !reflect.DeepEqual(res.Args, wantArgs) {
		t.Errorf("Compile(...) Args = %#v, want %#v", res.Args, wantArgs)
	}
}

// TestJoinScenarios migrates the ExampleSelectBuilder_join / nestedJoin
// scenarios: inner/left/right/cross joins, joins carrying extra conditions,
// and a subquery join target.
func TestJoinScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// ExampleSelectBuilder_join: an inner join with an extra IN
			// condition on the ON scope, plus a right join with a LIKE.
			name: "inner and right join with on conditions",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.Select("u.id", "u.name", "c.type", "p.nickname").From("user as u").
					JoinOn("contract as c", func(j *sqlk.Join) *sqlk.Join {
						return j.On("u.id", "=", "c.user_id").WhereIn("c.status", 1, 2, 5)
					}).
					RightJoinOn("person as p", func(j *sqlk.Join) *sqlk.Join {
						return j.On("u.id", "=", "p.user_id").WhereLike("p.surname", "%Du", sqlk.CaseSensitive())
					}).
					WhereRaw("u.modified_at > u.created_at + ?", 86400)
			},
			sql: `SELECT "u"."id", "u"."name", "c"."type", "p"."nickname" FROM "user" AS "u" ` +
				"\n" + `INNER JOIN "contract" AS "c" ON "u"."id" = "c"."user_id" AND "c"."status" IN (?, ?, ?)` +
				"\n" + `RIGHT JOIN "person" AS "p" ON "u"."id" = "p"."user_id" AND "p"."surname" like ? ` +
				`WHERE u.modified_at > u.created_at + ?`,
			args: []any{1, 2, 5, "%Du", 86400},
		},
		{
			// ExampleSelectBuilder_nestedJoin: a subquery as the join target.
			// The subquery's own As sets the derived-table alias.
			name: "nested join with a subquery target",
			build: func(q *sqlk.Query) *sqlk.Query {
				nested := sqlk.NewQuery().Select("b.id", "b.user_id").From("users2 as b").Where("b.age", ">", 20).As("b")
				return q.Select("a.id", "a.user_id").From("users as a").
					JoinSub(nested, func(j *sqlk.Join) *sqlk.Join { return j.On("a.user_id", "=", "b.user_id") })
			},
			sql: `SELECT "a"."id", "a"."user_id" FROM "users" AS "a" ` +
				"\n" + `INNER JOIN (SELECT "b"."id", "b"."user_id" FROM "users2" AS "b" WHERE "b"."age" > ?) AS "b" ON "a"."user_id" = "b"."user_id"`,
			args: []any{20},
		},
		{
			name: "left and cross join",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("user as u").
					LeftJoinEq("profile as p", "p.user_id", "u.id").
					CrossJoin("extra")
			},
			sql: `SELECT * FROM "user" AS "u" ` +
				"\n" + `LEFT JOIN "profile" AS "p" ON "p"."user_id" = "u"."id"` +
				"\n" + `CROSS JOIN "extra"`,
		},
	})
}

// TestGroupHavingOrderLimit migrates the ORDER BY / LIMIT / OFFSET and
// GROUP BY / HAVING scenarios (ExampleSelectBuilder_OrderByAsc/Desc and the
// full section ordering). sqlk's ascending order omits the ASC keyword
// (ascending is the default), a faithful rendering of the same scenario.
func TestGroupHavingOrderLimit(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// ExampleSelectBuilder_OrderByAsc: ascending order.
			name: "order ascending omits the asc keyword",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.Select("id", "name", "score").From("users").Where("score", ">", 0).OrderBy("name")
			},
			sql:  `SELECT "id", "name", "score" FROM "users" WHERE "score" > ? ORDER BY "name"`,
			args: []any{0},
		},
		{
			// ExampleSelectBuilder_OrderByDesc: descending order.
			name: "order descending",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.Select("id", "name", "score").From("users").Where("score", ">", 0).OrderByDesc("score")
			},
			sql:  `SELECT "id", "name", "score" FROM "users" WHERE "score" > ? ORDER BY "score" DESC`,
			args: []any{0},
		},
		{
			// ExampleSelectBuilder_OrderByAsc_multiple: mixed directions.
			name: "mixed order directions",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.Select("id", "name", "score").From("users").Where("score", ">", 0).OrderByDesc("score").OrderBy("name").OrderByDesc("id")
			},
			sql:  `SELECT "id", "name", "score" FROM "users" WHERE "score" > ? ORDER BY "score" DESC, "name", "id" DESC`,
			args: []any{0},
		},
		{
			name:  "group by with having",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").GroupBy("status").HavingNotIn("status", 4, 5) },
			sql:   `SELECT * FROM "users" GROUP BY "status" HAVING "status" NOT IN (?, ?)`,
			args:  []any{4, 5},
		},
		{
			// Full section ordering: where, group, having, order, limit.
			name: "full section order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").Where("age", ">", 18).GroupBy("city").Having("total", ">", 100).OrderBy("name").Limit(10)
			},
			sql:  `SELECT * FROM "users" WHERE "age" > ? GROUP BY "city" HAVING "total" > ? ORDER BY "name" LIMIT ?`,
			args: []any{18, 100, 10},
		},
	})
}

// TestInsertScenarios migrates ExampleInsertInto / ExampleInsertBuilder /
// ExampleInsertBuilder_subSelect. The return-id shape is dialect-specific
// and covered by TestLastIdMatrix.
func TestInsertScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// ExampleInsertInto: columns + one row of values.
			name: "cols and values",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("demo.user").InsertColumns([]string{"id", "name", "status"}, []any{4, "Sample", 2})
			},
			sql:  `INSERT INTO "demo"."user" ("id", "name", "status") VALUES (?, ?, ?)`,
			args: []any{4, "Sample", 2},
		},
		{
			// ExampleInsertBuilder: multiple rows share one column list; a
			// raw expression value (UNIX_TIMESTAMP(NOW())) is inlined.
			name: "multi-row values with a raw expression",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("demo.user").InsertRows([]string{"id", "name", "status", "created_at"},
					[]any{1, "Huan Du", 1, sqlk.NewUnsafeLiteral("UNIX_TIMESTAMP(NOW())")},
					[]any{2, "Charmy Liu", 1, 1234567890})
			},
			sql:  `INSERT INTO "demo"."user" ("id", "name", "status", "created_at") VALUES (?, ?, ?, UNIX_TIMESTAMP(NOW())), (?, ?, ?, ?)`,
			args: []any{1, "Huan Du", 1, 2, "Charmy Liu", 1, 1234567890},
		},
		{
			// ExampleInsertBuilder_subSelect: insert into select.
			name: "insert from select",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("demo.user").InsertFrom([]string{"id", "name"},
					sqlk.NewQuery().Select("id", "name").From("demo.test").WhereEq("id", 1))
			},
			sql:  `INSERT INTO "demo"."user" ("id", "name") SELECT "id", "name" FROM "demo"."test" WHERE "id" = ?`,
			args: []any{1},
		},
		{
			// ExampleInsertBuilder_NumValue: the key-value form.
			name: "key-value form sorts map keys",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("demo.user").Insert(sqlk.Record{"name": "Huan Du", "id": 1})
			},
			sql:  `INSERT INTO "demo"."user" ("id", "name") VALUES (?, ?)`,
			args: []any{1, "Huan Du"},
		},
		{
			// A NULL value is parameterized like any other value.
			name: "null value is parameterized",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("books").InsertColumns([]string{"id", "author", "isbn", "date"}, []any{1, "Author 1", "123456", nil})
			},
			sql:  `INSERT INTO "books" ("id", "author", "isbn", "date") VALUES (?, ?, ?, ?)`,
			args: []any{1, "Author 1", "123456", nil},
		},
	})
}

// TestUpdateScenarios migrates ExampleUpdate / ExampleUpdateBuilder and the
// Incr/Decr/Add/Sub assignment modifiers. sqlk has no UPDATE ... RETURNING
// (out of scope per spec), so the returning scenarios are reduced to the
// update body. The increment family covers go-sqlbuilder's Incr/Decr/Add/Sub.
func TestUpdateScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// ExampleUpdate: a raw set expression with a raw where.
			name: "raw set and raw where",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("demo.user").UpdateColumns([]string{"visited"}, []any{sqlk.NewUnsafeLiteral("visited + 1")}).WhereRaw("id = 1234")
			},
			sql: `UPDATE "demo"."user" SET "visited" = visited + 1 WHERE id = 1234`,
		},
		{
			// ExampleUpdateBuilder: assign + increment + raw set in one SET
			// clause, with a where section mirroring ExampleSelectBuilder.
			// sqlk's Increment verb replaces the whole set, so to keep all
			// three assignments in one SET (matching go-sqlbuilder), the
			// increment and raw expression are written as raw set values.
			name: "assign increment and raw set with where",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("demo.user").
					UpdateColumns(
						[]string{"type", "credit", "modified_at"},
						[]any{"sys", sqlk.NewUnsafeLiteral("credit + 1"), sqlk.NewUnsafeLiteral("UNIX_TIMESTAMP(NOW())")},
					).
					Where("id", ">", 1234).
					WhereLike("name", "%Du", sqlk.CaseSensitive()).
					WhereGroup(func(n *sqlk.Query) *sqlk.Query {
						return n.WhereNull("id_card").OrWhereIn("status", 1, 2, 5)
					}).
					WhereRaw("modified_at > created_at + ?", 86400).
					OrderBy("id")
			},
			// sqlk's UPDATE compiler does not carry ORDER BY into the
			// statement, so the trailing OrderBy is dropped (the where
			// section is what is asserted).
			sql:  `UPDATE "demo"."user" SET "type" = ?, "credit" = credit + 1, "modified_at" = UNIX_TIMESTAMP(NOW()) WHERE "id" > ? AND "name" like ? AND ("id_card" IS NULL OR "status" IN (?, ?, ?)) AND modified_at > created_at + ?`,
			args: []any{"sys", 1234, "%Du", 1, 2, 5, 86400},
		},
		{
			// TestUpdateAssignments: Incr / Decr / Add / Sub / Mul / Div.
			// sqlk exposes Increment (Incr/Add) and Decrement (Decr/Sub);
			// Mul and Div have no verb and are expressed as raw set values.
			name:  "increment defaults to one",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Increment("f") },
			sql:   `UPDATE "t" SET "f" = "f" + ?`,
			args:  []any{1},
		},
		{
			name:  "decrement defaults to one",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Decrement("f") },
			sql:   `UPDATE "t" SET "f" = "f" - ?`,
			args:  []any{1},
		},
		{
			name:  "add with amount",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Increment("f", 123) },
			sql:   `UPDATE "t" SET "f" = "f" + ?`,
			args:  []any{123},
		},
		{
			name:  "sub with amount",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Decrement("f", 123) },
			sql:   `UPDATE "t" SET "f" = "f" - ?`,
			args:  []any{123},
		},
		{
			name: "mul as a raw set expression",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").UpdateColumns([]string{"f"}, []any{sqlk.NewUnsafeLiteral("f * 123")})
			},
			sql: `UPDATE "t" SET "f" = f * 123`,
		},
		{
			name: "div as a raw set expression",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").UpdateColumns([]string{"f"}, []any{sqlk.NewUnsafeLiteral("f / 123")})
			},
			sql: `UPDATE "t" SET "f" = f / 123`,
		},
		{
			// TestUpdateBuilderReturning's ORDER BY + LIMIT scenario. sqlk's
			// UPDATE compiler does not carry ORDER BY or LIMIT into the
			// statement (they are select-section clauses), so the migrated
			// shape drops them — a faithful rendering of sqlk's behavior.
			name: "update with where drops order and limit",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("user").WhereEq("status", 1).Update(sqlk.Record{"name": "Test"}).OrderBy("id").Limit(5)
			},
			sql:  `UPDATE "user" SET "name" = ? WHERE "status" = ?`,
			args: []any{"Test", 1},
		},
	})
}

// TestDeleteScenarios migrates ExampleDeleteFrom / ExampleDeleteBuilder and
// the delete-with-join shape. The USING form is postgres-specific and
// covered by TestMysqlDeleteJoin's sibling shape on mysql; postgres in sqlk
// uses the base "DELETE target FROM table JOIN" form for joined deletes.
func TestDeleteScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// ExampleDeleteFrom: a raw where with a limit. sqlk's DELETE
			// compiler does not carry LIMIT into the statement (unlike
			// go-sqlbuilder's mysql flavor), so the migrated shape drops it.
			name:  "delete with raw where drops limit",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("demo.user").WhereRaw("status = 1").Limit(10).Delete() },
			sql:   `DELETE FROM "demo"."user" WHERE status = 1`,
		},
		{
			// ExampleDeleteBuilder: a where section mirroring the select.
			name: "delete with composed where",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("demo.user").
					Where("id", ">", 1234).
					WhereLike("name", "%Du", sqlk.CaseSensitive()).
					WhereGroup(func(n *sqlk.Query) *sqlk.Query {
						return n.WhereNull("id_card").OrWhereIn("status", 1, 2, 5)
					}).
					WhereRaw("modified_at > created_at + ?", 86400).
					Delete()
			},
			sql:  `DELETE FROM "demo"."user" WHERE "id" > ? AND "name" like ? AND ("id_card" IS NULL OR "status" IN (?, ?, ?)) AND modified_at > created_at + ?`,
			args: []any{1234, "%Du", 1, 2, 5, 86400},
		},
		{
			// TestDeleteBuilderReturning's ORDER BY + LIMIT scenario. As with
			// UPDATE, sqlk's DELETE compiler drops ORDER BY and LIMIT.
			name: "delete with where drops order and limit",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("user").WhereEq("status", 1).OrderBy("id").Limit(5).Delete()
			},
			sql:  `DELETE FROM "user" WHERE "status" = ?`,
			args: []any{1},
		},
	})
}

// TestCTEScenarios migrates ExampleWith / ExampleCTEBuilder / the CTE update
// and delete scenarios. go-sqlbuilder's CTETable(name, cols...) shape (a CTE
// with an explicit column list) has no direct sqlk verb; the column list is
// dropped and the body's own projection carries the columns, matching sqlk's
// With model. Recursive CTEs use sqlk's WithFunc with a self-referential body.
func TestCTEScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// ExampleWith: two CTEs, one referenced in FROM and JOIN.
			name: "multiple ctes with join",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.With("users", sqlk.NewQuery().Select("id", "name").From("users").WhereRaw("prime IS NOT NULL")).
					With("orders", sqlk.NewQuery().Select("id", "user_id").From("orders")).
					From("orders").Select("orders.id").
					JoinEq("users", "orders.user_id", "users.id").
					Limit(10)
			},
			sql: `WITH "users" AS (SELECT "id", "name" FROM "users" WHERE prime IS NOT NULL),` + "\n" +
				`"orders" AS (SELECT "id", "user_id" FROM "orders")` + "\n" +
				`SELECT "orders"."id" FROM "orders" ` + "\n" +
				`INNER JOIN "users" ON "orders"."user_id" = "users"."id" LIMIT ?`,
			args: []any{10},
		},
		{
			// ExampleCTEBuilder: a CTE referenced in FROM and JOIN with a
			// where on the CTE column.
			name: "cte referenced in from and join with where",
			build: func(q *sqlk.Query) *sqlk.Query {
				body := sqlk.NewQuery().Select("id", "name", "level").From("users").Where("level", ">=", 10)
				return q.With("valid_users", body).
					From("users").Select("valid_users.id", "valid_users.name", "orders.id").
					JoinEq("orders", "users.id", "orders.user_id").
					Where("orders.price", "<=", 200).
					WhereRaw("valid_users.level < orders.min_level").
					OrderByDesc("orders.price")
			},
			sql: `WITH "valid_users" AS (SELECT "id", "name", "level" FROM "users" WHERE "level" >= ?)` + "\n" +
				`SELECT "valid_users"."id", "valid_users"."name", "orders"."id" FROM "users" ` + "\n" +
				`INNER JOIN "orders" ON "users"."id" = "orders"."user_id" ` +
				`WHERE "orders"."price" <= ? AND valid_users.level < orders.min_level ORDER BY "orders"."price" DESC`,
			args: []any{10, 200},
		},
		{
			// ExampleCTEBuilder_update: a CTE driving an update.
			name: "cte driving an update",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.With("users", sqlk.NewQuery().Select("user_id").From("vip_users")).
					From("orders").UpdateColumns([]string{"transport_fee"}, []any{sqlk.NewUnsafeLiteral("0")}).
					WhereRaw("users.user_id = orders.user_id")
			},
			sql: `WITH "users" AS (SELECT "user_id" FROM "vip_users")` + "\n" +
				`UPDATE "orders" SET "transport_fee" = 0 WHERE users.user_id = orders.user_id`,
		},
		{
			// ExampleCTEBuilder_delete: a CTE driving a delete.
			name: "cte driving a delete",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.With("users", sqlk.NewQuery().Select("user_id").From("cheaters")).
					From("awards").WhereRaw("users.user_id = awards.user_id").Delete()
			},
			sql: `WITH "users" AS (SELECT "user_id" FROM "cheaters")` + "\n" +
				`DELETE FROM "awards" WHERE users.user_id = awards.user_id`,
		},
		{
			// ExampleWithRecursive: a recursive CTE. sqlk does not model a
			// RECURSIVE keyword (the body simply references the CTE alias as
			// a table), so the body is a UNION of an anchor and a
			// self-referential member, and the outer query joins the CTE.
			name: "recursive cte",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.WithFunc("source_accounts", func(sq *sqlk.Query) *sqlk.Query {
					anchor := sqlk.NewQuery().Select("p.id", "p.parent_id").From("accounts as p").WhereEq("p.id", 2)
					recursive := sqlk.NewQuery().Select("c.id", "c.parent_id").From("accounts as c").
						JoinEq("source_accounts as sa", "c.parent_id", "sa.id")
					return anchor.Union(recursive)
				}).Select("o.id", "o.date", "o.amount").From("orders as o").
					JoinEq("source_accounts", "o.account_id", "source_accounts.id")
			},
			sql: `WITH "source_accounts" AS (` +
				`SELECT "p"."id", "p"."parent_id" FROM "accounts" AS "p" WHERE "p"."id" = ? ` +
				`UNION SELECT "c"."id", "c"."parent_id" FROM "accounts" AS "c" ` +
				"\n" + `INNER JOIN "source_accounts" AS "sa" ON "c"."parent_id" = "sa"."id")` + "\n" +
				`SELECT "o"."id", "o"."date", "o"."amount" FROM "orders" AS "o" ` + "\n" +
				`INNER JOIN "source_accounts" ON "o"."account_id" = "source_accounts"."id"`,
			args: []any{2},
		},
	})
}

// TestCombineScenarios migrates ExampleUnion / ExampleUnionAll and the
// union order-by / limit scenarios. sqlk flattens combine members into a
// bare sequence without parentheses (matching sqlite's go-sqlbuilder form).
func TestCombineScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// ExampleUnion: two selects unioned, with an order by. go-sqlbuilder
			// wraps each member in parens and puts ORDER BY at the tail
			// ((SELECT ...) UNION (SELECT ...) ORDER BY ...). sqlk emits members
			// bare and attaches the ORDER BY to the main query before the
			// combine — a different shape, asserted here as sqlk renders it.
			name: "union with main query order by",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.Select("id", "name", "created_at").From("demo.user").Where("id", ">", 1234).
					Union(sqlk.NewQuery().Select("id", "avatar").From("demo.user_profile").WhereIn("status", 1, 2, 5)).
					OrderByDesc("created_at")
			},
			sql:  `SELECT "id", "name", "created_at" FROM "demo"."user" WHERE "id" > ? ORDER BY "created_at" DESC UNION SELECT "id", "avatar" FROM "demo"."user_profile" WHERE "status" IN (?, ?, ?)`,
			args: []any{1234, 1, 2, 5},
		},
		{
			// ExampleUnionAll: union all with pagination. As above, sqlk puts
			// the main query's order/limit before the combine, not after the
			// union as go-sqlbuilder does.
			name: "union all with main query pagination",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.Select("id", "name", "created_at").From("demo.user").Where("id", ">", 1234).
					UnionAll(sqlk.NewQuery().From("demo.user_profile")).
					OrderBy("created_at").Limit(100).Offset(5)
			},
			sql:  `SELECT "id", "name", "created_at" FROM "demo"."user" WHERE "id" > ? ORDER BY "created_at" LIMIT ? OFFSET ? UNION ALL SELECT * FROM "demo"."user_profile"`,
			args: []any{1234, 100, int64(5)},
		},
		{
			name:  "except",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("a").Except(sqlk.NewQuery().From("b")) },
			sql:   `SELECT * FROM "a" EXCEPT SELECT * FROM "b"`,
		},
		{
			name:  "intersect all",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("a").IntersectAll(sqlk.NewQuery().From("b")) },
			sql:   `SELECT * FROM "a" INTERSECT ALL SELECT * FROM "b"`,
		},
		{
			// A raw combine member (go-sqlbuilder Build("TABLE ...")).
			name:  "raw combine member",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("a").CombineRaw("UNION ALL SELECT * FROM b") },
			sql:   `SELECT * FROM "a" UNION ALL SELECT * FROM b`,
		},
	})
}

// TestAggregateScenarios covers sqlk's aggregate verbs. The go-sqlbuilder
// scenario this maps to is the raw `As("COUNT(*)", "t")` projection (in
// TestSelectScenarios); the verb forms here are sqlk-robustness coverage of
// the SqlKata-origin aggregate API rather than direct go-sqlbuilder
// migrations. The base compiler degrades FILTER to CASE WHEN; postgres and
// sqlite support FILTER (covered by TestFilterClauseMatrix).
func TestAggregateScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			name:  "count star",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("a").Count() },
			sql:   `SELECT COUNT(*) AS "count" FROM "a"`,
		},
		{
			name:  "count column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("a").Count("user_id") },
			sql:   `SELECT COUNT("user_id") AS "count" FROM "a"`,
		},
		{
			name:  "sum",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("a").Sum("total") },
			sql:   `SELECT SUM("total") AS "sum" FROM "a"`,
		},
		{
			name:  "avg",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("a").Avg("ttl") },
			sql:   `SELECT AVG("ttl") AS "avg" FROM "a"`,
		},
		{
			name:  "max",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("a").Max("latency") },
			sql:   `SELECT MAX("latency") AS "max" FROM "a"`,
		},
		{
			name:  "min",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("a").Min("latency") },
			sql:   `SELECT MIN("latency") AS "min" FROM "a"`,
		},
		{
			// SelectCount as a projection column (no alias).
			name:  "select count column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("a").SelectCount("user_id") },
			sql:   `SELECT COUNT("user_id") FROM "a"`,
		},
		{
			// Filter degrades to CASE WHEN on the base compiler.
			name: "aggregate filter degrades to case when",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("emp").SelectSum("salary", func(f *sqlk.Query) *sqlk.Query { return f.WhereEq("active", true) })
			},
			sql:  `SELECT SUM(CASE WHEN "active" = ? THEN "salary" END) FROM "emp"`,
			args: []any{true},
		},
	})
}

// TestFilterClauseMatrix covers the aggregate FILTER clause across dialects.
// FILTER is a SqlKata-origin sqlk feature (no go-sqlbuilder analog); included
// as cross-dialect robustness. postgres and sqlite support
// FILTER (WHERE ...); mysql, sqlserver, and oracle degrade to CASE WHEN.
func TestFilterClauseMatrix(t *testing.T) {
	build := func(q *sqlk.Query) *sqlk.Query {
		return q.From("a").SelectSum("total", func(f *sqlk.Query) *sqlk.Query { return f.WhereEq("country", "US") })
	}
	runDialectCases(t, []dialectCase{
		{
			name:  "aggregate filter",
			build: build,
			want: map[string]wantResult{
				sqlk.EnginePostgres:  {sql: `SELECT SUM("total") FILTER (WHERE "country" = ?) FROM "a"`, args: []any{"US"}},
				sqlk.EngineSqlite:    {sql: `SELECT SUM("total") FILTER (WHERE "country" = ?) FROM "a"`, args: []any{"US"}},
				sqlk.EngineMysql:     {sql: "SELECT SUM(CASE WHEN `country` = ? THEN `total` END) FROM `a`", args: []any{"US"}},
				sqlk.EngineSqlserver: {sql: `SELECT SUM(CASE WHEN [country] = ? THEN [total] END) FROM [a]`, args: []any{"US"}},
				sqlk.EngineOracle:    {sql: `SELECT SUM(CASE WHEN "country" = ? THEN "total" END) FROM "a"`, args: []any{"US"}},
			},
		},
	})
}

// TestLimitOffsetMatrix migrates ExampleSelectBuilder_limit_offset's flavor
// loop, restricted to the five sqlk dialects. sqlk treats an unset limit or
// offset as the zero value (a clause not called, or called with 0), not -1
// as go-sqlbuilder does, so the matrix is expressed in sqlk's unset terms:
// omit the call to mean "unset". The expected SQL is sqlk's per-dialect
// output.
func TestLimitOffsetMatrix(t *testing.T) {
	cases := []dialectCase{
		{
			// No limit and no offset -> no pagination section.
			name:  "no pagination",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("user") },
			want: map[string]wantResult{
				sqlk.EngineMysql:     {sql: "SELECT * FROM `user`"},
				sqlk.EnginePostgres:  {sql: `SELECT * FROM "user"`},
				sqlk.EngineSqlite:    {sql: `SELECT * FROM "user"`},
				sqlk.EngineSqlserver: {sql: `SELECT * FROM [user]`},
				sqlk.EngineOracle:    {sql: `SELECT * FROM "user"`},
			},
		},
		{
			// Limit only. mysql/postgres/sqlite emit LIMIT; sqlserver folds
			// to TOP; oracle emits OFFSET-FETCH with a zero offset and a
			// safe order.
			name:  "limit only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("user").Limit(1) },
			want: map[string]wantResult{
				sqlk.EngineMysql:     {sql: "SELECT * FROM `user` LIMIT ?", args: []any{1}},
				sqlk.EnginePostgres:  {sql: `SELECT * FROM "user" LIMIT ?`, args: []any{1}},
				sqlk.EngineSqlite:    {sql: `SELECT * FROM "user" LIMIT ?`, args: []any{1}},
				sqlk.EngineSqlserver: {sql: `SELECT TOP (?) * FROM [user]`, args: []any{1}},
				sqlk.EngineOracle:    {sql: `SELECT * FROM "user" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`, args: []any{int64(0), 1}},
			},
		},
		{
			// Offset only. mysql and sqlite add a ceiling LIMIT; postgres
			// emits a lone OFFSET; sqlserver and oracle emit OFFSET ROWS
			// with a safe order.
			name:  "offset only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("user").Offset(20) },
			want: map[string]wantResult{
				sqlk.EngineMysql:     {sql: "SELECT * FROM `user` LIMIT 18446744073709551615 OFFSET ?", args: []any{int64(20)}},
				sqlk.EnginePostgres:  {sql: `SELECT * FROM "user" OFFSET ?`, args: []any{int64(20)}},
				sqlk.EngineSqlite:    {sql: `SELECT * FROM "user" LIMIT -1 OFFSET ?`, args: []any{int64(20)}},
				sqlk.EngineSqlserver: {sql: `SELECT * FROM [user] ORDER BY (SELECT 0) OFFSET ? ROWS`, args: []any{int64(20)}},
				sqlk.EngineOracle:    {sql: `SELECT * FROM "user" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS`, args: []any{int64(20)}},
			},
		},
		{
			// Limit and a zero offset -> the offset is unset, so only LIMIT.
			name:  "limit with zero offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("user").Limit(1).Offset(0) },
			want: map[string]wantResult{
				sqlk.EngineMysql:     {sql: "SELECT * FROM `user` LIMIT ?", args: []any{1}},
				sqlk.EnginePostgres:  {sql: `SELECT * FROM "user" LIMIT ?`, args: []any{1}},
				sqlk.EngineSqlite:    {sql: `SELECT * FROM "user" LIMIT ?`, args: []any{1}},
				sqlk.EngineSqlserver: {sql: `SELECT TOP (?) * FROM [user]`, args: []any{1}},
				sqlk.EngineOracle:    {sql: `SELECT * FROM "user" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`, args: []any{int64(0), 1}},
			},
		},
		{
			// Limit and offset both set.
			name:  "limit and offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("user").Limit(1).Offset(1) },
			want: map[string]wantResult{
				sqlk.EngineMysql:     {sql: "SELECT * FROM `user` LIMIT ? OFFSET ?", args: []any{1, int64(1)}},
				sqlk.EnginePostgres:  {sql: `SELECT * FROM "user" LIMIT ? OFFSET ?`, args: []any{1, int64(1)}},
				sqlk.EngineSqlite:    {sql: `SELECT * FROM "user" LIMIT ? OFFSET ?`, args: []any{1, int64(1)}},
				sqlk.EngineSqlserver: {sql: `SELECT * FROM [user] ORDER BY (SELECT 0) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`, args: []any{int64(1), 1}},
				sqlk.EngineOracle:    {sql: `SELECT * FROM "user" ORDER BY (SELECT 0 FROM DUAL) OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`, args: []any{int64(1), 1}},
			},
		},
		{
			// Limit and offset with an order by (an existing order
			// suppresses the injected safe order on sqlserver/oracle).
			name:  "limit offset with order by",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("user").Limit(1).Offset(1).OrderBy("id") },
			want: map[string]wantResult{
				sqlk.EngineMysql:     {sql: "SELECT * FROM `user` ORDER BY `id` LIMIT ? OFFSET ?", args: []any{1, int64(1)}},
				sqlk.EnginePostgres:  {sql: `SELECT * FROM "user" ORDER BY "id" LIMIT ? OFFSET ?`, args: []any{1, int64(1)}},
				sqlk.EngineSqlite:    {sql: `SELECT * FROM "user" ORDER BY "id" LIMIT ? OFFSET ?`, args: []any{1, int64(1)}},
				sqlk.EngineSqlserver: {sql: `SELECT * FROM [user] ORDER BY [id] OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`, args: []any{int64(1), 1}},
				sqlk.EngineOracle:    {sql: `SELECT * FROM "user" ORDER BY "id" OFFSET ? ROWS FETCH NEXT ? ROWS ONLY`, args: []any{int64(1), 1}},
			},
		},
	}
	runDialectCases(t, cases)
}

// TestLastIdMatrix migrates TestInsertBuilderReturning's flavor loop: the
// return-id INSERT appends a dialect-specific last-id statement, except on
// oracle (which has none) and the base compiler. mysql appends nothing for a
// plain insert (return-id is a no-op flag without a dialect last-id).
func TestLastIdMatrix(t *testing.T) {
	build := func(q *sqlk.Query) *sqlk.Query {
		return q.From("user").InsertReturnId(sqlk.Record{"name": "Huan Du"})
	}
	runDialectCases(t, []dialectCase{
		{
			name:  "insert return id appends dialect last id",
			build: build,
			want: map[string]wantResult{
				sqlk.EngineSqlite:    {sql: `INSERT INTO "user" ("name") VALUES (?);select last_insert_rowid() as id`, args: []any{"Huan Du"}},
				sqlk.EnginePostgres:  {sql: `INSERT INTO "user" ("name") VALUES (?);SELECT lastval() AS id`, args: []any{"Huan Du"}},
				sqlk.EngineMysql:     {sql: "INSERT INTO `user` (`name`) VALUES (?);SELECT last_insert_id() as Id", args: []any{"Huan Du"}},
				sqlk.EngineSqlserver: {sql: `INSERT INTO [user] ([name]) VALUES (?);SELECT scope_identity() as Id`, args: []any{"Huan Du"}},
				sqlk.EngineOracle:    {sql: `INSERT INTO "user" ("name") VALUES (?)`, args: []any{"Huan Du"}},
			},
		},
	})
}

// TestIdentifierQuotingMatrix migrates the per-flavor identifier quoting
// shown across go-sqlbuilder's tests, restricted to the five sqlk dialects.
func TestIdentifierQuotingMatrix(t *testing.T) {
	build := func(q *sqlk.Query) *sqlk.Query {
		return q.From("users as u").Select("u.name as FullName")
	}
	runDialectCases(t, []dialectCase{
		{
			name:  "qualified and aliased identifiers",
			build: build,
			want: map[string]wantResult{
				sqlk.EngineSqlite:    {sql: `SELECT "u"."name" AS "FullName" FROM "users" AS "u"`},
				sqlk.EnginePostgres:  {sql: `SELECT "u"."name" AS "FullName" FROM "users" AS "u"`},
				sqlk.EngineMysql:     {sql: "SELECT `u`.`name` AS `FullName` FROM `users` AS `u`"},
				sqlk.EngineSqlserver: {sql: `SELECT [u].[name] AS [FullName] FROM [users] AS [u]`},
				sqlk.EngineOracle:    {sql: `SELECT "u"."name" "FullName" FROM "users" "u"`},
			},
		},
	})
}

// TestBooleanLiteralMatrix migrates the boolean literal rendering across
// dialects (sqlite 1/0, sqlserver cast(1/0 as bit), others true/false).
func TestBooleanLiteralMatrix(t *testing.T) {
	build := func(q *sqlk.Query) *sqlk.Query { return q.From("user").WhereTrue("is_active") }
	runDialectCases(t, []dialectCase{
		{
			name:  "where true",
			build: build,
			want: map[string]wantResult{
				sqlk.EngineSqlite:    {sql: `SELECT * FROM "user" WHERE "is_active" = 1`},
				sqlk.EnginePostgres:  {sql: `SELECT * FROM "user" WHERE "is_active" = true`},
				sqlk.EngineMysql:     {sql: "SELECT * FROM `user` WHERE `is_active` = true"},
				sqlk.EngineSqlserver: {sql: `SELECT * FROM [user] WHERE [is_active] = cast(1 as bit)`},
				sqlk.EngineOracle:    {sql: `SELECT * FROM "user" WHERE "is_active" = true`},
			},
		},
	})
}

// TestOracleInsertAll migrates ExampleInsertBuilder_flavorOracle: oracle
// multi-row inserts use INSERT ALL ... SELECT 1 FROM DUAL.
func TestOracleInsertAll(t *testing.T) {
	runCompileCases(t, compiler.NewOracle(), []compileCase{
		{
			name: "multi-row insert all",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("demo.user").InsertRows([]string{"id", "name", "status"},
					[]any{1, "Huan Du", 1}, []any{2, "Charmy Liu", 1})
			},
			sql:  `INSERT ALL INTO "demo"."user" ("id", "name", "status") VALUES (?, ?, ?) INTO "demo"."user" ("id", "name", "status") VALUES (?, ?, ?) SELECT 1 FROM DUAL`,
			args: []any{1, "Huan Du", 1, 2, "Charmy Liu", 1},
		},
		{
			// A single-row insert keeps the base shape.
			name: "single-row insert keeps base shape",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("demo.user").InsertRows([]string{"id", "name", "status"}, []any{1, "Huan Du", 1})
			},
			sql:  `INSERT INTO "demo"."user" ("id", "name", "status") VALUES (?, ?, ?)`,
			args: []any{1, "Huan Du", 1},
		},
	})
}

// TestMysqlDeleteJoin migrates the mysql multi-table DELETE form used when a
// delete carries a join (TestMysqlDeleteWithJoin). The from alias becomes the
// delete target.
func TestMysqlDeleteJoin(t *testing.T) {
	runCompileCases(t, compiler.NewMysql(), []compileCase{
		{
			name: "delete with join repeats the table as target",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("posts").
					Join("authors", "authors.id", "=", "posts.author_id").
					WhereEq("authors.id", 5).
					Delete()
			},
			sql:  "DELETE `posts` FROM `posts` \nINNER JOIN `authors` ON `authors`.`id` = `posts`.`author_id` WHERE `authors`.`id` = ?",
			args: []any{5},
		},
		{
			name: "delete with join and alias targets the alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("posts as p").
					Join("authors", "authors.id", "=", "p.author_id").
					WhereEq("authors.id", 5).
					Delete()
			},
			sql:  "DELETE `p` FROM `posts` AS `p` \nINNER JOIN `authors` ON `authors`.`id` = `p`.`author_id` WHERE `authors`.`id` = ?",
			args: []any{5},
		},
		{
			name:  "delete without join keeps the base shape",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("posts").WhereEq("id", 7).Delete() },
			sql:   "DELETE FROM `posts` WHERE `id` = ?",
			args:  []any{7},
		},
	})
}

// TestPostgresStringConditions migrates the postgres ILIKE behavior
// (cond_test's ILike/NotILike): case-insensitive matching compiles to ILIKE
// with the value keeping its case, instead of the LOWER wrapper.
func TestPostgresStringConditions(t *testing.T) {
	runCompileCases(t, compiler.NewPostgres(), []compileCase{
		{
			name:  "insensitive like compiles to ilike",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereLike("a", "%Upper Word%") },
			sql:   `SELECT * FROM "t" WHERE "a" ilike ?`,
			args:  []any{"%Upper Word%"},
		},
		{
			name: "sensitive like compiles to like",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").WhereLike("a", "%Upper Word%", sqlk.CaseSensitive())
			},
			sql:  `SELECT * FROM "t" WHERE "a" like ?`,
			args: []any{"%Upper Word%"},
		},
		{
			name:  "insensitive not like compiles to not ilike",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotLike("a", "%Upper Word%") },
			sql:   `SELECT * FROM "t" WHERE NOT ("a" ilike ?)`,
			args:  []any{"%Upper Word%"},
		},
	})
}

// TestValidationScenarios covers sqlk's compile-time validation
// (identifiable via errors.Is). These are sqlk-robustness cases, not direct
// go-sqlbuilder migrations — go-sqlbuilder's TestCondMisuse is about a Cond
// bound to the wrong builder, a misuse sqlk's API shape prevents.
func TestValidationScenarios(t *testing.T) {
	runErrCases(t, compiler.New(), []errCase{
		{
			// A non-whitelisted operator is rejected at compile time.
			name:    "non-whitelisted operator is rejected",
			build:   func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "startswith", 1) },
			wantErr: compiler.ErrOperatorNotAllowed,
		},
		{
			// A missing from target is rejected.
			name:    "missing from target is rejected",
			build:   func(q *sqlk.Query) *sqlk.Query { return q.Select("id") },
			wantErr: compiler.ErrNoFromTarget,
		},
	})
}

// TestInsensitiveLikeMatrix migrates cond_test's ILike scenario across
// dialects: case-insensitive matching renders as ILIKE on postgres (value
// keeps case) and as a LOWER(col) like ? wrapper on the other dialects
// (value lowercased). This is the TestCondWithFlavor ILIKE emulation.
func TestInsensitiveLikeMatrix(t *testing.T) {
	build := func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereLike("a", "%Huan%") }
	runDialectCases(t, []dialectCase{
		{
			name:  "insensitive like",
			build: build,
			want: map[string]wantResult{
				sqlk.EnginePostgres:  {sql: `SELECT * FROM "t" WHERE "a" ilike ?`, args: []any{"%Huan%"}},
				sqlk.EngineSqlite:    {sql: `SELECT * FROM "t" WHERE LOWER("a") like ?`, args: []any{"%huan%"}},
				sqlk.EngineMysql:     {sql: "SELECT * FROM `t` WHERE LOWER(`a`) like ?", args: []any{"%huan%"}},
				sqlk.EngineOracle:    {sql: `SELECT * FROM "t" WHERE LOWER("a") like ?`, args: []any{"%huan%"}},
				sqlk.EngineSqlserver: {sql: `SELECT * FROM [t] WHERE LOWER([a]) like ?`, args: []any{"%huan%"}},
			},
		},
	})
}

// TestIdentifierEscaping migrates modifiers_test's TestEscape scenario: an
// identifier containing the dialect's own quoting character escapes it by
// doubling. (go-sqlbuilder's Escape is a $-doubler for its placeholder
// syntax; the SQL-level analog in sqlk is per-dialect quote doubling.)
func TestIdentifierEscaping(t *testing.T) {
	runCompileCases(t, compiler.NewPostgres(), []compileCase{
		{
			name:  "postgres doubles inner double quote",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From(`Ta"ble`) },
			sql:   `SELECT * FROM "Ta""ble"`,
		},
	})
	runCompileCases(t, compiler.NewMysql(), []compileCase{
		{
			name:  "mysql doubles inner backtick",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Ta`ble") },
			sql:   "SELECT * FROM `Ta``ble`",
		},
	})
	runCompileCases(t, compiler.NewSqlserver(), []compileCase{
		{
			name:  "sqlserver doubles inner closing bracket",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("T").Select("Col]x") },
			sql:   `SELECT [Col]]x] FROM [T]`,
		},
	})
}

// TestWhereClauseScenarios migrates whereclause_test's core scenario: the
// same WHERE shape applied to SELECT, UPDATE, and DELETE (go-sqlbuilder
// shares one WhereClause across builders; sqlk expresses the same by building
// the same condition on each verb), plus the nested NOT IN subquery from
// TestWhereClauseSharedInstances.
func TestWhereClauseScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// ExampleWhereClause: the SELECT shape.
			name:  "where shape on select",
			build: func(q *sqlk.Query) *sqlk.Query { return q.Select("name", "level").From("users").WhereEq("id", 1234) },
			sql:   `SELECT "name", "level" FROM "users" WHERE "id" = ?`,
			args:  []any{1234},
		},
		{
			// ExampleWhereClause: the UPDATE shape (increment with where).
			name:  "where shape on update",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Increment("level", 10).WhereEq("id", 1234) },
			sql:   `UPDATE "users" SET "level" = "level" + ? WHERE "id" = ?`,
			args:  []any{10, 1234},
		},
		{
			// ExampleWhereClause_clearWhereClause: the DELETE shape.
			name:  "where shape on delete",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").WhereEq("id", 1234).Delete() },
			sql:   `DELETE FROM "users" WHERE "id" = ?`,
			args:  []any{1234},
		},
		{
			// TestWhereClauseSharedInstances: a NOT IN over a subquery that
			// itself carries a where.
			name: "nested not in subquery in where",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").WhereNotInSub("id", sqlk.NewQuery().From("t").WhereEq("id", 123))
			},
			sql:  `SELECT * FROM "t" WHERE "id" NOT IN (SELECT * FROM "t" WHERE "id" = ?)`,
			args: []any{123},
		},
	})
}
