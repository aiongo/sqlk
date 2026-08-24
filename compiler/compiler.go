// Package compiler compiles queries into placeholder SQL and ordered
// arguments. Placeholders are uniformly "?" across dialects; named-parameter
// views and rebinding are left to sqlx conventions. New returns the base
// compiler; each supported dialect has its own constructor.
package compiler

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/internal/core"
)

// Compiler compiles a query into placeholder SQL and ordered arguments.
// The base implementation quotes identifiers with double quotes and
// paginates with LIMIT ?/OFFSET ?; dialect differences are expressed as
// overridable hooks.
type Compiler struct {
	// engineCode is the compiler's dialect code; the empty string is the
	// base compiler. Clauses are filtered by engine scope against it.
	engineCode        string
	openingIdentifier string
	closingIdentifier string

	// columnAsKeyword and tableAsKeyword are the AS keywords for column and
	// table aliases (with a trailing space; the empty string omits the
	// keyword and joins the alias with a plain space, as in Oracle). Set by
	// dialect constructors.
	columnAsKeyword string
	tableAsKeyword  string

	// trueLiteral and falseLiteral are the dialect's boolean literals (e.g.
	// SQL Server's cast(1 as bit)). Set by dialect constructors.
	trueLiteral  string
	falseLiteral string

	// randomFunc is the dialect's random-ordering function (RANDOM() by
	// default; NEWID() on SQL Server). Set by dialect constructors.
	randomFunc string

	// omitSelectInsideExists reports whether compiling an EXISTS condition
	// replaces the subquery's projection with the constant 1, dropping the
	// SELECT list; true by default.
	omitSelectInsideExists bool

	// supportsFilterClause reports whether the dialect supports aggregate
	// FILTER (WHERE ...) clauses (PostgreSQL and SQLite do). Dialects
	// without it degrade a projected aggregate's filter to the equivalent
	// CASE WHEN form; false by default.
	supportsFilterClause bool

	// singleRowDummyTable is the dummy table that the single-row
	// projections of an ad-hoc value-table CTE select from (empty by
	// default, i.e. no FROM; Oracle selects from DUAL). Set by dialect
	// constructors.
	singleRowDummyTable string

	// singleInsertStart and multiInsertStart open single-row and multi-row
	// INSERTs; lastID is the last-insert-id statement appended for the
	// returnId form (empty by default, so returnId appends nothing). Set by
	// dialect constructors.
	singleInsertStart string
	multiInsertStart  string
	lastID            string

	// deleteWithJoinForm renders a DELETE with joins for the dialect: it
	// receives the target table, the delete target (the alias or the table
	// itself), and the compiled join section (starting with a newline);
	// the caller appends WHERE afterwards. The default form is
	// "DELETE target FROM table joins".
	deleteWithJoinForm func(table, target, joins string) string

	// limitOffsetForm renders the pagination section: it receives the
	// clause list (for dialects that need context, e.g. to add
	// ORDER BY (SELECT 0) when the query is unordered), plus the parsed
	// limit and offset (zero counts as unset), and returns the full
	// section text (the empty string emits nothing), binding limit/offset
	// via parameter as its form requires. The field holds a method value
	// bound to this compiler: New installs standardLimitOffset and dialect
	// constructors replace it (SQLite's OFFSET-only form with LIMIT -1,
	// SQL Server's OFFSET-FETCH).
	limitOffsetForm func(res *Result, clauses []core.Clause, limit int, offset int64) string

	// selectTopClause renders the head fragment injected after
	// "SELECT [DISTINCT] " when a dialect folds a limit-only pagination
	// into the SELECT head (SQL Server's TOP): it receives the parsed
	// limit and offset (zero counts as unset) and returns the fragment
	// with a trailing space (the empty string injects nothing). The limit
	// argument is bound by the implementation and must precede projection
	// arguments. New installs a default returning the empty string;
	// dialect constructors replace it.
	selectTopClause func(res *Result, limit int, offset int64) string

	// adhocTableForm renders the body of an ad-hoc value-table CTE: it
	// receives column names and row values (values may be Variable, bound
	// via parameter) and returns the value table's SELECT statement; the
	// surrounding "alias AS (body)" is added by compileCTE. New installs
	// standardAdHocTable (single-row projections joined by UNION ALL,
	// selecting from singleRowDummyTable); dialect constructors replace it
	// (SQL Server's VALUES table form).
	adhocTableForm func(res *Result, columns []string, rows [][]any) string

	// dateConditionForm renders a date-part condition: the implementation
	// produces the comparison text via wrap/operator/parameter. New
	// installs standardDateCondition (the "PART(column) operator ?" form);
	// dialect constructors replace it (e.g. SQLite's strftime form).
	dateConditionForm func(res *Result, cond *core.DateCondition) string

	// stringConditionForm renders a LIKE-family condition: the
	// implementation produces the comparison text via
	// wrap/operator/parameter (wildcard concatenation, case handling, the
	// ESCAPE clause, and overall negation). New installs
	// standardStringCondition (case-insensitive matching via LOWER(column)
	// with a lowercased value); dialect constructors replace it (e.g.
	// PostgreSQL's ILIKE form).
	stringConditionForm func(res *Result, cond *core.StringCondition) string

	// remainingInsertForm renders the rows after the first in a multi-row
	// INSERT: it receives the compiled target table and the insert clauses
	// after the first, and returns the statement tail appended after
	// "VALUES (first row)" (with its argument bindings). New installs
	// standardRemainingInserts (later rows continue as ", (...)", the
	// multi-row VALUES form); dialect constructors replace it (e.g.
	// Oracle's INSERT ALL INTO form).
	remainingInsertForm func(res *Result, table string, inserts []*core.InsertClause) string

	// wrapSelectForm wraps the compiled select sections as a whole (Oracle
	// legacy pagination rewraps the entire SELECT after the sections are
	// compiled): it receives the section output and the query being
	// compiled (for limit/offset parsing) and returns the wrapped output,
	// with pagination arguments appended after the body's. New installs
	// the identity; dialect constructors replace it (Oracle's legacy
	// ROWNUM form). The hook applies to every select form: top-level
	// queries, subqueries, and CTE bodies share one compilation path.
	wrapSelectForm func(res Result, q *core.Query) Result

	// operators is the whitelist of Where/Having operators; operators
	// outside it are rejected at the compilation entry point. Copied per
	// instance, so Whitelist extends only a single compiler.
	operators map[string]struct{}
}

