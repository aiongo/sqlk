package tutorial

import (
	"testing"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
)

// Examples from combine.md: the Union/Except/Intersect family and CombineRaw.

func TestUnion(t *testing.T) {
	phones := sqlk.NewQuery().From("Phones")
	laptops := sqlk.NewQuery().From("Laptops")
	assertSQL(t, compiler.NewSqlserver(),
		laptops.Union(phones),
		`SELECT * FROM [Laptops] UNION SELECT * FROM [Phones]`)
}

func TestExceptAllFunc(t *testing.T) {
	// Callback form: the member is built by the callback (the query-argument
	// forms are Except/ExceptAll).
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Laptops").
			ExceptAllFunc(func(q *sqlk.Query) *sqlk.Query { return q.From("OldLaptops") }),
		`SELECT * FROM [Laptops] EXCEPT ALL SELECT * FROM [OldLaptops]`)
}

func TestCombineRaw(t *testing.T) {
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Laptops").CombineRaw("union all select * from OldLaptops"),
		`SELECT * FROM [Laptops] union all select * from OldLaptops`)
	// Identifiers marked with [] are quoted by the compiler per dialect.
	assertSQL(t, compiler.NewSqlserver(),
		sqlk.NewQuery().From("Laptops").CombineRaw("union all select * from [OldLaptops]"),
		`SELECT * FROM [Laptops] union all select * from [OldLaptops]`)
	assertSQL(t, compiler.NewPostgres(),
		sqlk.NewQuery().From("Laptops").CombineRaw("union all select * from [OldLaptops]"),
		`SELECT * FROM "Laptops" union all select * from "OldLaptops"`)
}
