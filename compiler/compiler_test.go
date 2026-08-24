package compiler

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/aiongo/sqlk"
)

// compileCase is a table-driven case mapping a builder function to the
// expected SQL text and argument sequence.
type compileCase struct {
	name  string
	build func(*sqlk.Query) *sqlk.Query
	sql   string
	args  []any
}

// runCompileCases compiles each case and asserts the SQL text and
// argument sequence.
func runCompileCases(t *testing.T, comp *Compiler, cases []compileCase) {
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

// mustCompile compiles q, failing the test on error, and returns the
// compiled result.
func mustCompile(t *testing.T, comp *Compiler, q *sqlk.Query) Result {
	t.Helper()
	res, err := comp.Compile(q)
	if err != nil {
		t.Fatalf("Compile(...) error = %v, want nil", err)
	}
	return res
}

func TestCompileSelectBasics(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "from only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users") },
			sql:   `SELECT * FROM "Users"`,
		},
		{
			name:  "multiple columns",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").Select("Id", "Name") },
			sql:   `SELECT "Id", "Name" FROM "Users"`,
		},
		{
			name: "select appends across calls",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Select("Id").Select("Name", "Email")
			},
			sql: `SELECT "Id", "Name", "Email" FROM "Users"`,
		},
		{
			name: "where with operator",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Where("Id", "=", 1)
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" = ?`,
			args: []any{1},
		},
		{
			name: "where equality shorthand",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Age", 18)
			},
			sql:  `SELECT * FROM "Users" WHERE "Age" = ?`,
			args: []any{18},
		},
		{
			name: "operator is normalized to lowercase",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Where("Name", "LIKE", "j%")
			},
			sql:  `SELECT * FROM "Users" WHERE "Name" like ?`,
			args: []any{"j%"},
		},
		{
			name: "whitelisted compound operators",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Where("A", "<>", 1).Where("B", "<=>", 2)
			},
			sql:  `SELECT * FROM "Users" WHERE "A" <> ? AND "B" <=> ?`,
			args: []any{1, 2},
		},
		{
			name: "multiple wheres combine with AND",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Where("Id", ">", 1).WhereEq("Name", "x").Where("Age", "<=", 30)
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" > ? AND "Name" = ? AND "Age" <= ?`,
			args: []any{1, "x", 30},
		},
		{
			name: "limit only",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Limit(10)
			},
			sql:  `SELECT * FROM "Users" LIMIT ?`,
			args: []any{10},
		},
		{
			name: "offset only",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Offset(20)
			},
			sql:  `SELECT * FROM "Users" OFFSET ?`,
			args: []any{int64(20)},
		},
		{
			name: "limit and offset",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Limit(10).Offset(20)
			},
			sql:  `SELECT * FROM "Users" LIMIT ? OFFSET ?`,
			args: []any{10, int64(20)},
		},
		{
			name: "full chain",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").
					Select("Id", "Name").
					Where("Id", ">", 1).
					WhereEq("Age", 18).
					Limit(10).
					Offset(20)
			},
			sql:  `SELECT "Id", "Name" FROM "Users" WHERE "Id" > ? AND "Age" = ? LIMIT ? OFFSET ?`,
			args: []any{1, 18, 10, int64(20)},
		},
	})
}

func TestCompileProjectionShapes(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "column alias",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").Select("Name as FullName") },
			sql:   `SELECT "Name" AS "FullName" FROM "Users"`,
		},
		{
			name:  "qualified column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").Select("u.Id") },
			sql:   `SELECT "u"."Id" FROM "Users"`,
		},
		{
			name:  "qualified column with alias",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").Select("u.Id as UserId") },
			sql:   `SELECT "u"."Id" AS "UserId" FROM "Users"`,
		},
		{
			name:  "table alias",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users as u").Select("u.Id") },
			sql:   `SELECT "u"."Id" FROM "users" AS "u"`,
		},
		{
			name:  "raw column without bindings",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").SelectRaw("count(*) as Total") },
			sql:   `SELECT count(*) as Total FROM "Users"`,
		},
		{
			name: "raw column with bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectRaw("coalesce(?, ?)", "a", "b")
			},
			sql:  `SELECT coalesce(?, ?) FROM "Users"`,
			args: []any{"a", "b"},
		},
		{
			name:  "raw column identifier markers are wrapped",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").SelectRaw("max({Age}) as MaxAge") },
			sql:   `SELECT max("Age") as MaxAge FROM "Users"`,
		},
		{
			name:  "raw column bracket markers are wrapped",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").SelectRaw("[Id], [Name]") },
			sql:   `SELECT "Id", "Name" FROM "Users"`,
		},
		{
			name:  "escaped markers pass through unquoted",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").SelectRaw(`'\{1,2,3\}'::int\[\]`) },
			sql:   `SELECT '{1,2,3}'::int[] FROM "Users"`,
		},
		{
			name: "raw and plain columns keep call order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Select("Id").SelectRaw("1 as One").Select("Name")
			},
			sql: `SELECT "Id", 1 as One, "Name" FROM "Users"`,
		},
	})
}

func TestCompileSubQueryColumns(t *testing.T) {
	comp := New()

	tests := []compileCase{
		{
			name: "subquery column with alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectSub(sqlk.NewQuery().From("Logs").Select("Id").WhereEq("Type", "error"), "FirstErrorId")
			},
			sql:  `SELECT (SELECT "Id" FROM "Logs" WHERE "Type" = ?) AS "FirstErrorId" FROM "Users"`,
			args: []any{"error"},
		},
		{
			name: "subquery column without alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectSub(sqlk.NewQuery().From("Logs").Select("Id"), "")
			},
			sql: `SELECT (SELECT "Id" FROM "Logs") FROM "Users"`,
		},
		{
			name: "empty alias keeps the subquery own As alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectSub(sqlk.NewQuery().From("Logs").Select("Id").As("own"), "")
			},
			sql: `SELECT (SELECT "Id" FROM "Logs") AS "own" FROM "Users"`,
		},
		{
			name: "explicit alias overrides the subquery own As alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectSub(sqlk.NewQuery().From("Logs").Select("Id").As("own"), "given")
			},
			sql: `SELECT (SELECT "Id" FROM "Logs") AS "given" FROM "Users"`,
		},
		{
			name: "bindings follow placeholder order across column kinds",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").
					SelectRaw("greatest(?, ?)", 1, 2).
					SelectSub(sqlk.NewQuery().From("Logs").WhereEq("Id", 7).Select("Id"), "LogId").
					Select("Name").
					Where("Users.Id", ">", 100)
			},
			sql:  `SELECT greatest(?, ?), (SELECT "Id" FROM "Logs" WHERE "Id" = ?) AS "LogId", "Name" FROM "Users" WHERE "Users"."Id" > ?`,
			args: []any{1, 2, 7, 100},
		},
	}
	runCompileCases(t, comp, tests)

	t.Run("subquery is cloned at embed time", func(t *testing.T) {
		sub := sqlk.NewQuery().From("Logs").Select("Id")
		q := sqlk.NewQuery().From("Users").SelectSub(sub, "LogId")

		sub.WhereEq("Type", "error") // later mutation must not affect the embedded clause

		res := mustCompile(t, comp, q)
		want := `SELECT (SELECT "Id" FROM "Logs") AS "LogId" FROM "Users"`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("validation descends into subqueries", func(t *testing.T) {
		t.Run("operator outside whitelist inside subquery", func(t *testing.T) {
			sub := sqlk.NewQuery().From("Logs").Where("Type", "startswith", "x").Select("Id")
			_, err := comp.Compile(sqlk.NewQuery().From("Users").SelectSub(sub, "LogId"))
			if !errors.Is(err, ErrOperatorNotAllowed) {
				t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
			}
		})

		t.Run("missing from target inside subquery", func(t *testing.T) {
			sub := sqlk.NewQuery().Select("Id")
			_, err := comp.Compile(sqlk.NewQuery().From("Users").SelectSub(sub, "LogId"))
			if !errors.Is(err, ErrNoFromTarget) {
				t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
			}
		})
	})
}

func TestCompileDistinct(t *testing.T) {
	comp := New()

	tests := []compileCase{
		{
			name:  "distinct columns",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").Distinct().Select("Name", "Age") },
			sql:   `SELECT DISTINCT "Name", "Age" FROM "Users"`,
		},
		{
			name:  "distinct without columns",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").Distinct() },
			sql:   `SELECT DISTINCT * FROM "Users"`,
		},
		{
			name: "distinct combines with where and limit",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Distinct().Select("Name").WhereEq("Age", 18).Limit(10)
			},
			sql:  `SELECT DISTINCT "Name" FROM "Users" WHERE "Age" = ? LIMIT ?`,
			args: []any{18, 10},
		},
	}
	runCompileCases(t, comp, tests)

	t.Run("distinct survives Clone", func(t *testing.T) {
		res := mustCompile(t, comp, sqlk.NewQuery().From("Users").Select("Name").Distinct().Clone())
		want := `SELECT DISTINCT "Name" FROM "Users"`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
	})
}

func TestCompileFromShapes(t *testing.T) {
	comp := New()

	tests := []compileCase{
		{
			name: "from subquery with alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromSub(sqlk.NewQuery().From("Logs").Select("UserId", "Count").WhereEq("Type", "error"), "ErrLogs")
			},
			sql:  `SELECT * FROM (SELECT "UserId", "Count" FROM "Logs" WHERE "Type" = ?) AS "ErrLogs"`,
			args: []any{"error"},
		},
		{
			name: "from subquery without alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromSub(sqlk.NewQuery().From("Logs").Select("Id"), "")
			},
			sql: `SELECT * FROM (SELECT "Id" FROM "Logs")`,
		},
		{
			name: "empty alias keeps the subquery own As alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromSub(sqlk.NewQuery().From("Logs").Select("Id").As("own"), "")
			},
			sql: `SELECT * FROM (SELECT "Id" FROM "Logs") AS "own"`,
		},
		{
			name: "from raw sql with bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromRaw("generate_series(?, ?)", 1, 5).Select("d")
			},
			sql:  `SELECT "d" FROM generate_series(?, ?)`,
			args: []any{1, 5},
		},
		{
			name: "from raw sql identifier markers are wrapped",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromRaw("[Sub].[Table]").Select("Id")
			},
			sql: `SELECT "Id" FROM "Sub"."Table"`,
		},
		{
			name: "from subquery bindings precede where bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromSub(sqlk.NewQuery().From("Logs").WhereEq("Id", 7).Select("Id"), "L").WhereEq("L.Id", 9)
			},
			sql:  `SELECT * FROM (SELECT "Id" FROM "Logs" WHERE "Id" = ?) AS "L" WHERE "L"."Id" = ?`,
			args: []any{7, 9},
		},
		{
			name: "subquery column over from subquery",
			build: func(q *sqlk.Query) *sqlk.Query {
				inner := sqlk.NewQuery().From("Logs").Select("Id")
				return q.FromSub(sqlk.NewQuery().From("Users").SelectSub(inner, "LastLog").As("U"), "")
			},
			sql: `SELECT * FROM (SELECT (SELECT "Id" FROM "Logs") AS "LastLog" FROM "Users") AS "U"`,
		},
	}
	runCompileCases(t, comp, tests)

	t.Run("from later calls replace earlier targets", func(t *testing.T) {
		res := mustCompile(t, comp, sqlk.NewQuery().From("A").From("B"))
		want := `SELECT * FROM "B"`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("from subquery is cloned at embed time", func(t *testing.T) {
		sub := sqlk.NewQuery().From("Logs").Select("Id")
		q := sqlk.NewQuery().FromSub(sub, "L")

		sub.WhereEq("Type", "error")

		res := mustCompile(t, comp, q)
		want := `SELECT * FROM (SELECT "Id" FROM "Logs") AS "L"`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("validation descends into from subqueries", func(t *testing.T) {
		sub := sqlk.NewQuery().From("Logs").Where("Type", "startswith", "x").Select("Id")
		_, err := comp.Compile(sqlk.NewQuery().FromSub(sub, "L"))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
	})
}

func TestCompileAliasAndComment(t *testing.T) {
	comp := New()

	t.Run("As does not change the query own SQL", func(t *testing.T) {
		res := mustCompile(t, comp, sqlk.NewQuery().From("Users").Select("Id").As("u"))
		want := `SELECT "Id" FROM "Users"`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("As marks the query for outer reference", func(t *testing.T) {
		res := mustCompile(t, comp, sqlk.NewQuery().From("Users").Select("Id").
			FromSub(sqlk.NewQuery().From("Logs").As("L"), ""))
		want := `SELECT "Id" FROM (SELECT * FROM "Logs") AS "L"`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("comment prefixes the statement", func(t *testing.T) {
		res := mustCompile(t, comp, sqlk.NewQuery().From("Users").Comment("slow-query origin").Select("Id"))
		want := `/* slow-query origin */ SELECT "Id" FROM "Users"`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("comment with sections and bindings", func(t *testing.T) {
		res := mustCompile(t, comp, sqlk.NewQuery().From("Users").Comment("trace").WhereEq("Id", 1).Limit(3))
		want := `/* trace */ SELECT * FROM "Users" WHERE "Id" = ? LIMIT ?`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
		if !reflect.DeepEqual(res.Args, []any{1, 3}) {
			t.Errorf("Compile(...) Args = %#v, want [1 3]", res.Args)
		}
	})

	t.Run("comment cannot break out of the block", func(t *testing.T) {
		res := mustCompile(t, comp, sqlk.NewQuery().From("Users").Comment("evil */ DROP TABLE Users; --"))
		want := `/* evil * / DROP TABLE Users; -- */ SELECT * FROM "Users"`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("comment on subquery stays inside the parentheses", func(t *testing.T) {
		res := mustCompile(t, comp, sqlk.NewQuery().From("Users").
			SelectSub(sqlk.NewQuery().From("Logs").Comment("inner").Select("Id"), "LogId"))
		want := `SELECT (/* inner */ SELECT "Id" FROM "Logs") AS "LogId" FROM "Users"`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("alias and comment survive Clone", func(t *testing.T) {
		base := sqlk.NewQuery().From("Logs").Select("Id").As("L").Comment("trace")
		clone := base.Clone()

		res := mustCompile(t, comp, sqlk.NewQuery().FromSub(clone, ""))
		want := `SELECT * FROM (/* trace */ SELECT "Id" FROM "Logs") AS "L"`
		if res.SQL != want {
			t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
		}
	})
}

func TestWhenAndWhenNot(t *testing.T) {
	comp := New()

	compile := func(t *testing.T, q *sqlk.Query) Result {
		t.Helper()
		return mustCompile(t, comp, q)
	}

	addFlagFilter := func(q *sqlk.Query) *sqlk.Query { return q.WhereEq("Active", true) }

	t.Run("When applies the callback when true", func(t *testing.T) {
		res := compile(t, sqlk.NewQuery().From("Users").When(true, addFlagFilter))
		want := `SELECT * FROM "Users" WHERE "Active" = ?`
		if res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
		if !reflect.DeepEqual(res.Args, []any{true}) {
			t.Errorf("Args = %#v, want [true]", res.Args)
		}
	})

	t.Run("When skips the callback when false", func(t *testing.T) {
		res := compile(t, sqlk.NewQuery().From("Users").When(false, addFlagFilter))
		want := `SELECT * FROM "Users"`
		if res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("WhenNot applies the callback when false", func(t *testing.T) {
		res := compile(t, sqlk.NewQuery().From("Users").WhenNot(false, addFlagFilter))
		want := `SELECT * FROM "Users" WHERE "Active" = ?`
		if res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("WhenNot skips the callback when true", func(t *testing.T) {
		res := compile(t, sqlk.NewQuery().From("Users").WhenNot(true, addFlagFilter))
		want := `SELECT * FROM "Users"`
		if res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("conditional pagination without if branches", func(t *testing.T) {
		build := func(pageSize int) *sqlk.Query {
			return sqlk.NewQuery().From("Users").
				Select("Id").
				When(pageSize > 0, func(q *sqlk.Query) *sqlk.Query { return q.Limit(pageSize) }).
				WhenNot(pageSize > 0, func(q *sqlk.Query) *sqlk.Query { return q.Select("Name") })
		}

		limited := compile(t, build(10))
		if want := `SELECT "Id" FROM "Users" LIMIT ?`; limited.SQL != want {
			t.Errorf("SQL = %q, want %q", limited.SQL, want)
		}

		unlimited := compile(t, build(0))
		if want := `SELECT "Id", "Name" FROM "Users"`; unlimited.SQL != want {
			t.Errorf("SQL = %q, want %q", unlimited.SQL, want)
		}
	})
}

func TestCloneIndependence(t *testing.T) {
	comp := New()

	compile := func(t *testing.T, q *sqlk.Query) Result {
		t.Helper()
		return mustCompile(t, comp, q)
	}

	t.Run("variants diverge without affecting each other", func(t *testing.T) {
		base := sqlk.NewQuery().From("Users").Select("Id", "Name").Where("Age", ">", 18)

		adults := base.Clone().WhereEq("Active", true)
		minors := base.Clone().WhereEq("Active", false).Limit(5)

		if want := `SELECT "Id", "Name" FROM "Users" WHERE "Age" > ?`; compile(t, base).SQL != want {
			t.Errorf("base SQL = %q, want %q", compile(t, base).SQL, want)
		}
		if want := `SELECT "Id", "Name" FROM "Users" WHERE "Age" > ? AND "Active" = ?`; compile(t, adults).SQL != want {
			t.Errorf("adults SQL = %q, want %q", compile(t, adults).SQL, want)
		}
		if want := `SELECT "Id", "Name" FROM "Users" WHERE "Age" > ? AND "Active" = ? LIMIT ?`; compile(t, minors).SQL != want {
			t.Errorf("minors SQL = %q, want %q", compile(t, minors).SQL, want)
		}
	})

	t.Run("mutating the original after Clone leaves the clone untouched", func(t *testing.T) {
		base := sqlk.NewQuery().From("Users").Limit(10)
		clone := base.Clone()

		base.Limit(99)        // replaces the same slot
		base.WhereEq("Id", 1) // appends a clause

		if want := `SELECT * FROM "Users" LIMIT ?`; compile(t, clone).SQL != want {
			t.Errorf("clone SQL = %q, want %q", compile(t, clone).SQL, want)
		}
		if want := `SELECT * FROM "Users" WHERE "Id" = ? LIMIT ?`; compile(t, base).SQL != want {
			t.Errorf("base SQL = %q, want %q", compile(t, base).SQL, want)
		}
	})

	t.Run("nested subqueries are cloned deeply", func(t *testing.T) {
		base := sqlk.NewQuery().From("Users").
			FromSub(sqlk.NewQuery().From("Logs").WhereEq("Type", "error"), "L").
			SelectSub(sqlk.NewQuery().From("Events").Select("Id"), "Eid")

		variant := base.Clone().WhereEq("L.Type", "fatal")

		wantBase := `SELECT (SELECT "Id" FROM "Events") AS "Eid" FROM (SELECT * FROM "Logs" WHERE "Type" = ?) AS "L"`
		if got := compile(t, base); got.SQL != wantBase {
			t.Errorf("base SQL = %q, want %q", got.SQL, wantBase)
		} else if !reflect.DeepEqual(got.Args, []any{"error"}) {
			t.Errorf("base Args = %#v, want [error]", got.Args)
		}

		wantVariant := `SELECT (SELECT "Id" FROM "Events") AS "Eid" FROM (SELECT * FROM "Logs" WHERE "Type" = ?) AS "L" WHERE "L"."Type" = ?`
		if got := compile(t, variant); got.SQL != wantVariant {
			t.Errorf("variant SQL = %q, want %q", got.SQL, wantVariant)
		} else if !reflect.DeepEqual(got.Args, []any{"error", "fatal"}) {
			t.Errorf("variant Args = %#v, want [error fatal]", got.Args)
		}
	})
}

func TestCompileOrNotVariants(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "or where",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Age", 18).OrWhere("Score", ">", 90)
			},
			sql:  `SELECT * FROM "Users" WHERE "Age" = ? OR "Score" > ?`,
			args: []any{18, 90},
		},
		{
			name: "where not",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereNot("Age", "=", 18)
			},
			sql:  `SELECT * FROM "Users" WHERE NOT ("Age" = ?)`,
			args: []any{18},
		},
		{
			name: "or where not",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("A", 1).OrWhereNot("B", "=", 2)
			},
			sql:  `SELECT * FROM "Users" WHERE "A" = ? OR NOT ("B" = ?)`,
			args: []any{1, 2},
		},
		{
			name: "eq shorthand variants",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").OrWhereEq("A", 1).WhereNotEq("B", 2).OrWhereNotEq("C", 3)
			},
			sql:  `SELECT * FROM "Users" WHERE "A" = ? AND NOT ("B" = ?) OR NOT ("C" = ?)`,
			args: []any{1, 2, 3},
		},
		{
			name: "default connector is AND",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Where("A", ">", 1).WhereNot("B", "<", 2)
			},
			sql:  `SELECT * FROM "Users" WHERE "A" > ? AND NOT ("B" < ?)`,
			args: []any{1, 2},
		},
		{
			name: "not with non-equality operators keeps the comparison inside parentheses",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereNot("Age", "like", "%j")
			},
			sql:  `SELECT * FROM "Users" WHERE NOT ("Age" like ?)`,
			args: []any{"%j"},
		},
	})
}

func TestCompileNullAndBooleanConditions(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "where null",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereNull("DeletedAt") },
			sql:   `SELECT * FROM "Users" WHERE "DeletedAt" IS NULL`,
		},
		{
			name:  "where not null",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereNotNull("DeletedAt") },
			sql:   `SELECT * FROM "Users" WHERE "DeletedAt" IS NOT NULL`,
		},
		{
			name: "or where null and or where not null",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("A", 1).OrWhereNull("B").OrWhereNotNull("C")
			},
			sql:  `SELECT * FROM "Users" WHERE "A" = ? OR "B" IS NULL OR "C" IS NOT NULL`,
			args: []any{1},
		},
		{
			name:  "where true",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereTrue("IsActive") },
			sql:   `SELECT * FROM "Users" WHERE "IsActive" = true`,
		},
		{
			name:  "where false",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereFalse("IsActive") },
			sql:   `SELECT * FROM "Users" WHERE "IsActive" = false`,
		},
		{
			name: "or where true and or where false",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("A", 1).OrWhereTrue("B").OrWhereFalse("C")
			},
			sql:  `SELECT * FROM "Users" WHERE "A" = ? OR "B" = true OR "C" = false`,
			args: []any{1},
		},
		{
			name: "null and boolean literals emit no parameters",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereNull("A").WhereTrue("B").WhereEq("Id", 1).WhereFalse("C")
			},
			sql:  `SELECT * FROM "Users" WHERE "A" IS NULL AND "B" = true AND "Id" = ? AND "C" = false`,
			args: []any{1},
		},
	})
}