// builtinOperators is the built-in operator whitelist.
var builtinOperators = []string{
	"=", "<", ">", "<=", ">=", "<>", "!=", "<=>",
	"like", "not like",
	"ilike", "not ilike",
	"like binary", "not like binary",
	"rlike", "not rlike",
	"regexp", "not regexp",
	"similar to", "not similar to",
}

// New returns the base compiler.
func New() *Compiler {
	operators := make(map[string]struct{}, len(builtinOperators))
	for _, op := range builtinOperators {
		operators[op] = struct{}{}
	}
	c := &Compiler{
		openingIdentifier:      `"`,
		closingIdentifier:      `"`,
		columnAsKeyword:        "AS ",
		tableAsKeyword:         "AS ",
		trueLiteral:            "true",
		falseLiteral:           "false",
		randomFunc:             "RANDOM()",
		omitSelectInsideExists: true,
		singleInsertStart:      "INSERT INTO",
		multiInsertStart:       "INSERT INTO",
		deleteWithJoinForm: func(table, target, joins string) string {
			return "DELETE " + target + " FROM " + table + " " + joins
		},
		operators: operators,
	}
	c.limitOffsetForm = c.standardLimitOffset
	c.selectTopClause = c.noSelectTop
	c.adhocTableForm = c.standardAdHocTable
	c.dateConditionForm = c.standardDateCondition
	c.stringConditionForm = c.standardStringCondition
	c.remainingInsertForm = c.standardRemainingInserts
	c.wrapSelectForm = c.identityWrapSelect
	return c
}

// identityWrapSelect is the wrapSelectForm default: it returns the output
// unchanged.
func (c *Compiler) identityWrapSelect(res Result, _ *core.Query) Result {
	return res
}

// Whitelist adds operators to this compiler's whitelist so they can be used
// in Where/Having conditions; the extension affects only this instance. It
// returns the compiler for chaining.
func (c *Compiler) Whitelist(operators ...string) *Compiler {
	for _, op := range operators {
		c.operators[strings.ToLower(op)] = struct{}{}
	}
	return c
}

// SupportsLastId reports whether this dialect appends a last-insert-id
// statement to a returnId INSERT (sqlite, mysql, postgres, sqlserver do;
// the base compiler and Oracle do not). The execution layer uses it to
// reject InsertGetId up front with a distinguishable error instead of
// running an INSERT whose result scan would surface a misleading
// no-rows error.
func (c *Compiler) SupportsLastId() bool {
	return c.lastID != ""
}

// Result is the output of compilation: placeholder SQL text and arguments
// ordered to match the placeholders. scope is the variable-lookup scope
// chain the compiler maintains while descending; it is compile-time
// internal state, not part of the exported semantics.
type Result struct {
	SQL  string
	Args []any

	scope *core.VarScope
}

// Compile compiles the query. Build- and compile-time problems (missing
// from target, non-whitelisted operator, CTE missing an alias, malformed
// write clauses, undefined variable references) are aggregated into a
// single returned error distinguishable via errors.Is/As. Select queries
// are first rewritten by core.TransformAggregate (the original query is
// not modified; aggregate clauses become visible per this compiler's
// dialect); CTEs found in the query tree are then collected, deduplicated,
// and prepended as a WITH clause (see cte.go), and the tracking comment is
// prefixed last. Write verbs (insert/update/delete) are dispatched in
// write.go.
func (c *Compiler) Compile(q *sqlk.Query) (Result, error) {
	clauses := q.Clauses()
	var errs []error
	errs = append(errs, errList(c.validateScope(clauses, false, core.NewVarScope(q)))...)
	errs = append(errs, errList(c.validateWrite(q.Method(), clauses))...)
	if err := errors.Join(errs...); err != nil {
		return Result{}, err
	}

	var res Result
	if q.Method() == core.MethodSelect {
		compiled := core.TransformAggregate(q, c.engineCode)
		res = c.compileSections(compiled, nil)
		res = c.withCTEs(res, compiled)
	} else {
		res.scope = core.NewVarScope(q)
		switch q.Method() {
		case core.MethodInsert:
			res.SQL = c.compileInsert(&res, clauses)
		case core.MethodUpdate:
			res.SQL = c.compileUpdate(&res, clauses)
		case core.MethodDelete:
			res.SQL = c.compileDelete(&res, clauses)
		default:
			// Unreachable: a non-select method is only ever set by a write
			// verb, and all three are handled above.
		}
		res = c.withCTEs(res, q)
	}
	return c.withComment(res, q.CommentText()), nil
}

