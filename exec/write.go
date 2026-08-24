package exec

import (
	"context"
	"errors"

	"github.com/aiongo/sqlk"
)

// ErrLastIdUnsupported is returned by InsertGetId when the compiler's
// dialect appends no last-insert-id statement (the base compiler and
// Oracle): the ID cannot be retrieved, so the insert is not run. It is
// distinct from ErrNoRows (no matching row) so a caller does not mistake
// "unsupported" for "nothing matched" and retry, double-writing.
var ErrLastIdUnsupported = errors.New("compiler does not support retrieving the last insert id")

// Exec runs a write-verb query (insert/update/delete) and returns the number
// of affected rows; the query is built with the root package's write verbs
// (`Insert`, `Update`, `Delete`, `Increment`, `Decrement`, ...). For an
// INSERT that must retrieve the auto-incremented ID, use `InsertGetId`.
func (x *Executor) Exec(ctx context.Context, q *sqlk.Query) (int64, error) {
	res, err := x.compile(q)
	if err != nil {
		return 0, err
	}
	out, err := x.runner.ExecContext(ctx, res.SQL, res.Args...)
	if err != nil {
		return 0, err
	}
	affected, err := out.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// lastIdRow is the row InsertGetId scans: the dialect's LastId statement
// yields the auto-incremented value under the column alias id (sqlite:
// select last_insert_rowid() as id).
type lastIdRow[T Number] struct {
	Id T `db:"id"`
}

// InsertGetId builds a returnId INSERT from the key-value pairs on a copy of
// the query, runs it, and scans back the auto-incremented ID: the compiler
// appends the dialect's LastId statement, so the insert and the fetch share
// one round trip (sqlite's last_insert_rowid; the shared round trip is what
// keeps the connection consistent). A dialect without LastId support (the
// base compiler, Oracle) returns ErrLastIdUnsupported without running the
// insert, so the unsupported case is not mistaken for a no-rows match.
func (x *Executor) InsertGetId[T Number](ctx context.Context, q *sqlk.Query, data sqlk.Record) (T, error) {
	var zero T
	if !x.compiler.SupportsLastId() {
		return zero, ErrLastIdUnsupported
	}
	row, err := x.scanOne[lastIdRow[T]](ctx, q.Clone().InsertReturnId(data))
	if err != nil {
		return zero, err
	}
	return row.Id, nil
}