func TestCompileNestedGroupsAndWhereMap(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "nested group compiles to a parenthesized combination",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").
					WhereGroup(func(n *sqlk.Query) *sqlk.Query {
						return n.WhereEq("A", 1).OrWhereEq("B", 2)
					}).
					WhereEq("C", 3)
			},
			sql:  `SELECT * FROM "Users" WHERE ("A" = ? OR "B" = ?) AND "C" = ?`,
			args: []any{1, 2, 3},
		},
		{
			name: "groups nest to arbitrary depth",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereGroup(func(n *sqlk.Query) *sqlk.Query {
					return n.WhereEq("A", 1).OrWhereGroup(func(m *sqlk.Query) *sqlk.Query {
						return m.WhereEq("B", 2).WhereEq("C", 3)
					})
				})
			},
			sql:  `SELECT * FROM "Users" WHERE ("A" = ? OR ("B" = ? AND "C" = ?))`,
			args: []any{1, 2, 3},
		},
		{
			name: "where not group",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereNotGroup(func(n *sqlk.Query) *sqlk.Query {
					return n.WhereEq("A", 1).OrWhereEq("B", 2)
				})
			},
			sql:  `SELECT * FROM "Users" WHERE NOT ("A" = ? OR "B" = ?)`,
			args: []any{1, 2},
		},
		{
			name: "or where group and or where not group",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("A", 1).
					OrWhereGroup(func(n *sqlk.Query) *sqlk.Query {
						return n.WhereEq("B", 2).WhereEq("C", 3)
					}).
					OrWhereNotGroup(func(n *sqlk.Query) *sqlk.Query {
						return n.WhereEq("D", 4)
					})
			},
			sql:  `SELECT * FROM "Users" WHERE "A" = ? OR ("B" = ? AND "C" = ?) OR NOT ("D" = ?)`,
			args: []any{1, 2, 3, 4},
		},
		{
			name: "empty group is omitted while a non-empty group stays",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").
					WhereGroup(func(n *sqlk.Query) *sqlk.Query { return n.WhereEq("A", 1) }).
					WhereGroup(func(n *sqlk.Query) *sqlk.Query { return n })
			},
			sql:  `SELECT * FROM "Users" WHERE ("A" = ?)`,
			args: []any{1},
		},
		{
			name: "a query of only an empty group has no where section",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereGroup(func(n *sqlk.Query) *sqlk.Query { return n })
			},
			sql: `SELECT * FROM "Users"`,
		},
		{
			name: "where map joins pairs with AND in sorted key order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereMap(sqlk.Record{"Name": "x", "Age": 18, "City": "go"})
			},
			sql:  `SELECT * FROM "Users" WHERE "Age" = ? AND "City" = ? AND "Name" = ?`,
			args: []any{18, "go", "x"},
		},
		{
			name: "where map combines with other conditions",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Where("Id", ">", 1).WhereMap(sqlk.Record{"B": 2, "A": 1})
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" > ? AND "A" = ? AND "B" = ?`,
			args: []any{1, 1, 2},
		},
	})
}

func TestCompileBetweenConditions(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "between is a closed interval",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereBetween("Age", 18, 24) },
			sql:   `SELECT * FROM "Users" WHERE "Age" BETWEEN ? AND ?`,
			args:  []any{18, 24},
		},
		{
			name:  "not between",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereNotBetween("Age", 18, 24) },
			sql:   `SELECT * FROM "Users" WHERE "Age" NOT BETWEEN ? AND ?`,
			args:  []any{18, 24},
		},
		{
			name: "or between and or not between",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("A", 1).OrWhereBetween("Age", 18, 24).OrWhereNotBetween("Score", 60, 90)
			},
			sql:  `SELECT * FROM "Users" WHERE "A" = ? OR "Age" BETWEEN ? AND ? OR "Score" NOT BETWEEN ? AND ?`,
			args: []any{1, 18, 24, 60, 90},
		},
	})
}

func TestCompileInConditions(t *testing.T) {
	idsSub := func() *sqlk.Query {
		return sqlk.NewQuery().From("Logs").Select("UserId").WhereEq("Type", "error")
	}

	runCompileCases(t, New(), []compileCase{
		{
			name:  "in with a value list",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereIn("Id", 1, 2, 3) },
			sql:   `SELECT * FROM "Users" WHERE "Id" IN (?, ?, ?)`,
			args:  []any{1, 2, 3},
		},
		{
			name:  "not in with a value list",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereNotIn("Id", 1, 2) },
			sql:   `SELECT * FROM "Users" WHERE "Id" NOT IN (?, ?)`,
			args:  []any{1, 2},
		},
		{
			name: "or in and or not in",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("A", 1).OrWhereIn("Id", 2).OrWhereNotIn("Type", "x")
			},
			sql:  `SELECT * FROM "Users" WHERE "A" = ? OR "Id" IN (?) OR "Type" NOT IN (?)`,
			args: []any{1, 2, "x"},
		},
		{
			name:  "in with an empty list is a false tautology instead of invalid SQL",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereIn("Id") },
			sql:   `SELECT * FROM "Users" WHERE 1 = 0 /* IN [empty list] */`,
		},
		{
			name:  "not in with an empty list is a true tautology",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").WhereNotIn("Id") },
			sql:   `SELECT * FROM "Users" WHERE 1 = 1 /* NOT IN [empty list] */`,
		},
		{
			name: "in over a subquery",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereInSub("Id", idsSub())
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" IN (SELECT "UserId" FROM "Logs" WHERE "Type" = ?)`,
			args: []any{"error"},
		},
		{
			name: "not in over a subquery and or variants",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("A", 1).
					WhereNotInSub("Id", idsSub()).
					OrWhereInSub("Type", idsSub()).
					OrWhereNotInSub("Ref", idsSub())
			},
			sql:  `SELECT * FROM "Users" WHERE "A" = ? AND "Id" NOT IN (SELECT "UserId" FROM "Logs" WHERE "Type" = ?) OR "Type" IN (SELECT "UserId" FROM "Logs" WHERE "Type" = ?) OR "Ref" NOT IN (SELECT "UserId" FROM "Logs" WHERE "Type" = ?)`,
			args: []any{1, "error", "error", "error"},
		},
		{
			name: "bindings interleave across condition kinds in call order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereIn("A", 1, 2).WhereBetween("B", 3, 4).WhereEq("C", 5)
			},
			sql:  `SELECT * FROM "Users" WHERE "A" IN (?, ?) AND "B" BETWEEN ? AND ? AND "C" = ?`,
			args: []any{1, 2, 3, 4, 5},
		},
	})
}

func TestCompileWhereColumnsAndSub(t *testing.T) {
	countSub := func() *sqlk.Query {
		return sqlk.NewQuery().From("Table2").WhereColumns("Table2.Column", "=", "Table.MyCol").Select("Id")
	}

	runCompileCases(t, New(), []compileCase{
		{
			name:  "where columns compares two columns",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereColumns("A", "=", "B") },
			sql:   `SELECT * FROM "Table" WHERE "A" = "B"`,
		},
		{
			name: "or where columns",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereEq("A", 1).OrWhereColumns("u.Id", ">=", "o.Id")
			},
			sql:  `SELECT * FROM "Table" WHERE "A" = ? OR "u"."Id" >= "o"."Id"`,
			args: []any{1},
		},
		{
			name: "where sub compares a subquery with a value",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereSub(countSub(), "<", 1)
			},
			sql:  `SELECT * FROM "Table" WHERE (SELECT "Id" FROM "Table2" WHERE "Table2"."Column" = "Table"."MyCol") < ?`,
			args: []any{1},
		},
		{
			name: "where sub eq shorthand defaults to equality",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereSubEq(countSub(), 1)
			},
			sql:  `SELECT * FROM "Table" WHERE (SELECT "Id" FROM "Table2" WHERE "Table2"."Column" = "Table"."MyCol") = ?`,
			args: []any{1},
		},
		{
			name: "or where sub and or where sub eq",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereNull("MyCol").OrWhereSub(countSub(), "<", 1).OrWhereSubEq(countSub(), 2)
			},
			sql:  `SELECT * FROM "Table" WHERE "MyCol" IS NULL OR (SELECT "Id" FROM "Table2" WHERE "Table2"."Column" = "Table"."MyCol") < ? OR (SELECT "Id" FROM "Table2" WHERE "Table2"."Column" = "Table"."MyCol") = ?`,
			args: []any{1, 2},
		},
		{
			name: "subquery embedded in a condition is cloned at embed time",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := countSub()
				q.From("Table").WhereSubEq(sub, 1)
				sub.WhereEq("Extra", "later") // later mutation must not affect the embedded condition
				return q
			},
			sql:  `SELECT * FROM "Table" WHERE (SELECT "Id" FROM "Table2" WHERE "Table2"."Column" = "Table"."MyCol") = ?`,
			args: []any{1},
		},
	})
}

func TestCompileWhereRaw(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "where raw with bindings and identifier markers",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereRaw("[Id] > ? or [Id] < ?", 10, 20)
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" > ? or "Id" < ?`,
			args: []any{10, 20},
		},
		{
			name: "or where raw",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("A", 1).OrWhereRaw("[Json]->>'country' = ?", "go")
			},
			sql:  `SELECT * FROM "Users" WHERE "A" = ? OR "Json"->>'country' = ?`,
			args: []any{1, "go"},
		},
		{
			name: "where raw keeps placeholder order among modeled conditions",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereRaw("mod([Id], ?) = 0", 2).WhereEq("Name", "x")
			},
			sql:  `SELECT * FROM "Users" WHERE mod("Id", ?) = 0 AND "Name" = ?`,
			args: []any{2, "x"},
		},
	})
}

func TestCompileLikeConditions(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			// The default is case insensitive: the column is wrapped in
			// LOWER and the pattern value is lowercased.
			name: "where like is case insensitive by default",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table1").WhereLike("Column1", "%Upper Word%")
			},
			sql:  `SELECT * FROM "Table1" WHERE LOWER("Column1") like ?`,
			args: []any{"%upper word%"},
		},
		{
			name: "where like case sensitive keeps both sides verbatim",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table1").WhereLike("Column1", "%Upper Word%", sqlk.CaseSensitive())
			},
			sql:  `SELECT * FROM "Table1" WHERE "Column1" like ?`,
			args: []any{"%Upper Word%"},
		},
		{
			name: "starts appends the wildcard prefix",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereStarts("Name", "John")
			},
			sql:  `SELECT * FROM "Users" WHERE LOWER("Name") like ?`,
			args: []any{"john%"},
		},
		{
			name: "ends prepends the wildcard suffix",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEnds("Name", "son")
			},
			sql:  `SELECT * FROM "Users" WHERE LOWER("Name") like ?`,
			args: []any{"%son"},
		},
		{
			name: "contains wraps the value in wildcards",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereContains("Name", "oh")
			},
			sql:  `SELECT * FROM "Users" WHERE LOWER("Name") like ?`,
			args: []any{"%oh%"},
		},
		{
			name: "case sensitive variants of starts ends contains",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").
					WhereStarts("A", "Foo", sqlk.CaseSensitive()).
					WhereEnds("B", "Bar", sqlk.CaseSensitive()).
					WhereContains("C", "Baz", sqlk.CaseSensitive())
			},
			sql:  `SELECT * FROM "Users" WHERE "A" like ? AND "B" like ? AND "C" like ?`,
			args: []any{"Foo%", "%Bar", "%Baz%"},
		},
		{
			// Escape sequences in the pattern reach the argument verbatim,
			// and the ESCAPE clause follows the comparison.
			name: "escape character appends the ESCAPE clause",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table1").WhereLike("Column1", `TestString\%`, sqlk.EscapeLike(`\`))
			},
			sql:  `SELECT * FROM "Table1" WHERE LOWER("Column1") like ? ESCAPE '\'`,
			args: []any{`teststring\%`},
		},
		{
			name: "escape applies to starts ends contains wildcards",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table1").
					WhereStarts("A", `TestString\%`, sqlk.EscapeLike(`\`)).
					WhereEnds("B", `TestString\%`, sqlk.EscapeLike(`\`)).
					WhereContains("C", `TestString\%`, sqlk.EscapeLike(`\`))
			},
			sql:  `SELECT * FROM "Table1" WHERE LOWER("A") like ? ESCAPE '\' AND LOWER("B") like ? ESCAPE '\' AND LOWER("C") like ? ESCAPE '\'`,
			args: []any{`teststring\%%`, `%teststring\%`, `%teststring\%%`},
		},
		{
			name: "escape combines with case sensitivity",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table1").WhereContains("Column1", `Word\%`, sqlk.CaseSensitive(), sqlk.EscapeLike(`\`))
			},
			sql:  `SELECT * FROM "Table1" WHERE "Column1" like ? ESCAPE '\'`,
			args: []any{`%Word\%%`},
		},
		{
			name: "blank escape option is ignored, treated as unset",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table1").WhereLike("Column1", "x", sqlk.EscapeLike("  "))
			},
			sql:  `SELECT * FROM "Table1" WHERE LOWER("Column1") like ?`,
			args: []any{"x"},
		},
		{
			name: "not variants negate the whole comparison",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereNotLike("A", "%x%").WhereNotStarts("B", "y").WhereNotEnds("C", "z").WhereNotContains("D", "w")
			},
			sql:  `SELECT * FROM "Users" WHERE NOT (LOWER("A") like ?) AND NOT (LOWER("B") like ?) AND NOT (LOWER("C") like ?) AND NOT (LOWER("D") like ?)`,
			args: []any{"%x%", "y%", "%z", "%w%"},
		},
		{
			name: "or variants connect with OR",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").
					WhereEq("Id", 1).
					OrWhereLike("A", "%x%").
					OrWhereStarts("B", "y").
					OrWhereEnds("C", "z").
					OrWhereContains("D", "w").
					OrWhereNotLike("E", "%v%").
					OrWhereNotStarts("F", "u").
					OrWhereNotEnds("G", "t").
					OrWhereNotContains("H", "s")
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" = ? OR LOWER("A") like ? OR LOWER("B") like ? OR LOWER("C") like ? OR LOWER("D") like ? OR NOT (LOWER("E") like ?) OR NOT (LOWER("F") like ?) OR NOT (LOWER("G") like ?) OR NOT (LOWER("H") like ?)`,
			args: []any{1, "%x%", "y%", "%z", "%w%", "%v%", "u%", "%t", "%s%"},
		},
		{
			name: "qualified columns are lowered as a whole",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereContains("u.Name", "oh")
			},
			sql:  `SELECT * FROM "Users" WHERE LOWER("u"."Name") like ?`,
			args: []any{"%oh%"},
		},
		{
			name: "like conditions keep placeholder order among modeled conditions",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Id", 1).WhereContains("Name", "oh").WhereNotNull("DeletedAt")
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" = ? AND LOWER("Name") like ? AND "DeletedAt" IS NOT NULL`,
			args: []any{1, "%oh%"},
		},
	})
}

func TestCompileExistsConditions(t *testing.T) {
	commentCount := func() *sqlk.Query {
		return sqlk.NewQuery().From("Comments").WhereColumns("Comments.PostId", "=", "Posts.Id")
	}

	runCompileCases(t, New(), []compileCase{
		{
			name: "where exists omits the select list by default",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").WhereExists(commentCount())
			},
			sql: `SELECT * FROM "Posts" WHERE EXISTS (SELECT 1 FROM "Comments" WHERE "Comments"."PostId" = "Posts"."Id")`,
		},
		{
			name: "where not exists",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").WhereNotExists(commentCount())
			},
			sql: `SELECT * FROM "Posts" WHERE NOT EXISTS (SELECT 1 FROM "Comments" WHERE "Comments"."PostId" = "Posts"."Id")`,
		},
		{
			name: "or variants connect with OR",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").WhereEq("Id", 1).OrWhereExists(commentCount()).OrWhereNotExists(commentCount())
			},
			sql:  `SELECT * FROM "Posts" WHERE "Id" = ? OR EXISTS (SELECT 1 FROM "Comments" WHERE "Comments"."PostId" = "Posts"."Id") OR NOT EXISTS (SELECT 1 FROM "Comments" WHERE "Comments"."PostId" = "Posts"."Id")`,
			args: []any{1},
		},
		{
			name: "subquery bindings are merged in placeholder order",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Comments").WhereEq("PostId", 7).WhereEq("Visible", true)
				return q.From("Posts").WhereEq("Author", "go").WhereExists(sub)
			},
			sql:  `SELECT * FROM "Posts" WHERE "Author" = ? AND EXISTS (SELECT 1 FROM "Comments" WHERE "PostId" = ? AND "Visible" = ?)`,
			args: []any{"go", 7, true},
		},
		{
			name: "subquery embedded in an exists condition is cloned at embed time",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := commentCount()
				q.From("Posts").WhereExists(sub)
				sub.WhereEq("Extra", "later") // later mutation must not affect the embedded condition
				return q
			},
			sql: `SELECT * FROM "Posts" WHERE EXISTS (SELECT 1 FROM "Comments" WHERE "Comments"."PostId" = "Posts"."Id")`,
		},
		{
			name: "replacing the projection for exists does not disturb the embedded query",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Comments").Select("Id").WhereEq("PostId", 7)
				q.From("Posts").WhereExists(sub)
				if got := mustCompile(t, New(), sub).SQL; got != `SELECT "Id" FROM "Comments" WHERE "PostId" = ?` {
					t.Errorf("embedded subquery SQL = %q, want its own projection preserved", got)
				}
				return q
			},
			sql:  `SELECT * FROM "Posts" WHERE EXISTS (SELECT 1 FROM "Comments" WHERE "PostId" = ?)`,
			args: []any{7},
		},
	})

	t.Run("omit select inside exists can be turned off", func(t *testing.T) {
		// With omission disabled, the subquery keeps its own projection.
		comp := New()
		comp.omitSelectInsideExists = false
		sub := sqlk.NewQuery().From("Comments").Select("Id").WhereColumns("Comments.PostId", "=", "Posts.Id")
		res := mustCompile(t, comp, sqlk.NewQuery().From("Posts").WhereExists(sub))
		want := `SELECT * FROM "Posts" WHERE EXISTS (SELECT "Id" FROM "Comments" WHERE "Comments"."PostId" = "Posts"."Id")`
		if res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
	})
}

func TestCompileDateConditions(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "where date part compares the extracted part",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereDatePart("year", "Stamp", "=", "2018")
			},
			sql:  `SELECT * FROM "Table" WHERE YEAR("Stamp") = ?`,
			args: []any{"2018"},
		},
		{
			name: "date part names are normalized regardless of input case",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereDatePart("MiNuTe", "Stamp", ">", 25)
			},
			sql:  `SELECT * FROM "Table" WHERE MINUTE("Stamp") > ?`,
			args: []any{25},
		},
		{
			name: "common parts",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").
					WhereDatePart("month", "A", "=", 9).
					WhereDatePart("day", "B", "=", 15).
					WhereDatePart("hour", "C", "=", 15).
					WhereDatePart("second", "D", "=", 59)
			},
			sql:  `SELECT * FROM "Table" WHERE MONTH("A") = ? AND DAY("B") = ? AND HOUR("C") = ? AND SECOND("D") = ?`,
			args: []any{9, 15, 15, 59},
		},
		{
			name: "where date targets the date part",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereDate("Stamp", "=", "2018-04-01")
			},
			sql:  `SELECT * FROM "Table" WHERE DATE("Stamp") = ?`,
			args: []any{"2018-04-01"},
		},
		{
			name: "where time targets the time part",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereTime("Stamp", "=", "19:01:10")
			},
			sql:  `SELECT * FROM "Table" WHERE TIME("Stamp") = ?`,
			args: []any{"19:01:10"},
		},
		{
			name: "eq shorthands default to equality",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereDatePartEq("year", "A", 2018).WhereDateEq("B", "2026-08-23").WhereTimeEq("C", "01:02:03")
			},
			sql:  `SELECT * FROM "Table" WHERE YEAR("A") = ? AND DATE("B") = ? AND TIME("C") = ?`,
			args: []any{2018, "2026-08-23", "01:02:03"},
		},
		{
			name: "not variants negate the whole comparison",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereNotDatePart("year", "A", "=", 2018).WhereNotDate("B", "=", "2026-08-23").WhereNotTime("C", "=", "01:02:03")
			},
			sql:  `SELECT * FROM "Table" WHERE NOT (YEAR("A") = ?) AND NOT (DATE("B") = ?) AND NOT (TIME("C") = ?)`,
			args: []any{2018, "2026-08-23", "01:02:03"},
		},
		{
			name: "or variants connect with OR",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").
					WhereEq("Id", 1).
					OrWhereDatePart("year", "A", "=", 2018).
					OrWhereNotDatePart("year", "B", "=", 2019).
					OrWhereDate("C", "=", "2026-08-23").
					OrWhereNotDate("D", "=", "2026-08-24").
					OrWhereTime("E", "=", "01:02:03").
					OrWhereNotTime("F", "=", "01:02:04")
			},
			sql:  `SELECT * FROM "Table" WHERE "Id" = ? OR YEAR("A") = ? OR NOT (YEAR("B") = ?) OR DATE("C") = ? OR NOT (DATE("D") = ?) OR TIME("E") = ? OR NOT (TIME("F") = ?)`,
			args: []any{1, 2018, 2019, "2026-08-23", "2026-08-24", "01:02:03", "01:02:04"},
		},
		{
			name: "eq not shorthands",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").
					WhereNotDatePartEq("year", "A", 2018).
					OrWhereDatePartEq("year", "B", 2019).
					OrWhereNotDatePartEq("year", "C", 2020).
					WhereNotDateEq("D", "2026-08-23").
					OrWhereDateEq("E", "2026-08-24").
					OrWhereNotDateEq("F", "2026-08-25").
					WhereNotTimeEq("G", "01:02:03").
					OrWhereTimeEq("H", "01:02:04").
					OrWhereNotTimeEq("I", "01:02:05")
			},
			sql:  `SELECT * FROM "Table" WHERE NOT (YEAR("A") = ?) OR YEAR("B") = ? OR NOT (YEAR("C") = ?) AND NOT (DATE("D") = ?) OR DATE("E") = ? OR NOT (DATE("F") = ?) AND NOT (TIME("G") = ?) OR TIME("H") = ? OR NOT (TIME("I") = ?)`,
			args: []any{2018, 2019, 2020, "2026-08-23", "2026-08-24", "2026-08-25", "01:02:03", "01:02:04", "01:02:05"},
		},
		{
			name: "non equality operators pass the whitelist",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereDatePart("year", "A", ">=", 2020).WhereTime("B", "<", "12:00:00")
			},
			sql:  `SELECT * FROM "Table" WHERE YEAR("A") >= ? AND TIME("B") < ?`,
			args: []any{2020, "12:00:00"},
		},
	})
}

