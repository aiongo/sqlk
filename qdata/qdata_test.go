package qdata

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// The qdata seam joins the main seam: JSON -> qdata.QData -> Validate
// (asserted via errors.Is/As or messages) -> ToQuery -> the caller's choice
// of dialect compiler -> assert the SQL text and argument sequence. Only
// externally observable behavior is asserted; intermediate structures are
// not inspected.

// mustUnmarshal decodes wire-format JSON straight into a qdata.QData and
// fails the test on error.
func mustUnmarshal(t *testing.T, payload string) *QData {
	t.Helper()
	var q QData
	if err := json.Unmarshal([]byte(payload), &q); err != nil {
		t.Fatalf("json.Unmarshal(%s) error = %v", payload, err)
	}
	return &q
}

func TestUnmarshalQuery(t *testing.T) {
	tests := []struct {
		name    string
		payload string
		want    QData
	}{
		{
			name: "wire keys mapped",
			payload: `{
				"from": ["Users"],
				"select": ["Id", "Name"],
				"filter": {
					"group_op": "and",
					"rules": [{"field": "Age", "op": "gt", "data": 18}],
					"groups": [{"group_op": "or", "rules": [{"field": "Name", "op": "bw", "data": "a"}]}]
				},
				"orderby": [{"by": "Name", "xsc": "desc"}],
				"top": 5,
				"skip": 10,
				"count": false
			}`,
			want: QData{
				From:   []string{"Users"},
				Select: []string{"Id", "Name"},
				Filter: Filter{
					GroupOp: "and",
					Rules:   []Rule{{Field: "Age", Op: "gt", Data: float64(18)}},
					Groups:  []Filter{{GroupOp: "or", Rules: []Rule{{Field: "Name", Op: "bw", Data: "a"}}}},
				},
				OrderBy: []OrderBy{{By: "Name", Xsc: "desc"}},
				Top:     5,
				Skip:    10,
			},
		},
		{
			name:    "multiple from targets parsed in order",
			payload: `{"from": ["Users", "Orders", "Payments"]}`,
			want:    QData{From: []string{"Users", "Orders", "Payments"}},
		},
		{
			name:    "missing pagination keys default to zero",
			payload: `{"from": ["Users"]}`,
			want:    QData{From: []string{"Users"}},
		},
		{
			// Zero is the zero value: an explicit 0 unmarshals exactly like
			// a missing key (ToQuery treats both as "no limit").
			name:    "explicit zero top accepted",
			payload: `{"from": ["Users"], "top": 0, "skip": 0}`,
			want:    QData{From: []string{"Users"}},
		},
		{
			name:    "legacy keys ignored",
			payload: `{"from": ["Users"], "entity": "Posts", "selects": ["Id"], "includes": ["Dept"], "sorts": [{"by": "Name"}], "limit": {"top": 5}}`,
			want:    QData{From: []string{"Users"}},
		},
		{
			name:    "count parsed",
			payload: `{"from": ["Users"], "count": true}`,
			want:    QData{From: []string{"Users"}, Count: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mustUnmarshal(t, tt.payload)
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("Unmarshal(...) = %#v, want %#v", *got, tt.want)
			}
		})
	}
}

// problemCount unwraps Validate's joined error and returns the number of
// problems.
func problemCount(t *testing.T, err error) int {
	t.Helper()
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("Validate() error %T does not unwrap to a problem list", err)
	}
	return len(joined.Unwrap())
}

