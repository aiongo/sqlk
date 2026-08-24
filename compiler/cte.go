package compiler

import (
	"strings"

	"github.com/aiongo/sqlk/internal/core"
)

// CTE collection and compilation: CTEs found in the query tree are
// prepended to the compiled output as a WITH clause, their bound
// arguments preceding the body's.

// cteFinder collects the CTE clauses of a query tree: aliases are
// deduplicated, nested definitions are placed before their referents, and
// collection covers every nested scope, so CTEs inside projections,
// source tables, conditions, and join targets are all collected. The
// query is only read, never modified.
type cteFinder struct {
	engine string
	seen   map[string]struct{}
}

// find returns the CTEs in the query (including all its nested scopes),
// ordered by declaration order at each level, nested dependencies first,
// and subquery discoveries after; only the first occurrence of a
// duplicated alias is kept.
func (f *cteFinder) find(q *core.Query) []core.Clause {
	return f.findClauses(q.Clauses())
}

// findClauses collects CTEs from a clause list (query, join scope, or
// condition group).
func (f *cteFinder) findClauses(clauses []core.Clause) []core.Clause {
	var result []core.Clause
	for _, cl := range core.Components(clauses, core.Cte, f.engine) {
		alias := cteAlias(cl)
		if _, dup := f.seen[alias]; dup {
			continue
		}
		f.seen[alias] = struct{}{}
		result = append(result, cl)
		if cte, ok := cl.(*core.QueryCTEClause); ok {
			// Nested definitions are hoisted before their referents:
			// referenced CTEs must appear earlier in the WITH list.
			result = append(f.findClauses(cte.Query.Clauses()), result...)
		}
	}
	for _, cl := range clauses {
		if cl.Tag() == core.Cte || !f.visible(cl) {
			continue
		}
		if sub := core.SubQueryOf(cl); sub != nil {
			result = append(result, f.findClauses(sub.Clauses())...)
		}
		if join := core.JoinOf(cl); join != nil {
			result = append(result, f.findClauses(join.Clauses())...)
		}
		if group := core.GroupOf(cl); group != nil {
			result = append(result, f.findClauses(group.Clauses())...)
		}
	}
	return result
}

// visible reports whether the clause is visible to the current dialect
// (the same rule the core.Components scope filter applies, reused for
// descending into non-Cte sections).
func (f *cteFinder) visible(cl core.Clause) bool {
	return f.engine == "" || cl.Engine() == "" || cl.Engine() == f.engine
}

// withCTEs prepends the CTEs collected from the query tree to the
// compiled output as a WITH clause, with CTE arguments preceding the
// body's; returned unchanged when there are no CTEs.
func (c *Compiler) withCTEs(res Result, q *core.Query) Result {
	finder := &cteFinder{engine: c.engineCode, seen: map[string]struct{}{}}
	ctes := finder.find(q)
	if len(ctes) == 0 {
		return res
	}
	parts := make([]string, 0, len(ctes))
	var cteRes Result
	for _, cl := range ctes {
		parts = append(parts, c.compileCTE(&cteRes, cl))
	}
	res.SQL = "WITH " + strings.Join(parts, ",\n") + "\n" + res.SQL
	res.Args = append(cteRes.Args, res.Args...)
	return res
}

// compileCTE compiles a single CTE definition as "alias AS (body)",
// recording bindings in res. The body compiles with itself as the root
// scope: its output is hoisted into the outermost WITH, decoupled from
// its definition site, and does not look up variables along the defining
// chain.
func (c *Compiler) compileCTE(res *Result, cl core.Clause) string {
	switch cte := cl.(type) {
	case *core.QueryCTEClause:
		sub := c.compileSelect(cte.Query, nil)
		res.Args = append(res.Args, sub.Args...)
		return c.wrapValue(cte.Alias) + " AS (" + sub.SQL + ")"
	case *core.RawCTEClause:
		res.Args = append(res.Args, cte.Bindings...)
		return c.wrapValue(cte.Alias) + " AS (" + c.wrapIdentifiers(cte.Expression) + ")"
	case *core.AdHocTableCTEClause:
		// Shape already guaranteed by validation: every row's value count
		// matches the column count.
		return c.wrapValue(cte.Alias) + " AS (" + c.adhocTableForm(res, cte.Columns, cte.Rows) + ")"
	default:
		// CTE clause forms are a closed set within the library.
		return ""
	}
}

// standardAdHocTable is the default ad-hoc value-table body: each row is
// a "SELECT ? AS column [, ...] [FROM dummy]" projection, and multiple
// rows are joined with UNION ALL (the dummy table comes from
// singleRowDummyTable).
func (c *Compiler) standardAdHocTable(res *Result, columns []string, rows [][]any) string {
	wrapped := make([]string, len(columns))
	for i, col := range columns {
		wrapped[i] = "? AS " + c.wrap(col)
	}
	row := "SELECT " + strings.Join(wrapped, ", ")
	if c.singleRowDummyTable != "" {
		row += " FROM " + c.singleRowDummyTable
	}
	parts := make([]string, len(rows))
	for i, values := range rows {
		parts[i] = row
		for _, value := range values {
			c.parameter(res, value)
		}
	}
	return strings.Join(parts, " UNION ALL ")
}

// validateCTE aggregates the shape problems of a single CTE clause
// (missing alias, malformed ad-hoc value table).
func validateCTE(cl core.Clause) []error {
	alias := cteAlias(cl)
	if strings.TrimSpace(alias) == "" {
		return []error{ErrCTEMissingAlias}
	}
	table, ok := cl.(*core.AdHocTableCTEClause)
	if !ok {
		return nil
	}
	if len(table.Columns) == 0 || len(table.Rows) == 0 {
		return []error{&CTETableError{Alias: alias, Columns: len(table.Columns), Rows: len(table.Rows)}}
	}
	for _, row := range table.Rows {
		if len(row) != len(table.Columns) {
			return []error{&CTETableError{
				Alias:   alias,
				Columns: len(table.Columns),
				Rows:    len(table.Rows),
				Values:  len(row),
			}}
		}
	}
	return nil
}

// cteAlias returns the CTE clause's alias (empty for non-CTE clauses).
func cteAlias(cl core.Clause) string {
	switch cte := cl.(type) {
	case *core.QueryCTEClause:
		return cte.Alias
	case *core.RawCTEClause:
		return cte.Alias
	case *core.AdHocTableCTEClause:
		return cte.Alias
	}
	return ""
}