func TestWhitelistExtension(t *testing.T) {
	comp := New().Whitelist("&&", "~=", "Matches")

	runCompileCases(t, comp, []compileCase{
		{
			name: "custom operators become usable",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Where("A", "&&", 1).Where("B", "~=", 2)
			},
			sql:  `SELECT * FROM "Users" WHERE "A" && ? AND "B" ~= ?`,
			args: []any{1, 2},
		},
		{
			name: "custom operators are normalized to lowercase",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Where("A", "MATCHES", 1)
			},
			sql:  `SELECT * FROM "Users" WHERE "A" matches ?`,
			args: []any{1},
		},
		{
			name: "custom operators work inside groups and condition subqueries",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Logs").Where("Type", "&&", "error").Select("Id")
				return q.From("Users").
					WhereGroup(func(n *sqlk.Query) *sqlk.Query {
						return n.Where("A", "&&", 1)
					}).
					WhereInSub("Id", sub)
			},
			sql:  `SELECT * FROM "Users" WHERE ("A" && ?) AND "Id" IN (SELECT "Id" FROM "Logs" WHERE "Type" && ?)`,
			args: []any{1, "error"},
		},
	})

	t.Run("extension applies only to the configured instance", func(t *testing.T) {
		_, err := New().Compile(sqlk.NewQuery().From("Users").Where("A", "&&", 1))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
	})
}

func TestCompileValidation(t *testing.T) {
	comp := New()

	t.Run("no from target", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().Select("Id"))
		if !errors.Is(err, ErrNoFromTarget) {
			t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
		}
	})

	t.Run("operator outside whitelist", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("Users").Where("Age", "startswith", 18))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
		opErr, ok := errors.AsType[*OperatorError](err)
		if !ok {
			t.Fatalf("Compile(...) error = %v, want an *OperatorError", err)
		}
		if opErr.Column != "Age" || opErr.Operator != "startswith" {
			t.Errorf("OperatorError = {Column: %q, Operator: %q}, want {Age, startswith}", opErr.Column, opErr.Operator)
		}
	})

	t.Run("problems are aggregated into one error", func(t *testing.T) {
		// A missing from target plus two bad operators all surface at once.
		q := sqlk.NewQuery().
			Where("A", "startswith", 1).
			Where("B", "~!", 2)

		_, err := comp.Compile(q)
		if err == nil {
			t.Fatal("Compile(...) error = nil, want aggregated error")
		}
		if !errors.Is(err, ErrNoFromTarget) {
			t.Errorf("errors.Is(err, ErrNoFromTarget) = false, want true")
		}

		joined, ok := err.(interface{ Unwrap() []error })
		if !ok {
			t.Fatalf("error %v does not unwrap to a joined error list", err)
		}
		var opErrs []*OperatorError
		for _, e := range joined.Unwrap() {
			if opErr, ok := errors.AsType[*OperatorError](e); ok {
				opErrs = append(opErrs, opErr)
			}
		}
		if len(opErrs) != 2 {
			t.Fatalf("aggregated error holds %d OperatorError, want 2 (%v)", len(opErrs), err)
		}
		if opErrs[0].Column != "A" || opErrs[1].Column != "B" {
			t.Errorf("OperatorError columns = %q, %q, want A, B", opErrs[0].Column, opErrs[1].Column)
		}
	})

	t.Run("operator checks are case insensitive", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("Users").Where("Name", "STAR*", "x"))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
	})

	t.Run("operator checks cover column-comparison conditions", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("Table").WhereColumns("A", "startswith", "B"))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
		opErr, ok := errors.AsType[*OperatorError](err)
		if !ok {
			t.Fatalf("Compile(...) error = %v, want an *OperatorError", err)
		}
		if opErr.Column != "A" || opErr.Operator != "startswith" {
			t.Errorf("OperatorError = {Column: %q, Operator: %q}, want {A, startswith}", opErr.Column, opErr.Operator)
		}
	})

	t.Run("operator checks cover subquery-value conditions", func(t *testing.T) {
		sub := sqlk.NewQuery().From("Logs").Select("Id")
		_, err := comp.Compile(sqlk.NewQuery().From("Table").WhereSub(sub, "~!", 1))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
		opErr, ok := errors.AsType[*OperatorError](err)
		if !ok {
			t.Fatalf("Compile(...) error = %v, want an *OperatorError", err)
		}
		if opErr.Operator != "~!" {
			t.Errorf("OperatorError.Operator = %q, want ~!", opErr.Operator)
		}
	})

	t.Run("operator checks cover date-part conditions", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("Table").WhereDatePart("year", "Stamp", "startswith", 2018))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
		opErr, ok := errors.AsType[*OperatorError](err)
		if !ok {
			t.Fatalf("Compile(...) error = %v, want an *OperatorError", err)
		}
		if opErr.Column != "Stamp" || opErr.Operator != "startswith" {
			t.Errorf("OperatorError = {Column: %q, Operator: %q}, want {Stamp, startswith}", opErr.Column, opErr.Operator)
		}
	})

	t.Run("escape characters longer than one character are rejected", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("Table").WhereLike("A", "x", sqlk.EscapeLike("ab")))
		if !errors.Is(err, ErrInvalidEscapeCharacter) {
			t.Fatalf("Compile(...) error = %v, want ErrInvalidEscapeCharacter", err)
		}
		escErr, ok := errors.AsType[*EscapeCharacterError](err)
		if !ok {
			t.Fatalf("Compile(...) error = %v, want an *EscapeCharacterError", err)
		}
		if escErr.Escape != "ab" {
			t.Errorf("EscapeCharacterError.Escape = %q, want ab", escErr.Escape)
		}
	})

	t.Run("escape character validation descends into groups", func(t *testing.T) {
		q := sqlk.NewQuery().From("Table").WhereGroup(func(n *sqlk.Query) *sqlk.Query {
			return n.WhereContains("A", "x", sqlk.EscapeLike(`\%`))
		})
		_, err := comp.Compile(q)
		if !errors.Is(err, ErrInvalidEscapeCharacter) {
			t.Fatalf("Compile(...) error = %v, want ErrInvalidEscapeCharacter", err)
		}
	})

	t.Run("single quote as escape character is rejected", func(t *testing.T) {
		// ESCAPE ''' would emit an invalid string literal; the single quote
		// is rejected even though it is one rune (a Go-side check beyond the
		// SqlKata baseline).
		_, err := comp.Compile(sqlk.NewQuery().From("Table").WhereLike("A", "x", sqlk.EscapeLike("'")))
		if !errors.Is(err, ErrInvalidEscapeCharacter) {
			t.Fatalf("Compile(...) error = %v, want ErrInvalidEscapeCharacter", err)
		}
	})

	t.Run("single multibyte rune passes as one character", func(t *testing.T) {
		res, err := comp.Compile(sqlk.NewQuery().From("Table").WhereLike("A", "x", sqlk.EscapeLike("→")))
		if err != nil {
			t.Fatalf("Compile(...) error = %v, want nil", err)
		}
		if want := `SELECT * FROM "Table" WHERE LOWER("A") like ? ESCAPE '→'`; res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("like-family conditions compile without whitelist friction", func(t *testing.T) {
		// starts/ends/contains map to like internally and skip the whitelist.
		res, err := comp.Compile(sqlk.NewQuery().From("Table").
			WhereStarts("A", "x").
			WhereEnds("B", "y").
			WhereContains("C", "z"))
		if err != nil {
			t.Fatalf("Compile(...) error = %v, want nil", err)
		}
		if want := `SELECT * FROM "Table" WHERE LOWER("A") like ? AND LOWER("B") like ? AND LOWER("C") like ?`; res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("validation descends into condition subqueries and groups", func(t *testing.T) {
		t.Run("missing from target inside an in-subquery", func(t *testing.T) {
			sub := sqlk.NewQuery().Select("Id")
			_, err := comp.Compile(sqlk.NewQuery().From("Users").WhereInSub("Id", sub))
			if !errors.Is(err, ErrNoFromTarget) {
				t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
			}
		})

		t.Run("missing from target inside an exists subquery", func(t *testing.T) {
			sub := sqlk.NewQuery().Select("Id")
			_, err := comp.Compile(sqlk.NewQuery().From("Users").WhereExists(sub))
			if !errors.Is(err, ErrNoFromTarget) {
				t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
			}
		})

		t.Run("operator outside whitelist inside a group", func(t *testing.T) {
			q := sqlk.NewQuery().From("Users").WhereGroup(func(n *sqlk.Query) *sqlk.Query {
				return n.Where("A", "startswith", 1)
			})
			_, err := comp.Compile(q)
			if !errors.Is(err, ErrOperatorNotAllowed) {
				t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
			}
		})

		t.Run("operator outside whitelist inside a where-sub subquery", func(t *testing.T) {
			sub := sqlk.NewQuery().From("Logs").Where("Type", "startswith", "x").Select("Id")
			_, err := comp.Compile(sqlk.NewQuery().From("Users").WhereSubEq(sub, 1))
			if !errors.Is(err, ErrOperatorNotAllowed) {
				t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
			}
		})

		t.Run("groups are not standalone selects and skip the from check", func(t *testing.T) {
			// A group scope carries conditions only; having no from
			// target is normal and must not fail validation.
			res, err := comp.Compile(sqlk.NewQuery().From("Users").WhereGroup(func(n *sqlk.Query) *sqlk.Query {
				return n.WhereEq("A", 1)
			}))
			if err != nil {
				t.Fatalf("Compile(...) error = %v, want nil", err)
			}
			if want := `SELECT * FROM "Users" WHERE ("A" = ?)`; res.SQL != want {
				t.Errorf("Compile(...) SQL = %q, want %q", res.SQL, want)
			}
		})
	})
}

func TestConditionClausesSurviveClone(t *testing.T) {
	comp := New()
	compile := func(t *testing.T, q *sqlk.Query) Result {
		t.Helper()
		return mustCompile(t, comp, q)
	}

	t.Run("groups and condition subqueries are cloned deeply", func(t *testing.T) {
		base := sqlk.NewQuery().From("Users").
			WhereGroup(func(n *sqlk.Query) *sqlk.Query {
				return n.WhereEq("A", 1).OrWhereEq("B", 2)
			}).
			WhereInSub("Id", sqlk.NewQuery().From("Logs").WhereEq("Type", "error").Select("UserId"))

		variant := base.Clone().WhereEq("C", 3)
		base.Limit(5)

		wantBase := `SELECT * FROM "Users" WHERE ("A" = ? OR "B" = ?) AND "Id" IN (SELECT "UserId" FROM "Logs" WHERE "Type" = ?) LIMIT ?`
		if got := compile(t, base); got.SQL != wantBase {
			t.Errorf("base SQL = %q, want %q", got.SQL, wantBase)
		} else if !reflect.DeepEqual(got.Args, []any{1, 2, "error", 5}) {
			t.Errorf("base Args = %#v, want [1 2 error 5]", got.Args)
		}

		wantVariant := `SELECT * FROM "Users" WHERE ("A" = ? OR "B" = ?) AND "Id" IN (SELECT "UserId" FROM "Logs" WHERE "Type" = ?) AND "C" = ?`
		if got := compile(t, variant); got.SQL != wantVariant {
			t.Errorf("variant SQL = %q, want %q", got.SQL, wantVariant)
		} else if !reflect.DeepEqual(got.Args, []any{1, 2, "error", 3}) {
			t.Errorf("variant Args = %#v, want [1 2 error 3]", got.Args)
		}
	})

	t.Run("like date and exists conditions survive clone", func(t *testing.T) {
		base := sqlk.NewQuery().From("Users").
			WhereContains("Name", "oh").
			WhereDatePart("year", "JoinedAt", ">", 2020).
			WhereExists(sqlk.NewQuery().From("Logs").WhereEq("Type", "error").Select("UserId"))

		variant := base.Clone().WhereEq("C", 3)
		base.WhereEq("D", 4)

		wantBase := `SELECT * FROM "Users" WHERE LOWER("Name") like ? AND YEAR("JoinedAt") > ? AND EXISTS (SELECT 1 FROM "Logs" WHERE "Type" = ?) AND "D" = ?`
		if got := compile(t, base); got.SQL != wantBase {
			t.Errorf("base SQL = %q, want %q", got.SQL, wantBase)
		} else if !reflect.DeepEqual(got.Args, []any{"%oh%", 2020, "error", 4}) {
			t.Errorf("base Args = %#v, want [%%oh%% 2020 error 4]", got.Args)
		}

		wantVariant := `SELECT * FROM "Users" WHERE LOWER("Name") like ? AND YEAR("JoinedAt") > ? AND EXISTS (SELECT 1 FROM "Logs" WHERE "Type" = ?) AND "C" = ?`
		if got := compile(t, variant); got.SQL != wantVariant {
			t.Errorf("variant SQL = %q, want %q", got.SQL, wantVariant)
		} else if !reflect.DeepEqual(got.Args, []any{"%oh%", 2020, "error", 3}) {
			t.Errorf("variant Args = %#v, want [%%oh%% 2020 error 3]", got.Args)
		}
	})
}

func TestCompileAggregateForms(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "count defaults to star",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Count() },
			sql:   `SELECT COUNT(*) AS "count" FROM "A"`,
		},
		{
			name:  "count named column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Count("UserId") },
			sql:   `SELECT COUNT("UserId") AS "count" FROM "A"`,
		},
		{
			name:  "count qualified column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Count("a.UserId") },
			sql:   `SELECT COUNT("a"."UserId") AS "count" FROM "A"`,
		},
		{
			name:  "sum",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Sum("PacketsDropped") },
			sql:   `SELECT SUM("PacketsDropped") AS "sum" FROM "A"`,
		},
		{
			name:  "avg",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Avg("TTL") },
			sql:   `SELECT AVG("TTL") AS "avg" FROM "A"`,
		},
		{
			name:  "max",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Max("LatencyMs") },
			sql:   `SELECT MAX("LatencyMs") AS "max" FROM "A"`,
		},
		{
			name:  "min",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Min("LatencyMs") },
			sql:   `SELECT MIN("LatencyMs") AS "min" FROM "A"`,
		},
		{
			name:  "generic aggregate verb",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Aggregate("sum", "Total") },
			sql:   `SELECT SUM("Total") AS "sum" FROM "A"`,
		},
		{
			name: "later aggregate replaces the earlier one",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Sum("Total").Count()
			},
			sql: `SELECT COUNT(*) AS "count" FROM "A"`,
		},
		{
			name: "wheres are kept in aggregate form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").WhereEq("Id", 1).Count()
			},
			sql:  `SELECT COUNT(*) AS "count" FROM "A" WHERE "Id" = ?`,
			args: []any{1},
		},
		{
			name: "limit is dropped in aggregate form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Limit(10).Count()
			},
			sql: `SELECT COUNT(*) AS "count" FROM "A"`,
		},
	})
}

func TestCompileAggregateWrap(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "count over multiple columns wraps a subquery",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Count("ColumnA", "ColumnB") },
			sql:   `SELECT COUNT(*) AS "count" FROM (SELECT 1 FROM "A" WHERE "ColumnA" IS NOT NULL AND "ColumnB" IS NOT NULL) AS "countQuery"`,
		},
		{
			name:  "distinct count wraps a subquery",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Distinct().Count() },
			sql:   `SELECT COUNT(*) AS "count" FROM (SELECT DISTINCT * FROM "A") AS "countQuery"`,
		},
		{
			name: "distinct count over multiple columns wraps the aggregate columns",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Distinct().Count("ColumnA", "ColumnB")
			},
			sql: `SELECT COUNT(*) AS "count" FROM (SELECT DISTINCT "ColumnA", "ColumnB" FROM "A") AS "countQuery"`,
		},
		{
			name:  "distinct single column still wraps",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Distinct().Count("X") },
			sql:   `SELECT COUNT(*) AS "count" FROM (SELECT DISTINCT "X" FROM "A") AS "countQuery"`,
		},
		{
			name: "wrap alias follows the aggregate type",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Aggregate("max", "Latency", "Uptime")
			},
			sql: `SELECT MAX(*) AS "max" FROM (SELECT 1 FROM "A" WHERE "Latency" IS NOT NULL AND "Uptime" IS NOT NULL) AS "maxQuery"`,
		},
		{
			name: "inner query keeps its wheres before the not-null guards",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").WhereEq("Id", 1).Count("CA", "CB")
			},
			sql:  `SELECT COUNT(*) AS "count" FROM (SELECT 1 FROM "A" WHERE "Id" = ? AND "CA" IS NOT NULL AND "CB" IS NOT NULL) AS "countQuery"`,
			args: []any{1},
		},
		{
			name: "distinct wrap keeps inner wheres",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").WhereEq("Id", 1).Distinct().Count("CA")
			},
			sql:  `SELECT COUNT(*) AS "count" FROM (SELECT DISTINCT "CA" FROM "A" WHERE "Id" = ?) AS "countQuery"`,
			args: []any{1},
		},
		{
			name: "limit is dropped inside the wrapped inner query",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Limit(5).Distinct().Count()
			},
			sql: `SELECT COUNT(*) AS "count" FROM (SELECT DISTINCT * FROM "A") AS "countQuery"`,
		},
		{
			name: "select clauses are superseded by the aggregate form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Select("X").Count("CA", "CB")
			},
			sql: `SELECT COUNT(*) AS "count" FROM (SELECT 1 FROM "A" WHERE "CA" IS NOT NULL AND "CB" IS NOT NULL) AS "countQuery"`,
		},
	})

	t.Run("compiling twice yields the same sql", func(t *testing.T) {
		// The rewrite works on a copy: repeated compiles must not
		// accumulate IS NOT NULL guards.
		comp := New()
		q := sqlk.NewQuery().From("A").Count("CA", "CB")
		first := mustCompile(t, comp, q)
		second := mustCompile(t, comp, q)
		if first.SQL != second.SQL {
			t.Errorf("second Compile SQL = %q, want same as first %q", second.SQL, first.SQL)
		}
		if !reflect.DeepEqual(first.Args, second.Args) {
			t.Errorf("second Compile Args = %#v, want same as first %#v", second.Args, first.Args)
		}
	})
}

func TestCompileAggregateSubQueries(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "aggregate query as a scalar projection subquery",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectSub(sqlk.NewQuery().From("Logs").Count(), "LogCount")
			},
			sql: `SELECT (SELECT COUNT(*) AS "count" FROM "Logs") AS "LogCount" FROM "Users"`,
		},
		{
			name: "distinct single-column aggregate subquery compiles to COUNT(DISTINCT ...)",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectSub(sqlk.NewQuery().From("Logs").Distinct().Count("UserId"), "Authors")
			},
			sql: `SELECT (SELECT COUNT(DISTINCT "UserId") AS "count" FROM "Logs") AS "Authors" FROM "Users"`,
		},
		{
			name: "aggregate query as the from target",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromSub(sqlk.NewQuery().From("Logs").WhereEq("Type", "error").Count(), "T")
			},
			sql:  `SELECT * FROM (SELECT COUNT(*) AS "count" FROM "Logs" WHERE "Type" = ?) AS "T"`,
			args: []any{"error"},
		},
		{
			name: "aggregate subquery compared to a value",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereSub(sqlk.NewQuery().From("Logs").WhereEq("UserId", 1).Count(), ">", 10)
			},
			sql:  `SELECT * FROM "Users" WHERE (SELECT COUNT(*) AS "count" FROM "Logs" WHERE "UserId" = ?) > ?`,
			args: []any{1, 10},
		},
	})
}

func TestCompileSelectAggregateColumns(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "select count column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").SelectCount("UserId") },
			sql:   `SELECT COUNT("UserId") FROM "A"`,
		},
		{
			name:  "select count star",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").SelectCount("*") },
			sql:   `SELECT COUNT(*) FROM "A"`,
		},
		{
			name:  "select sum column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").SelectSum("Total") },
			sql:   `SELECT SUM("Total") FROM "A"`,
		},
		{
			name: "aggregate projection columns mix with plain columns",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Select("Name").SelectAvg("TTL").SelectMin("Latency").SelectMax("Uptime")
			},
			sql: `SELECT "Name", AVG("TTL"), MIN("Latency"), MAX("Uptime") FROM "A"`,
		},
		{
			name:  "generic select aggregate verb",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").SelectAggregate("sum", "Total") },
			sql:   `SELECT SUM("Total") FROM "A"`,
		},
		{
			name:  "column alias moves outside the aggregate",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").SelectSum("Total as T") },
			sql:   `SELECT SUM("Total") AS "T" FROM "A"`,
		},
		{
			name:  "qualified column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").SelectCount("a.UserId") },
			sql:   `SELECT COUNT("a"."UserId") FROM "A"`,
		},
		{
			name: "filter compiles to CASE WHEN on the base compiler",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").SelectSum("Total", func(f *sqlk.Query) *sqlk.Query {
					return f.WhereEq("Country", "US")
				})
			},
			sql:  `SELECT SUM(CASE WHEN "Country" = ? THEN "Total" END) FROM "A"`,
			args: []any{"US"},
		},
		{
			name: "filter keeps the column alias after CASE WHEN",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").SelectAvg("TTL as Average", func(f *sqlk.Query) *sqlk.Query {
					return f.WhereEq("Tier", "gold")
				})
			},
			sql:  `SELECT AVG(CASE WHEN "Tier" = ? THEN "TTL" END) AS "Average" FROM "A"`,
			args: []any{"gold"},
		},
		{
			name: "filter conditions combine with their connectors",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").SelectCount("Id", func(f *sqlk.Query) *sqlk.Query {
					return f.WhereEq("A", 1).OrWhereEq("B", 2)
				})
			},
			sql:  `SELECT COUNT(CASE WHEN "A" = ? OR "B" = ? THEN "Id" END) FROM "A"`,
			args: []any{1, 2},
		},
		{
			name: "filter callback without conditions compiles to a plain aggregate",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").SelectSum("Total", func(f *sqlk.Query) *sqlk.Query { return f })
			},
			sql: `SELECT SUM("Total") FROM "A"`,
		},
	})

	t.Run("filter-aware dialect compiles to FILTER (WHERE ...)", func(t *testing.T) {
		comp := New()
		comp.supportsFilterClause = true

		runCompileCases(t, comp, []compileCase{
			{
				name: "filter clause form",
				build: func(q *sqlk.Query) *sqlk.Query {
					return q.From("A").SelectSum("Total", func(f *sqlk.Query) *sqlk.Query {
						return f.WhereEq("Country", "US")
					})
				},
				sql:  `SELECT SUM("Total") FILTER (WHERE "Country" = ?) FROM "A"`,
				args: []any{"US"},
			},
			{
				name: "filter clause form keeps the alias",
				build: func(q *sqlk.Query) *sqlk.Query {
					return q.From("A").SelectMin("Latency as Floor", func(f *sqlk.Query) *sqlk.Query {
						return f.Where("Latency", ">", 0)
					})
				},
				sql:  `SELECT MIN("Latency") FILTER (WHERE "Latency" > ?) AS "Floor" FROM "A"`,
				args: []any{0},
			},
			{
				name: "without filter nothing changes",
				build: func(q *sqlk.Query) *sqlk.Query {
					return q.From("A").SelectMax("Uptime")
				},
				sql: `SELECT MAX("Uptime") FROM "A"`,
			},
		})
	})
}

