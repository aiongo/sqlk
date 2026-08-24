package tutorial

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/aiongo/sqlk/compiler"
	"github.com/aiongo/sqlk/qdata"
)

// Examples from qdata.md: JSON wire format -> qdata.QData -> core Query ->
// dialect compile.

// compileQData unmarshals the wire-format JSON, converts it, and compiles it
// with the sqlite dialect (the seam that verifies the doc examples: JSON ->
// qdata.QData -> ToQuery -> Compile).
func compileQData(t *testing.T, payload string, hook qdata.Hook, wantSQL string, wantArgs ...any) {
	t.Helper()
	var q qdata.QData
	if err := json.Unmarshal([]byte(payload), &q); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	query, err := q.ToQuery(hook)
	if err != nil {
		t.Fatalf("ToQuery: %v", err)
	}
	assertSQL(t, compiler.NewSqlite(), query, wantSQL, wantArgs...)
}

func TestQDataFrontPage(t *testing.T) {
	// The wire format in full: from/select/filter (with nested groups)/
	// orderby/top/skip. Note: encoding/json decodes JSON numbers as float64,
	// and the argument sequence carries them as such.
	compileQData(t, `{
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
	}`, nil,
		`SELECT "Id", "Title", count(1) as Count FROM "Posts" WHERE "Status" = ? AND "Score" >= ? AND ("Title" like ? OR "Title" like ?) ORDER BY CreatedAt DESC LIMIT ?`,
		"active", float64(10), "Go%", "%kata", 20)
}

func TestQDataDefaults(t *testing.T) {
	// Pagination has no default: a missing top emits no LIMIT clause;
	// group_op defaults to and.
	compileQData(t, `{"from": ["Posts"]}`, nil,
		`SELECT * FROM "Posts"`)
}

func TestQDataConventionJoins(t *testing.T) {
	// The first from element is the main table; the remaining elements become
	// conventional JOINs: `<main>.<x>_id = <x>.<x>_id`.
	compileQData(t, `{"from": ["Posts", "Authors"], "top": 5}`, nil,
		`SELECT * FROM "Posts"`+
			" \n"+`INNER JOIN "Authors" ON "Posts"."Authors_id" = "Authors"."Authors_id"`+
			` LIMIT ?`,
		5)
}

func TestQDataOperatorSemantics(t *testing.T) {
	// bw/ew/cn produce prefix/suffix/contains patterns; is/ns produce NULL
	// tests; in accepts an array or a single scalar as data; a missing (or
	// zero) top emits no LIMIT clause.
	compileQData(t, `{
		"from": ["Posts"],
		"filter": {"rules": [
			{"field": "AuthorId", "op": "ns"},
			{"field": "Lang", "op": "in", "data": "en"},
			{"field": "Title", "op": "cn", "data": "go"}
		]},
		"top": 0
	}`, nil,
		`SELECT * FROM "Posts" WHERE "AuthorId" IS NOT NULL AND "Lang" IN (?) AND "Title" like ?`,
		"en", "%go%")
}

func TestQDataCount(t *testing.T) {
	// count=true builds a COUNT aggregate query (WHERE and conventional JOINs
	// are kept; projection, ordering, and pagination are not applied).
	compileQData(t, `{
		"from": ["Posts"],
		"filter": {"rules": [{"field": "Status", "op": "eq", "data": "active"}]},
		"count": true
	}`, nil,
		`SELECT COUNT(*) AS "count" FROM "Posts" WHERE "Status" = ?`, "active")
}

func TestQDataProgrammatic(t *testing.T) {
	// Programmatic construction: the New entry point with With* chaining;
	// pagination has no default, use WithTop explicitly when needed.
	q := qdata.New().
		WithFrom("Posts").
		WithSelect("Id", "Title").
		WithFilter(*qdata.NewFilter().
			WithRule(*qdata.NewRule("Status", qdata.OpEq, "active")).
			WithRule(*qdata.NewRule("Score", qdata.OpGt, 10))).
		WithOrderBy(*qdata.NewOrderBy("CreatedAt", "desc")).
		WithTop(20).
		WithCount(false)
	query, err := q.ToQuery()
	if err != nil {
		t.Fatalf("ToQuery: %v", err)
	}
	assertSQL(t, compiler.NewSqlite(), query,
		`SELECT "Id", "Title" FROM "Posts" WHERE "Status" = ? AND "Score" > ? ORDER BY CreatedAt DESC LIMIT ?`,
		"active", 10, 20)
}

// allowHook is the hook example from the docs: a column allowlist guard.
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

func TestQDataHook(t *testing.T) {
	hook := allowHook{columns: map[string]bool{"Id": true, "Title": true, "Status": true}}
	compileQData(t, `{
		"from": ["Posts"],
		"select": ["Id", "Title"],
		"filter": {"rules": [{"field": "Status", "op": "eq", "data": "active"}]}
	}`, hook,
		`SELECT "Id", "Title" FROM "Posts" WHERE "Status" = ?`,
		"active")

	// Fields outside the allowlist are rejected by the hook and the error
	// propagates unchanged.
	var q qdata.QData
	if err := json.Unmarshal([]byte(`{
		"from": ["Posts"],
		"select": ["Id", "Password"]
	}`), &q); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	_, err := q.ToQuery(hook)
	if err == nil || !strings.Contains(err.Error(), "column not allowed: Password") {
		t.Fatalf("ToQuery error = %v, want column not allowed: Password", err)
	}
}

func TestQDataValidation(t *testing.T) {
	// Validate aggregates: it returns all problems at once, each matched via
	// errors.Is/As.
	var q qdata.QData
	if err := json.Unmarshal([]byte(`{
		"filter": {"group_op": "xor", "rules": [
			{"field": "Status", "op": "matches", "data": "x"},
			{"field": "", "op": "eq", "data": 1}
		]},
		"orderby": [{"by": "Date", "xsc": "down"}],
		"top": -1
	}`), &q); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	err := q.Validate()
	for _, want := range []error{
		qdata.ErrFromRequired,
		qdata.ErrInvalidGroupOp,
		qdata.ErrInvalidOp,
		qdata.ErrRuleFieldRequired,
		qdata.ErrInvalidOrderByDirection,
		qdata.ErrInvalidPagination,
	} {
		if !errors.Is(err, want) {
			t.Errorf("Validate error = %v, want errors.Is(%v)", err, want)
		}
	}
}