// errList flattens a possibly errors.Join-aggregated error into a flat
// list of individual problems, so the compilation entry point returns a
// single-level aggregation (callers read problems one by one via
// Unwrap() []error, with no nested aggregation).
func errList(err error) []error {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		return joined.Unwrap()
	}
	return []error{err}
}

// validateScope aggregates every build- or compile-time problem this
// compiler can detect. groupScope marks whether the clause list is a
// condition group or a join scope: both carry only conditions (a join
// scope also carries its join target), are never compiled as standalone
// SELECTs, and are exempt from the from-target check. scope is the
// variable-lookup scope chain: Variable references in value positions
// resolve against it, and it is pushed in lockstep with compilation
// (subqueries push, condition groups and join scopes reuse the current
// level, CTE bodies are their own root), so that validation passes
// exactly when compilation resolves.
func (c *Compiler) validateScope(clauses []core.Clause, groupScope bool, scope *core.VarScope) error {
	var errs []error

	if !groupScope && c.one(clauses, core.From) == nil {
		errs = append(errs, ErrNoFromTarget)
	}

	// where and having share one set of condition forms and one
	// whitelist; validate them together.
	for _, of := range []core.Component{core.Where, core.Having} {
		for _, cl := range c.components(clauses, of) {
			if op, column, ok := conditionOperator(cl); ok {
				if _, err := c.checkOperator(op); err != nil {
					errs = append(errs, &OperatorError{Column: column, Operator: op})
				}
			}
			if str, ok := cl.(*core.StringCondition); ok {
				if str.Escape != "" && !validEscape(str.Escape) {
					errs = append(errs, &EscapeCharacterError{Escape: str.Escape})
				}
			}
		}
	}

	// CTE shape checks (missing alias, ad-hoc value-table shape) are
	// aggregated with the condition checks; CTE bodies descend through
	// SubQueryOf below.
	for _, cl := range c.components(clauses, core.Cte) {
		errs = append(errs, validateCTE(cl)...)
	}

	// Variable references in value positions are checked one by one (the
	// same enumeration parameter dispatches; see checkVariables): an
	// unresolved reference would silently bind the zero value during
	// compilation, so it must be rejected here.
	for _, cl := range clauses {
		errs = append(errs, checkVariables(cl, scope)...)
	}

	// Embedded subqueries and condition groups are aggregated recursively
	// in the same pass; core identifies the carriers, and this loop covers
	// condition and structural clauses alike. Join scopes behave like
	// condition groups (group-scope semantics: exempt from the from-target
	// check, with ON-condition whitelist and escape checks, nested
	// subqueries, and groups all descending, variables using the current
	// scope level).
	for _, cl := range clauses {
		if join := core.JoinOf(cl); join != nil {
			if err := c.validateScope(join.Clauses(), true, scope); err != nil {
				errs = append(errs, err)
			}
		}
		if sub := core.SubQueryOf(cl); sub != nil {
			if _, ok := cl.(*core.CombineClause); ok && sub.Method() != core.MethodSelect {
				// Combine members must be select queries.
				errs = append(errs, ErrCombineNotSelect)
			}
			subScope := scope.Push(sub)
			if _, ok := cl.(*core.QueryCTEClause); ok {
				// CTE bodies are their own root: their output is hoisted
				// to the outermost level, decoupled from the definition
				// site, and does not look up variables along the defining
				// chain.
				subScope = core.NewVarScope(sub)
			}
			if err := c.validateScope(sub.Clauses(), false, subScope); err != nil {
				errs = append(errs, err)
			}
		}
		if group := core.GroupOf(cl); group != nil {
			if err := c.validateScope(group.Clauses(), true, scope); err != nil {
				errs = append(errs, err)
			}
		}
	}

	return errors.Join(errs...)
}

// checkVariables validates that Variable references in a clause's value
// positions resolve on the scope chain (the enumeration matches what
// parameter dispatches during compilation: condition values, between
// endpoints, in-list elements, write values, and ad-hoc value tables;
// bound parameters of raw expressions do not resolve variables and are
// not enumerated).
func checkVariables(cl core.Clause, scope *core.VarScope) []error {
	var values []any
	switch typed := cl.(type) {
	case *core.BasicCondition:
		values = append(values, typed.Value)
	case *core.BetweenCondition:
		values = append(values, typed.Lower, typed.Higher)
	case *core.InCondition:
		values = append(values, typed.Values...)
	case *core.SubQueryCondition:
		values = append(values, typed.Value)
	case *core.DateCondition:
		values = append(values, typed.Value)
	case *core.InsertClause:
		values = append(values, typed.Values...)
	case *core.UpdateSetClause:
		values = append(values, typed.Values...)
	case *core.AdHocTableCTEClause:
		for _, row := range typed.Rows {
			values = append(values, row...)
		}
		// Ad-hoc value tables share CTE-body semantics: their output is
		// hoisted into WITH, decoupled from the definition site, so values
		// never resolve along the defining chain (compilation runs on an
		// empty chain) and variable references are always unresolved,
		// because a value table has no Define of its own. Reject them
		// here instead of letting unresolved references reach the driver.
		scope = nil
	default:
		return nil
	}
	var errs []error
	for _, value := range values {
		if ref, ok := value.(core.Variable); ok {
			if _, found := scope.Lookup(ref.Name); !found {
				errs = append(errs, &VariableError{Name: ref.Name})
			}
		}
	}
	return errs
}