func TestAggregateValidationAndClone(t *testing.T) {
	comp := New()

	t.Run("aggregate form still requires a from target", func(t *testing.T) {
		if _, err := comp.Compile(sqlk.NewQuery().Count()); !errors.Is(err, ErrNoFromTarget) {
			t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
		}
	})

	t.Run("filter operators go through the whitelist", func(t *testing.T) {
		q := sqlk.NewQuery().From("A").SelectSum("Total", func(f *sqlk.Query) *sqlk.Query {
			return f.Where("Country", "startswith", "U")
		})
		_, err := comp.Compile(q)
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
		opErr, ok := errors.AsType[*OperatorError](err)
		if !ok {
			t.Fatalf("Compile(...) error = %v, want an *OperatorError", err)
		}
		if opErr.Column != "Country" {
			t.Errorf("OperatorError.Column = %q, want Country", opErr.Column)
		}
	})

	t.Run("aggregate clauses and filters survive clone", func(t *testing.T) {
		base := sqlk.NewQuery().From("A").
			Count("CA", "CB").
			SelectSum("Total", func(f *sqlk.Query) *sqlk.Query {
				return f.WhereEq("Country", "US")
			})

		variant := base.Clone()
		base.Sum("Other").WhereEq("Id", 1)

		wantVariant := `SELECT COUNT(*) AS "count" FROM (SELECT 1 FROM "A" WHERE "CA" IS NOT NULL AND "CB" IS NOT NULL) AS "countQuery"`
		if got := mustCompile(t, comp, variant); got.SQL != wantVariant {
			t.Errorf("variant SQL = %q, want %q", got.SQL, wantVariant)
		}

		wantBase := `SELECT SUM("Other") AS "sum" FROM "A" WHERE "Id" = ?`
		if got := mustCompile(t, comp, base); got.SQL != wantBase {
			t.Errorf("base SQL = %q, want %q", got.SQL, wantBase)
		} else if !reflect.DeepEqual(got.Args, []any{1}) {
			t.Errorf("base Args = %#v, want [1]", got.Args)
		}
	})

	t.Run("cloned filters are independent of the original callback scope", func(t *testing.T) {
		scope := sqlk.NewQuery().WhereEq("Country", "US")
		q := sqlk.NewQuery().From("A").SelectAggregate("sum", "Total", func(f *sqlk.Query) *sqlk.Query {
			return scope
		})

		variant := q.Clone()
		scope.WhereEq("Tier", "gold")

		want := `SELECT SUM(CASE WHEN "Country" = ? THEN "Total" END) FROM "A"`
		if got := mustCompile(t, comp, variant); got.SQL != want {
			t.Errorf("variant SQL = %q, want %q", got.SQL, want)
		}
	})
}

func TestCompileHavingCoreFamily(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "having with operator",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").Having("Total", ">", 1) },
			sql:   `SELECT * FROM "Users" HAVING "Total" > ?`,
			args:  []any{1},
		},
		{
			name:  "having equality shorthand",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingEq("Total", 10) },
			sql:   `SELECT * FROM "Users" HAVING "Total" = ?`,
			args:  []any{10},
		},
		{
			name: "multiple havings combine with AND",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Having("Total", ">", 1).HavingEq("Count", 2)
			},
			sql:  `SELECT * FROM "Users" HAVING "Total" > ? AND "Count" = ?`,
			args: []any{1, 2},
		},
		{
			name: "or having",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Having("Total", ">", 1).OrHaving("Count", "=", 2)
			},
			sql:  `SELECT * FROM "Users" HAVING "Total" > ? OR "Count" = ?`,
			args: []any{1, 2},
		},
		{
			name: "having not",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingNot("Total", "=", 1)
			},
			sql:  `SELECT * FROM "Users" HAVING NOT ("Total" = ?)`,
			args: []any{1},
		},
		{
			name: "or having not",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingEq("A", 1).OrHavingNot("B", "=", 2)
			},
			sql:  `SELECT * FROM "Users" HAVING "A" = ? OR NOT ("B" = ?)`,
			args: []any{1, 2},
		},
		{
			name: "eq shorthand variants",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").OrHavingEq("A", 1).HavingNotEq("B", 2).OrHavingNotEq("C", 3)
			},
			sql:  `SELECT * FROM "Users" HAVING "A" = ? AND NOT ("B" = ?) OR NOT ("C" = ?)`,
			args: []any{1, 2, 3},
		},
		{
			name: "having map joins pairs with AND in sorted key order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingMap(sqlk.Record{"Name": "x", "Age": 18})
			},
			sql:  `SELECT * FROM "Users" HAVING "Age" = ? AND "Name" = ?`,
			args: []any{18, "x"},
		},
		{
			name: "where and having land in their own sections",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Id", 1).HavingEq("Total", 2)
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" = ? HAVING "Total" = ?`,
			args: []any{1, 2},
		},
	})
}

func TestCompileHavingNullAndBoolean(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "having null",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingNull("Total") },
			sql:   `SELECT * FROM "Users" HAVING "Total" IS NULL`,
		},
		{
			name:  "having not null",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingNotNull("Total") },
			sql:   `SELECT * FROM "Users" HAVING "Total" IS NOT NULL`,
		},
		{
			name: "or having null and or having not null",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingEq("A", 1).OrHavingNull("B").OrHavingNotNull("C")
			},
			sql:  `SELECT * FROM "Users" HAVING "A" = ? OR "B" IS NULL OR "C" IS NOT NULL`,
			args: []any{1},
		},
		{
			name:  "having true",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingTrue("IsActive") },
			sql:   `SELECT * FROM "Users" HAVING "IsActive" = true`,
		},
		{
			name:  "having false",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingFalse("IsActive") },
			sql:   `SELECT * FROM "Users" HAVING "IsActive" = false`,
		},
		{
			name: "or having true and or having false",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingEq("A", 1).OrHavingTrue("B").OrHavingFalse("C")
			},
			sql:  `SELECT * FROM "Users" HAVING "A" = ? OR "B" = true OR "C" = false`,
			args: []any{1},
		},
	})
}

func TestCompileHavingLikeFamily(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "having like defaults to case insensitive",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingLike("City", "Taipei%")
			},
			sql:  `SELECT * FROM "Users" HAVING LOWER("City") like ?`,
			args: []any{"taipei%"},
		},
		{
			name: "case sensitive option skips lowering",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingLike("City", "Taipei%", sqlk.CaseSensitive())
			},
			sql:  `SELECT * FROM "Users" HAVING "City" like ?`,
			args: []any{"Taipei%"},
		},
		{
			name: "starts, ends and contains append their wildcards",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingStarts("A", "x").HavingEnds("B", "y").HavingContains("C", "z")
			},
			sql:  `SELECT * FROM "Users" HAVING LOWER("A") like ? AND LOWER("B") like ? AND LOWER("C") like ?`,
			args: []any{"x%", "%y", "%z%"},
		},
		{
			name: "escape character appends an escape clause",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingLike("A", `x\%`, sqlk.EscapeLike(`\`))
			},
			sql:  `SELECT * FROM "Users" HAVING LOWER("A") like ? ESCAPE '\'`,
			args: []any{`x\%`},
		},
		{
			name: "not variants wrap the match in NOT",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingNotContains("A", "x").HavingNotLike("B", "y%")
			},
			sql:  `SELECT * FROM "Users" HAVING NOT (LOWER("A") like ?) AND NOT (LOWER("B") like ?)`,
			args: []any{"%x%", "y%"},
		},
		{
			name: "or variants connect with OR",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingEq("Id", 1).
					OrHavingStarts("A", "x").
					OrHavingNotEnds("B", "y")
			},
			sql:  `SELECT * FROM "Users" HAVING "Id" = ? OR LOWER("A") like ? OR NOT (LOWER("B") like ?)`,
			args: []any{1, "x%", "%y"},
		},
	})
}

func TestCompileHavingBetweenAndIn(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "having between",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingBetween("Total", 1, 10) },
			sql:   `SELECT * FROM "Users" HAVING "Total" BETWEEN ? AND ?`,
			args:  []any{1, 10},
		},
		{
			name:  "having not between",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingNotBetween("Total", 1, 10) },
			sql:   `SELECT * FROM "Users" HAVING "Total" NOT BETWEEN ? AND ?`,
			args:  []any{1, 10},
		},
		{
			name: "or having between and or having not between",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingEq("A", 1).OrHavingBetween("B", 2, 3).OrHavingNotBetween("C", 4, 5)
			},
			sql:  `SELECT * FROM "Users" HAVING "A" = ? OR "B" BETWEEN ? AND ? OR "C" NOT BETWEEN ? AND ?`,
			args: []any{1, 2, 3, 4, 5},
		},
		{
			name:  "having in",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingIn("Total", 1, 2, 3) },
			sql:   `SELECT * FROM "Users" HAVING "Total" IN (?, ?, ?)`,
			args:  []any{1, 2, 3},
		},
		{
			name:  "having not in",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingNotIn("Total", 1, 2) },
			sql:   `SELECT * FROM "Users" HAVING "Total" NOT IN (?, ?)`,
			args:  []any{1, 2},
		},
		{
			name:  "empty in compiles to a constant false placeholder",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingIn("Total") },
			sql:   `SELECT * FROM "Users" HAVING 1 = 0 /* IN [empty list] */`,
		},
		{
			name: "having in subquery",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Logs").Select("UserId")
				return q.From("Users").HavingInSub("Id", sub)
			},
			sql: `SELECT * FROM "Users" HAVING "Id" IN (SELECT "UserId" FROM "Logs")`,
		},
		{
			name: "or having not in subquery",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Logs").Select("UserId")
				return q.From("Users").HavingEq("A", 1).OrHavingNotInSub("Id", sub)
			},
			sql:  `SELECT * FROM "Users" HAVING "A" = ? OR "Id" NOT IN (SELECT "UserId" FROM "Logs")`,
			args: []any{1},
		},
		{
			name: "not in sub variant",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Logs").Select("UserId")
				return q.From("Users").HavingNotInSub("Id", sub).OrHavingInSub("Ref", sub)
			},
			sql: `SELECT * FROM "Users" HAVING "Id" NOT IN (SELECT "UserId" FROM "Logs") OR "Ref" IN (SELECT "UserId" FROM "Logs")`,
		},
	})
}

func TestCompileHavingGroupNested(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "nested group compiles to a parenthesized combination",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingGroup(func(n *sqlk.Query) *sqlk.Query {
					return n.WhereEq("A", 1).OrWhereEq("B", 2)
				})
			},
			sql:  `SELECT * FROM "Users" HAVING ("A" = ? OR "B" = ?)`,
			args: []any{1, 2},
		},
		{
			name: "groups nest to arbitrary depth",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingGroup(func(n *sqlk.Query) *sqlk.Query {
					return n.WhereEq("A", 1).OrWhereGroup(func(m *sqlk.Query) *sqlk.Query {
						return m.WhereEq("B", 2).WhereEq("C", 3)
					})
				})
			},
			sql:  `SELECT * FROM "Users" HAVING ("A" = ? OR ("B" = ? AND "C" = ?))`,
			args: []any{1, 2, 3},
		},
		{
			name: "having not group",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingNotGroup(func(n *sqlk.Query) *sqlk.Query {
					return n.WhereEq("A", 1).OrWhereEq("B", 2)
				})
			},
			sql:  `SELECT * FROM "Users" HAVING NOT ("A" = ? OR "B" = ?)`,
			args: []any{1, 2},
		},
		{
			name: "or having group and or having not group",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingEq("A", 1).
					OrHavingGroup(func(n *sqlk.Query) *sqlk.Query {
						return n.WhereEq("B", 2).WhereEq("C", 3)
					}).
					OrHavingNotGroup(func(n *sqlk.Query) *sqlk.Query {
						return n.WhereEq("D", 4)
					})
			},
			sql:  `SELECT * FROM "Users" HAVING "A" = ? OR ("B" = ? AND "C" = ?) OR NOT ("D" = ?)`,
			args: []any{1, 2, 3, 4},
		},
		{
			name: "empty group is omitted while a non-empty group stays",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").
					HavingGroup(func(n *sqlk.Query) *sqlk.Query { return n.WhereEq("A", 1) }).
					HavingGroup(func(n *sqlk.Query) *sqlk.Query { return n })
			},
			sql:  `SELECT * FROM "Users" HAVING ("A" = ?)`,
			args: []any{1},
		},
		{
			name: "a query of only an empty having group has no having section",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingGroup(func(n *sqlk.Query) *sqlk.Query { return n })
			},
			sql: `SELECT * FROM "Users"`,
		},
		{
			// A group scope carries where-section conditions only: having
			// verbs inside the callback contribute nothing, so the group
			// is omitted as empty.
			name: "having verbs inside a group callback are outside the group scope",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingGroup(func(n *sqlk.Query) *sqlk.Query {
					return n.HavingEq("A", 1)
				})
			},
			sql: `SELECT * FROM "Users"`,
		},
	})
}

func TestCompileHavingRawColumnsSubExistsDate(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "having raw with bindings",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingRaw("COUNT(*) > ?", 5) },
			sql:   `SELECT * FROM "Users" HAVING COUNT(*) > ?`,
			args:  []any{5},
		},
		{
			name: "having raw wraps identifier markers",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingRaw("SUM({Total}) > ?", 10)
			},
			sql:  `SELECT * FROM "Users" HAVING SUM("Total") > ?`,
			args: []any{10},
		},
		{
			name: "or having raw",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingEq("A", 1).OrHavingRaw("COUNT(*) > ?", 2)
			},
			sql:  `SELECT * FROM "Users" HAVING "A" = ? OR COUNT(*) > ?`,
			args: []any{1, 2},
		},
		{
			name:  "having columns",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingColumns("Total", ">", "Quota") },
			sql:   `SELECT * FROM "Users" HAVING "Total" > "Quota"`,
		},
		{
			name: "or having columns",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").HavingEq("A", 1).OrHavingColumns("Total", ">", "Quota")
			},
			sql:  `SELECT * FROM "Users" HAVING "A" = ? OR "Total" > "Quota"`,
			args: []any{1},
		},
		{
			name: "having sub and eq shorthand",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Logs").Select("Total")
				return q.From("Users").HavingSub(sub, ">", 100).HavingSubEq(sub, 50)
			},
			sql:  `SELECT * FROM "Users" HAVING (SELECT "Total" FROM "Logs") > ? AND (SELECT "Total" FROM "Logs") = ?`,
			args: []any{100, 50},
		},
		{
			name: "or having sub",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Logs").Select("Total")
				return q.From("Users").HavingEq("A", 1).OrHavingSub(sub, "<", 10).OrHavingSubEq(sub, 20)
			},
			sql:  `SELECT * FROM "Users" HAVING "A" = ? OR (SELECT "Total" FROM "Logs") < ? OR (SELECT "Total" FROM "Logs") = ?`,
			args: []any{1, 10, 20},
		},
		{
			name: "having exists omits the select list",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Logs").WhereEq("Type", "x")
				return q.From("Users").HavingExists(sub)
			},
			sql:  `SELECT * FROM "Users" HAVING EXISTS (SELECT 1 FROM "Logs" WHERE "Type" = ?)`,
			args: []any{"x"},
		},
		{
			name: "having not exists and or variants",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Logs")
				return q.From("Users").HavingNotExists(sub).OrHavingExists(sub).OrHavingNotExists(sub)
			},
			sql: `SELECT * FROM "Users" HAVING NOT EXISTS (SELECT 1 FROM "Logs") OR EXISTS (SELECT 1 FROM "Logs") OR NOT EXISTS (SELECT 1 FROM "Logs")`,
		},
		{
			name:  "having date part",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").HavingDatePart("year", "Stamp", "=", 2018) },
			sql:   `SELECT * FROM "Users" HAVING YEAR("Stamp") = ?`,
			args:  []any{2018},
		},
		{
			name: "date part variants and eq shorthands",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").
					HavingNotDatePart("month", "A", "=", 1).
					OrHavingDatePart("day", "B", "=", 2).
					OrHavingNotDatePartEq("hour", "C", 3).
					HavingDatePartEq("minute", "D", 4)
			},
			sql:  `SELECT * FROM "Users" HAVING NOT (MONTH("A") = ?) OR DAY("B") = ? OR NOT (HOUR("C") = ?) AND MINUTE("D") = ?`,
			args: []any{1, 2, 3, 4},
		},
		{
			name: "date and time helpers target their fixed parts",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").
					HavingDate("CreatedAt", "=", 2024).
					HavingNotDate("UpdatedAt", "=", 2023).
					HavingTimeEq("SeenAt", 12).
					OrHavingNotTime("PingAt", "=", 13)
			},
			sql:  `SELECT * FROM "Users" HAVING DATE("CreatedAt") = ? AND NOT (DATE("UpdatedAt") = ?) AND TIME("SeenAt") = ? OR NOT (TIME("PingAt") = ?)`,
			args: []any{2024, 2023, 12, 13},
		},
	})
}

func TestCompileGroupBy(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "single column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").GroupBy("City") },
			sql:   `SELECT * FROM "Users" GROUP BY "City"`,
		},
		{
			name: "multiple columns accumulate across calls",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").GroupBy("City", "Age").GroupBy("Tier")
			},
			sql: `SELECT * FROM "Users" GROUP BY "City", "Age", "Tier"`,
		},
		{
			name:  "qualified names are wrapped per part",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").GroupBy("u.City") },
			sql:   `SELECT * FROM "Users" GROUP BY "u"."City"`,
		},
		{
			name: "combines with projection and where",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Select("City").WhereEq("Age", 18).GroupBy("City")
			},
			sql:  `SELECT "City" FROM "Users" WHERE "Age" = ? GROUP BY "City"`,
			args: []any{18},
		},
	})
}

func TestCompileGroupByHaving(t *testing.T) {
	comp := New()

	t.Run("full group by and having SQL", func(t *testing.T) {
		q := sqlk.NewQuery().From("Users").
			Select("City").
			GroupBy("City").
			Having("Total", ">", 5)
		want := `SELECT "City" FROM "Users" GROUP BY "City" HAVING "Total" > ?`
		if got := mustCompile(t, comp, q); got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		} else if !reflect.DeepEqual(got.Args, []any{5}) {
			t.Errorf("Args = %#v, want [5]", got.Args)
		}
	})

	t.Run("sections keep the SQL order where, group, having, limit", func(t *testing.T) {
		q := sqlk.NewQuery().From("Users").
			Where("Age", ">", 18).
			GroupBy("City").
			Having("Total", ">", 100).
			Limit(10)
		want := `SELECT * FROM "Users" WHERE "Age" > ? GROUP BY "City" HAVING "Total" > ? LIMIT ?`
		if got := mustCompile(t, comp, q); got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		} else if !reflect.DeepEqual(got.Args, []any{18, 100, 10}) {
			t.Errorf("Args = %#v, want [18 100 10]", got.Args)
		}
	})

	t.Run("grouped aggregate projection keeps having on the same level", func(t *testing.T) {
		q := sqlk.NewQuery().From("Users").
			SelectSum("Total").
			GroupBy("City").
			Having("Total", ">", 10)
		want := `SELECT SUM("Total") FROM "Users" GROUP BY "City" HAVING "Total" > ?`
		if got := mustCompile(t, comp, q); got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})

	// The aggregate rewrite drops the group section.
	t.Run("count over a grouped query drops the group section", func(t *testing.T) {
		q := sqlk.NewQuery().From("Users").GroupBy("City").Count()
		want := `SELECT COUNT(*) AS "count" FROM "Users"`
		if got := mustCompile(t, comp, q); got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})

	t.Run("multi-column aggregate wrap strips groups but keeps havings inside", func(t *testing.T) {
		q := sqlk.NewQuery().From("Users").
			GroupBy("City").
			Having("Total", ">", 5).
			Count("CA", "CB")
		want := `SELECT COUNT(*) AS "count" FROM (SELECT 1 FROM "Users" WHERE "CA" IS NOT NULL AND "CB" IS NOT NULL HAVING "Total" > ?) AS "countQuery"`
		if got := mustCompile(t, comp, q); got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})

	t.Run("group and having clauses survive clone", func(t *testing.T) {
		base := sqlk.NewQuery().From("Users").
			GroupBy("City").
			Having("Total", ">", 5).
			HavingGroup(func(n *sqlk.Query) *sqlk.Query {
				return n.WhereNull("Flag").OrWhereEq("Flag", 1)
			})

		variant := base.Clone()
		base.GroupBy("Extra").HavingEq("Other", 9)

		wantVariant := `SELECT * FROM "Users" GROUP BY "City" HAVING "Total" > ? AND ("Flag" IS NULL OR "Flag" = ?)`
		if got := mustCompile(t, comp, variant); got.SQL != wantVariant {
			t.Errorf("variant SQL = %q, want %q", got.SQL, wantVariant)
		} else if !reflect.DeepEqual(got.Args, []any{5, 1}) {
			t.Errorf("variant Args = %#v, want [5 1]", got.Args)
		}

		wantBase := `SELECT * FROM "Users" GROUP BY "City", "Extra" HAVING "Total" > ? AND ("Flag" IS NULL OR "Flag" = ?) AND "Other" = ?`
		if got := mustCompile(t, comp, base); got.SQL != wantBase {
			t.Errorf("base SQL = %q, want %q", got.SQL, wantBase)
		}
	})
}

