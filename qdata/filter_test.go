package qdata

import (
	"errors"
	"reflect"
	"testing"

	"github.com/aiongo/sqlk/compiler"
)

// Main-seam cases for the filter operators: the sixteen operator codes,
// nested groups, empty-data skipping, is/ns ignoring data, in/ni with array
// and scalar data, and the Hook.Rule pointcut. Seam and assertion
// conventions live in query_test.go (decode -> ToQuery -> compile -> assert
// SQL text and argument sequence).

func TestValidateRules(t *testing.T) {
	t.Run("all sixteen operator codes are valid", func(t *testing.T) {
		for _, op := range []string{
			OpEq, OpNe, OpLt, OpLe, OpGt, OpGe,
			OpIn, OpNi, OpIs, OpNs,
			OpBw, OpBn, OpEw, OpEn, OpCn, OpNc,
		} {
			q := New().WithFrom("Users").WithFilter(
				*NewFilter().WithRule(*NewRule("F", op, "v")),
			)
			if err := q.Validate(); err != nil {
				t.Errorf("Validate() with op %q error = %v, want nil", op, err)
			}
		}
	})

	t.Run("is and ns no longer rejected unlike legacy implementation", func(t *testing.T) {
		// is and ns are valid operator codes and must not be rejected.
		q := New().WithFrom("Users").WithFilter(*NewFilter().
			WithRule(*NewRule("A", OpIs, nil)).WithRule(*NewRule("B", OpNs, nil)))
		if err := q.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("empty field and invalid op rejected", func(t *testing.T) {
		q := New().WithFrom("Users").WithFilter(*NewFilter().
			WithRule(*NewRule("", OpEq, "v")).WithRule(*NewRule("F", "eq1", "v")))
		err := q.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want problems")
		}
		if got := problemCount(t, err); got != 2 {
			t.Errorf("problem count = %d, want 2", got)
		}
		if !errors.Is(err, ErrRuleFieldRequired) {
			t.Errorf("errors.Is(err, ErrRuleFieldRequired) = false, want true")
		}
		if !errors.Is(err, ErrInvalidOp) {
			t.Errorf("errors.Is(err, ErrInvalidOp) = false, want true")
		}
		var opErr *OpError
		if !errors.As(err, &opErr) || *opErr != (OpError{Field: "F", Op: "eq1"}) {
			t.Errorf("errors.As(err, *OpError) = %+v, want Field F Op eq1", opErr)
		}
	})

	t.Run("empty op rejected as invalid op", func(t *testing.T) {
		q := New().WithFrom("Users").WithFilter(*NewFilter().
			WithRule(Rule{Field: "F", Data: "v"}))
		if err := q.Validate(); !errors.Is(err, ErrInvalidOp) {
			t.Errorf("Validate() error = %v, want ErrInvalidOp", err)
		}
	})

	t.Run("empty data rules still participate in validation", func(t *testing.T) {
		q := New().WithFrom("Users").WithFilter(*NewFilter().
			WithRule(*NewRule("", OpEq, "")).WithRule(*NewRule("F", "eq1", nil)))
		err := q.Validate()
		if !errors.Is(err, ErrRuleFieldRequired) {
			t.Errorf("errors.Is(err, ErrRuleFieldRequired) = false, want true")
		}
		if !errors.Is(err, ErrInvalidOp) {
			t.Errorf("errors.Is(err, ErrInvalidOp) = false, want true")
		}
	})

	t.Run("rules in nested groups validated", func(t *testing.T) {
		q := New().WithFrom("Users").WithFilter(*NewFilter().WithGroup(
			Filter{GroupOp: GroupOpOr, Rules: []Rule{{Field: "F", Op: "bad"}}},
		))
		if err := q.Validate(); !errors.Is(err, ErrInvalidOp) {
			t.Errorf("Validate() error = %v, want ErrInvalidOp", err)
		}
	})
}

func TestFilterOperatorSeam(t *testing.T) {
	runSeamCases(t, []seamCase{
		{
			name:    "eq ne and comparison operators",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Age", "op": "eq", "data": 18}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Age" = ?`,
			args:    []any{float64(18)},
		},
		{
			name:    "ne compiles to not equal",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Age", "op": "ne", "data": 18}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Age" != ?`,
			args:    []any{float64(18)},
		},
		{
			name:    "lt compiles to less",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Age", "op": "lt", "data": 18}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Age" < ?`,
			args:    []any{float64(18)},
		},
		{
			name:    "le compiles to less or equal",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Age", "op": "le", "data": 18}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Age" <= ?`,
			args:    []any{float64(18)},
		},
		{
			name:    "gt compiles to greater",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Age", "op": "gt", "data": 18}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Age" > ?`,
			args:    []any{float64(18)},
		},
		{
			name:    "ge compiles to greater or equal",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Age", "op": "ge", "data": 18}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Age" >= ?`,
			args:    []any{float64(18)},
		},
		{
			name:    "in with array data",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "ShipVia", "op": "in", "data": ["FE", "UPS"]}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "ShipVia" IN (?, ?)`,
			args:    []any{"FE", "UPS"},
		},
		{
			name:    "in with scalar data",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "ShipVia", "op": "in", "data": "FE"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "ShipVia" IN (?)`,
			args:    []any{"FE"},
		},
		{
			name:    "ni with array data",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "ShipVia", "op": "ni", "data": ["FE", "UPS"]}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "ShipVia" NOT IN (?, ?)`,
			args:    []any{"FE", "UPS"},
		},
		{
			name:    "is compiles to IS NULL ignoring data",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Note", "op": "is", "data": "ignored"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Note" IS NULL`,
		},
		{
			name:    "ns compiles to IS NOT NULL ignoring data",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Note", "op": "ns", "data": "ignored"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Note" IS NOT NULL`,
		},
		{
			name:    "bw compiles to prefix pattern",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Name", "op": "bw", "data": "da"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Name" like ?`,
			args:    []any{"da%"},
		},
		{
			name:    "bn compiles to negated prefix pattern",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Name", "op": "bn", "data": "da"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE NOT ("Name" like ?)`,
			args:    []any{"da%"},
		},
		{
			name:    "ew compiles to suffix pattern",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Name", "op": "ew", "data": "da"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Name" like ?`,
			args:    []any{"%da"},
		},
		{
			name:    "en compiles to negated suffix pattern",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Name", "op": "en", "data": "da"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE NOT ("Name" like ?)`,
			args:    []any{"%da"},
		},
		{
			name:    "cn compiles to contains pattern",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Name", "op": "cn", "data": "da"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Name" like ?`,
			args:    []any{"%da%"},
		},
		{
			name:    "nc compiles to negated contains pattern",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Name", "op": "nc", "data": "da"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE NOT ("Name" like ?)`,
			args:    []any{"%da%"},
		},
		{
			name:    "or group connects in ns and like family with or",
			payload: `{"from": ["Users"], "filter": {"group_op": "or", "rules": [{"field": "S", "op": "in", "data": ["a", "b"]}, {"field": "N", "op": "ns"}, {"field": "Name", "op": "ew", "data": "da"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "S" IN (?, ?) OR "N" IS NOT NULL OR "Name" like ?`,
			args:    []any{"a", "b", "%da"},
		},
		{
			name:    "or group connects negated forms with or",
			payload: `{"from": ["Users"], "filter": {"group_op": "or", "rules": [{"field": "S", "op": "ni", "data": "a"}, {"field": "Name", "op": "bn", "data": "da"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "S" NOT IN (?) OR NOT ("Name" like ?)`,
			args:    []any{"a", "da%"},
		},
		{
			name:    "like family stringifies non-string data",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "Age", "op": "bw", "data": 18}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Age" like ?`,
			args:    []any{"18%"},
		},
	})
}