// conditionOperator extracts the operator and locating column carried by a
// condition clause (ok is false for forms without an operator).
// Two-column comparisons locate by the first column; comparisons of a
// subquery to a value have no column, and it is empty. Condition forms
// carrying operators are a closed set within the library: a new form must
// be registered here, and its whitelist-rejection path must be covered by
// a seam test (a validation gap cannot be observed in compiled output).
func conditionOperator(cl core.Clause) (op, column string, ok bool) {
	switch cond := cl.(type) {
	case *core.BasicCondition:
		return cond.Operator, cond.Column, true
	case *core.TwoColumnsCondition:
		return cond.Operator, cond.First, true
	case *core.SubQueryCondition:
		return cond.Operator, "", true
	case *core.StringCondition:
		// starts/ends/contains all compile to like; validate the
		// effective operator.
		return "like", cond.Column, true
	case *core.DateCondition:
		return cond.Operator, cond.Column, true
	}
	return "", "", false
}

// validEscape reports whether s is an admissible LIKE escape character: a
// single rune other than the string delimiter. Escaping with the delimiter
// itself would emit an invalid string literal, so the single quote is
// rejected even though it is one rune (this check is Go-side and goes
// beyond the SqlKata baseline, which only implements ESCAPE on Postgres and
// validates nothing).
func validEscape(s string) bool {
	if utf8.RuneCountInString(s) != 1 {
		return false
	}
	return s != "'"
}

// compileSelect compiles a query and prefixes its tracking comment (the
// entry point for subquery forms). parent is the scope chain established
// by the enclosing compilation; the query is pushed onto it so variable
// references in its value positions can look up the parent chain.
// Parentless compilations (CTE bodies) pass nil.
func (c *Compiler) compileSelect(q *sqlk.Query, parent *core.VarScope) Result {
	return c.withComment(c.compileSections(q, parent), q.CommentText())
}

// compileSections compiles the query's sections and joins the non-empty
// ones (join sections are separated by newlines, the rest by spaces).
// CTE prepending and comments are not handled here: both apply only to
// the outermost output, added by Compile in the order sections, then
// CTEs, then comment. parent is as in compileSelect: nil means a scope
// rooted at q itself.
func (c *Compiler) compileSections(q *sqlk.Query, parent *core.VarScope) Result {
	var res Result
	if parent == nil {
		res.scope = core.NewVarScope(q)
	} else {
		res.scope = parent.Push(q)
	}
	clauses := q.Clauses()
	sections := []string{
		c.compileColumns(&res, q),
		c.compileFrom(&res, clauses),
		c.compileJoins(&res, clauses),
		c.compileWheres(&res, clauses),
		c.compileGroups(&res, clauses),
		c.compileHavings(&res, clauses),
		c.compileOrders(&res, clauses),
		c.compileLimitOffset(&res, clauses),
		c.compileCombines(&res, clauses),
	}
	kept := sections[:0]
	for _, s := range sections {
		if s != "" {
			kept = append(kept, s)
		}
	}
	res.SQL = strings.Join(kept, " ")
	return c.wrapSelectForm(res, q)
}

// withComment prefixes the compiled output with a database-side comment
// clause (returned unchanged when there is no comment).
func (c *Compiler) withComment(res Result, comment string) Result {
	if comment == "" {
		return res
	}
	// "*/" would close the comment early; replace it with the equivalent
	// "* /" to keep the comment boundary intact.
	comment = strings.ReplaceAll(comment, "*/", "* /")
	res.SQL = "/* " + comment + " */ " + res.SQL
	return res
}

// compileColumns compiles the projection: aggregate forms take the
// aggregate branch (see compileAggregateColumns); otherwise an empty
// column list yields * and multiple columns are joined with commas, and
// the distinct flag produces DISTINCT. Dialect forms that fold a
// limit-only pagination into the SELECT head (e.g. SQL Server's TOP)
// inject it via the selectTopClause hook in both branches: top-level
// aggregate-form pagination was already stripped by the aggregate rewrite,
// but an aggregate inlined as a subquery bypasses that rewrite and keeps
// its Limit, so the aggregate branch injects TOP too (matching the C#
// base, which injects TOP after CompileColumns handles the aggregate).
func (c *Compiler) compileColumns(res *Result, q *sqlk.Query) string {
	top := c.selectTopClause(res, c.limitOf(q.Clauses()), c.offsetOf(q.Clauses()))
	if agg, ok := c.one(q.Clauses(), core.Aggregate).(*core.AggregateClause); ok {
		return c.compileAggregateColumns(agg, q.IsDistinct(), top)
	}
	columns := c.components(q.Clauses(), core.Select)
	parts := make([]string, 0, len(columns))
	for _, cl := range columns {
		parts = append(parts, c.compileColumn(res, cl))
	}
	selectList := "*"
	if len(parts) > 0 {
		selectList = strings.Join(parts, ", ")
	}
	distinct := ""
	if q.IsDistinct() {
		distinct = "DISTINCT "
	}
	return "SELECT " + distinct + top + selectList
}