func TestHavingValidation(t *testing.T) {
	comp := New()

	t.Run("operator outside whitelist in the having section", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("Users").Having("Age", "startswith", 18))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
		opErr, ok := errors.AsType[*OperatorError](err)
		if !ok {
			t.Fatalf("Compile(...) error = %v, want an *OperatorError", err)
		}
		if opErr.Column != "Age" || opErr.Operator != "startswith" {
			t.Errorf("OperatorError = {Column: %q, Operator: %q}, want {Age, startswith}", opErr.Column, opErr.Operator)
		}
	})

	t.Run("operator problems across where and having aggregate together", func(t *testing.T) {
		q := sqlk.NewQuery().From("Users").
			Where("A", "startswith", 1).
			Having("B", "~!", 2)

		_, err := comp.Compile(q)
		joined, ok := err.(interface{ Unwrap() []error })
		if !ok {
			t.Fatalf("error %v does not unwrap to a joined error list", err)
		}
		var opErrs []*OperatorError
		for _, e := range joined.Unwrap() {
			if opErr, ok := errors.AsType[*OperatorError](e); ok {
				opErrs = append(opErrs, opErr)
			}
		}
		if len(opErrs) != 2 {
			t.Fatalf("aggregated error holds %d OperatorError, want 2 (%v)", len(opErrs), err)
		}
		if opErrs[0].Column != "A" || opErrs[1].Column != "B" {
			t.Errorf("OperatorError columns = %q, %q, want A, B", opErrs[0].Column, opErrs[1].Column)
		}
	})

	t.Run("escape characters longer than one character are rejected in having", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("Users").HavingContains("A", "x", sqlk.EscapeLike("ab")))
		if !errors.Is(err, ErrInvalidEscapeCharacter) {
			t.Fatalf("Compile(...) error = %v, want ErrInvalidEscapeCharacter", err)
		}
	})

	t.Run("operator checks descend into having groups", func(t *testing.T) {
		q := sqlk.NewQuery().From("Users").HavingGroup(func(n *sqlk.Query) *sqlk.Query {
			return n.Where("A", "startswith", 1)
		})
		_, err := comp.Compile(q)
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
	})

	t.Run("from checks descend into having in-subqueries", func(t *testing.T) {
		sub := sqlk.NewQuery().Select("Id")
		_, err := comp.Compile(sqlk.NewQuery().From("Users").HavingInSub("Id", sub))
		if !errors.Is(err, ErrNoFromTarget) {
			t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
		}
	})
}

func TestCompileGroupByRaw(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "raw expression passes verbatim",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").GroupByRaw("City, Age") },
			sql:   `SELECT * FROM "Users" GROUP BY City, Age`,
		},
		{
			name:  "raw expression with bindings",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").GroupByRaw("coalesce(City, ?)", "unknown") },
			sql:   `SELECT * FROM "Users" GROUP BY coalesce(City, ?)`,
			args:  []any{"unknown"},
		},
		{
			name:  "identifier markers are wrapped",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").GroupByRaw("[City], [Age]") },
			sql:   `SELECT * FROM "Users" GROUP BY "City", "Age"`,
		},
		{
			name: "raw and plain group columns keep call order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").GroupBy("City").GroupByRaw("[Age]").GroupBy("Tier")
			},
			sql: `SELECT * FROM "Users" GROUP BY "City", "Age", "Tier"`,
		},
		{
			name: "bindings follow placeholder order among modeled clauses",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Id", 1).GroupByRaw("date_trunc('day', {CreatedAt})")
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" = ? GROUP BY date_trunc('day', "CreatedAt")`,
			args: []any{1},
		},
	})
}

func TestCompileOrderBy(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "single column",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").OrderBy("Name") },
			sql:   `SELECT * FROM "Users" ORDER BY "Name"`,
		},
		{
			name: "multiple columns accumulate across calls",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").OrderBy("Name", "Age").OrderBy("Tier")
			},
			sql: `SELECT * FROM "Users" ORDER BY "Name", "Age", "Tier"`,
		},
		{
			name:  "descending",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").OrderByDesc("Age") },
			sql:   `SELECT * FROM "Users" ORDER BY "Age" DESC`,
		},
		{
			name:  "descending over multiple columns",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").OrderByDesc("Age", "Name") },
			sql:   `SELECT * FROM "Users" ORDER BY "Age" DESC, "Name" DESC`,
		},
		{
			name: "ascending and descending mix in call order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").OrderBy("Name").OrderByDesc("Age").OrderBy("Tier")
			},
			sql: `SELECT * FROM "Users" ORDER BY "Name", "Age" DESC, "Tier"`,
		},
		{
			name:  "qualified names are wrapped per part",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users as u").OrderBy("u.Name") },
			sql:   `SELECT * FROM "Users" AS "u" ORDER BY "u"."Name"`,
		},
		{
			name: "repeated columns are kept as given",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").OrderBy("A").OrderBy("A")
			},
			sql: `SELECT * FROM "Users" ORDER BY "A", "A"`,
		},
		{
			name: "sections keep the SQL order where, group, having, order, limit",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").
					Where("Age", ">", 18).
					GroupBy("City").
					Having("Total", ">", 100).
					OrderBy("Name").
					Limit(10)
			},
			sql:  `SELECT * FROM "Users" WHERE "Age" > ? GROUP BY "City" HAVING "Total" > ? ORDER BY "Name" LIMIT ?`,
			args: []any{18, 100, 10},
		},
		{
			name: "order compiles inside subqueries",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectSub(sqlk.NewQuery().From("Logs").Select("Id").OrderByDesc("CreatedAt"), "LastLog")
			},
			sql: `SELECT (SELECT "Id" FROM "Logs" ORDER BY "CreatedAt" DESC) AS "LastLog" FROM "Users"`,
		},
	})
}

func TestCompileOrderByRaw(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "raw expression passes verbatim",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").OrderByRaw("col1 desc, col2") },
			sql:   `SELECT * FROM "Users" ORDER BY col1 desc, col2`,
		},
		{
			name: "raw expression with bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").OrderByRaw("mod(Id, ?), Name", 2)
			},
			sql:  `SELECT * FROM "Users" ORDER BY mod(Id, ?), Name`,
			args: []any{2},
		},
		{
			name:  "identifier markers are wrapped",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").OrderByRaw("[col1] desc, [col2]") },
			sql:   `SELECT * FROM "Users" ORDER BY "col1" desc, "col2"`,
		},
		{
			name: "raw and modeled order clauses keep call order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").OrderBy("Name").OrderByRaw("[Age] desc").OrderByDesc("Tier")
			},
			sql: `SELECT * FROM "Users" ORDER BY "Name", "Age" desc, "Tier" DESC`,
		},
		{
			name: "bindings follow placeholder order among modeled conditions",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Id", 1).OrderByRaw("mod(Id, ?)", 2).Limit(5)
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" = ? ORDER BY mod(Id, ?) LIMIT ?`,
			args: []any{1, 2, 5},
		},
	})
}

func TestCompileOrderByRandom(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "random compiles to the base compiler default function",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").OrderByRandom() },
			sql:   `SELECT * FROM "Users" ORDER BY RANDOM()`,
		},
		{
			name: "random mixes with modeled order clauses in call order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").OrderBy("Name").OrderByRandom().OrderByDesc("Age")
			},
			sql: `SELECT * FROM "Users" ORDER BY "Name", RANDOM(), "Age" DESC`,
		},
	})

	t.Run("random function is a dialect override point", func(t *testing.T) {
		// The random function is a dialect override point: setting the
		// compiler field replaces the default RANDOM().
		comp := New()
		comp.randomFunc = "NEWID()"
		res := mustCompile(t, comp, sqlk.NewQuery().From("Users").OrderByRandom())
		want := `SELECT * FROM "Users" ORDER BY NEWID()`
		if res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
	})
}

func TestTakeSkipForPage(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name:  "take folds to limit",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").Take(10) },
			sql:   `SELECT * FROM "Users" LIMIT ?`,
			args:  []any{10},
		},
		{
			name:  "skip folds to offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").Skip(20) },
			sql:   `SELECT * FROM "Users" OFFSET ?`,
			args:  []any{int64(20)},
		},
		{
			name: "take and skip combine",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Skip(20).Take(10)
			},
			sql:  `SELECT * FROM "Users" LIMIT ? OFFSET ?`,
			args: []any{10, int64(20)},
		},
		{
			name: "take and limit share the same clause slot",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Limit(5).Take(7)
			},
			sql:  `SELECT * FROM "Users" LIMIT ?`,
			args: []any{7},
		},
		{
			name: "skip and offset share the same clause slot",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Offset(5).Skip(9)
			},
			sql:  `SELECT * FROM "Users" OFFSET ?`,
			args: []any{int64(9)},
		},
		{
			name:  "for page 2 with per page 10",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").ForPage(2, 10) },
			sql:   `SELECT * FROM "Users" LIMIT ? OFFSET ?`,
			args:  []any{10, int64(10)},
		},
		{
			name:  "for page 1 lands on offset zero which is omitted",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").ForPage(1, 10) },
			sql:   `SELECT * FROM "Users" LIMIT ?`,
			args:  []any{10},
		},
		{
			name:  "for page without per page defaults to 15",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users").ForPage(3) },
			sql:   `SELECT * FROM "Users" LIMIT ? OFFSET ?`,
			args:  []any{15, int64(30)},
		},
		{
			name: "for page composes with where, order and full pagination",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Active", true).OrderByDesc("CreatedAt").ForPage(2, 5)
			},
			sql:  `SELECT * FROM "Users" WHERE "Active" = ? ORDER BY "CreatedAt" DESC LIMIT ? OFFSET ?`,
			args: []any{true, 5, int64(5)},
		},
	})

	t.Run("for page matches the explicit limit offset composition", func(t *testing.T) {
		comp := New()
		paged := mustCompile(t, comp, sqlk.NewQuery().From("Users").ForPage(4, 25))
		explicit := mustCompile(t, comp, sqlk.NewQuery().From("Users").Limit(25).Offset(75))
		if paged.SQL != explicit.SQL {
			t.Errorf("ForPage SQL = %q, want same as Limit/Offset %q", paged.SQL, explicit.SQL)
		}
		if !reflect.DeepEqual(paged.Args, explicit.Args) {
			t.Errorf("ForPage Args = %#v, want same as Limit/Offset %#v", paged.Args, explicit.Args)
		}
	})
}

func TestAggregateFormStripsOrderAndGroup(t *testing.T) {
	comp := New()

	t.Run("order is dropped in aggregate form", func(t *testing.T) {
		q := sqlk.NewQuery().From("Users").OrderBy("Name").OrderByDesc("Age").Count()
		want := `SELECT COUNT(*) AS "count" FROM "Users"`
		if got := mustCompile(t, comp, q); got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})

	t.Run("random and raw orders are dropped too", func(t *testing.T) {
		q := sqlk.NewQuery().From("Users").OrderByRandom().OrderByRaw("col1 desc").Sum("Total")
		want := `SELECT SUM("Total") AS "sum" FROM "Users"`
		if got := mustCompile(t, comp, q); got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})

	t.Run("order and group are stripped inside the wrapped inner query", func(t *testing.T) {
		q := sqlk.NewQuery().From("Users").
			OrderBy("Name").
			GroupBy("City").
			Count("CA", "CB")
		want := `SELECT COUNT(*) AS "count" FROM (SELECT 1 FROM "Users" WHERE "CA" IS NOT NULL AND "CB" IS NOT NULL) AS "countQuery"`
		if got := mustCompile(t, comp, q); got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})

	t.Run("orders survive in non-aggregate subqueries embedded in an aggregate query", func(t *testing.T) {
		// Stripping touches only the query's own sections; orders inside
		// an embedded exists subquery survive.
		q := sqlk.NewQuery().From("Users").
			WhereExists(sqlk.NewQuery().From("Logs").WhereEq("Type", "error").OrderBy("CreatedAt")).
			Count()
		want := `SELECT COUNT(*) AS "count" FROM "Users" WHERE EXISTS (SELECT 1 FROM "Logs" WHERE "Type" = ? ORDER BY "CreatedAt")`
		if got := mustCompile(t, comp, q); got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})
}

func TestOrderAndPaginationSurviveClone(t *testing.T) {
	comp := New()
	compile := func(t *testing.T, q *sqlk.Query) Result {
		t.Helper()
		return mustCompile(t, comp, q)
	}

	t.Run("order clauses are cloned deeply", func(t *testing.T) {
		base := sqlk.NewQuery().From("Users").
			OrderBy("Name").
			OrderByRaw("mod(Id, ?)", 2).
			OrderByRandom()

		variant := base.Clone().OrderByDesc("Age")
		base.Limit(5)

		wantVariant := `SELECT * FROM "Users" ORDER BY "Name", mod(Id, ?), RANDOM(), "Age" DESC`
		if got := compile(t, variant); got.SQL != wantVariant {
			t.Errorf("variant SQL = %q, want %q", got.SQL, wantVariant)
		} else if !reflect.DeepEqual(got.Args, []any{2}) {
			t.Errorf("variant Args = %#v, want [2]", got.Args)
		}

		wantBase := `SELECT * FROM "Users" ORDER BY "Name", mod(Id, ?), RANDOM() LIMIT ?`
		if got := compile(t, base); got.SQL != wantBase {
			t.Errorf("base SQL = %q, want %q", got.SQL, wantBase)
		} else if !reflect.DeepEqual(got.Args, []any{2, 5}) {
			t.Errorf("base Args = %#v, want [2 5]", got.Args)
		}
	})

	t.Run("pagination set via for page survives clone", func(t *testing.T) {
		base := sqlk.NewQuery().From("Users").ForPage(2, 10)

		variant := base.Clone().Take(3)
		base.Skip(30)

		wantVariant := `SELECT * FROM "Users" LIMIT ? OFFSET ?`
		if got := compile(t, variant); got.SQL != wantVariant {
			t.Errorf("variant SQL = %q, want %q", got.SQL, wantVariant)
		} else if !reflect.DeepEqual(got.Args, []any{3, int64(10)}) {
			t.Errorf("variant Args = %#v, want [3 10]", got.Args)
		}

		wantBase := `SELECT * FROM "Users" LIMIT ? OFFSET ?`
		if got := compile(t, base); got.SQL != wantBase {
			t.Errorf("base SQL = %q, want %q", got.SQL, wantBase)
		} else if !reflect.DeepEqual(got.Args, []any{10, int64(30)}) {
			t.Errorf("base Args = %#v, want [10 30]", got.Args)
		}
	})
}

func TestCompileJoinBasics(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			// Join sections are newline separated.
			name: "inner join with equality shorthand",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").JoinEq("countries", "countries.id", "users.country_id")
			},
			sql: "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"countries\".\"id\" = \"users\".\"country_id\"",
		},
		{
			name: "inner join with explicit operator",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").Join("countries", "countries.id", ">=", "users.country_id")
			},
			sql: "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"countries\".\"id\" >= \"users\".\"country_id\"",
		},
		{
			name: "left join",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").LeftJoinEq("countries", "countries.id", "users.country_id")
			},
			sql: "SELECT * FROM \"users\" \nLEFT JOIN \"countries\" ON \"countries\".\"id\" = \"users\".\"country_id\"",
		},
		{
			name: "right join",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").RightJoinEq("countries", "countries.id", "users.country_id")
			},
			sql: "SELECT * FROM \"users\" \nRIGHT JOIN \"countries\" ON \"countries\".\"id\" = \"users\".\"country_id\"",
		},
		{
			name:  "cross join emits no ON clause",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").CrossJoin("countries") },
			sql:   "SELECT * FROM \"users\" \nCROSS JOIN \"countries\"",
		},
		{
			name: "multiple joins accumulate newline separated",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").
					JoinEq("countries", "countries.id", "users.country_id").
					LeftJoinEq("profiles", "profiles.user_id", "users.id")
			},
			sql: "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"countries\".\"id\" = \"users\".\"country_id\"\nLEFT JOIN \"profiles\" ON \"profiles\".\"user_id\" = \"users\".\"id\"",
		},
		{
			name: "join target with alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users as u").JoinEq("countries as c", "c.id", "u.country_id")
			},
			sql: "SELECT * FROM \"users\" AS \"u\" \nINNER JOIN \"countries\" AS \"c\" ON \"c\".\"id\" = \"u\".\"country_id\"",
		},
		{
			name: "qualified join target is wrapped per part",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").JoinEq("meta.countries", "meta.countries.id", "users.country_id")
			},
			sql: "SELECT * FROM \"users\" \nINNER JOIN \"meta\".\"countries\" ON \"meta\".\"countries\".\"id\" = \"users\".\"country_id\"",
		},
		{
			name: "sections keep the SQL order join before where",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").
					JoinEq("countries", "countries.id", "users.country_id").
					WhereEq("users.age", 18).
					GroupBy("countries.name").
					Having("total", ">", 5).
					OrderBy("users.name").
					Limit(10)
			},
			sql:  "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"countries\".\"id\" = \"users\".\"country_id\" WHERE \"users\".\"age\" = ? GROUP BY \"countries\".\"name\" HAVING \"total\" > ? ORDER BY \"users\".\"name\" LIMIT ?",
			args: []any{18, 5, 10},
		},
	})
}

func TestCompileJoinSubQuery(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "subquery target with its own alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				stats := sqlk.NewQuery().From("Stats").Select("UserId", "Score").As("s")
				return q.From("Users").JoinSub(stats, func(j *sqlk.Join) *sqlk.Join {
					return j.On("s.UserId", "=", "Users.Id")
				})
			},
			sql: "SELECT * FROM \"Users\" \nINNER JOIN (SELECT \"UserId\", \"Score\" FROM \"Stats\") AS \"s\" ON \"s\".\"UserId\" = \"Users\".\"Id\"",
		},
		{
			name: "left and right variants of subquery targets",
			build: func(q *sqlk.Query) *sqlk.Query {
				stats := func() *sqlk.Query { return sqlk.NewQuery().From("Stats").Select("UserId").As("s") }
				return q.From("Users").
					LeftJoinSub(stats(), func(j *sqlk.Join) *sqlk.Join { return j.On("s.UserId", "=", "Users.Id") }).
					RightJoinSub(stats(), func(j *sqlk.Join) *sqlk.Join { return j.On("s.UserId", "<>", "Users.Id") })
			},
			sql: "SELECT * FROM \"Users\" \nLEFT JOIN (SELECT \"UserId\" FROM \"Stats\") AS \"s\" ON \"s\".\"UserId\" = \"Users\".\"Id\"\nRIGHT JOIN (SELECT \"UserId\" FROM \"Stats\") AS \"s\" ON \"s\".\"UserId\" <> \"Users\".\"Id\"",
		},
		{
			name: "subquery bindings land before outer conditions",
			build: func(q *sqlk.Query) *sqlk.Query {
				stats := sqlk.NewQuery().From("Stats").WhereEq("Kind", "daily").Select("UserId", "Score").As("s")
				return q.From("Users").
					JoinSub(stats, func(j *sqlk.Join) *sqlk.Join { return j.On("s.UserId", "=", "Users.Id") }).
					WhereEq("Users.Active", true)
			},
			sql:  "SELECT * FROM \"Users\" \nINNER JOIN (SELECT \"UserId\", \"Score\" FROM \"Stats\" WHERE \"Kind\" = ?) AS \"s\" ON \"s\".\"UserId\" = \"Users\".\"Id\" WHERE \"Users\".\"Active\" = ?",
			args: []any{"daily", true},
		},
		{
			name: "subquery target is cloned at embed time",
			build: func(q *sqlk.Query) *sqlk.Query {
				stats := sqlk.NewQuery().From("Stats").Select("UserId").As("s")
				q.From("Users").JoinSub(stats, func(j *sqlk.Join) *sqlk.Join {
					return j.On("s.UserId", "=", "Users.Id")
				})
				stats.WhereEq("Kind", "daily") // later mutation must not affect the embedded join
				return q
			},
			sql: "SELECT * FROM \"Users\" \nINNER JOIN (SELECT \"UserId\" FROM \"Stats\") AS \"s\" ON \"s\".\"UserId\" = \"Users\".\"Id\"",
		},
		{
			name: "joins compile inside subqueries",
			build: func(q *sqlk.Query) *sqlk.Query {
				inner := sqlk.NewQuery().From("users").
					JoinEq("countries", "countries.id", "users.country_id").
					Select("users.id")
				return q.FromSub(inner, "uc")
			},
			sql: "SELECT * FROM (SELECT \"users\".\"id\" FROM \"users\" \nINNER JOIN \"countries\" ON \"countries\".\"id\" = \"users\".\"country_id\") AS \"uc\"",
		},
	})
}

