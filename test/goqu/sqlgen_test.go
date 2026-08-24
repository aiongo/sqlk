package goqu

import (
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Scenario tests for the base compiler: each case pairs build code with the
// expected SQL text and argument sequence. The compiler always emits
// placeholders; the only escape hatch for inlining a literal is UnsafeLiteral.
// Dialect-specific output is covered by the per-dialect compiler tests.

// goquValuer implements driver.Valuer: the compiler never calls Value(), the
// instance enters the argument sequence untouched, and database/sql evaluates
// it at execution time.
type goquValuer struct{}

func (v goquValuer) Value() (driver.Value, error) {
	return []byte("Hello World"), nil
}

// compileCase is a table-driven case: build code paired with the expected SQL
// text and argument sequence.
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

// goquErrCase is a case expected to fail compilation; wantErr is matched with
// errors.Is.
type goquErrCase struct {
	name    string
	build   func(*sqlk.Query) *sqlk.Query
	wantErr error
}

// runGoquErrCases compiles each case and asserts the error is identifiable
// via errors.Is.
func runGoquErrCases(t *testing.T, comp *compiler.Compiler, cases []goquErrCase) {
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

// TestGoquValueBinding checks that values at parameter positions pass through
// untouched: no type normalization, no zero-value rewriting, and no
// compile-time rejection of unencodable types (database/sql reports those at
// execution time). Slices are not expanded here; set membership goes through
// WhereIn.
func TestGoquValueBinding(t *testing.T) {
	var typedNilBool *bool
	var typedNilTime *time.Time
	ts, err := time.Parse(time.RFC3339, "2019-10-01T15:01:00Z")
	if err != nil {
		t.Fatalf("time.Parse(...) error = %v", err)
	}
	valuer := goquValuer{}
	type unsupportedValue struct{}

	runCompileCases(t, compiler.New(), []compileCase{
		{
			name: "int family passes through",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").Where("a", "=", int16(10)).Where("b", "=", uint64(10))
			},
			sql:  `SELECT * FROM "t" WHERE "a" = ? AND "b" = ?`,
			args: []any{int16(10), uint64(10)},
		},
		{
			name: "float family passes through",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").Where("a", "=", float32(10.01)).Where("b", "=", float64(10.01))
			},
			sql:  `SELECT * FROM "t" WHERE "a" = ? AND "b" = ?`,
			args: []any{float32(10.01), float64(10.01)},
		},
		{
			// Quote handling belongs to the driver: parameterized values are
			// never escaped.
			name:  "string with quote passes through",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", "Hello'") },
			sql:   `SELECT * FROM "t" WHERE "a" = ?`,
			args:  []any{"Hello'"},
		},
		{
			name:  "bytes pass through",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", []byte("Hello'")) },
			sql:   `SELECT * FROM "t" WHERE "a" = ?`,
			args:  []any{[]byte("Hello'")},
		},
		{
			name:  "bool passes through",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", true).Where("b", "=", false) },
			sql:   `SELECT * FROM "t" WHERE "a" = ? AND "b" = ?`,
			args:  []any{true, false},
		},
		{
			name:  "time passes through",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", ts).Where("b", "=", &ts) },
			sql:   `SELECT * FROM "t" WHERE "a" = ? AND "b" = ?`,
			args:  []any{ts, &ts},
		},
		{
			// Untyped nil and typed nil pointers bind as plain parameters;
			// IS NULL checks go through WhereNull.
			name:  "nil and typed nil bind as parameters",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", nil).Where("b", "=", typedNilBool) },
			sql:   `SELECT * FROM "t" WHERE "a" = ? AND "b" = ?`,
			args:  []any{nil, typedNilBool},
		},
		{
			name:  "typed nil time binds as parameter",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", typedNilTime) },
			sql:   `SELECT * FROM "t" WHERE "a" = ?`,
			args:  []any{typedNilTime},
		},
		{
			name:  "valuer passes through",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", valuer) },
			sql:   `SELECT * FROM "t" WHERE "a" = ?`,
			args:  []any{valuer},
		},
		{
			// Struct values pass through; database/sql rejects them at
			// execution time.
			name:  "struct passes through to exec-time rejection",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", unsupportedValue{}) },
			sql:   `SELECT * FROM "t" WHERE "a" = ?`,
			args:  []any{unsupportedValue{}},
		},
	})
}