func TestValidateAggregates(t *testing.T) {
	t.Run("valid defaults pass", func(t *testing.T) {
		q := New().WithFrom("Users").
			WithFilter(*NewFilter().WithGroup(Filter{}).WithGroup(Filter{GroupOp: "or"}).WithGroup(
				Filter{GroupOp: "and", Groups: []Filter{{}}},
			)).
			WithOrderBy(*NewOrderBy("Name", ""), OrderBy{By: "Age", Xsc: "asc"}, OrderBy{By: "Id", Xsc: "desc"})
		if err := q.Validate(); err != nil {
			t.Errorf("Validate() error = %v, want nil", err)
		}
	})

	t.Run("empty from element rejected like empty list", func(t *testing.T) {
		for _, from := range [][]string{nil, {}, {""}, {"Users", ""}} {
			if err := New().WithFrom(from...).Validate(); !errors.Is(err, ErrFromRequired) {
				t.Errorf("Validate() with from %#v error = %v, want ErrFromRequired", from, err)
			}
		}
	})

	t.Run("aggregates all problems at once", func(t *testing.T) {
		q := New().WithFrom().
			WithOrderBy(OrderBy{By: "Name", Xsc: "up"}, OrderBy{By: ""}).
			WithFilter(Filter{GroupOp: "and", Groups: []Filter{{GroupOp: "xor"}}}).
			WithTop(-1).WithSkip(-2)
		err := q.Validate()
		if err == nil {
			t.Fatal("Validate() error = nil, want aggregated problems")
		}
		if got := problemCount(t, err); got != 6 {
			t.Errorf("problem count = %d, want 6", got)
		}
		for _, sentinel := range []error{
			ErrFromRequired,
			ErrOrderByByRequired,
			ErrInvalidOrderByDirection,
			ErrInvalidGroupOp,
			ErrInvalidPagination,
		} {
			if !errors.Is(err, sentinel) {
				t.Errorf("errors.Is(err, %v) = false, want true", sentinel)
			}
		}
		var groupOpErr *GroupOpError
		if !errors.As(err, &groupOpErr) || *groupOpErr != (GroupOpError{Value: "xor"}) {
			t.Errorf("errors.As(err, *GroupOpError) = %+v, want Value xor", groupOpErr)
		}
		var dirErr *OrderByDirectionError
		if !errors.As(err, &dirErr) || *dirErr != (OrderByDirectionError{By: "Name", Xsc: "up"}) {
			t.Errorf("errors.As(err, *OrderByDirectionError) = %+v, want By Name Xsc up", dirErr)
		}
		var pageErr *PaginationError
		if !errors.As(err, &pageErr) || *pageErr != (PaginationError{Field: "top", Value: -1}) {
			t.Errorf("errors.As(err, *PaginationError) = %+v, want Field top Value -1", pageErr)
		}
	})
}

// seamCase is one table-driven case for the main seam: (JSON wire format,
// Hook, dialect) -> (expected SQL, expected arguments).
type seamCase struct {
	name    string
	payload string
	hook    Hook
	comp    *compiler.Compiler
	sql     string
	args    []any
}

// runSeamCases walks each case through "decode -> ToQuery -> compile" and
// asserts the SQL and the argument sequence; a nil comp means the base
// compiler.
func runSeamCases(t *testing.T, cases []seamCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			comp := tt.comp
			if comp == nil {
				comp = compiler.New()
			}
			query, err := mustUnmarshal(t, tt.payload).ToQuery(tt.hook)
			if err != nil {
				t.Fatalf("ToQuery(...) error = %v, want nil", err)
			}
			res, err := comp.Compile(query)
			if err != nil {
				t.Fatalf("Compile(...) error = %v, want nil", err)
			}
			if res.SQL != tt.sql {
				t.Errorf("SQL = %q, want %q", res.SQL, tt.sql)
			}
			if !reflect.DeepEqual(res.Args, tt.args) {
				t.Errorf("Args = %#v, want %#v", res.Args, tt.args)
			}
		})
	}
}