func TestCompileJoinOnCallback(t *testing.T) {
	joinEq := func(first, second string) func(*sqlk.Join) *sqlk.Join {
		return func(j *sqlk.Join) *sqlk.Join { return j.On(first, "=", second) }
	}

	runCompileCases(t, New(), []compileCase{
		{
			name: "callback with the On family builds the predicate",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").JoinOn("countries", func(j *sqlk.Join) *sqlk.Join {
					return j.On("countries.id", "=", "users.country_id").
						OnNot("countries.blocked", "=", "users.flag").
						OrOn("countries.fallback", "=", "users.other")
				})
			},
			sql: "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"countries\".\"id\" = \"users\".\"country_id\" AND NOT \"countries\".\"blocked\" = \"users\".\"flag\" OR \"countries\".\"fallback\" = \"users\".\"other\"",
		},
		{
			name: "left and right variants take the callback form too",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").
					LeftJoinOn("profiles", func(j *sqlk.Join) *sqlk.Join { return j.On("profiles.user_id", "=", "users.id") }).
					RightJoinOn("meta", func(j *sqlk.Join) *sqlk.Join { return j.On("meta.id", "=", "users.meta_id") })
			},
			sql: "SELECT * FROM \"users\" \nLEFT JOIN \"profiles\" ON \"profiles\".\"user_id\" = \"users\".\"id\"\nRIGHT JOIN \"meta\" ON \"meta\".\"id\" = \"users\".\"meta_id\"",
		},
		{
			name: "or on not variant",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").JoinOn("countries", func(j *sqlk.Join) *sqlk.Join {
					return j.On("a", "=", "b").OrOnNot("c", "=", "d")
				})
			},
			sql: "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"a\" = \"b\" OR NOT \"c\" = \"d\"",
		},
		{
			name: "callback carries the full Where capability on the ON scope",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").JoinOn("countries", func(j *sqlk.Join) *sqlk.Join {
					return j.On("countries.id", "=", "users.country_id").
						WhereEq("countries.active", true).
						WhereStarts("countries.name", "en").
						WhereBetween("countries.size", 1, 9)
				})
			},
			sql:  "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"countries\".\"id\" = \"users\".\"country_id\" AND \"countries\".\"active\" = ? AND LOWER(\"countries\".\"name\") like ? AND \"countries\".\"size\" BETWEEN ? AND ?",
			args: []any{true, "en%", 1, 9},
		},
		{
			name: "nested groups inside the ON scope",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").JoinOn("countries", func(j *sqlk.Join) *sqlk.Join {
					return j.On("a", "=", "b").WhereGroup(func(n *sqlk.Query) *sqlk.Query {
						return n.WhereNull("c").OrWhereEq("d", 1)
					})
				})
			},
			sql:  "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"a\" = \"b\" AND (\"c\" IS NULL OR \"d\" = ?)",
			args: []any{1},
		},
		{
			name: "exists and in-subqueries work inside the ON scope",
			build: func(q *sqlk.Query) *sqlk.Query {
				refs := sqlk.NewQuery().From("Refs").WhereColumns("Refs.UId", "=", "users.id")
				return q.From("users").JoinOn("countries", func(j *sqlk.Join) *sqlk.Join {
					return j.On("a", "=", "b").
						WhereInSub("users.tier", sqlk.NewQuery().From("Tiers").Select("Id")).
						WhereExists(refs)
				})
			},
			sql: "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"a\" = \"b\" AND \"users\".\"tier\" IN (SELECT \"Id\" FROM \"Tiers\") AND EXISTS (SELECT 1 FROM \"Refs\" WHERE \"Refs\".\"UId\" = \"users\".\"id\")",
		},
		{
			name: "raw conditions inside the ON scope",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").JoinOn("countries", func(j *sqlk.Join) *sqlk.Join {
					return j.On("a", "=", "b").WhereRaw("[a].[x] > ? or [a].[y] < ?", 1, 2)
				})
			},
			sql:  "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"a\" = \"b\" AND \"a\".\"x\" > ? or \"a\".\"y\" < ?",
			args: []any{1, 2},
		},
		{
			name: "callback producing no conditions omits the ON clause",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").JoinOn("countries", func(j *sqlk.Join) *sqlk.Join { return j })
			},
			sql: "SELECT * FROM \"users\" \nINNER JOIN \"countries\"",
		},
		{
			name: "on conditions precede outer wheres in the argument order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").
					JoinOn("countries", joinEq("a", "b")).
					WhereEq("users.id", 7)
			},
			sql:  "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"a\" = \"b\" WHERE \"users\".\"id\" = ?",
			args: []any{7},
		},
		{
			name: "joins are kept in aggregate form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").JoinEq("countries", "countries.id", "users.country_id").Count()
			},
			sql: "SELECT COUNT(*) AS \"count\" FROM \"users\" \nINNER JOIN \"countries\" ON \"countries\".\"id\" = \"users\".\"country_id\"",
		},
	})
}

func TestJoinValidation(t *testing.T) {
	comp := New()

	t.Run("operator outside whitelist inside the ON scope", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("users").JoinOn("countries", func(j *sqlk.Join) *sqlk.Join {
			return j.On("a", "startswith", "b")
		}))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
		opErr, ok := errors.AsType[*OperatorError](err)
		if !ok {
			t.Fatalf("Compile(...) error = %v, want an *OperatorError", err)
		}
		if opErr.Column != "a" || opErr.Operator != "startswith" {
			t.Errorf("OperatorError = {Column: %q, Operator: %q}, want {a, startswith}", opErr.Column, opErr.Operator)
		}
	})

	t.Run("operator checks cover the simple shorthand form", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("users").Join("countries", "a", "startswith", "b"))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
	})

	t.Run("missing from target inside a join subquery", func(t *testing.T) {
		stats := sqlk.NewQuery().Select("UserId").As("s")
		_, err := comp.Compile(sqlk.NewQuery().From("Users").JoinSub(stats, func(j *sqlk.Join) *sqlk.Join {
			return j.On("s.UserId", "=", "Users.Id")
		}))
		if !errors.Is(err, ErrNoFromTarget) {
			t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
		}
	})

	t.Run("operator problems inside a join subquery are surfaced", func(t *testing.T) {
		stats := sqlk.NewQuery().From("Stats").Where("Kind", "startswith", "x").Select("UserId").As("s")
		_, err := comp.Compile(sqlk.NewQuery().From("Users").JoinSub(stats, func(j *sqlk.Join) *sqlk.Join {
			return j.On("s.UserId", "=", "Users.Id")
		}))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
	})

	t.Run("escape character validation descends into the ON scope", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("users").JoinOn("countries", func(j *sqlk.Join) *sqlk.Join {
			return j.WhereLike("a", "x", sqlk.EscapeLike("ab"))
		}))
		if !errors.Is(err, ErrInvalidEscapeCharacter) {
			t.Fatalf("Compile(...) error = %v, want ErrInvalidEscapeCharacter", err)
		}
	})

	t.Run("the join scope itself needs no from target", func(t *testing.T) {
		res, err := comp.Compile(sqlk.NewQuery().From("users").JoinOn("countries", func(j *sqlk.Join) *sqlk.Join {
			return j.On("a", "=", "b")
		}))
		if err != nil {
			t.Fatalf("Compile(...) error = %v, want nil", err)
		}
		if want := "SELECT * FROM \"users\" \nINNER JOIN \"countries\" ON \"a\" = \"b\""; res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
	})
}

func TestJoinSurviveClone(t *testing.T) {
	comp := New()
	compile := func(t *testing.T, q *sqlk.Query) Result {
		t.Helper()
		return mustCompile(t, comp, q)
	}

	t.Run("joins with subquery targets are cloned deeply", func(t *testing.T) {
		base := sqlk.NewQuery().From("Users").
			JoinEq("Countries", "Countries.Id", "Users.CountryId").
			JoinSub(sqlk.NewQuery().From("Stats").Select("UserId").As("s"), func(j *sqlk.Join) *sqlk.Join {
				return j.On("s.UserId", "=", "Users.Id").WhereEq("s.Score", 100)
			})

		variant := base.Clone().WhereEq("Users.Active", true)
		base.LeftJoinEq("Profiles", "Profiles.UserId", "Users.Id")

		wantVariant := "SELECT * FROM \"Users\" \nINNER JOIN \"Countries\" ON \"Countries\".\"Id\" = \"Users\".\"CountryId\"\nINNER JOIN (SELECT \"UserId\" FROM \"Stats\") AS \"s\" ON \"s\".\"UserId\" = \"Users\".\"Id\" AND \"s\".\"Score\" = ? WHERE \"Users\".\"Active\" = ?"
		if got := compile(t, variant); got.SQL != wantVariant {
			t.Errorf("variant SQL = %q, want %q", got.SQL, wantVariant)
		} else if !reflect.DeepEqual(got.Args, []any{100, true}) {
			t.Errorf("variant Args = %#v, want [100 true]", got.Args)
		}

		wantBase := "SELECT * FROM \"Users\" \nINNER JOIN \"Countries\" ON \"Countries\".\"Id\" = \"Users\".\"CountryId\"\nINNER JOIN (SELECT \"UserId\" FROM \"Stats\") AS \"s\" ON \"s\".\"UserId\" = \"Users\".\"Id\" AND \"s\".\"Score\" = ?\nLEFT JOIN \"Profiles\" ON \"Profiles\".\"UserId\" = \"Users\".\"Id\""
		if got := compile(t, base); got.SQL != wantBase {
			t.Errorf("base SQL = %q, want %q", got.SQL, wantBase)
		} else if !reflect.DeepEqual(got.Args, []any{100}) {
			t.Errorf("base Args = %#v, want [100]", got.Args)
		}
	})
}

func TestCompileCTEForms(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "query form hoists the CTE in front of the body",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").With("a", sqlk.NewQuery().From("B").Where("Id", ">", 5))
			},
			sql:  "WITH \"a\" AS (SELECT * FROM \"B\" WHERE \"Id\" > ?)\nSELECT * FROM \"A\"",
			args: []any{5},
		},
		{
			// Multiple CTEs are comma separated in declaration order.
			name: "multiple ctes keep declaration order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").
					With("A", sqlk.NewQuery().From("A")).
					With("B", sqlk.NewQuery().From("B")).
					With("C", sqlk.NewQuery().From("C"))
			},
			sql: "WITH \"A\" AS (SELECT * FROM \"A\"),\n\"B\" AS (SELECT * FROM \"B\"),\n\"C\" AS (SELECT * FROM \"C\")\nSELECT * FROM \"A\"",
		},
		{
			name: "callback form builds the CTE body inline",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Races").WithFunc("range", func(sq *sqlk.Query) *sqlk.Query {
					return sq.From("Sequence").Select("Number").Where("Number", "<", 78)
				})
			},
			sql:  "WITH \"range\" AS (SELECT \"Number\" FROM \"Sequence\" WHERE \"Number\" < ?)\nSELECT * FROM \"Races\"",
			args: []any{78},
		},
		{
			name: "callback returning nil keeps the pre-call query",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").WithFunc("a", func(sq *sqlk.Query) *sqlk.Query {
					sq.From("B").WhereEq("Kind", "daily")
					return nil
				})
			},
			sql:  "WITH \"a\" AS (SELECT * FROM \"B\" WHERE \"Kind\" = ?)\nSELECT * FROM \"A\"",
			args: []any{"daily"},
		},
		{
			name: "raw form wraps identifier markers per dialect",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").WithRaw("prod", "SELECT {c} AS {name} FROM {Products} WHERE {Price} > ?", 10)
			},
			sql:  "WITH \"prod\" AS (SELECT \"c\" AS \"name\" FROM \"Products\" WHERE \"Price\" > ?)\nSELECT * FROM \"A\"",
			args: []any{10},
		},
		{
			// The base compiler has no dummy single-row table.
			name: "ad-hoc table with a single row",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").WithTable("rows", []string{"a"}, []any{1})
			},
			sql:  "WITH \"rows\" AS (SELECT ? AS \"a\")\nSELECT * FROM \"rows\"",
			args: []any{1},
		},
		{
			// Multiple rows are joined with UNION ALL.
			name: "ad-hoc table with multiple rows unions them",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").
					WithTable("rows", []string{"a", "b", "c"}, []any{1, 2, 3}, []any{4, 5, 6})
			},
			sql:  "WITH \"rows\" AS (SELECT ? AS \"a\", ? AS \"b\", ? AS \"c\" UNION ALL SELECT ? AS \"a\", ? AS \"b\", ? AS \"c\")\nSELECT * FROM \"rows\"",
			args: []any{1, 2, 3, 4, 5, 6},
		},
		{
			// All CTE bindings precede body bindings, in declaration order.
			name: "cte bindings land before body bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").
					WithFunc("othercte", func(sq *sqlk.Query) *sqlk.Query {
						return sq.From("othertable").WhereEq("othertable.status", "A")
					}).
					WhereEq("rows.foo", "bar").
					WithTable("rows", []string{"a", "b", "c"}, []any{1, 2, 3}, []any{4, 5, 6}).
					WhereEq("rows.baz", "buzz")
			},
			sql:  "WITH \"othercte\" AS (SELECT * FROM \"othertable\" WHERE \"othertable\".\"status\" = ?),\n\"rows\" AS (SELECT ? AS \"a\", ? AS \"b\", ? AS \"c\" UNION ALL SELECT ? AS \"a\", ? AS \"b\", ? AS \"c\")\nSELECT * FROM \"rows\" WHERE \"rows\".\"foo\" = ? AND \"rows\".\"baz\" = ?",
			args: []any{"A", 1, 2, 3, 4, 5, 6, "bar", "buzz"},
		},
		{
			name: "comment stays in front of the WITH clause",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Comment("trace").With("a", sqlk.NewQuery().From("B"))
			},
			sql: "/* trace */ WITH \"a\" AS (SELECT * FROM \"B\")\nSELECT * FROM \"A\"",
		},
	})
}

func TestCompileCTECollection(t *testing.T) {
	// CTEs defined inside other CTEs are collected and hoisted.
	t.Run("cascaded cte dependencies are hoisted in front", func(t *testing.T) {
		cte1 := sqlk.NewQuery().From("Table1").Select("Column1", "Column2").WhereEq("Column2", 1)
		cte2 := sqlk.NewQuery().From("Table2").With("cte1", cte1).Select("Column3", "Column4")
		cte2.JoinEq("cte1", "Column1", "Column3")
		cte2.WhereEq("Column4", 2)

		q := sqlk.NewQuery().With("cte2", cte2).From("cte2").WhereEq("Column3", 5)

		want := "WITH \"cte1\" AS (SELECT \"Column1\", \"Column2\" FROM \"Table1\" WHERE \"Column2\" = ?),\n" +
			"\"cte2\" AS (SELECT \"Column3\", \"Column4\" FROM \"Table2\" \n" +
			"INNER JOIN \"cte1\" ON \"Column1\" = \"Column3\" WHERE \"Column4\" = ?)\n" +
			"SELECT * FROM \"cte2\" WHERE \"Column3\" = ?"
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{1, 2, 5}) {
			t.Errorf("Args = %#v, want [1 2 5]", got.Args)
		}
	})

	// A CTE referenced from several levels is emitted once.
	t.Run("multi-referenced cte is emitted once", func(t *testing.T) {
		cte1 := sqlk.NewQuery().From("Table1").Select("Column1", "Column2").WhereEq("Column2", 1)
		cte2 := sqlk.NewQuery().From("Table2").With("cte1", cte1).Select("Column3", "Column4")
		cte2.JoinEq("cte1", "Column1", "Column3")
		cte2.WhereEq("Column4", 2)
		cte3 := sqlk.NewQuery().From("Table3").With("cte1", cte1).Select("Column3_3", "Column3_4")
		cte3.JoinEq("cte1", "Column1", "Column3_3")
		cte3.WhereEq("Column3_4", 33)

		q := sqlk.NewQuery().With("cte2", cte2).With("cte3", cte3).From("cte2").WhereEq("Column3", 5)

		want := "WITH \"cte1\" AS (SELECT \"Column1\", \"Column2\" FROM \"Table1\" WHERE \"Column2\" = ?),\n" +
			"\"cte2\" AS (SELECT \"Column3\", \"Column4\" FROM \"Table2\" \n" +
			"INNER JOIN \"cte1\" ON \"Column1\" = \"Column3\" WHERE \"Column4\" = ?),\n" +
			"\"cte3\" AS (SELECT \"Column3_3\", \"Column3_4\" FROM \"Table3\" \n" +
			"INNER JOIN \"cte1\" ON \"Column1\" = \"Column3_3\" WHERE \"Column3_4\" = ?)\n" +
			"SELECT * FROM \"cte2\" WHERE \"Column3\" = ?"
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{1, 2, 33, 5}) {
			t.Errorf("Args = %#v, want [1 2 33 5]", got.Args)
		}
	})

	t.Run("cte defined inside a from-subquery is collected", func(t *testing.T) {
		inner := sqlk.NewQuery().From("Seq").WithFunc("range", func(sq *sqlk.Query) *sqlk.Query {
			return sq.From("seqtbl").Select("Id").Where("Id", "<", 33)
		}).Select("Id")
		q := sqlk.NewQuery().FromSub(inner, "t")

		want := "WITH \"range\" AS (SELECT \"Id\" FROM \"seqtbl\" WHERE \"Id\" < ?)\n" +
			"SELECT * FROM (SELECT \"Id\" FROM \"Seq\") AS \"t\""
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{33}) {
			t.Errorf("Args = %#v, want [33]", got.Args)
		}
	})

	t.Run("cte defined inside a condition subquery is collected", func(t *testing.T) {
		authors := sqlk.NewQuery().From("Users").Select("Name").WhereEq("Status", "Available").
			WithFunc("range", func(sq *sqlk.Query) *sqlk.Query {
				return sq.From("seqtbl").Select("Id").Where("Id", "<", 33)
			})
		q := sqlk.NewQuery().From("Races").WhereInSub("RaceAuthor", authors).Where("Id", ">", 55)

		want := "WITH \"range\" AS (SELECT \"Id\" FROM \"seqtbl\" WHERE \"Id\" < ?)\n" +
			"SELECT * FROM \"Races\" WHERE \"RaceAuthor\" IN (SELECT \"Name\" FROM \"Users\" WHERE \"Status\" = ?) AND \"Id\" > ?"
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{33, "Available", 55}) {
			t.Errorf("Args = %#v, want [33 Available 55]", got.Args)
		}
	})

	t.Run("duplicate alias across levels is emitted once", func(t *testing.T) {
		inner := sqlk.NewQuery().From("a").With("a", sqlk.NewQuery().From("Other"))
		q := sqlk.NewQuery().With("a", sqlk.NewQuery().From("Log")).FromSub(inner, "t")

		want := "WITH \"a\" AS (SELECT * FROM \"Log\")\nSELECT * FROM (SELECT * FROM \"a\") AS \"t\""
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})

	t.Run("ctes survive the aggregate wrap transform", func(t *testing.T) {
		q := sqlk.NewQuery().From("A").
			With("a", sqlk.NewQuery().From("B").WhereEq("x", 1)).
			Distinct().
			Count("Col")

		want := "WITH \"a\" AS (SELECT * FROM \"B\" WHERE \"x\" = ?)\n" +
			"SELECT COUNT(*) AS \"count\" FROM (SELECT DISTINCT \"Col\" FROM \"A\") AS \"countQuery\""
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{1}) {
			t.Errorf("Args = %#v, want [1]", got.Args)
		}
	})

	t.Run("ctes survive Clone", func(t *testing.T) {
		base := sqlk.NewQuery().From("A").With("a", sqlk.NewQuery().From("B").WhereEq("x", 1))
		variant := base.Clone().WhereEq("y", 2)
		base.With("extra", sqlk.NewQuery().From("C"))

		want := "WITH \"a\" AS (SELECT * FROM \"B\" WHERE \"x\" = ?)\nSELECT * FROM \"A\" WHERE \"y\" = ?"
		got := mustCompile(t, New(), variant)
		if got.SQL != want {
			t.Errorf("variant SQL = %q, want %q", got.SQL, want)
		}
	})
}

func TestCTEValidation(t *testing.T) {
	comp := New()

	t.Run("missing alias is rejected", func(t *testing.T) {
		q := sqlk.NewQuery().From("A").With("", sqlk.NewQuery().From("B"))
		_, err := comp.Compile(q)
		if !errors.Is(err, ErrCTEMissingAlias) {
			t.Fatalf("Compile(...) error = %v, want ErrCTEMissingAlias", err)
		}
	})

	t.Run("whitespace-only alias is rejected", func(t *testing.T) {
		q := sqlk.NewQuery().From("A").WithRaw(" ", "SELECT 1")
		_, err := comp.Compile(q)
		if !errors.Is(err, ErrCTEMissingAlias) {
			t.Fatalf("Compile(...) error = %v, want ErrCTEMissingAlias", err)
		}
	})

	t.Run("ad-hoc table shape problems are rejected", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			build   func(*sqlk.Query) *sqlk.Query
			columns int
			rows    int
			values  int
		}{
			{
				name:    "no columns",
				build:   func(q *sqlk.Query) *sqlk.Query { return q.From("A").WithTable("rows", nil, []any{1}) },
				columns: 0,
				rows:    1,
			},
			{
				name:    "no rows",
				build:   func(q *sqlk.Query) *sqlk.Query { return q.From("A").WithTable("rows", []string{"a"}) },
				columns: 1,
			},
			{
				name: "row does not match column count",
				build: func(q *sqlk.Query) *sqlk.Query {
					return q.From("A").WithTable("rows", []string{"a", "b"}, []any{1, 2, 3})
				},
				columns: 2,
				rows:    1,
				values:  3,
			},
			{
				// Rows are validated individually: ragged rows are rejected
				// even when the total divides, or values would pair across
				// rows incorrectly.
				name: "ragged rows are rejected even when the total divides",
				build: func(q *sqlk.Query) *sqlk.Query {
					return q.From("A").WithTable("rows", []string{"a", "b"}, []any{1}, []any{2, 3, 4})
				},
				columns: 2,
				rows:    2,
				values:  1,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := comp.Compile(tt.build(sqlk.NewQuery()))
				if !errors.Is(err, ErrInvalidCTETable) {
					t.Fatalf("Compile(...) error = %v, want ErrInvalidCTETable", err)
				}
				tableErr, ok := errors.AsType[*CTETableError](err)
				if !ok {
					t.Fatalf("Compile(...) error = %v, want a *CTETableError", err)
				}
				if tableErr.Alias != "rows" || tableErr.Columns != tt.columns || tableErr.Rows != tt.rows || tableErr.Values != tt.values {
					t.Errorf("CTETableError = {Alias: %q, Columns: %d, Rows: %d, Values: %d}, want {rows, %d, %d, %d}",
						tableErr.Alias, tableErr.Columns, tableErr.Rows, tableErr.Values, tt.columns, tt.rows, tt.values)
				}
			})
		}
	})

	t.Run("cte problems aggregate with other compile problems", func(t *testing.T) {
		q := sqlk.NewQuery().With("", sqlk.NewQuery().From("B")).Select("Id")
		_, err := comp.Compile(q)
		if !errors.Is(err, ErrCTEMissingAlias) {
			t.Errorf("errors.Is(err, ErrCTEMissingAlias) = false, want true (%v)", err)
		}
		if !errors.Is(err, ErrNoFromTarget) {
			t.Errorf("errors.Is(err, ErrNoFromTarget) = false, want true (%v)", err)
		}
	})

	t.Run("cte body is validated recursively", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("A").WithFunc("a", func(sq *sqlk.Query) *sqlk.Query {
			return sq.From("B").Where("X", "~~", 1)
		}))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}

		_, err = comp.Compile(sqlk.NewQuery().From("A").WithFunc("a", func(sq *sqlk.Query) *sqlk.Query {
			return sq.Select("x")
		}))
		if !errors.Is(err, ErrNoFromTarget) {
			t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
		}
	})
}