// TestGoquWhereOperators covers comparison operators, the LIKE family, and
// the regexp family, all of which compile through the operator whitelist;
// operators outside the whitelist are rejected.
func TestGoquWhereOperators(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			name:  "eq",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", 1) },
			sql:   `SELECT * FROM "t" WHERE "a" = ?`,
			args:  []any{1},
		},
		{
			name:  "neq",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "!=", 1) },
			sql:   `SELECT * FROM "t" WHERE "a" != ?`,
			args:  []any{1},
		},
		{
			name:  "gt gte",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", ">", 1).Where("b", ">=", 2) },
			sql:   `SELECT * FROM "t" WHERE "a" > ? AND "b" >= ?`,
			args:  []any{1, 2},
		},
		{
			name:  "lt lte",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "<", 1).Where("b", "<=", 2) },
			sql:   `SELECT * FROM "t" WHERE "a" < ? AND "b" <= ?`,
			args:  []any{1, 2},
		},
		{
			name:  "like",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "LIKE", "a%") },
			sql:   `SELECT * FROM "t" WHERE "a" like ?`,
			args:  []any{"a%"},
		},
		{
			name:  "not like",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "NOT LIKE", "a%") },
			sql:   `SELECT * FROM "t" WHERE "a" not like ?`,
			args:  []any{"a%"},
		},
		{
			name: "ilike and not ilike",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").Where("a", "ILIKE", "a%").Where("b", "NOT ILIKE", "a%")
			},
			sql:  `SELECT * FROM "t" WHERE "a" ilike ? AND "b" not ilike ?`,
			args: []any{"a%", "a%"},
		},
		{
			// The regexp family goes through the whitelist; the pattern is an
			// ordinary parameter.
			name: "regexp family operators",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").Where("a", "regexp", "[ab]").Where("b", "rlike", "[ab]")
			},
			sql:  `SELECT * FROM "t" WHERE "a" regexp ? AND "b" rlike ?`,
			args: []any{"[ab]", "[ab]"},
		},
	})
	runGoquErrCases(t, compiler.New(), []goquErrCase{
		{
			name:    "operator outside whitelist is rejected",
			build:   func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "badOp", 1) },
			wantErr: compiler.ErrOperatorNotAllowed,
		},
		{
			name:    "bitwise operator is not whitelisted",
			build:   func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "&", 1) },
			wantErr: compiler.ErrOperatorNotAllowed,
		},
	})
}

// TestGoquInBetweenConditions covers set membership over value lists and
// subqueries, and BETWEEN ranges over numbers and strings.
func TestGoquInBetweenConditions(t *testing.T) {
	sub := sqlk.NewQuery().From("test2").Select("id")
	runCompileCases(t, compiler.New(), []compileCase{
		{
			name:  "in values",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereIn("a", 1, 2, 3) },
			sql:   `SELECT * FROM "t" WHERE "a" IN (?, ?, ?)`,
			args:  []any{1, 2, 3},
		},
		{
			name:  "not in values",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotIn("a", 1, 2, 3) },
			sql:   `SELECT * FROM "t" WHERE "a" NOT IN (?, ?, ?)`,
			args:  []any{1, 2, 3},
		},
		{
			name:  "in subquery",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereInSub("a", sub) },
			sql:   `SELECT * FROM "t" WHERE "a" IN (SELECT "id" FROM "test2")`,
		},
		{
			name:  "not in subquery",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotInSub("a", sub) },
			sql:   `SELECT * FROM "t" WHERE "a" NOT IN (SELECT "id" FROM "test2")`,
		},
		{
			name:  "between numbers",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereBetween("a", 1, 2) },
			sql:   `SELECT * FROM "t" WHERE "a" BETWEEN ? AND ?`,
			args:  []any{1, 2},
		},
		{
			name:  "not between numbers",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotBetween("a", 1, 2) },
			sql:   `SELECT * FROM "t" WHERE "a" NOT BETWEEN ? AND ?`,
			args:  []any{1, 2},
		},
		{
			name:  "between strings",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereBetween("a", "aaa", "zzz") },
			sql:   `SELECT * FROM "t" WHERE "a" BETWEEN ? AND ?`,
			args:  []any{"aaa", "zzz"},
		},
		{
			name:  "not between strings",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotBetween("a", "aaa", "zzz") },
			sql:   `SELECT * FROM "t" WHERE "a" NOT BETWEEN ? AND ?`,
			args:  []any{"aaa", "zzz"},
		},
	})
}