func TestToQueryCompileSeam(t *testing.T) {
	runSeamCases(t, []seamCase{
		{
			name:    "from only emits no limit clause",
			payload: `{"from": ["Users"]}`,
			sql:     `SELECT * FROM "Users"`,
		},
		{
			name:    "select projection",
			payload: `{"from": ["Users"], "select": ["Id", "Name"]}`,
			sql:     `SELECT "Id", "Name" FROM "Users"`,
		},
		{
			name:    "select qualified and aliased columns",
			payload: `{"from": ["Users"], "select": ["u.Id", "u.Name as name"]}`,
			sql:     `SELECT "u"."Id", "u"."Name" AS "name" FROM "Users"`,
		},
		{
			name:    "select raw expression on open paren",
			payload: `{"from": ["Users"], "select": ["Id", "count(*) as total"]}`,
			sql:     `SELECT "Id", count(*) as total FROM "Users"`,
		},
		{
			name:    "additional from targets build convention joins",
			payload: `{"from": ["Users", "Orders", "Payments"]}`,
			sql: `SELECT * FROM "Users"` +
				" \n" + `INNER JOIN "Orders" ON "Users"."Orders_id" = "Orders"."Orders_id"` +
				"\n" + `INNER JOIN "Payments" ON "Users"."Payments_id" = "Payments"."Payments_id"`,
		},
		{
			name:    "orderby by field and direction, raw by expression",
			payload: `{"from": ["Users"], "orderby": [{"by": "Name"}, {"by": "Age", "xsc": "desc"}, {"by": "mod(Id, 2)", "xsc": "desc"}]}`,
			sql:     `SELECT * FROM "Users" ORDER BY Name ASC, Age DESC, mod(Id, 2) DESC`,
		},
		{
			name:    "top applies limit",
			payload: `{"from": ["Users"], "top": 5}`,
			sql:     `SELECT * FROM "Users" LIMIT ?`,
			args:    []any{5},
		},
		{
			name:    "top with skip",
			payload: `{"from": ["Users"], "top": 5, "skip": 10}`,
			sql:     `SELECT * FROM "Users" LIMIT ? OFFSET ?`,
			args:    []any{5, int64(10)},
		},
		{
			name:    "skip without top is not applied",
			payload: `{"from": ["Users"], "skip": 10}`,
			sql:     `SELECT * FROM "Users"`,
		},
		{
			name:    "explicit zero top omits limit clause",
			payload: `{"from": ["Users"], "top": 0}`,
			sql:     `SELECT * FROM "Users"`,
		},
		{
			name:    "count builds aggregate query ignoring select orderby top",
			payload: `{"from": ["Users"], "select": ["Id"], "orderby": [{"by": "Name"}], "top": 5, "count": true}`,
			sql:     `SELECT COUNT(*) AS "count" FROM "Users"`,
		},
		{
			name:    "count keeps convention joins",
			payload: `{"from": ["Users", "Orders"], "count": true}`,
			sql: `SELECT COUNT(*) AS "count" FROM "Users"` +
				" \n" + `INNER JOIN "Orders" ON "Users"."Orders_id" = "Orders"."Orders_id"`,
		},
		{
			name:    "filter rule compiles to where condition",
			payload: `{"from": ["Users"], "filter": {"group_op": "and", "rules": [{"field": "Age", "op": "gt", "data": 18}]}}`,
			sql:     `SELECT * FROM "Users" WHERE "Age" > ?`,
			args:    []any{float64(18)},
		},
		{
			name:    "caller chooses dialect compiler",
			payload: `{"from": ["Users"], "select": ["Id"], "top": 20}`,
			comp:    compiler.NewSqlserver(),
			sql:     `SELECT TOP (?) [Id] FROM [Users]`,
			args:    []any{20},
		},
	})
}

func TestToQueryRejectsInvalid(t *testing.T) {
	q := mustUnmarshal(t, `{"select": ["Id"]}`)
	if _, err := q.ToQuery(nil); !errors.Is(err, ErrFromRequired) {
		t.Errorf("ToQuery(nil) error = %v, want ErrFromRequired", err)
	}
}

// stubHook assembles a Hook from function fields; unset pointcuts pass
// through.
type stubHook struct {
	from     func([]string) ([]string, error)
	selects  func(string) (string, error)
	orderBys func(string) (string, error)
	rules    func(Rule) (Rule, error)
}

func (h stubHook) From(from []string) ([]string, error) {
	if h.from == nil {
		return from, nil
	}
	return h.from(from)
}

func (h stubHook) Select(column string) (string, error) {
	if h.selects == nil {
		return column, nil
	}
	return h.selects(column)
}

func (h stubHook) OrderBy(by string) (string, error) {
	if h.orderBys == nil {
		return by, nil
	}
	return h.orderBys(by)
}

func (h stubHook) Rule(rule Rule) (Rule, error) {
	if h.rules == nil {
		return rule, nil
	}
	return h.rules(rule)
}

