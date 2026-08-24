// Package tutorial holds verification tests for every example in the tutorial
// docs (docs/tutorial): each build snippet and its SQL output shown there is
// checked verbatim here, so when the docs and the library drift apart these
// assertions fail first.
//
// Convention: SQL in the docs mirrors real compiler output -- placeholders are
// always "?" and arguments are shown as an ordered list; multi-line output
// (join sections, WITH prefixes) keeps the compiler's line breaks.
package tutorial

import (
	"reflect"
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// assertSQL compiles the query and asserts the SQL text and argument sequence
// (the seam that verifies the doc examples). When wantArgs is omitted it
// asserts that there are no arguments.
func assertSQL(t *testing.T, comp *compiler.Compiler, q *sqlk.Query, wantSQL string, wantArgs ...any) {
	t.Helper()
	res, err := comp.Compile(q)
	if err != nil {
		t.Fatalf("Compile(...) error = %v, want nil", err)
	}
	if res.SQL != wantSQL {
		t.Errorf("Compile(...) SQL =\n%s\nwant\n%s", res.SQL, wantSQL)
	}
	if !reflect.DeepEqual(res.Args, wantArgs) {
		t.Errorf("Compile(...) Args = %#v, want %#v", res.Args, wantArgs)
	}
}