// compileAggregateColumns compiles the projection of an aggregate form: a
// single column produces "TYPE(col) AS type", with DISTINCT prefixed to
// the column when distinct (reachable when an aggregate form is inlined
// as a subquery, e.g. COUNT(DISTINCT ...)); zero or multiple columns
// produce "SELECT 1" here, because the real aggregate has already moved
// to the outer query via core.TransformAggregate (top-level multi-column
// or distinct aggregates are always rewritten into a wrapping subquery).
// top is the dialect's SELECT-head fragment (e.g. SQL Server's TOP), empty
// for dialects without a head-folded limit; it is injected after SELECT so
// an aggregate subquery with a Limit keeps its pagination.
func (c *Compiler) compileAggregateColumns(agg *core.AggregateClause, distinct bool, top string) string {
	if len(agg.Columns) != 1 {
		return "SELECT " + top + "1"
	}
	sql := c.wrap(agg.Columns[0])
	if distinct {
		sql = "DISTINCT " + sql
	}
	return "SELECT " + top + strings.ToUpper(agg.Type) + "(" + sql + ") " + c.columnAsKeyword + c.wrapValue(agg.Type)
}

// compileColumn compiles a single projection clause (plain column, raw
// expression, or subquery).
func (c *Compiler) compileColumn(res *Result, cl core.Clause) string {
	switch col := cl.(type) {
	case *core.ColumnClause:
		return c.wrap(col.Name)
	case *core.RawColumnClause:
		return c.compileRaw(res, col.Expression, col.Bindings)
	case *core.QueryColumnClause:
		sql := c.compileSubQuery(res, col.Query)
		return "(" + sql + ")" + c.columnAlias(col.Query.Alias())
	case *core.AggregateColumnClause:
		return c.compileAggregateColumn(res, col)
	default:
		// Projection clause forms are a closed set within the library.
		return ""
	}
}

// compileAggregateColumn compiles a projected aggregate column: the
// aggregate wraps a single column, and an alias inside the column
// expression moves outside the aggregate parentheses. With a filter
// attached, dialects supporting the FILTER clause produce
// "TYPE(col) FILTER (WHERE ...)"; the others degrade to the CASE WHEN
// equivalent.
func (c *Compiler) compileAggregateColumn(res *Result, col *core.AggregateColumnClause) string {
	agg := strings.ToUpper(col.Aggregate)
	column, alias := splitAlias(col.Column)
	aliasSQL := c.columnAlias(alias)
	compiled := agg + "(" + c.wrap(column) + ")" + aliasSQL
	if col.Filter == nil {
		return compiled
	}
	filter := c.compileConditions(res, c.components(col.Filter.Clauses(), core.Where))
	if filter == "" {
		return compiled
	}
	if c.supportsFilterClause {
		return agg + "(" + c.wrap(column) + ") FILTER (WHERE " + filter + ")" + aliasSQL
	}
	return agg + "(CASE WHEN " + filter + " THEN " + c.wrap(column) + " END)" + aliasSQL
}

// compileCombines compiles the combine section: each combine clause
// compiles to "OPERATION [ALL] member-SELECT" or a raw expression, joined
// with spaces and appended to the statement tail. Members compile as
// standalone SELECTs: their own pagination/ordering is inlined per their
// own semantics, and CTEs inside members are hoisted to the outermost
// WITH by withCTEs. The main query's own pagination section precedes the
// combine section.
func (c *Compiler) compileCombines(res *Result, clauses []core.Clause) string {
	combines := c.components(clauses, core.Combine)
	if len(combines) == 0 {
		return ""
	}
	parts := make([]string, 0, len(combines))
	for _, cl := range combines {
		switch combine := cl.(type) {
		case *core.CombineClause:
			operation := strings.ToUpper(combine.Operation)
			if combine.All {
				operation += " ALL"
			}
			parts = append(parts, operation+" "+c.compileSubQuery(res, combine.Query))
		case *core.RawCombineClause:
			parts = append(parts, c.compileRaw(res, combine.Expression, combine.Bindings))
		default:
			// Combine clause forms are a closed set within the library.
			continue
		}
	}
	return strings.Join(parts, " ")
}

// compileSubQuery compiles an embedded subquery, merges its arguments
// into the outer argument sequence, and returns its SQL; the subquery is
// pushed onto the outer scope chain so variable references in its value
// positions can look up the parent chain.
func (c *Compiler) compileSubQuery(res *Result, sub *core.Query) string {
	subRes := c.compileSelect(sub, res.scope)
	res.Args = append(res.Args, subRes.Args...)
	return subRes.SQL
}

// columnAlias renders the " AS alias" fragment (empty when there is no
// alias).
func (c *Compiler) columnAlias(alias string) string {
	if alias == "" {
		return ""
	}
	return " " + c.columnAsKeyword + c.wrapValue(alias)
}

// compileFrom compiles the source table (a missing from target is already
// prevented by validation).
func (c *Compiler) compileFrom(res *Result, clauses []core.Clause) string {
	from := c.one(clauses, core.From)
	return "FROM " + c.compileTable(res, from)
}

// compileTable compiles the single form of a source-table clause: table
// name, raw expression, or subquery.
func (c *Compiler) compileTable(res *Result, cl core.Clause) string {
	switch from := cl.(type) {
	case *core.FromClause:
		return c.wrap(from.Table)
	case *core.RawFromClause:
		return c.compileRaw(res, from.Expression, from.Bindings)
	case *core.QueryFromClause:
		sql := c.compileSubQuery(res, from.Query)
		return "(" + sql + ")" + c.tableAlias(from.Query.Alias())
	default:
		// Source-table forms are a closed set within the library.
		return ""
	}
}