func TestFilterGroupSeam(t *testing.T) {
	runSeamCases(t, []seamCase{
		{
			name:    "empty group op defaults to and",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "A", "op": "eq", "data": "a"}, {"field": "B", "op": "eq", "data": "b"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "A" = ? AND "B" = ?`,
			args:    []any{"a", "b"},
		},
		{
			name:    "or group op connects rules with or",
			payload: `{"from": ["Users"], "filter": {"group_op": "or", "rules": [{"field": "A", "op": "eq", "data": "a"}, {"field": "B", "op": "eq", "data": "b"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "A" = ? OR "B" = ?`,
			args:    []any{"a", "b"},
		},
		{
			name: "legacy simple query baseline or rules with qualified field",
			// Baseline: or rules, one with a qualified field.
			payload: `{
				"from": ["abc"],
				"filter": {
					"group_op": "or",
					"rules": [
						{"field": "f1", "op": "eq", "data": "f1v"},
						{"field": "f2.id", "op": "eq", "data": "f2v"}
					]
				},
				"orderby": [{"by": "xxx"}]
			}`,
			sql:  `SELECT * FROM "abc" WHERE "f1" = ? OR "f2"."id" = ? ORDER BY xxx ASC`,
			args: []any{"f1v", "f2v"},
		},
		{
			name: "legacy complex query baseline nested groups at depth three",
			// Baseline: three levels of nested groups, each layer's group_op
			// in effect.
			payload: `{
				"from": ["abc"],
				"filter": {
					"group_op": "or",
					"rules": [
						{"field": "f1", "op": "eq", "data": "f1v"},
						{"field": "f2.id", "op": "eq", "data": "f2v"}
					],
					"groups": [
						{
							"group_op": "and",
							"rules": [
								{"field": "f211", "op": "eq", "data": "f211v"},
								{"field": "f212", "op": "eq", "data": "f212v"}
							],
							"groups": [
								{
									"group_op": "and",
									"rules": [
										{"field": "f311", "op": "eq", "data": "f311v"},
										{"field": "f312", "op": "eq", "data": "f312v"}
									]
								}
							]
						},
						{
							"group_op": "and",
							"rules": [
								{"field": "f221", "op": "eq", "data": "f221v"},
								{"field": "f222", "op": "eq", "data": "f222v"}
							]
						}
					]
				},
				"orderby": [{"by": "xxx"}],
				"top": 30
			}`,
			sql: `SELECT * FROM "abc" WHERE "f1" = ? OR "f2"."id" = ? OR ` +
				`("f211" = ? AND "f212" = ? AND ("f311" = ? AND "f312" = ?)) OR ` +
				`("f221" = ? AND "f222" = ?) ORDER BY xxx ASC LIMIT ?`,
			args: []any{"f1v", "f2v", "f211v", "f212v", "f311v", "f312v", "f221v", "f222v", 30},
		},
		{
			name:    "subgroup with its own or inside and parent",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "A", "op": "eq", "data": "a"}], "groups": [{"group_op": "or", "rules": [{"field": "B", "op": "eq", "data": "b"}, {"field": "C", "op": "eq", "data": "c"}]}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "A" = ? AND ("B" = ? OR "C" = ?)`,
			args:    []any{"a", "b", "c"},
		},
		{
			name:    "count keeps filter while ignoring select orderby limit",
			payload: `{"from": ["Users"], "select": ["Id"], "orderby": [{"by": "Name"}], "top": 5, "count": true, "filter": {"rules": [{"field": "Age", "op": "gt", "data": 18}]}}`,
			sql:     `SELECT COUNT(*) AS "count" FROM "Users" WHERE "Age" > ?`,
			args:    []any{float64(18)},
		},
	})
}

func TestFilterEmptyDataSkipped(t *testing.T) {
	runSeamCases(t, []seamCase{
		{
			name:    "empty string and null data rules skipped",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "A", "op": "eq", "data": ""}, {"field": "B", "op": "eq", "data": "b"}, {"field": "C", "op": "eq", "data": null}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "B" = ?`,
			args:    []any{"b"},
		},
		{
			name:    "missing data key skipped as nil",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "A", "op": "eq"}]}}`,
			sql:     `SELECT * FROM "Users"`,
		},
		{
			name:    "empty array data for in skipped",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "S", "op": "in", "data": []}]}}`,
			sql:     `SELECT * FROM "Users"`,
		},
		{
			name:    "group with only empty data rules omitted entirely",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "A", "op": "eq", "data": "a"}], "groups": [{"group_op": "and", "rules": [{"field": "B", "op": "eq", "data": ""}]}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "A" = ?`,
			args:    []any{"a"},
		},
		{
			name:    "is and ns never skipped on empty data",
			payload: `{"from": ["Users"], "filter": {"rules": [{"field": "N", "op": "is", "data": ""}, {"field": "M", "op": "ns"}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "N" IS NULL AND "M" IS NOT NULL`,
		},
		{
			name:    "all rules empty leaves no where clause",
			payload: `{"from": ["Users"], "filter": {"group_op": "or", "rules": [{"field": "A", "op": "eq", "data": ""}], "groups": [{"group_op": "and", "rules": [{"field": "B", "op": "bw", "data": null}]}]}}`,
			sql:     `SELECT * FROM "Users"`,
		},
	})
}