func TestCompileCombine(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			// Members concatenate as bare SELECTs with no parentheses.
			name:  "union",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Phones").Union(sqlk.NewQuery().From("Laptops")) },
			sql:   `SELECT * FROM "Phones" UNION SELECT * FROM "Laptops"`,
		},
		{
			name:  "union all",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Phones").UnionAll(sqlk.NewQuery().From("Laptops")) },
			sql:   `SELECT * FROM "Phones" UNION ALL SELECT * FROM "Laptops"`,
		},
		{
			name:  "except",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Phones").Except(sqlk.NewQuery().From("Tablets")) },
			sql:   `SELECT * FROM "Phones" EXCEPT SELECT * FROM "Tablets"`,
		},
		{
			name:  "except all",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Phones").ExceptAll(sqlk.NewQuery().From("Tablets")) },
			sql:   `SELECT * FROM "Phones" EXCEPT ALL SELECT * FROM "Tablets"`,
		},
		{
			name:  "intersect",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Phones").Intersect(sqlk.NewQuery().From("Tablets")) },
			sql:   `SELECT * FROM "Phones" INTERSECT SELECT * FROM "Tablets"`,
		},
		{
			name:  "intersect all",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Phones").IntersectAll(sqlk.NewQuery().From("Tablets")) },
			sql:   `SELECT * FROM "Phones" INTERSECT ALL SELECT * FROM "Tablets"`,
		},
		{
			// Combine is the shared base of the combine verb family.
			name: "combine verb with operation and all flag",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").Combine("union", true, sqlk.NewQuery().From("Laptops"))
			},
			sql: `SELECT * FROM "Phones" UNION ALL SELECT * FROM "Laptops"`,
		},
		{
			// Member condition arguments follow the main query's.
			name: "member bindings follow main query bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").Where("Price", "<", 3000).
					Union(sqlk.NewQuery().From("Laptops").WhereEq("Type", "A"))
			},
			sql:  `SELECT * FROM "Phones" WHERE "Price" < ? UNION SELECT * FROM "Laptops" WHERE "Type" = ?`,
			args: []any{3000, "A"},
		},
		{
			// Multiple combines accumulate in call order.
			name: "multiple combines keep call order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").
					Union(sqlk.NewQuery().From("Laptops")).
					Union(sqlk.NewQuery().From("Tablets"))
			},
			sql: `SELECT * FROM "Phones" UNION SELECT * FROM "Laptops" UNION SELECT * FROM "Tablets"`,
		},
		{
			// The callback form builds the member inline.
			name: "callback members",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").Where("Price", "<", 3000).
					UnionFunc(func(m *sqlk.Query) *sqlk.Query { return m.From("Laptops") }).
					UnionAllFunc(func(m *sqlk.Query) *sqlk.Query { return m.From("Tablets") })
			},
			sql:  `SELECT * FROM "Phones" WHERE "Price" < ? UNION SELECT * FROM "Laptops" UNION ALL SELECT * FROM "Tablets"`,
			args: []any{3000},
		},
		{
			// A nil-returning callback keeps the scope query, as with
			// JoinOn and WithFunc.
			name: "callback returning nil keeps the scope query",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").UnionFunc(func(m *sqlk.Query) *sqlk.Query {
					m.From("Laptops")
					return nil
				})
			},
			sql: `SELECT * FROM "Phones" UNION SELECT * FROM "Laptops"`,
		},
		{
			// Identifier markers in a raw member are quoted per dialect;
			// bindings follow placeholder order.
			name: "raw member with bindings and identifier marks",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").UnionRaw("UNION SELECT * FROM {Laptops} WHERE [Type] = ?", "A")
			},
			sql:  `SELECT * FROM "Phones" UNION SELECT * FROM "Laptops" WHERE "Type" = ?`,
			args: []any{"A"},
		},
		{
			// CombineRaw is the base of the raw forms; the expression
			// carries its own operator prefix.
			name: "combine raw without bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Mobiles").CombineRaw("UNION ALL SELECT * FROM Devices")
			},
			sql: `SELECT * FROM "Mobiles" UNION ALL SELECT * FROM Devices`,
		},
		{
			// IntersectRaw is to CombineRaw what UnionRaw and ExceptRaw are.
			name: "intersect raw with identifier marks",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Mobiles").IntersectRaw("INTERSECT SELECT * FROM {Watches}")
			},
			sql: `SELECT * FROM "Mobiles" INTERSECT SELECT * FROM "Watches"`,
		},
		{
			// Query members and raw members mix in call order.
			name: "query members mix with raw members",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").Where("Price", "<", 3000).
					Union(sqlk.NewQuery().From("Laptops")).
					ExceptRaw("EXCEPT SELECT * FROM {Archived} WHERE [Year] < ?", 2020)
			},
			sql:  `SELECT * FROM "Phones" WHERE "Price" < ? UNION SELECT * FROM "Laptops" EXCEPT SELECT * FROM "Archived" WHERE "Year" < ?`,
			args: []any{3000, 2020},
		},
		{
			// A member's own projection and order compile with the member.
			name: "member keeps its own projection and order",
			build: func(q *sqlk.Query) *sqlk.Query {
				member := sqlk.NewQuery().From("Laptops").Select("Brand").OrderBy("Price")
				return q.From("Phones").Union(member)
			},
			sql: `SELECT * FROM "Phones" UNION SELECT "Brand" FROM "Laptops" ORDER BY "Price"`,
		},
		{
			// Nested combines flatten into a single sequence.
			name: "nested combine member",
			build: func(q *sqlk.Query) *sqlk.Query {
				member := sqlk.NewQuery().From("B").Union(sqlk.NewQuery().From("C"))
				return q.From("A").Union(member)
			},
			sql: `SELECT * FROM "A" UNION SELECT * FROM "B" UNION SELECT * FROM "C"`,
		},
		{
			// A single-column aggregate compiles as is and keeps its combines.
			name: "aggregate main query keeps its combines",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").Union(sqlk.NewQuery().From("Laptops")).Count()
			},
			sql: `SELECT COUNT(*) AS "count" FROM "Phones" UNION SELECT * FROM "Laptops"`,
		},
		{
			// The main query's pagination section precedes the combines.
			name: "main pagination precedes the combine section",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").Limit(5).Union(sqlk.NewQuery().From("Laptops"))
			},
			sql:  `SELECT * FROM "Phones" LIMIT ? UNION SELECT * FROM "Laptops"`,
			args: []any{5},
		},
	})
}

func TestCompileCombinePaginationAndCTE(t *testing.T) {
	// A paginated member compiles with its own pagination inline.
	t.Run("paginated member keeps its own limit offset", func(t *testing.T) {
		tablets := sqlk.NewQuery().From("Tablets").Where("Price", ">", 2000).ForPage(2)
		q := sqlk.NewQuery().From("Phones").Where("Price", "<", 3000).
			Union(sqlk.NewQuery().From("Laptops").Where("Price", ">", 1000)).
			UnionAll(tablets)

		want := `SELECT * FROM "Phones" WHERE "Price" < ? UNION SELECT * FROM "Laptops" WHERE "Price" > ? ` +
			`UNION ALL SELECT * FROM "Tablets" WHERE "Price" > ? LIMIT ? OFFSET ?`
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{3000, 1000, 2000, 15, int64(15)}) {
			t.Errorf("Args = %#v, want [3000 1000 2000 15 15]", got.Args)
		}
	})

	// CTEs defined in combine members are collected across the whole
	// tree and hoisted to the outer WITH, with their bindings first.
	t.Run("member cte is hoisted to the outer with clause", func(t *testing.T) {
		member := sqlk.NewQuery().
			With("cheap", sqlk.NewQuery().From("Deals").Where("Price", "<", 800)).
			From("cheap").WhereEq("Kind", "gaming")
		q := sqlk.NewQuery().From("Phones").WhereEq("Brand", "y").Union(member)

		want := "WITH \"cheap\" AS (SELECT * FROM \"Deals\" WHERE \"Price\" < ?)\n" +
			"SELECT * FROM \"Phones\" WHERE \"Brand\" = ? UNION SELECT * FROM \"cheap\" WHERE \"Kind\" = ?"
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{800, "y", "gaming"}) {
			t.Errorf("Args = %#v, want [800 y gaming]", got.Args)
		}
	})

	// A CTE alias shared by the main query and a member is emitted once;
	// the first occurrence wins.
	t.Run("duplicate alias between main query and member is emitted once", func(t *testing.T) {
		member := sqlk.NewQuery().With("c1", sqlk.NewQuery().From("Other")).From("c1")
		q := sqlk.NewQuery().With("c1", sqlk.NewQuery().From("Log")).From("c1").Union(member)

		want := "WITH \"c1\" AS (SELECT * FROM \"Log\")\n" +
			"SELECT * FROM \"c1\" UNION SELECT * FROM \"c1\""
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})
}

func TestCombineValidationAndClone(t *testing.T) {
	comp := New()

	// Members are standalone SELECTs: from-target and operator checks
	// descend into them.
	t.Run("member without from target is rejected", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("A").Union(sqlk.NewQuery().Select("x")))
		if !errors.Is(err, ErrNoFromTarget) {
			t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
		}
	})

	t.Run("member operator problems are rejected", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("A").Union(
			sqlk.NewQuery().From("B").Where("X", "~~", 1)))
		if !errors.Is(err, ErrOperatorNotAllowed) {
			t.Fatalf("Compile(...) error = %v, want ErrOperatorNotAllowed", err)
		}
	})

	// Members of nested combines are validated recursively too.
	t.Run("nested member problems are rejected", func(t *testing.T) {
		inner := sqlk.NewQuery().From("B").Union(sqlk.NewQuery().Select("x"))
		_, err := comp.Compile(sqlk.NewQuery().From("A").Union(inner))
		if !errors.Is(err, ErrNoFromTarget) {
			t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
		}
	})

	// Members are deep-copied at embed time.
	t.Run("member is deep-copied at embed time", func(t *testing.T) {
		member := sqlk.NewQuery().From("Laptops")
		q := sqlk.NewQuery().From("Phones").Union(member)
		member.WhereEq("Type", "A")

		want := `SELECT * FROM "Phones" UNION SELECT * FROM "Laptops"`
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})

	// Clone deep-copies combine clauses.
	t.Run("combines survive Clone", func(t *testing.T) {
		base := sqlk.NewQuery().From("A").Union(sqlk.NewQuery().From("B").WhereEq("x", 1))
		variant := base.Clone().WhereEq("y", 2)
		base.Union(sqlk.NewQuery().From("C"))

		want := `SELECT * FROM "A" WHERE "y" = ? UNION SELECT * FROM "B" WHERE "x" = ?`
		got := mustCompile(t, New(), variant)
		if got.SQL != want {
			t.Errorf("variant SQL = %q, want %q", got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{2, 1}) {
			t.Errorf("variant Args = %#v, want [2 1]", got.Args)
		}
	})
}

func TestCompileInsert(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			// Map keys fold into columns and values in sorted key order,
			// since Go map iteration is unordered.
			name: "key-value form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Insert(sqlk.Record{"Name": "The User", "Age": 18})
			},
			sql:  `INSERT INTO "Table" ("Age", "Name") VALUES (?, ?)`,
			args: []any{18, "The User"},
		},
		{
			// NULL is expressed as a parameter placeholder.
			name: "null value is parameterized",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Books").InsertColumns([]string{"Id", "Author", "ISBN", "Date"},
					[]any{1, "Author 1", "123456", nil})
			},
			sql:  `INSERT INTO "Books" ("Id", "Author", "ISBN", "Date") VALUES (?, ?, ?, ?)`,
			args: []any{1, "Author 1", "123456", nil},
		},
		{
			// The return-id form only sets a flag; the base compiler has
			// no last-id statement and appends nothing.
			name: "return id flag without dialect last id",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").InsertReturnId(sqlk.Record{"Name": "x"})
			},
			sql:  `INSERT INTO "Table" ("Name") VALUES (?)`,
			args: []any{"x"},
		},
		{
			// A raw from target passes through verbatim.
			name: "raw from target",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromRaw("Table.With.Dots").Insert(sqlk.Record{"Name": "The User"})
			},
			sql:  `INSERT INTO Table.With.Dots ("Name") VALUES (?)`,
			args: []any{"The User"},
		},
		{
			// Multiple rows share one column list.
			name: "multi-row values",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("expensive_cars").InsertRows([]string{"name", "brand", "year"},
					[]any{"Chiron", "Bugatti", nil},
					[]any{"Huayra", "Pagani", 2012},
					[]any{"Reventon roadster", "Lamborghini", 2009})
			},
			sql:  `INSERT INTO "expensive_cars" ("name", "brand", "year") VALUES (?, ?, ?), (?, ?, ?), (?, ?, ?)`,
			args: []any{"Chiron", "Bugatti", nil, "Huayra", "Pagani", 2012, "Reventon roadster", "Lamborghini", 2009},
		},
		{
			// insert into select, with the subquery's own pagination inline.
			name: "insert from query",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("expensive_cars").InsertFrom([]string{"name", "model", "year"},
					sqlk.NewQuery().From("cars").Where("price", ">", 100).ForPage(2, 10))
			},
			sql:  `INSERT INTO "expensive_cars" ("name", "model", "year") SELECT * FROM "cars" WHERE "price" > ? LIMIT ? OFFSET ?`,
			args: []any{100, 10, int64(10)},
		},
		{
			// An empty column list produces no column section.
			name: "insert from query without columns",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Logs").InsertFrom(nil, sqlk.NewQuery().From("Events"))
			},
			sql: `INSERT INTO "Logs" SELECT * FROM "Events"`,
		},
		{
			// Repeated calls keep the last one.
			name: "repeated insert replaces",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Insert(sqlk.Record{"A": 1}).Insert(sqlk.Record{"B": 2})
			},
			sql:  `INSERT INTO "Table" ("B") VALUES (?)`,
			args: []any{2},
		},
		{
			// The method switches; the stale update clauses are not read.
			name: "insert after update switches method",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Update(sqlk.Record{"A": 1}).Insert(sqlk.Record{"B": 2})
			},
			sql:  `INSERT INTO "Table" ("B") VALUES (?)`,
			args: []any{2},
		},
		{
			// Query clauses such as WHERE do not participate in INSERT.
			name: "where clauses do not leak into insert",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereEq("Id", 1).Insert(sqlk.Record{"A": 1})
			},
			sql:  `INSERT INTO "Table" ("A") VALUES (?)`,
			args: []any{1},
		},
	})
}

func TestCompileInsertCTEAndClone(t *testing.T) {
	// CTE hoisting applies to write verbs too; CTE bindings precede the
	// write values.
	t.Run("cte prefixes insert from query", func(t *testing.T) {
		q := sqlk.NewQuery().From("expensive_cars").
			With("old_cards", sqlk.NewQuery().From("all_cars").Where("year", "<", 2000)).
			InsertFrom([]string{"name", "model", "year"},
				sqlk.NewQuery().From("old_cars").Where("price", ">", 100))

		want := "WITH \"old_cards\" AS (SELECT * FROM \"all_cars\" WHERE \"year\" < ?)\n" +
			"INSERT INTO \"expensive_cars\" (\"name\", \"model\", \"year\") SELECT * FROM \"old_cars\" WHERE \"price\" > ?"
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{2000, 100}) {
			t.Errorf("Args = %#v, want [2000 100]", got.Args)
		}
	})

	// The insert-from subquery is deep-copied at embed time.
	t.Run("insert-from member is deep-copied at embed time", func(t *testing.T) {
		sub := sqlk.NewQuery().From("Source")
		q := sqlk.NewQuery().From("Target").InsertFrom([]string{"a"}, sub)
		sub.WhereEq("x", 1)

		want := `INSERT INTO "Target" ("a") SELECT * FROM "Source"`
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})

	// Key-value pairs fold at call time; later map edits do not leak in.
	t.Run("insert map is snapshotted at call time", func(t *testing.T) {
		data := sqlk.Record{"A": 1}
		q := sqlk.NewQuery().From("Table").Insert(data)
		data["B"] = 2

		want := `INSERT INTO "Table" ("A") VALUES (?)`
		got := mustCompile(t, New(), q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
	})

	// Clone deep-copies write clauses.
	t.Run("insert clauses survive Clone", func(t *testing.T) {
		base := sqlk.NewQuery().From("Table").Insert(sqlk.Record{"A": 1})
		variant := base.Clone()
		base.Insert(sqlk.Record{"B": 2})

		want := `INSERT INTO "Table" ("A") VALUES (?)`
		got := mustCompile(t, New(), variant)
		if got.SQL != want {
			t.Errorf("variant SQL = %q, want %q", got.SQL, want)
		}
	})
}

func TestCompileUpdate(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			// Map keys fold into assignments in sorted key order.
			name: "key-value form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Update(sqlk.Record{"Name": "The User", "Age": 18})
			},
			sql:  `UPDATE "Table" SET "Age" = ?, "Name" = ?`,
			args: []any{18, "The User"},
		},
		{
			// NULL values are parameterized like any other value.
			name: "columns and values with where",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Books").WhereEq("Id", 1).UpdateColumns(
					[]string{"Author", "Date", "Version"}, []any{"Author 1", nil, nil})
			},
			sql:  `UPDATE "Books" SET "Author" = ?, "Date" = ?, "Version" = ? WHERE "Id" = ?`,
			args: []any{"Author 1", nil, nil, 1},
		},
		{
			name: "raw from target",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromRaw("Table.With.Dots").Update(sqlk.Record{"Name": "The User"})
			},
			sql:  `UPDATE Table.With.Dots SET "Name" = ?`,
			args: []any{"The User"},
		},
		{
			// The default increment is 1.
			name: "increment defaults to one",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Increment("Total")
			},
			sql:  `UPDATE "Table" SET "Total" = "Total" + ?`,
			args: []any{1},
		},
		{
			// Increment with an explicit amount and wheres.
			name: "increment with amount and wheres",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereEq("Name", "A").Increment("Total", 2)
			},
			sql:  `UPDATE "Table" SET "Total" = "Total" + ? WHERE "Name" = ?`,
			args: []any{2, "A"},
		},
		{
			// Decrement compiles to a minus with a positive amount argument.
			name: "decrement with amount and wheres",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").WhereEq("Name", "A").Decrement("Total", 2)
			},
			sql:  `UPDATE "Table" SET "Total" = "Total" - ? WHERE "Name" = ?`,
			args: []any{2, "A"},
		},
		{
			name: "decrement defaults to one",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Decrement("Total")
			},
			sql:  `UPDATE "Table" SET "Total" = "Total" - ?`,
			args: []any{1},
		},
		{
			// Increment replaces any previous update set.
			name: "increment replaces a previous update set",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Update(sqlk.Record{"A": 1}).Increment("Total")
			},
			sql:  `UPDATE "Table" SET "Total" = "Total" + ?`,
			args: []any{1},
		},
		{
			// Update replaces a previous increment set.
			name: "update replaces a previous increment",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Increment("Total").Update(sqlk.Record{"A": 1})
			},
			sql:  `UPDATE "Table" SET "A" = ?`,
			args: []any{1},
		},
		{
			// CTE hoisting applies to UPDATE as well.
			name: "cte prefixes update",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Books").
					With("OldBooks", sqlk.NewQuery().From("Books").Where("Date", "<", 2000)).
					Where("Price", ">", 100).
					Update(sqlk.Record{"Price": 150})
			},
			sql:  "WITH \"OldBooks\" AS (SELECT * FROM \"Books\" WHERE \"Date\" < ?)\nUPDATE \"Books\" SET \"Price\" = ? WHERE \"Price\" > ?",
			args: []any{2000, 150, 100},
		},
	})
}

func TestCompileDelete(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			// A plain delete keeps the base shape.
			name:  "basic delete",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Posts").Delete() },
			sql:   `DELETE FROM "Posts"`,
		},
		{
			name: "delete with where",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").WhereEq("Id", 5).Delete()
			},
			sql:  `DELETE FROM "Posts" WHERE "Id" = ?`,
			args: []any{5},
		},
		{
			// With a join the statement becomes
			// "DELETE target FROM table JOIN"; without an alias the
			// target is the table itself.
			name: "delete with join",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").
					Join("Authors", "Authors.Id", "=", "Posts.AuthorId").
					WhereEq("Authors.Id", 5).
					Delete()
			},
			sql:  `DELETE "Posts" FROM "Posts" ` + "\n" + `INNER JOIN "Authors" ON "Authors"."Id" = "Posts"."AuthorId" WHERE "Authors"."Id" = ?`,
			args: []any{5},
		},
		{
			// The from alias becomes the delete target.
			name: "delete with join and alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts as P").
					Join("Authors", "Authors.Id", "=", "P.AuthorId").
					WhereEq("Authors.Id", 5).
					Delete()
			},
			sql:  `DELETE "P" FROM "Posts" AS "P" ` + "\n" + `INNER JOIN "Authors" ON "Authors"."Id" = "P"."AuthorId" WHERE "Authors"."Id" = ?`,
			args: []any{5},
		},
		{
			// A raw target has no alias branch; the expression itself is
			// the delete target.
			name: "delete with join on raw target",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.FromRaw("Table.With.Dots").
					Join("Authors", "Authors.Id", "=", "Table.AuthorId").
					Delete()
			},
			sql: `DELETE Table.With.Dots FROM Table.With.Dots ` + "\n" + `INNER JOIN "Authors" ON "Authors"."Id" = "Table"."AuthorId"`,
		},
	})

	// The join-delete statement shape is a dialect override point; the
	// compiler still appends the WHERE clause.
	t.Run("join delete form is a dialect override point", func(t *testing.T) {
		comp := New()
		comp.deleteWithJoinForm = func(table, target, joins string) string {
			return "DELETE FROM " + table + " USING " + strings.TrimSpace(joins)
		}
		q := sqlk.NewQuery().From("Posts").
			Join("Authors", "Authors.Id", "=", "Posts.AuthorId").
			WhereEq("Authors.Id", 5).
			Delete()

		want := `DELETE FROM "Posts" USING INNER JOIN "Authors" ON "Authors"."Id" = "Posts"."AuthorId" WHERE "Authors"."Id" = ?`
		got := mustCompile(t, comp, q)
		if got.SQL != want {
			t.Errorf("SQL = %q, want %q", got.SQL, want)
		}
		if !reflect.DeepEqual(got.Args, []any{5}) {
			t.Errorf("Args = %#v, want [5]", got.Args)
		}
	})
}