// tableAlias renders the source table's " AS alias" fragment (empty when
// there is no alias).
func (c *Compiler) tableAlias(alias string) string {
	if alias == "" {
		return ""
	}
	return " " + c.tableAsKeyword + c.wrapValue(alias)
}

// compileJoins compiles joins: each join renders as
// "TYPE target [ON conditions]", separated by newlines between joins and
// between the FROM section and the first join.
func (c *Compiler) compileJoins(res *Result, clauses []core.Clause) string {
	joins := c.components(clauses, core.Joins)
	if len(joins) == 0 {
		return ""
	}
	parts := make([]string, 0, len(joins))
	for _, cl := range joins {
		join := core.JoinOf(cl)
		if join == nil {
			// Join-section clause forms are a closed set within the
			// library (the Join type itself).
			continue
		}
		parts = append(parts, c.compileJoin(res, join))
	}
	return "\n" + strings.Join(parts, "\n")
}

// compileJoin compiles one join: the target reuses the from-section table
// compilation and the ON conditions reuse the condition compilation; an
// empty condition set (e.g. CROSS JOIN or an empty callback) omits ON.
func (c *Compiler) compileJoin(res *Result, join *core.Join) string {
	target := c.one(join.Clauses(), core.From)
	sql := join.Type() + " " + c.compileTable(res, target)
	conditions := c.compileConditions(res, c.components(join.Clauses(), core.Where))
	if conditions != "" {
		sql += " ON " + conditions
	}
	return sql
}

// compileWheres compiles filter conditions: the condition list is joined
// by compileConditions and prefixed with WHERE.
func (c *Compiler) compileWheres(res *Result, clauses []core.Clause) string {
	return c.compileConditionSection(res, clauses, core.Where, "WHERE")
}

// compileHavings compiles post-grouping filters: the same condition
// compilation surface as Where, prefixed with HAVING.
func (c *Compiler) compileHavings(res *Result, clauses []core.Clause) string {
	return c.compileConditionSection(res, clauses, core.Having, "HAVING")
}

// compileConditionSection compiles one condition section: the list is
// joined by compileConditions and prefixed with the section keyword; an
// empty list emits no section.
func (c *Compiler) compileConditionSection(res *Result, clauses []core.Clause, of core.Component, keyword string) string {
	sql := c.compileConditions(res, c.components(clauses, of))
	if sql == "" {
		return ""
	}
	return keyword + " " + sql
}

// compileGroups compiles grouping: group columns are joined with commas
// and prefixed with GROUP BY (group columns reuse the projection column
// forms).
func (c *Compiler) compileGroups(res *Result, clauses []core.Clause) string {
	columns := c.components(clauses, core.Group)
	if len(columns) == 0 {
		return ""
	}
	parts := make([]string, 0, len(columns))
	for _, cl := range columns {
		parts = append(parts, c.compileColumn(res, cl))
	}
	return "GROUP BY " + strings.Join(parts, ", ")
}

// compileOrders compiles ordering: order columns are joined with commas
// and prefixed with ORDER BY; descending appends DESC, raw expressions
// are emitted as-is after identifier-marker substitution, and the
// random-order placeholder compiles to the dialect's random function.
func (c *Compiler) compileOrders(res *Result, clauses []core.Clause) string {
	orders := c.components(clauses, core.Order)
	if len(orders) == 0 {
		return ""
	}
	parts := make([]string, 0, len(orders))
	for _, cl := range orders {
		switch order := cl.(type) {
		case *core.OrderClause:
			sql := c.wrap(order.Column)
			if !order.Ascending {
				sql += " DESC"
			}
			parts = append(parts, sql)
		case *core.RawOrderClause:
			parts = append(parts, c.compileRaw(res, order.Expression, order.Bindings))
		case *core.RandomOrderClause:
			parts = append(parts, c.randomFunc)
		default:
			// Order clause forms are a closed set within the library.
			continue
		}
	}
	return "ORDER BY " + strings.Join(parts, ", ")
}

// compileConditions compiles a condition list: the first condition
// carries no connective, and the rest are joined by AND/OR per their Or
// flag; conditions that compile to nothing (e.g. an omitted empty group)
// are skipped and do not consume a connective slot.
func (c *Compiler) compileConditions(res *Result, conditions []core.Clause) string {
	parts := make([]string, 0, len(conditions))
	for _, cl := range conditions {
		sql := c.compileCondition(res, cl)
		if sql == "" {
			continue
		}
		if len(parts) > 0 {
			if connector, ok := cl.(core.Condition); ok && connector.IsOr() {
				sql = "OR " + sql
			} else {
				sql = "AND " + sql
			}
		}
		parts = append(parts, sql)
	}
	return strings.Join(parts, " ")
}