func TestToQueryHook(t *testing.T) {
	t.Run("nil hook passes through", func(t *testing.T) {
		query, err := mustUnmarshal(t, `{"from": ["Users"], "select": ["Id"]}`).ToQuery(nil)
		if err != nil {
			t.Fatalf("ToQuery(nil) error = %v, want nil", err)
		}
		res := mustCompile(t, compiler.New(), query)
		if res.SQL != `SELECT "Id" FROM "Users"` {
			t.Errorf("SQL = %q, want %q", res.SQL, `SELECT "Id" FROM "Users"`)
		}
	})

	t.Run("hook error propagates as is", func(t *testing.T) {
		errForbidden := errors.New("from is not allowed")
		hook := stubHook{from: func([]string) ([]string, error) { return nil, errForbidden }}
		if _, err := mustUnmarshal(t, `{"from": ["Secret"]}`).ToQuery(hook); !errors.Is(err, errForbidden) {
			t.Errorf("ToQuery(hook) error = %v, want errForbidden as is", err)
		}
	})

	t.Run("hook emptying from is rejected", func(t *testing.T) {
		hook := stubHook{from: func([]string) ([]string, error) { return nil, nil }}
		if _, err := mustUnmarshal(t, `{"from": ["Users"]}`).ToQuery(hook); !errors.Is(err, ErrFromRequired) {
			t.Errorf("ToQuery(hook) error = %v, want ErrFromRequired", err)
		}
	})

	t.Run("hook emptying orderby by is rejected", func(t *testing.T) {
		hook := stubHook{orderBys: func(string) (string, error) { return "", nil }}
		payload := `{"from": ["Users"], "orderby": [{"by": "Name"}]}`
		if _, err := mustUnmarshal(t, payload).ToQuery(hook); !errors.Is(err, ErrOrderByByRequired) {
			t.Errorf("ToQuery(hook) error = %v, want ErrOrderByByRequired", err)
		}
	})

	t.Run("hook rewrites from list joins included", func(t *testing.T) {
		hook := stubHook{from: func(from []string) ([]string, error) {
			return []string{"People", from[0], "Payments"}, nil
		}}
		query, err := mustUnmarshal(t, `{"from": ["Users", "Orders"]}`).ToQuery(hook)
		if err != nil {
			t.Fatalf("ToQuery(hook) error = %v, want nil", err)
		}
		res := mustCompile(t, compiler.New(), query)
		want := `SELECT * FROM "People"` +
			" \n" + `INNER JOIN "Users" ON "People"."Users_id" = "Users"."Users_id"` +
			"\n" + `INNER JOIN "Payments" ON "People"."Payments_id" = "Payments"."Payments_id"`
		if res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
	})

	t.Run("hook dropping include removes join", func(t *testing.T) {
		hook := stubHook{from: func(from []string) ([]string, error) {
			return from[:1], nil
		}}
		query, err := mustUnmarshal(t, `{"from": ["Users", "Secret"]}`).ToQuery(hook)
		if err != nil {
			t.Fatalf("ToQuery(hook) error = %v, want nil", err)
		}
		res := mustCompile(t, compiler.New(), query)
		if res.SQL != `SELECT * FROM "Users"` {
			t.Errorf("SQL = %q, want %q", res.SQL, `SELECT * FROM "Users"`)
		}
	})

	t.Run("hook transforms select and orderby", func(t *testing.T) {
		hook := stubHook{
			selects:  func(column string) (string, error) { return "u." + column, nil },
			orderBys: func(by string) (string, error) { return "lower(" + by + ")", nil },
		}
		query, err := mustUnmarshal(t, `{"from": ["Users"], "select": ["Name", "Age"], "orderby": [{"by": "Name"}]}`).ToQuery(hook)
		if err != nil {
			t.Fatalf("ToQuery(hook) error = %v, want nil", err)
		}
		res := mustCompile(t, compiler.New(), query)
		want := `SELECT "u"."Name", "u"."Age" FROM "Users" ORDER BY lower(Name) ASC`
		if res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
		if len(res.Args) != 0 {
			t.Errorf("Args = %#v, want empty", res.Args)
		}
	})

	t.Run("select transformed into raw expression", func(t *testing.T) {
		hook := stubHook{selects: func(column string) (string, error) {
			if column == "total" {
				return "count(*)", nil
			}
			return column, nil
		}}
		query, err := mustUnmarshal(t, `{"from": ["Users"], "select": ["total"]}`).ToQuery(hook)
		if err != nil {
			t.Fatalf("ToQuery(hook) error = %v, want nil", err)
		}
		res := mustCompile(t, compiler.New(), query)
		if res.SQL != `SELECT count(*) FROM "Users"` {
			t.Errorf("SQL = %q, want %q", res.SQL, `SELECT count(*) FROM "Users"`)
		}
	})

	t.Run("count query goes through from hook only", func(t *testing.T) {
		hook := stubHook{
			from:    func([]string) ([]string, error) { return []string{"People"}, nil },
			selects: func(string) (string, error) { return "Id", nil },
		}
		query, err := mustUnmarshal(t, `{"from": ["Users"], "select": ["Name"], "count": true}`).ToQuery(hook)
		if err != nil {
			t.Fatalf("ToQuery(hook) error = %v, want nil", err)
		}
		res := mustCompile(t, compiler.New(), query)
		if res.SQL != `SELECT COUNT(*) AS "count" FROM "People"` {
			t.Errorf("SQL = %q, want %q", res.SQL, `SELECT COUNT(*) AS "count" FROM "People"`)
		}
	})
}