func TestFilterHook(t *testing.T) {
	t.Run("rule hook rewrites field as identifier", func(t *testing.T) {
		hook := stubHook{rules: func(r Rule) (Rule, error) {
			r.Field = "u." + r.Field
			return r, nil
		}}
		query, err := mustUnmarshal(t, `{"from": ["Users"], "filter": {"rules": [{"field": "Age", "op": "gt", "data": 18}]}}`).ToQuery(hook)
		if err != nil {
			t.Fatalf("ToQuery(hook) error = %v, want nil", err)
		}
		res := mustCompile(t, compiler.New(), query)
		if res.SQL != `SELECT * FROM "Users" WHERE "u"."Age" > ?` {
			t.Errorf("SQL = %q, want %q", res.SQL, `SELECT * FROM "Users" WHERE "u"."Age" > ?`)
		}
		if !reflect.DeepEqual(res.Args, []any{float64(18)}) {
			t.Errorf("Args = %#v, want [18]", res.Args)
		}
	})

	t.Run("rule hook error propagates as is", func(t *testing.T) {
		errForbidden := errors.New("field is not allowed")
		hook := stubHook{rules: func(Rule) (Rule, error) { return Rule{}, errForbidden }}
		payload := `{"from": ["Users"], "filter": {"rules": [{"field": "Secret", "op": "eq", "data": "x"}]}}`
		if _, err := mustUnmarshal(t, payload).ToQuery(hook); !errors.Is(err, errForbidden) {
			t.Errorf("ToQuery(hook) error = %v, want errForbidden as is", err)
		}
	})

	t.Run("rule hook emptying field is rejected", func(t *testing.T) {
		hook := stubHook{rules: func(Rule) (Rule, error) { return Rule{Field: "", Op: OpEq, Data: "x"}, nil }}
		payload := `{"from": ["Users"], "filter": {"rules": [{"field": "Age", "op": "eq", "data": "x"}]}}`
		if _, err := mustUnmarshal(t, payload).ToQuery(hook); !errors.Is(err, ErrRuleFieldRequired) {
			t.Errorf("ToQuery(hook) error = %v, want ErrRuleFieldRequired", err)
		}
	})

	t.Run("rule hook invalidating op is rejected", func(t *testing.T) {
		hook := stubHook{rules: func(r Rule) (Rule, error) { r.Op = "eq1"; return r, nil }}
		payload := `{"from": ["Users"], "filter": {"rules": [{"field": "Age", "op": "eq", "data": "x"}]}}`
		if _, err := mustUnmarshal(t, payload).ToQuery(hook); !errors.Is(err, ErrInvalidOp) {
			t.Errorf("ToQuery(hook) error = %v, want ErrInvalidOp", err)
		}
	})

	t.Run("rule hook sees skipped rules and may fill data", func(t *testing.T) {
		hook := stubHook{rules: func(r Rule) (Rule, error) {
			if r.Data == "" || r.Data == nil {
				r.Data = "filled"
			}
			return r, nil
		}}
		payload := `{"from": ["Users"], "filter": {"rules": [{"field": "A", "op": "eq", "data": ""}]}}`
		query, err := mustUnmarshal(t, payload).ToQuery(hook)
		if err != nil {
			t.Fatalf("ToQuery(hook) error = %v, want nil", err)
		}
		res := mustCompile(t, compiler.New(), query)
		if res.SQL != `SELECT * FROM "Users" WHERE "A" = ?` {
			t.Errorf("SQL = %q, want %q", res.SQL, `SELECT * FROM "Users" WHERE "A" = ?`)
		}
		if !reflect.DeepEqual(res.Args, []any{"filled"}) {
			t.Errorf("Args = %#v, want [filled]", res.Args)
		}
	})

	t.Run("rule hook applies to nested group rules", func(t *testing.T) {
		hook := stubHook{rules: func(r Rule) (Rule, error) {
			if r.Field == "B" {
				return Rule{}, errors.New("field B is not allowed")
			}
			return r, nil
		}}
		payload := `{"from": ["Users"], "filter": {"groups": [{"group_op": "or", "rules": [{"field": "B", "op": "eq", "data": "b"}]}]}}`
		if _, err := mustUnmarshal(t, payload).ToQuery(hook); err == nil {
			t.Error("ToQuery(hook) error = nil, want rejection")
		}
	})
}