// TestGoquConditionCombinations covers AND/OR chains, nested groups, and
// key-value constraints. Map values bind literally: use WhereNull/WhereTrue/
// WhereIn for null, boolean, and set semantics.
func TestGoquConditionCombinations(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			name:  "and list",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", "b").Where("c", "!=", 1) },
			sql:   `SELECT * FROM "t" WHERE "a" = ? AND "c" != ?`,
			args:  []any{"b", 1},
		},
		{
			name:  "or list",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", "b").OrWhere("c", "!=", 1) },
			sql:   `SELECT * FROM "t" WHERE "a" = ? OR "c" != ?`,
			args:  []any{"b", 1},
		},
		{
			name: "or over nested and group",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").Where("a", "=", "b").OrWhereGroup(func(g *sqlk.Query) *sqlk.Query {
					return g.Where("c", "!=", 1).Where("d", "=", 10)
				})
			},
			sql:  `SELECT * FROM "t" WHERE "a" = ? OR ("c" != ? AND "d" = ?)`,
			args: []any{"b", 1, 10},
		},
		{
			name: "and over nested or group",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").Where("a", "=", "b").WhereGroup(func(g *sqlk.Query) *sqlk.Query {
					return g.Where("c", "!=", 1).OrWhere("d", "=", 10)
				})
			},
			sql:  `SELECT * FROM "t" WHERE "a" = ? AND ("c" != ? OR "d" = ?)`,
			args: []any{"b", 1, 10},
		},
		{
			// Map keys are sorted so the output is deterministic.
			name:  "constraints map is and-joined",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereMap(sqlk.Record{"a": 1, "b": "c"}) },
			sql:   `SELECT * FROM "t" WHERE "a" = ? AND "b" = ?`,
			args:  []any{1, "c"},
		},
		{
			name: "constraints map mixed with or variant",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").WhereMap(sqlk.Record{"a": 1}).OrWhereEq("b", true)
			},
			sql:  `SELECT * FROM "t" WHERE "a" = ? OR "b" = ?`,
			args: []any{1, true},
		},
		{
			name:  "eq or is null",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Where("a", "=", 10).OrWhereNull("a") },
			sql:   `SELECT * FROM "t" WHERE "a" = ? OR "a" IS NULL`,
			args:  []any{10},
		},
		{
			name:  "negated condition wraps in not",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereNotEq("a", 1) },
			sql:   `SELECT * FROM "t" WHERE NOT ("a" = ?)`,
			args:  []any{1},
		},
		{
			name:  "raw condition with function comparand",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").WhereRaw("{d} = NOW()") },
			sql:   `SELECT * FROM "t" WHERE "d" = NOW()`,
		},
	})
}