func TestToQueryHookChain(t *testing.T) {
	t.Run("hooks run in argument order, each seeing the previous rewrite", func(t *testing.T) {
		first := stubHook{selects: func(column string) (string, error) { return "a." + column, nil }}
		second := stubHook{selects: func(column string) (string, error) { return "b." + column, nil }}
		query, err := mustUnmarshal(t, `{"from": ["Users"], "select": ["Name"]}`).ToQuery(first, second)
		if err != nil {
			t.Fatalf("ToQuery(first, second) error = %v, want nil", err)
		}
		// first prefixes "a.", second prefixes "b." on top: the compiled
		// three-part identifier proves second saw first's rewrite.
		res := mustCompile(t, compiler.New(), query)
		if res.SQL != `SELECT "b"."a"."Name" FROM "Users"` {
			t.Errorf("SQL = %q, want %q", res.SQL, `SELECT "b"."a"."Name" FROM "Users"`)
		}
	})

	t.Run("error from a later hook propagates as is", func(t *testing.T) {
		errDenied := errors.New("select is not allowed")
		second := stubHook{selects: func(string) (string, error) { return "", errDenied }}
		if _, err := mustUnmarshal(t, `{"from": ["Users"], "select": ["Name"]}`).ToQuery(stubHook{}, second); !errors.Is(err, errDenied) {
			t.Errorf("ToQuery(stubHook{}, second) error = %v, want errDenied as is", err)
		}
	})

	t.Run("later hook can still tighten validation", func(t *testing.T) {
		second := stubHook{from: func([]string) ([]string, error) { return nil, nil }}
		if _, err := mustUnmarshal(t, `{"from": ["Users"]}`).ToQuery(stubHook{}, second); !errors.Is(err, ErrFromRequired) {
			t.Errorf("ToQuery(stubHook{}, second) error = %v, want ErrFromRequired", err)
		}
	})

	t.Run("no hooks and a single nil entry behave the same", func(t *testing.T) {
		payload := `{"from": ["Users"], "select": ["Id", "Name"]}`
		plain, err := mustUnmarshal(t, payload).ToQuery()
		if err != nil {
			t.Fatalf("ToQuery() error = %v, want nil", err)
		}
		withNil, err := mustUnmarshal(t, payload).ToQuery(nil)
		if err != nil {
			t.Fatalf("ToQuery(nil) error = %v, want nil", err)
		}
		a := mustCompile(t, compiler.New(), plain)
		b := mustCompile(t, compiler.New(), withNil)
		if a.SQL != b.SQL || len(a.Args) != len(b.Args) {
			t.Errorf("ToQuery() = %q, ToQuery(nil) = %q, want identical output", a.SQL, b.SQL)
		}
	})
}

// mustCompile compiles and asserts no error, for subtests asserting the
// result directly.
func mustCompile(t *testing.T, comp *compiler.Compiler, q *sqlk.Query) compiler.Result {
	t.Helper()
	res, err := comp.Compile(q)
	if err != nil {
		t.Fatalf("Compile(...) error = %v, want nil", err)
	}
	return res
}

func TestProgrammaticBuildCompiles(t *testing.T) {
	t.Run("fluent with methods mirror wire fields", func(t *testing.T) {
		q := New().
			WithFrom("Users").
			WithSelect("Id", "Name").
			WithOrderBy(*NewOrderBy("Name", OrderByDesc)).
			WithFilter(*NewFilter().WithGroupOp(GroupOpAnd).WithRule(*NewRule("Age", "gt", 18))).
			WithTop(5).WithSkip(10)
		res := mustCompile(t, compiler.New(), mustToQuery(t, q))
		want := `SELECT "Id", "Name" FROM "Users" WHERE "Age" > ? ORDER BY Name DESC LIMIT ? OFFSET ?`
		if res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
		if !reflect.DeepEqual(res.Args, []any{18, 5, int64(10)}) {
			t.Errorf("Args = %#v, want [18 5 10]", res.Args)
		}
	})

	t.Run("new query emits no limit clause", func(t *testing.T) {
		q := New().WithFrom("Users")
		res := mustCompile(t, compiler.New(), mustToQuery(t, q))
		if res.SQL != `SELECT * FROM "Users"` {
			t.Errorf("SQL = %q, want %q", res.SQL, `SELECT * FROM "Users"`)
		}
	})

	t.Run("variadic from builds convention joins", func(t *testing.T) {
		q := New().WithFrom("Users", "Orders")
		res := mustCompile(t, compiler.New(), mustToQuery(t, q))
		want := `SELECT * FROM "Users"` +
			" \n" + `INNER JOIN "Orders" ON "Users"."Orders_id" = "Orders"."Orders_id"`
		if res.SQL != want {
			t.Errorf("SQL = %q, want %q", res.SQL, want)
		}
	})
}

// mustToQuery converts and asserts no error, for subtests asserting the
// result directly.
func mustToQuery(t *testing.T, q *QData) *sqlk.Query {
	t.Helper()
	query, err := q.ToQuery(nil)
	if err != nil {
		t.Fatalf("ToQuery(nil) error = %v, want nil", err)
	}
	return query
}