// compileCondition dispatches compilation by condition form; where Not
// lands varies by form (parenthesized negation, keyword negation, or a
// flipped operator).
func (c *Compiler) compileCondition(res *Result, cl core.Clause) string {
	switch cond := cl.(type) {
	case *core.BasicCondition:
		sql := c.wrap(cond.Column) + " " + c.operator(cond.Operator) + " " + c.parameter(res, cond.Value)
		if cond.IsNot() {
			return "NOT (" + sql + ")"
		}
		return sql
	case *core.RawCondition:
		// Raw conditions do not participate in overall negation: their Not
		// is expressed by the expression itself.
		return c.compileRaw(res, cond.Expression, cond.Bindings)
	case *core.NullCondition:
		null := "IS NULL"
		if cond.IsNot() {
			null = "IS NOT NULL"
		}
		return c.wrap(cond.Column) + " " + null
	case *core.BooleanCondition:
		value := c.falseLiteral
		if cond.Value {
			value = c.trueLiteral
		}
		op := "="
		if cond.IsNot() {
			op = "!="
		}
		return c.wrap(cond.Column) + " " + op + " " + value
	case *core.TwoColumnsCondition:
		not := ""
		if cond.IsNot() {
			not = "NOT "
		}
		return not + c.wrap(cond.First) + " " + c.operator(cond.Operator) + " " + c.wrap(cond.Second)
	case *core.BetweenCondition:
		between := "BETWEEN"
		if cond.IsNot() {
			between = "NOT BETWEEN"
		}
		return c.wrap(cond.Column) + " " + between + " " + c.parameter(res, cond.Lower) + " AND " + c.parameter(res, cond.Higher)
	case *core.InCondition:
		if len(cond.Values) == 0 {
			// An empty list would make an invalid empty IN; use a
			// tautology/contradiction placeholder instead.
			if cond.IsNot() {
				return "1 = 1 /* NOT IN [empty list] */"
			}
			return "1 = 0 /* IN [empty list] */"
		}
		return c.wrap(cond.Column) + " " + c.inOperator(cond.IsNot()) + " (" + c.parameterize(res, cond.Values) + ")"
	case *core.InQueryCondition:
		return c.wrap(cond.Column) + " " + c.inOperator(cond.IsNot()) + " (" + c.compileSubQuery(res, cond.Query) + ")"
	case *core.SubQueryCondition:
		return "(" + c.compileSubQuery(res, cond.Query) + ") " + c.operator(cond.Operator) + " " + c.parameter(res, cond.Value)
	case *core.StringCondition:
		return c.stringConditionForm(res, cond)
	case *core.DateCondition:
		return c.dateConditionForm(res, cond)
	case *core.ExistsCondition:
		op := "EXISTS"
		if cond.IsNot() {
			op = "NOT EXISTS"
		}
		return op + " (" + c.compileSubQuery(res, cond.SubQuery(c.omitSelectInsideExists)) + ")"
	case *core.NestedCondition:
		inner := c.compileConditions(res, c.components(cond.Query.Clauses(), core.Where))
		if inner == "" {
			return ""
		}
		if cond.IsNot() {
			return "NOT (" + inner + ")"
		}
		return "(" + inner + ")"
	default:
		// Condition forms are a closed set within the library.
		return ""
	}
}

// lower wraps a column expression in a lowercase form (LOWER(...) in the
// base dialect) for case-insensitive LIKE-family comparisons; dialects
// handle their differences through their hooks.
func (c *Compiler) lower(column string) string {
	return "LOWER(" + column + ")"
}

// inOperator renders the set-membership keyword, NOT IN when negated.
func (c *Compiler) inOperator(not bool) string {
	if not {
		return "NOT IN"
	}
	return "IN"
}

// operator returns the whitelist-normalized operator (invalid operators
// are already rejected by validation, so no error here).
func (c *Compiler) operator(op string) string {
	normalized, _ := c.checkOperator(op)
	return normalized
}

// compileLimitOffset compiles pagination: it parses limit and offset from
// the clauses and renders the section via the dialect hook (the default
// form is standardLimitOffset).
func (c *Compiler) compileLimitOffset(res *Result, clauses []core.Clause) string {
	return c.limitOffsetForm(res, clauses, c.limitOf(clauses), c.offsetOf(clauses))
}

// limitOf parses the limit from the clauses (0 when unset or not a
// LimitClause).
func (c *Compiler) limitOf(clauses []core.Clause) int {
	if l, ok := c.one(clauses, core.Limit).(*core.LimitClause); ok {
		return l.Limit
	}
	return 0
}

// offsetOf parses the offset from the clauses (0 when unset or not an
// OffsetClause).
func (c *Compiler) offsetOf(clauses []core.Clause) int64 {
	if o, ok := c.one(clauses, core.Offset).(*core.OffsetClause); ok {
		return o.Offset
	}
	return 0
}

// standardLimitOffset is the default pagination section: limit and offset
// both bind as arguments, limit first and offset second; a zero value
// counts as unset.
func (c *Compiler) standardLimitOffset(res *Result, _ []core.Clause, limit int, offset int64) string {
	switch {
	case limit == 0 && offset == 0:
		return ""
	case offset == 0:
		return "LIMIT " + c.parameter(res, limit)
	case limit == 0:
		return "OFFSET " + c.parameter(res, offset)
	default:
		return "LIMIT " + c.parameter(res, limit) + " OFFSET " + c.parameter(res, offset)
	}
}

// noSelectTop is the selectTopClause default: the base form does not fold
// pagination into the SELECT head.
func (c *Compiler) noSelectTop(_ *Result, _ int, _ int64) string {
	return ""
}

// standardDateCondition is the default date-part condition,
// "PART(column) operator ?", with overall negation wrapped as NOT (...).
func (c *Compiler) standardDateCondition(res *Result, cond *core.DateCondition) string {
	sql := strings.ToUpper(cond.Part) + "(" + c.wrap(cond.Column) + ") " + c.operator(cond.Operator) + " " + c.parameter(res, cond.Value)
	if cond.IsNot() {
		return "NOT (" + sql + ")"
	}
	return sql
}