// TestGoquProjectionScenarios covers projections: plain and aliased columns,
// raw function and boolean expressions, subquery projections, derived tables,
// qualified names, and identifier escaping.
func TestGoquProjectionScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			name:  "aliased column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Select("a as b") },
			sql:   `SELECT "a" AS "b" FROM "t"`,
		},
		{
			// [] marks identifiers inside raw expressions; the compiler quotes
			// them per dialect.
			name:  "aliased literal function",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").SelectRaw("count(*) AS [count]") },
			sql:   `SELECT count(*) AS "count" FROM "t"`,
		},
		{
			name: "sql functions",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").SelectRaw("MIN([a])").SelectRaw("COALESCE([a], ?)", "a")
			},
			sql:  `SELECT MIN("a"), COALESCE("a", ?) FROM "t"`,
			args: []any{"a"},
		},
		{
			name:  "aliased boolean expression",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").SelectRaw("([a] = ?) AS [x]", 1) },
			sql:   `SELECT ("a" = ?) AS "x" FROM "t"`,
			args:  []any{1},
		},
		{
			name: "case via raw projection",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").SelectRaw("CASE WHEN [a] > ? THEN [a] - 1 ELSE [a] END", 1)
			},
			sql:  `SELECT CASE WHEN "a" > ? THEN "a" - 1 ELSE "a" END FROM "t"`,
			args: []any{1},
		},
		{
			name: "subquery as projection column",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("t").SelectSub(sqlk.NewQuery().From("u").Select("x"), "y")
			},
			sql: `SELECT (SELECT "x" FROM "u") AS "y" FROM "t"`,
		},
		{
			name:  "subquery as from target",
			build: func(q *sqlk.Query) *sqlk.Query { return q.FromSub(sqlk.NewQuery().From("u").Select("x"), "y") },
			sql:   `SELECT * FROM (SELECT "x" FROM "u") AS "y"`,
		},
		{
			name:  "raw projection with bindings",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").SelectRaw("[b] = ? or [c] = ?", "a", 1) },
			sql:   `SELECT "b" = ? or "c" = ? FROM "t"`,
			args:  []any{"a", 1},
		},
		{
			name:  "raw projection without bindings",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").SelectRaw("[b]::DATE = '2010-09-02'") },
			sql:   `SELECT "b"::DATE = '2010-09-02' FROM "t"`,
		},
		{
			name:  "qualified names and qualified star",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("t").Select("table.col", "schema.table.col", "table.*") },
			sql:   `SELECT "table"."col", "schema"."table"."col", "table".* FROM "t"`,
		},
		{
			// A quote character inside an identifier is escaped by doubling.
			name:  "closing identifier is escaped by doubling",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From(`a"b`) },
			sql:   `SELECT * FROM "a""b"`,
		},
	})
}

// TestGoquSelectClauseScenarios covers select-clause modifiers: distinct,
// grouping, having, ordering, pagination, and joins.
func TestGoquSelectClauseScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			name:  "distinct",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Distinct() },
			sql:   `SELECT DISTINCT * FROM "test"`,
		},
		{
			name:  "group by single",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").GroupBy("a") },
			sql:   `SELECT * FROM "test" GROUP BY "a"`,
		},
		{
			name:  "group by multiple",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").GroupBy("a", "b") },
			sql:   `SELECT * FROM "test" GROUP BY "a", "b"`,
		},
		{
			name:  "having single",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Having("a", "=", "b") },
			sql:   `SELECT * FROM "test" HAVING "a" = ?`,
			args:  []any{"b"},
		},
		{
			name:  "having multiple is and-joined",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Having("a", "=", "b").Having("b", "=", "c") },
			sql:   `SELECT * FROM "test" HAVING "a" = ? AND "b" = ?`,
			args:  []any{"b", "c"},
		},
		{
			name:  "order by mixed directions",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").OrderBy("a").OrderByDesc("b") },
			sql:   `SELECT * FROM "test" ORDER BY "a", "b" DESC`,
		},
		{
			name:  "limit binds parameter",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Limit(10) },
			sql:   `SELECT * FROM "test" LIMIT ?`,
			args:  []any{10},
		},
		{
			name:  "offset binds parameter",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Offset(10) },
			sql:   `SELECT * FROM "test" OFFSET ?`,
			args:  []any{int64(10)},
		},
		{
			name:  "limit and offset bind in order",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Limit(10).Offset(20) },
			sql:   `SELECT * FROM "test" LIMIT ? OFFSET ?`,
			args:  []any{10, int64(20)},
		},
		{
			name: "cross left right joins chained",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("test").
					CrossJoin("test2").
					LeftJoinOn("test3", func(j *sqlk.Join) *sqlk.Join { return j.Where("a", "=", "foo") }).
					RightJoin("test4", "x", "=", "y")
			},
			sql:  "SELECT * FROM \"test\" \nCROSS JOIN \"test2\"\nLEFT JOIN \"test3\" ON \"a\" = ?\nRIGHT JOIN \"test4\" ON \"x\" = \"y\"",
			args: []any{"foo"},
		},
		{
			name: "join on condition with argument",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("test").LeftJoinOn("test2", func(j *sqlk.Join) *sqlk.Join { return j.Where("a", "=", "foo") })
			},
			sql:  "SELECT * FROM \"test\" \nLEFT JOIN \"test2\" ON \"a\" = ?",
			args: []any{"foo"},
		},
	})
}