func TestWriteValidation(t *testing.T) {
	comp := New()

	t.Run("subquery target is rejected", func(t *testing.T) {
		for _, tt := range []struct {
			name  string
			build func(*sqlk.Query) *sqlk.Query
		}{
			{
				name: "insert",
				build: func(q *sqlk.Query) *sqlk.Query {
					return q.FromSub(sqlk.NewQuery().From("Inner"), "t").Insert(sqlk.Record{"A": 1})
				},
			},
			{
				name: "update",
				build: func(q *sqlk.Query) *sqlk.Query {
					return q.FromSub(sqlk.NewQuery().From("Inner"), "t").Update(sqlk.Record{"A": 1})
				},
			},
			{
				name:  "delete",
				build: func(q *sqlk.Query) *sqlk.Query { return q.FromSub(sqlk.NewQuery().From("Inner"), "t").Delete() },
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := comp.Compile(tt.build(sqlk.NewQuery()))
				if !errors.Is(err, ErrInvalidWriteTarget) {
					t.Fatalf("Compile(...) error = %v, want ErrInvalidWriteTarget", err)
				}
			})
		}
	})

	t.Run("value shape problems are rejected", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			build   func(*sqlk.Query) *sqlk.Query
			columns int
			values  int
		}{
			{
				name:    "empty insert map",
				build:   func(q *sqlk.Query) *sqlk.Query { return q.From("A").Insert(sqlk.Record{}) },
				columns: 0,
			},
			{
				name:    "empty update map",
				build:   func(q *sqlk.Query) *sqlk.Query { return q.From("A").Update(sqlk.Record{}) },
				columns: 0,
			},
			{
				name:    "insert columns without values",
				build:   func(q *sqlk.Query) *sqlk.Query { return q.From("A").InsertColumns([]string{"a", "b"}, nil) },
				columns: 2,
			},
			{
				name: "update values do not match columns",
				build: func(q *sqlk.Query) *sqlk.Query {
					return q.From("A").UpdateColumns([]string{"a", "b"}, []any{1, 2, 3})
				},
				columns: 2,
				values:  3,
			},
			{
				name: "multi-row insert with a ragged row",
				build: func(q *sqlk.Query) *sqlk.Query {
					return q.From("A").InsertRows([]string{"a", "b"}, []any{1, 2}, []any{3})
				},
				columns: 2,
				values:  1,
			},
		} {
			t.Run(tt.name, func(t *testing.T) {
				_, err := comp.Compile(tt.build(sqlk.NewQuery()))
				if !errors.Is(err, ErrInvalidWriteValues) {
					t.Fatalf("Compile(...) error = %v, want ErrInvalidWriteValues", err)
				}
				shapeErr, ok := errors.AsType[*WriteValuesError](err)
				if !ok {
					t.Fatalf("Compile(...) error = %v, want a *WriteValuesError", err)
				}
				if shapeErr.Columns != tt.columns || shapeErr.Values != tt.values {
					t.Errorf("WriteValuesError = {Columns: %d, Values: %d}, want {%d, %d}",
						shapeErr.Columns, shapeErr.Values, tt.columns, tt.values)
				}
			})
		}
	})

	// InsertRows with columns but no rows carries no value rows at all; it
	// is its own sentinel, distinct from a malformed-row WriteValuesError.
	t.Run("insert without rows is rejected with ErrNoInsertRows", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("A").InsertRows([]string{"a", "b"}))
		if !errors.Is(err, ErrNoInsertRows) {
			t.Fatalf("Compile(...) error = %v, want ErrNoInsertRows", err)
		}
		if errors.Is(err, ErrInvalidWriteValues) {
			t.Errorf("errors.Is(err, ErrInvalidWriteValues) = true, want false (%v)", err)
		}
	})

	// Row-values and insert-from-select clauses cannot share one statement;
	// For engine scoping can surface both against the same dialect. Either
	// ordering is rejected up front (no panic, no silent drop).
	t.Run("mixed insert form is rejected", func(t *testing.T) {
		mysql := NewMysql()
		for _, build := range []struct {
			name  string
			query *sqlk.Query
		}{
			{
				name: "rows then insert-from",
				query: sqlk.NewQuery().Insert(sqlk.Record{"A": 1}).
					For(sqlk.EngineMysql, func(m *sqlk.Query) *sqlk.Query {
						return m.InsertFrom([]string{"A"}, sqlk.NewQuery().From("Src").Select("A"))
					}),
			},
			{
				name: "insert-from then rows",
				query: sqlk.NewQuery().InsertFrom([]string{"A"}, sqlk.NewQuery().From("Src").Select("A")).
					For(sqlk.EngineMysql, func(m *sqlk.Query) *sqlk.Query {
						return m.Insert(sqlk.Record{"A": 1})
					}),
			},
		} {
			t.Run(build.name, func(t *testing.T) {
				_, err := mysql.Compile(build.query)
				if !errors.Is(err, ErrMixedInsertForm) {
					t.Fatalf("Compile(...) error = %v, want ErrMixedInsertForm", err)
				}
			})
		}
	})

	// Combine clauses belong to select queries only.
	t.Run("combine on a write query is rejected", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("A").Insert(sqlk.Record{"x": 1}).
			Union(sqlk.NewQuery().From("B")))
		if !errors.Is(err, ErrCombineNotSelect) {
			t.Fatalf("Compile(...) error = %v, want ErrCombineNotSelect", err)
		}
	})

	t.Run("combine member must be a select query", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("A").
			Union(sqlk.NewQuery().From("B").Delete()))
		if !errors.Is(err, ErrCombineNotSelect) {
			t.Fatalf("Compile(...) error = %v, want ErrCombineNotSelect", err)
		}
	})

	t.Run("write problems aggregate with other compile problems", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().InsertColumns([]string{"a"}, []any{1, 2}))
		if !errors.Is(err, ErrInvalidWriteValues) {
			t.Errorf("errors.Is(err, ErrInvalidWriteValues) = false, want true (%v)", err)
		}
		if !errors.Is(err, ErrNoFromTarget) {
			t.Errorf("errors.Is(err, ErrNoFromTarget) = false, want true (%v)", err)
		}
	})

	t.Run("insert-from subquery is validated recursively", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("A").
			InsertFrom([]string{"x"}, sqlk.NewQuery().Select("x")))
		if !errors.Is(err, ErrNoFromTarget) {
			t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
		}
	})
}

// forEngine returns a base compiler that filters clauses by the given
// engine code. Engine-scope behavior is independent of the concrete
// dialect, so these tests drive it by setting the code directly.
func forEngine(engine string) *Compiler {
	c := New()
	c.engineCode = engine
	return c
}

func TestForEngineScope(t *testing.T) {
	scoped := func(q *sqlk.Query) *sqlk.Query {
		return q.From("Users").
			For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Where("Id", ">", 1) }).
			Where("Name", "=", "x")
	}
	// The generic compiler has no engine view: engine-tagged clauses are
	// all visible to it. Per-engine filtering is exercised below with
	// compilers that carry an engine code.
	runCompileCases(t, New(), []compileCase{
		{
			name:  "generic compiler sees engine-scoped clauses too",
			build: scoped,
			sql:   `SELECT * FROM "Users" WHERE "Id" > ? AND "Name" = ?`,
			args:  []any{1, "x"},
		},
		{
			name: "engine scope ends after the callback",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Where("A", "=", 1) }).
					Where("B", "=", 2)
			},
			sql:  `SELECT * FROM "T" WHERE "A" = ? AND "B" = ?`,
			args: []any{1, 2},
		},
		{
			name: "single-slot verbs replace within their engine scope only",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").Limit(10).For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Limit(5) })
			},
			sql:  `SELECT * FROM "T" LIMIT ?`,
			args: []any{10},
		},
		{
			name: "unscoped from coexists with scoped one",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.From("M") }).From("B")
			},
			sql: `SELECT * FROM "B"`,
		},
		{
			name: "engine-scoped limit is dropped by the aggregate rewrite",
			build: func(q *sqlk.Query) *sqlk.Query {
				// The aggregate rewrite strips limit/order/group in every
				// engine scope, not just the visible one.
				return q.From("A").Count("x").For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Limit(5) })
			},
			sql: `SELECT COUNT("x") AS "count" FROM "A"`,
		},
		{
			name: "nil callback is a no-op",
			build: func(q *sqlk.Query) *sqlk.Query {
				// A nil fn returns the query unchanged and leaves no
				// clause behind (it panicked before the apply
				// consolidation).
				return q.From("T").Where("A", "=", 1).For(sqlk.EngineMysql, nil)
			},
			sql:  `SELECT * FROM "T" WHERE "A" = ?`,
			args: []any{1},
		},
	})
	runCompileCases(t, forEngine(sqlk.EngineMysql), []compileCase{
		{
			name:  "matching engine keeps scoped clauses",
			build: scoped,
			sql:   `SELECT * FROM "Users" WHERE "Id" > ? AND "Name" = ?`,
			args:  []any{1, "x"},
		},
		{
			name: "engine-scoped from shadows the unscoped one",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.From("M") }).From("B")
			},
			sql: `SELECT * FROM "M"`,
		},
		{
			name: "engine-scoped select appends before later unscoped ones",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Select("A") }).Select("B")
			},
			sql: `SELECT "A", "B" FROM "T"`,
		},
		{
			name: "engine-scoped limit replaces unscoped one for that engine",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").Limit(10).For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Limit(5) })
			},
			sql:  `SELECT * FROM "T" LIMIT ?`,
			args: []any{5},
		},
		{
			name: "engine-scoped aggregate wraps for that engine",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Select("x").For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Count("ca", "cb") })
			},
			sql: `SELECT COUNT(*) AS "count" FROM (SELECT 1 FROM "A" WHERE "ca" IS NOT NULL AND "cb" IS NOT NULL) AS "countQuery"`,
		},
		{
			name: "engine-scoped cte is collected for that engine only",
			build: func(q *sqlk.Query) *sqlk.Query {
				body := sqlk.NewQuery().From("seqtbl").Select("Id").Where("Id", "<", 33)
				return q.From("Races").For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.With("range", body) }).Where("Id", ">", 55)
			},
			sql:  "WITH \"range\" AS (SELECT \"Id\" FROM \"seqtbl\" WHERE \"Id\" < ?)\nSELECT * FROM \"Races\" WHERE \"Id\" > ?",
			args: []any{33, 55},
		},
		{
			name: "engine-scoped write verbs coexist across engines",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Update(sqlk.Record{"A": 1}) }).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Update(sqlk.Record{"B": 2}) })
			},
			sql:  `UPDATE "T" SET "A" = ?`,
			args: []any{1},
		},
	})

	// From another engine's view: mysql-tagged clauses are invisible
	// while untagged clauses compile normally.
	runCompileCases(t, forEngine(sqlk.EnginePostgres), []compileCase{
		{
			name:  "other engine ignores scoped clauses",
			build: scoped,
			sql:   `SELECT * FROM "Users" WHERE "Name" = ?`,
			args:  []any{"x"},
		},
		{
			name: "clauses after the callback are unscoped",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Where("A", "=", 1) }).
					Where("B", "=", 2)
			},
			sql:  `SELECT * FROM "T" WHERE "B" = ?`,
			args: []any{2},
		},
		{
			name: "engine-scoped select is invisible",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Select("A") }).Select("B")
			},
			sql: `SELECT "B" FROM "T"`,
		},
		{
			name: "engine-scoped aggregate is not transformed",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Select("x").For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Count("ca", "cb") })
			},
			sql: `SELECT "x" FROM "A"`,
		},
		{
			name: "engine-scoped cte is not collected",
			build: func(q *sqlk.Query) *sqlk.Query {
				body := sqlk.NewQuery().From("seqtbl").Select("Id").Where("Id", "<", 33)
				return q.From("Races").For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.With("range", body) }).Where("Id", ">", 55)
			},
			sql:  `SELECT * FROM "Races" WHERE "Id" > ?`,
			args: []any{55},
		},
	})

	t.Run("scoped from only is invisible to other engines", func(t *testing.T) {
		_, err := forEngine(sqlk.EnginePostgres).Compile(sqlk.NewQuery().For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.From("M") }))
		if !errors.Is(err, ErrNoFromTarget) {
			t.Fatalf("Compile(...) error = %v, want ErrNoFromTarget", err)
		}
	})

	t.Run("write clauses scoped to other engines are rejected", func(t *testing.T) {
		_, err := forEngine(sqlk.EnginePostgres).Compile(sqlk.NewQuery().From("T").
			For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Update(sqlk.Record{"A": 1}) }))
		if !errors.Is(err, ErrNoVisibleWriteClause) {
			t.Fatalf("Compile(...) error = %v, want ErrNoVisibleWriteClause", err)
		}
		_, err = forEngine(sqlk.EnginePostgres).Compile(sqlk.NewQuery().From("T").
			For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Insert(sqlk.Record{"A": 1}) }))
		if !errors.Is(err, ErrNoVisibleWriteClause) {
			t.Fatalf("Compile(...) error = %v, want ErrNoVisibleWriteClause (insert)", err)
		}
	})
}

func TestDefineVariable(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "where value references a definition",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Products").Define("@name", "Anto").Where("ProductName", "=", sqlk.NewVariable("@name"))
			},
			sql:  `SELECT * FROM "Products" WHERE "ProductName" = ?`,
			args: []any{"Anto"},
		},
		{
			name: "later definition of the same name wins",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").Define("@x", 1).Define("@x", 2).WhereEq("A", sqlk.NewVariable("@x"))
			},
			sql:  `SELECT * FROM "T" WHERE "A" = ?`,
			args: []any{2},
		},
		{
			name: "in-list entries resolve individually",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").Define("@a", 1).Define("@b", 2).WhereIn("Id", sqlk.NewVariable("@a"), sqlk.NewVariable("@b"))
			},
			sql:  `SELECT * FROM "T" WHERE "Id" IN (?, ?)`,
			args: []any{1, 2},
		},
		{
			name: "between bounds resolve",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").Define("@lo", 18).WhereBetween("Age", sqlk.NewVariable("@lo"), 60)
			},
			sql:  `SELECT * FROM "T" WHERE "Age" BETWEEN ? AND ?`,
			args: []any{18, 60},
		},
		{
			name: "subquery resolves its own definitions",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Products").Avg("UnitPrice").
					Define("@UnitsInStock", 10).
					Where("UnitsInStock", ">", sqlk.NewVariable("@UnitsInStock"))
				return q.From("Products").WhereSub(sub, "<", 100)
			},
			sql:  `SELECT * FROM "Products" WHERE (SELECT AVG("UnitPrice") AS "avg" FROM "Products" WHERE "UnitsInStock" > ?) < ?`,
			args: []any{10, 100},
		},
		{
			name: "nested subquery resolves definitions up the parent chain",
			build: func(q *sqlk.Query) *sqlk.Query {
				sub := sqlk.NewQuery().From("Orders").Select("Id").Where("CustomerId", "=", sqlk.NewVariable("@cid"))
				return q.From("Users").Define("@cid", 7).WhereInSub("Id", sub)
			},
			sql:  `SELECT * FROM "Users" WHERE "Id" IN (SELECT "Id" FROM "Orders" WHERE "CustomerId" = ?)`,
			args: []any{7},
		},
		{
			name: "two levels of chain lookup",
			build: func(q *sqlk.Query) *sqlk.Query {
				inner := sqlk.NewQuery().From("Logs").Where("At", ">", sqlk.NewVariable("@since"))
				mid := sqlk.NewQuery().From("Users").Select("Id").WhereExists(inner)
				return q.From("Accounts").Define("@since", "2024-01-01").WhereInSub("Owner", mid)
			},
			sql:  `SELECT * FROM "Accounts" WHERE "Owner" IN (SELECT "Id" FROM "Users" WHERE EXISTS (SELECT 1 FROM "Logs" WHERE "At" > ?))`,
			args: []any{"2024-01-01"},
		},
		{
			name: "join on-conditions resolve against the outer query's scope",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Define("@role", "admin").
					JoinOn("Roles", func(j *sqlk.Join) *sqlk.Join {
						return j.On("Roles.UserId", "=", "Users.Id").Where("Roles.Name", "=", sqlk.NewVariable("@role"))
					})
			},
			sql:  "SELECT * FROM \"Users\" \nINNER JOIN \"Roles\" ON \"Roles\".\"UserId\" = \"Users\".\"Id\" AND \"Roles\".\"Name\" = ?",
			args: []any{"admin"},
		},
		{
			name: "update values reference definitions",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Define("@count", 5).Update(sqlk.Record{"Count": sqlk.NewVariable("@count")})
			},
			sql:  `UPDATE "Table" SET "Count" = ?`,
			args: []any{5},
		},
		{
			name: "insert values reference definitions",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Define("@count", 5).Insert(sqlk.Record{"Count": sqlk.NewVariable("@count")})
			},
			sql:  `INSERT INTO "Table" ("Count") VALUES (?)`,
			args: []any{5},
		},
		{
			name: "definitions survive the aggregate rewrite",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Define("@x", 3).WhereEq("Id", sqlk.NewVariable("@x")).Count("ca", "cb")
			},
			sql:  `SELECT COUNT(*) AS "count" FROM (SELECT 1 FROM "A" WHERE "Id" = ? AND "ca" IS NOT NULL AND "cb" IS NOT NULL) AS "countQuery"`,
			args: []any{3},
		},
		{
			name: "aggregate filter scope resolves against the outer query",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").Define("@t", "x").SelectSum("Total", func(f *sqlk.Query) *sqlk.Query {
					return f.Where("Type", "=", sqlk.NewVariable("@t"))
				})
			},
			sql:  `SELECT SUM(CASE WHEN "Type" = ? THEN "Total" END) FROM "A"`,
			args: []any{"x"},
		},
		{
			name: "cte body resolves its own definitions",
			build: func(q *sqlk.Query) *sqlk.Query {
				body := sqlk.NewQuery().From("Products").Define("@unit", 10).Where("UnitPrice", ">", sqlk.NewVariable("@unit"))
				return q.With("prodCTE", body).From("prodCTE")
			},
			sql:  "WITH \"prodCTE\" AS (SELECT * FROM \"Products\" WHERE \"UnitPrice\" > ?)\nSELECT * FROM \"prodCTE\"",
			args: []any{10},
		},
	})
}

func TestVariableValidation(t *testing.T) {
	comp := New()

	t.Run("undefined variable is rejected", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("T").WhereEq("A", sqlk.NewVariable("@missing")))
		if !errors.Is(err, ErrVariableNotDefined) {
			t.Fatalf("Compile(...) error = %v, want ErrVariableNotDefined", err)
		}
		varErr, ok := errors.AsType[*VariableError](err)
		if !ok {
			t.Fatalf("Compile(...) error = %v, want a *VariableError", err)
		}
		if varErr.Name != "@missing" {
			t.Errorf("VariableError.Name = %q, want @missing", varErr.Name)
		}
	})

	t.Run("chain miss inside nested subqueries is rejected", func(t *testing.T) {
		sub := sqlk.NewQuery().From("Orders").Where("CustomerId", "=", sqlk.NewVariable("@cid"))
		_, err := comp.Compile(sqlk.NewQuery().From("Users").WhereInSub("Id", sub))
		if !errors.Is(err, ErrVariableNotDefined) {
			t.Fatalf("Compile(...) error = %v, want ErrVariableNotDefined", err)
		}
	})

	t.Run("cte bodies do not see outer definitions", func(t *testing.T) {
		// Embedded CTE bodies carry no parent chain; variables resolve
		// within the body itself.
		body := sqlk.NewQuery().From("Products").Where("UnitPrice", ">", sqlk.NewVariable("@unit"))
		_, err := comp.Compile(sqlk.NewQuery().From("prodCTE").Define("@unit", 10).With("prodCTE", body))
		if !errors.Is(err, ErrVariableNotDefined) {
			t.Fatalf("Compile(...) error = %v, want ErrVariableNotDefined", err)
		}
	})

	t.Run("undefined variables inside condition groups are rejected", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("T").WhereGroup(func(g *sqlk.Query) *sqlk.Query {
			return g.WhereEq("A", sqlk.NewVariable("@missing"))
		}))
		if !errors.Is(err, ErrVariableNotDefined) {
			t.Fatalf("Compile(...) error = %v, want ErrVariableNotDefined", err)
		}
	})

	t.Run("undefined variables in write values are rejected", func(t *testing.T) {
		_, err := comp.Compile(sqlk.NewQuery().From("T").Update(sqlk.Record{"A": sqlk.NewVariable("@missing")}))
		if !errors.Is(err, ErrVariableNotDefined) {
			t.Fatalf("Compile(...) error = %v, want ErrVariableNotDefined (update)", err)
		}
		_, err = comp.Compile(sqlk.NewQuery().From("T").Insert(sqlk.Record{"A": sqlk.NewVariable("@missing")}))
		if !errors.Is(err, ErrVariableNotDefined) {
			t.Fatalf("Compile(...) error = %v, want ErrVariableNotDefined (insert)", err)
		}
	})

	t.Run("adhoc cte values cannot reference variables", func(t *testing.T) {
		// Ad-hoc table values behave like CTE bodies: they do not resolve
		// along the definition chain, even when an outer definition of
		// the same name exists.
		_, err := comp.Compile(sqlk.NewQuery().From("T").Define("@x", 1).
			WithTable("vals", []string{"n"}, []any{sqlk.NewVariable("@x")}))
		if !errors.Is(err, ErrVariableNotDefined) {
			t.Fatalf("Compile(...) error = %v, want ErrVariableNotDefined", err)
		}
	})
}

func TestUnsafeLiteral(t *testing.T) {
	runCompileCases(t, New(), []compileCase{
		{
			name: "where value is inlined without a parameter",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").Where("CreatedAt", "<", sqlk.NewUnsafeLiteral("NOW()"))
			},
			sql: `SELECT * FROM "T" WHERE "CreatedAt" < NOW()`,
		},
		{
			name:  "single quotes are doubled on construction",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("T").Where("Note", "=", sqlk.NewUnsafeLiteral("it's")) },
			sql:   `SELECT * FROM "T" WHERE "Note" = it''s`,
		},
		{
			name: "in-list mixes literals and parameters in order",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").WhereIn("Id", sqlk.NewUnsafeLiteral("1"), 2)
			},
			sql:  `SELECT * FROM "T" WHERE "Id" IN (1, ?)`,
			args: []any{2},
		},
		{
			name: "insert value is inlined",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Table").Insert(sqlk.Record{"Count": sqlk.NewUnsafeLiteral("Count + 1")})
			},
			sql: `INSERT INTO "Table" ("Count") VALUES (Count + 1)`,
		},
		{
			name: "update value is inlined and skips the parameter slot",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("MyTable").Update(sqlk.Record{"Name": "The User", "Address": sqlk.NewUnsafeLiteral("@address")})
			},
			sql:  `UPDATE "MyTable" SET "Address" = @address, "Name" = ?`,
			args: []any{"The User"},
		},
		{
			name: "between bound is inlined",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").WhereBetween("Age", 18, sqlk.NewUnsafeLiteral("65"))
			},
			sql:  `SELECT * FROM "T" WHERE "Age" BETWEEN ? AND 65`,
			args: []any{18},
		},
	})
}
