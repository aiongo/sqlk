// Package exec is a lightweight execution layer over sqlx: a connection plus
// a compiler build a uniform handle, so `DB` and transaction (`Tx`) sources
// expose exactly the same API and business code moves in and out of
// transactions without change. Every method takes a context.Context first,
// and timeouts and cancellation reach the driver.
//
//	db := exec.New(sqlxDB, compiler.NewSqlite())
//	cars, err := db.Get[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Honda"))
//
// The layer depends only on sqlx, the root package, and the compiler
// (dependencies point strictly downward); it does not depend on qdata. The
// library binds itself to no database driver: drivers are registered by the
// caller (or by tests).
package exec

import (
	"context"
	"database/sql"

	"github.com/jmoiron/sqlx"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// runner is the connection execution surface the layer needs: both *sqlx.DB
// and *sqlx.Tx satisfy it, which is the pivot that makes the DB and Tx
// handles uniform (a transaction handle shares its host DB's execution
// path).
type runner interface {
	GetContext(ctx context.Context, dest any, query string, args ...any) error
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Executor is the uniform execution surface shared by DB and Tx: it holds a
// connection execution surface (a runner), a compiler, and a compile-log
// callback, and carries all execution methods. DB and Tx embed *Executor and
// thereby expose identical method sets; writing data access against an
// *Executor parameter works inside and outside transactions alike:
//
//	func loadCars(ctx context.Context, x *exec.Executor) ([]Car, error) {
//		return x.Get[Car](ctx, sqlk.NewQuery().From("Cars"))
//	}
//	loadCars(ctx, db.Executor)   // db  *exec.DB
//	loadCars(ctx, tx.Executor)   // tx  *exec.Tx
type Executor struct {
	runner   runner
	compiler *compiler.Compiler
	logger   func(compiler.Result)
}

// DB is an execution handle bound to *sqlx.DB: the embedded Executor provides
// all execution methods, and Begin additionally opens a transaction yielding
// a uniform Tx handle (compiler and log callback carry over).
type DB struct {
	*Executor
	db *sqlx.DB
}

// New builds a DB execution handle from a connection and a compiler.
func New(db *sqlx.DB, c *compiler.Compiler, opts ...Option) *DB {
	return &DB{Executor: newExecutor(db, c, opts), db: db}
}

// Begin opens a transaction and returns a uniform Tx handle; the compiler
// and the compile-log callback are inherited from this handle. Callers that
// must set transaction options first, per database/sql convention, can use
// sqlx's BeginTxx and wrap the result with NewTx.
func (d *DB) Begin(ctx context.Context) (*Tx, error) {
	tx, err := d.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{Executor: d.Executor.withRunner(tx), tx: tx}, nil
}

// Tx is a transaction execution handle bound to *sqlx.Tx: the embedded
// Executor is fully uniform with the DB handle, so business code switches
// without noticing; `Commit` and `Rollback` forward to the underlying
// transaction.
type Tx struct {
	*Executor
	tx *sqlx.Tx
}

// NewTx builds a Tx execution handle from a transaction connection and a
// compiler, for callers managing the transaction lifecycle themselves and
// wrapping an existing *sqlx.Tx.
func NewTx(tx *sqlx.Tx, c *compiler.Compiler, opts ...Option) *Tx {
	return &Tx{Executor: newExecutor(tx, c, opts), tx: tx}
}

// Commit commits the transaction.
func (t *Tx) Commit() error { return t.tx.Commit() }

// Rollback rolls back the transaction.
func (t *Tx) Rollback() error { return t.tx.Rollback() }

// Option configures an execution handle at construction time.
type Option func(*Executor)

// WithLogger attaches a compile-log callback: after each statement compiles
// successfully and before it executes, the callback receives the compiled
// result (SQL text and ordered arguments) for troubleshooting and auditing;
// statements that fail to compile never trigger it. Transaction handles
// opened with `DB.Begin` inherit the callback.
func WithLogger(fn func(compiler.Result)) Option {
	return func(x *Executor) { x.logger = fn }
}

// newExecutor assembles an execution surface.
func newExecutor(r runner, c *compiler.Compiler, opts []Option) *Executor {
	x := &Executor{runner: r, compiler: c}
	for _, opt := range opts {
		opt(x)
	}
	return x
}

// withRunner returns a copy of the execution surface with the connection
// execution surface swapped; the compiler and log callback carry over (used
// by Begin to yield a uniform transaction handle).
func (x *Executor) withRunner(r runner) *Executor {
	clone := *x
	clone.runner = r
	return &clone
}

// compile compiles the query and fires the compile-log callback; build and
// compile problems (aggregated at the compiler entry) pass through as is and
// do not trigger the callback.
func (x *Executor) compile(q *sqlk.Query) (compiler.Result, error) {
	res, err := x.compiler.Compile(q)
	if err != nil {
		return compiler.Result{}, err
	}
	if x.logger != nil {
		x.logger(res)
	}
	return res, nil
}