// TestGoquCTEScenarios covers WITH clauses preceding selects and the three
// write verbs, with multiple CTEs comma-joined.
func TestGoquCTEScenarios(t *testing.T) {
	cte := func(q *sqlk.Query) *sqlk.Query { return q.With("a", sqlk.NewQuery().From("b")) }
	runCompileCases(t, compiler.New(), []compileCase{
		{
			name:  "single cte before select",
			build: func(q *sqlk.Query) *sqlk.Query { return cte(q).From("a") },
			sql:   "WITH \"a\" AS (SELECT * FROM \"b\")\nSELECT * FROM \"a\"",
		},
		{
			name: "multiple ctes comma joined",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.With("a", sqlk.NewQuery().From("b")).With("c", sqlk.NewQuery().From("d")).From("a")
			},
			sql: "WITH \"a\" AS (SELECT * FROM \"b\"),\n\"c\" AS (SELECT * FROM \"d\")\nSELECT * FROM \"a\"",
		},
		{
			name:  "cte before insert",
			build: func(q *sqlk.Query) *sqlk.Query { return cte(q).From("t").Insert(sqlk.Record{"a": 1}) },
			sql:   "WITH \"a\" AS (SELECT * FROM \"b\")\nINSERT INTO \"t\" (\"a\") VALUES (?)",
			args:  []any{1},
		},
		{
			name:  "cte before update",
			build: func(q *sqlk.Query) *sqlk.Query { return cte(q).From("t").Update(sqlk.Record{"a": 1}) },
			sql:   "WITH \"a\" AS (SELECT * FROM \"b\")\nUPDATE \"t\" SET \"a\" = ?",
			args:  []any{1},
		},
		{
			name:  "cte before delete",
			build: func(q *sqlk.Query) *sqlk.Query { return cte(q).From("t").Delete() },
			sql:   "WITH \"a\" AS (SELECT * FROM \"b\")\nDELETE FROM \"t\"",
		},
	})
}

// TestGoquCombineScenarios covers combined queries: the four set operations
// with and without ALL, chained in call order. Members are not parenthesized.
func TestGoquCombineScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			name:  "union",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Union(sqlk.NewQuery().From("foo")) },
			sql:   `SELECT * FROM "test" UNION SELECT * FROM "foo"`,
		},
		{
			name:  "union all",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").UnionAll(sqlk.NewQuery().From("foo")) },
			sql:   `SELECT * FROM "test" UNION ALL SELECT * FROM "foo"`,
		},
		{
			name:  "intersect",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Intersect(sqlk.NewQuery().From("foo")) },
			sql:   `SELECT * FROM "test" INTERSECT SELECT * FROM "foo"`,
		},
		{
			name:  "intersect all",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").IntersectAll(sqlk.NewQuery().From("foo")) },
			sql:   `SELECT * FROM "test" INTERSECT ALL SELECT * FROM "foo"`,
		},
		{
			name: "chained compounds keep call order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("test").Union(sqlk.NewQuery().From("foo")).Intersect(sqlk.NewQuery().From("bar"))
			},
			sql: `SELECT * FROM "test" UNION SELECT * FROM "foo" INTERSECT SELECT * FROM "bar"`,
		},
	})
}

