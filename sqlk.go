// Package sqlk is a SQL query builder: a single `Query` type carries every
// verb, chained calls accumulate clauses, and the compiler package compiles
// the result into placeholder SQL and ordered arguments for the dialect.
//
//	q := sqlk.NewQuery().From("Users").Select("Id", "Name").WhereEq("Id", 1)
//	res, err := compiler.New().Compile(q)
//	// res.SQL  == `SELECT "Id", "Name" FROM "Users" WHERE "Id" = ?`
//	// res.Args == []any{1}
package sqlk

import "github.com/aiongo/sqlk/internal/core"

// Query is the fluent builder; the building surface lives in internal/core
// and is re-exported here as an alias. Create it with `NewQuery`; the zero
// value is not usable.
type Query = core.Query

// Record is one row of column/value pairs: the shape carried by the
// key-value forms of the write verbs (`Insert`, `InsertReturnId`,
// `Update`) and the equality-shorthand condition maps (`WhereMap`,
// `HavingMap`). It is an alias for map[string]any, so a plain
// map[string]any literal is interchangeable with it.
type Record = core.Record

// Engine constants are the dialect codes used by `For` to scope clauses to a
// single dialect and matched by the dialect compilers; user code scopes
// clauses through these instead of string literals.
const (
	EngineSqlite    = "sqlite"
	EngineSqlserver = "sqlserver"
	EnginePostgres  = "postgres"
	EngineMysql     = "mysql"
	EngineOracle    = "oracle"
)

// NewQuery returns an empty query; it is the entry point of building.
func NewQuery() *Query {
	return core.NewQuery()
}

// Join is the scope for ON conditions: callbacks of verbs like `JoinOn` and
// `JoinSub` receive a join whose join type and target are already set;
// append column-pair conditions with `On`, `OnNot`, `OrOn`, or `OrOnNot`,
// or arbitrary conditions with the `Where` family (capabilities equal to
// `Where`).
type Join = core.Join

// MatchOption customizes how the Like family (`WhereLike`, `WhereStarts`,
// `WhereEnds`, `WhereContains`, and variants) matches; options apply in the
// order given, later ones overriding earlier ones.
type MatchOption = core.MatchOption

// CaseSensitive returns an option that makes Like-family comparisons
// case-sensitive. The default is insensitive: the column expression is
// wrapped in LOWER(...) and the pattern value is lowercased.
func CaseSensitive() MatchOption { return core.CaseSensitive() }

// EscapeLike returns an option setting the escape character for the Like
// family, compiled into an ESCAPE '<char>' clause; whitespace counts as
// unset and more than one character is rejected at the compiler entry.
func EscapeLike(char string) MatchOption { return core.EscapeLike(char) }

// Variable is a reference to a query variable, created by `NewVariable` and
// usable as a value in parameter positions; it is an alias for the internal
// clause-model type, matching the alias surface of `Query` and `Join` (the
// core import path never appears in signatures).
type Variable = core.Variable

// NewVariable returns a reference to a query variable, usable as a value in
// parameter positions (Where/In/Between values, Insert/Update write values,
// and so on): at compile time the definition of the same name made on this
// query's `Define` is looked up first, then the parent chain upward; a found
// value binds as a plain parameter (placeholder plus argument). A name
// undefined along the whole chain is rejected at the compiler entry. Like
// family values are complete pattern strings and do not accept variable
// references.
func NewVariable(name string) Variable { return core.NewVariable(name) }

// UnsafeLiteral is a trusted literal inlined verbatim into the SQL, created
// by `NewUnsafeLiteral`; it is an alias for the internal clause-model type,
// matching the alias surface of `Query` and `Join`.
type UnsafeLiteral = core.UnsafeLiteral

// NewUnsafeLiteral returns a trusted literal that is not parameterized: the
// text is inlined verbatim into the SQL and never enters the argument
// sequence; single quotes in the text are doubled to preserve the string
// literal boundary. It is an explicit escape hatch for trusted content that
// cannot be parameterized (function calls, column fragments); never feed it
// user input.
func NewUnsafeLiteral(value string) UnsafeLiteral { return core.NewUnsafeLiteral(value) }