func TestFilterProgrammaticData(t *testing.T) {
	// Wire-format JSON delivers arrays only as []any; programmatic building
	// via NewRule can pass concrete slice types, and the widening path is
	// pinned here.
	t.Run("in with typed string slice data", func(t *testing.T) {
		q := New().WithFrom("Users").
			WithFilter(*NewFilter().WithRule(*NewRule("S", OpIn, []string{"a", "b"})))
		res := mustCompile(t, compiler.New(), mustToQuery(t, q))
		if res.SQL != `SELECT * FROM "Users" WHERE "S" IN (?, ?)` {
			t.Errorf("SQL = %q, want %q", res.SQL, `SELECT * FROM "Users" WHERE "S" IN (?, ?)`)
		}
		if !reflect.DeepEqual(res.Args, []any{"a", "b"}) {
			t.Errorf("Args = %#v, want [a b]", res.Args)
		}
	})

	t.Run("in with empty typed slice data skipped", func(t *testing.T) {
		q := New().WithFrom("Users").
			WithFilter(*NewFilter().WithRule(*NewRule("S", OpIn, []int{})))
		res := mustCompile(t, compiler.New(), mustToQuery(t, q))
		if res.SQL != `SELECT * FROM "Users"` {
			t.Errorf("SQL = %q, want %q", res.SQL, `SELECT * FROM "Users"`)
		}
	})
}