// TestGoquInsertScenarios covers insert shapes: key-value, columns plus
// values, nil values, multi-row, and insert-from-query, plus the rejection of
// missing targets, empty data, and ragged rows.
func TestGoquInsertScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// Map keys are sorted so the output is deterministic.
			name:  "key value insert",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Insert(sqlk.Record{"b": "b1", "a": "a1"}) },
			sql:   `INSERT INTO "test" ("a", "b") VALUES (?, ?)`,
			args:  []any{"a1", "b1"},
		},
		{
			name:  "nil value binds parameter",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").InsertColumns([]string{"a"}, []any{nil}) },
			sql:   `INSERT INTO "test" ("a") VALUES (?)`,
			args:  []any{nil},
		},
		{
			name: "multi row insert",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("test").InsertRows([]string{"a", "b"},
					[]any{"a1", "b1"}, []any{"a2", "b2"}, []any{"a3", "b3"})
			},
			sql:  `INSERT INTO "test" ("a", "b") VALUES (?, ?), (?, ?), (?, ?)`,
			args: []any{"a1", "b1", "a2", "b2", "a3", "b3"},
		},
		{
			name: "insert from query without columns",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("test").InsertFrom(nil, sqlk.NewQuery().From("other").Select("c", "d"))
			},
			sql: `INSERT INTO "test" SELECT "c", "d" FROM "other"`,
		},
		{
			name: "insert from query with columns",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("test").InsertFrom([]string{"a", "b"}, sqlk.NewQuery().From("other").Select("c", "d"))
			},
			sql: `INSERT INTO "test" ("a", "b") SELECT "c", "d" FROM "other"`,
		},
	})
	runGoquErrCases(t, compiler.New(), []goquErrCase{
		{
			name:    "insert without target table",
			build:   func(q *sqlk.Query) *sqlk.Query { return sqlk.NewQuery().InsertColumns([]string{"a"}, []any{1}) },
			wantErr: compiler.ErrNoFromTarget,
		},
		{
			// An empty insert is rejected rather than compiled to DEFAULT
			// VALUES.
			name:    "insert with empty data",
			build:   func(q *sqlk.Query) *sqlk.Query { return q.From("test").Insert(sqlk.Record{}) },
			wantErr: compiler.ErrInvalidWriteValues,
		},
		{
			// Each row must match the column list length.
			name: "multi row insert with ragged row",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("test").InsertRows([]string{"a", "b"}, []any{"a1", "b1"}, []any{"a2"})
			},
			wantErr: compiler.ErrInvalidWriteValues,
		},
	})
}

// TestGoquUpdateScenarios covers update assignments (string, nil, and bool
// values) and WHERE clauses, plus the rejection of missing targets and empty
// assignment sets.
func TestGoquUpdateScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			// Map keys are sorted so the output is deterministic.
			name: "set with string null and bool values",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("test").Update(sqlk.Record{"a": true, "b": nil, "c": "x"})
			},
			sql:  `UPDATE "test" SET "a" = ?, "b" = ?, "c" = ?`,
			args: []any{true, nil, "x"},
		},
		{
			name: "set via columns and values",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("test").UpdateColumns([]string{"a", "b"}, []any{"b", "c"})
			},
			sql:  `UPDATE "test" SET "a" = ?, "b" = ?`,
			args: []any{"b", "c"},
		},
		{
			name: "update with where",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("test").Update(sqlk.Record{"a": "b"}).Where("c", "=", 1)
			},
			sql:  `UPDATE "test" SET "a" = ? WHERE "c" = ?`,
			args: []any{"b", 1},
		},
	})
	runGoquErrCases(t, compiler.New(), []goquErrCase{
		{
			name:    "update without target table",
			build:   func(q *sqlk.Query) *sqlk.Query { return sqlk.NewQuery().Update(sqlk.Record{"a": 1}) },
			wantErr: compiler.ErrNoFromTarget,
		},
		{
			// An empty assignment set is rejected.
			name:    "update with empty set values",
			build:   func(q *sqlk.Query) *sqlk.Query { return q.From("test").Update(sqlk.Record{}) },
			wantErr: compiler.ErrInvalidWriteValues,
		},
	})
}

// TestGoquDeleteScenarios covers basic deletes and deletes with plain or raw
// WHERE clauses.
func TestGoquDeleteScenarios(t *testing.T) {
	runCompileCases(t, compiler.New(), []compileCase{
		{
			name:  "delete from",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Delete() },
			sql:   `DELETE FROM "test"`,
		},
		{
			// {} marks an identifier inside a raw expression; the compiler
			// quotes it per dialect.
			name:  "delete with raw where",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Delete().WhereRaw("{a} = ?", 1) },
			sql:   `DELETE FROM "test" WHERE "a" = ?`,
			args:  []any{1},
		},
		{
			name:  "delete with basic where",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("test").Delete().Where("a", "=", 1) },
			sql:   `DELETE FROM "test" WHERE "a" = ?`,
			args:  []any{1},
		},
	})
	runGoquErrCases(t, compiler.New(), []goquErrCase{
		{
			name:    "delete without target table",
			build:   func(q *sqlk.Query) *sqlk.Query { return sqlk.NewQuery().Delete() },
			wantErr: compiler.ErrNoFromTarget,
		},
	})
}