// standardStringCondition is the default LIKE-family condition:
// starts/ends/contains concatenate their wildcards at compile time and
// use the like operator; case-insensitive matching goes through
// LOWER(column) with a lowercased value; the ESCAPE clause follows, and
// overall negation wraps as NOT (...).
func (c *Compiler) standardStringCondition(res *Result, cond *core.StringCondition) string {
	value := cond.Value
	switch cond.Operator {
	case "starts":
		value += "%"
	case "ends":
		value = "%" + value
	case "contains":
		value = "%" + value + "%"
	}
	column := c.wrap(cond.Column)
	if !cond.CaseSensitive {
		column = c.lower(column)
		value = strings.ToLower(value)
	}
	sql := column + " " + c.operator("like") + " " + c.parameter(res, value)
	if cond.Escape != "" {
		sql += " ESCAPE '" + cond.Escape + "'"
	}
	if cond.IsNot() {
		return "NOT (" + sql + ")"
	}
	return sql
}

// parameter records a value into the argument sequence and returns the
// placeholder. UnsafeLiteral is not parameterized: its literal text is
// returned directly. Variable resolves its Define definition along the
// scope chain, and the resolved value binds as an ordinary argument
// (undefined references are already rejected by validateScope).
func (c *Compiler) parameter(res *Result, value any) string {
	switch v := value.(type) {
	case core.UnsafeLiteral:
		return v.Value
	case core.Variable:
		resolved, _ := res.scope.Lookup(v.Name)
		res.Args = append(res.Args, resolved)
		return "?"
	}
	res.Args = append(res.Args, value)
	return "?"
}

// checkOperator reports whether the operator is whitelisted and returns
// its lowercased normalized form.
func (c *Compiler) checkOperator(op string) (string, error) {
	op = strings.ToLower(op)
	if _, ok := c.operators[op]; ok {
		return op, nil
	}
	return "", ErrOperatorNotAllowed
}

// components filters clauses by this compiler's dialect code.
func (c *Compiler) components(clauses []core.Clause, of core.Component) []core.Clause {
	return core.Components(clauses, of, c.engineCode)
}

// one returns the single clause for this compiler's dialect code.
func (c *Compiler) one(clauses []core.Clause, of core.Component) core.Clause {
	return core.One(clauses, of, c.engineCode)
}

// wrap quotes a column/table identifier, handling "expr as alias"
// splitting and qualified a.b names.
func (c *Compiler) wrap(value string) string {
	if expr, alias := splitAlias(value); alias != "" {
		return c.wrap(expr) + " " + c.columnAsKeyword + c.wrapValue(alias)
	}
	if strings.Contains(value, ".") {
		parts := strings.Split(value, ".")
		for j, part := range parts {
			parts[j] = c.wrapValue(part)
		}
		return strings.Join(parts, ".")
	}
	return c.wrapValue(value)
}

// splitAlias splits "expr as alias" into its two parts (case-insensitive,
// the last " as " wins); alias is empty when there is none.
func splitAlias(value string) (expr, alias string) {
	if i := strings.LastIndex(strings.ToLower(value), " as "); i > 0 {
		return value[:i], value[i+4:]
	}
	return value, ""
}

// wrapValue quotes a single identifier value: the closing quote is
// escaped by doubling, and * passes through unchanged.
func (c *Compiler) wrapValue(value string) string {
	if value == "*" {
		return value
	}
	if c.openingIdentifier == "" && c.closingIdentifier == "" {
		return value
	}
	return c.openingIdentifier + strings.ReplaceAll(value, c.closingIdentifier, c.closingIdentifier+c.closingIdentifier) + c.closingIdentifier
}

// compileRaw compiles a raw SQL expression: bindings are appended in
// placeholder order, and the expression is returned as-is after
// identifier markers are substituted.
func (c *Compiler) compileRaw(res *Result, expression string, bindings []any) string {
	res.Args = append(res.Args, bindings...)
	return c.wrapIdentifiers(expression)
}

// wrapIdentifiers substitutes the identifier markers in a raw SQL
// expression: {} and [] (when not backslash-escaped) are replaced with
// the current dialect's quotes, and "\marker" keeps the literal marker
// with the backslash removed.
func (c *Compiler) wrapIdentifiers(input string) string {
	input = replaceUnlessEscaped(input, "{", c.openingIdentifier)
	input = replaceUnlessEscaped(input, "}", c.closingIdentifier)
	input = replaceUnlessEscaped(input, "[", c.openingIdentifier)
	return replaceUnlessEscaped(input, "]", c.closingIdentifier)
}

// replaceUnlessEscaped replaces old with repl; "\old" counts as escaped:
// the backslash is dropped and old is kept.
func replaceUnlessEscaped(input, old, repl string) string {
	var b strings.Builder
	b.Grow(len(input))
	for i := 0; i < len(input); {
		if input[i] == '\\' && strings.HasPrefix(input[i+1:], old) {
			b.WriteString(old)
			i += 1 + len(old)
			continue
		}
		if strings.HasPrefix(input[i:], old) {
			b.WriteString(repl)
			i += len(old)
			continue
		}
		b.WriteByte(input[i])
		i++
	}
	return b.String()
}
