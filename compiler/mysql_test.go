package compiler

import (
	"reflect"
	"testing"

	"github.com/aiongo/sqlk"
)

// Cases for the mysql dialect, built with NewMysql. Dialect specifics
// covered here: backtick identifier quoting with doubling escapes, the
// unsigned bigint LIMIT ceiling that accompanies a lone OFFSET,
// last_insert_id appended for return-id inserts, and the multi-table
// DELETE form used with JOINs.

func TestMysqlIdentifiers(t *testing.T) {
	runCompileCases(t, NewMysql(), []compileCase{
		{
			name:  "basic select",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Select("id", "name") },
			sql:   "SELECT `id`, `name` FROM `users`",
		},
		{
			name:  "table alias",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users as u").Select("id", "name") },
			sql:   "SELECT `id`, `name` FROM `users` AS `u`",
		},
		{
			name:  "qualified and aliased columns",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Users as u").Select("u.Name as FullName") },
			sql:   "SELECT `u`.`Name` AS `FullName` FROM `Users` AS `u`",
		},
		{
			name:  "inner backtick is escaped by doubling",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Ta`ble") },
			sql:   "SELECT * FROM `Ta``ble`",
		},
		{
			// Identifier markers in raw expressions wrap in backticks.
			name: "raw expression identifier markers",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").SelectRaw("[Id], [Name], {Age}")
			},
			sql: "SELECT `Id`, `Name`, `Age` FROM `Users`",
		},
		{
			name:  "count alias",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("A").Count() },
			sql:   "SELECT COUNT(*) AS `count` FROM `A`",
		},
		{
			// The join section starts on a new line; qualified names are
			// quoted per part.
			name: "basic join",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("users").Join("countries", "countries.id", "=", "users.country_id")
			},
			sql: "SELECT * FROM `users` \nINNER JOIN `countries` ON `countries`.`id` = `users`.`country_id`",
		},
	})
}

func TestMysqlLimitOffset(t *testing.T) {
	runCompileCases(t, NewMysql(), []compileCase{
		{
			name:  "no limit nor offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table") },
			sql:   "SELECT * FROM `Table`",
		},
		{
			name:  "limit only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(10) },
			sql:   "SELECT * FROM `Table` LIMIT ?",
			args:  []any{10},
		},
		{
			// MySql rejects OFFSET without LIMIT, so a lone OFFSET is
			// accompanied by the unsigned bigint ceiling as the LIMIT.
			name:  "offset only",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Offset(20) },
			sql:   "SELECT * FROM `Table` LIMIT 18446744073709551615 OFFSET ?",
			args:  []any{int64(20)},
		},
		{
			name:  "limit and offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").Limit(5).Offset(20) },
			sql:   "SELECT * FROM `Table` LIMIT ? OFFSET ?",
			args:  []any{5, int64(20)},
		},
		{
			name:  "select with limit",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Select("id", "name").Limit(10) },
			sql:   "SELECT `id`, `name` FROM `users` LIMIT ?",
			args:  []any{10},
		},
		{
			name:  "select with offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Offset(10) },
			sql:   "SELECT * FROM `users` LIMIT 18446744073709551615 OFFSET ?",
			args:  []any{int64(10)},
		},
		{
			name:  "select with limit and offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("users").Offset(10).Limit(5) },
			sql:   "SELECT * FROM `users` LIMIT ? OFFSET ?",
			args:  []any{5, int64(10)},
		},
		{
			name:  "for page folds to limit and offset",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").ForPage(2, 10) },
			sql:   "SELECT * FROM `Table` LIMIT ? OFFSET ?",
			args:  []any{10, int64(10)},
		},
		{
			// The mysql-scoped offset feeds the offset-only form.
			name: "engine-specific offset",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").
					For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.Offset(5) }).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Offset(10) })
			},
			sql:  "SELECT * FROM `mytable` LIMIT 18446744073709551615 OFFSET ?",
			args: []any{int64(5)},
		},
		{
			// Limit clauses scoped to other engines leave no pagination
			// section on the mysql side.
			name: "engine-specific limit on other engines",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").
					For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.Limit(5) }).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Limit(10) })
			},
			sql: "SELECT * FROM `mytable`",
		},
		{
			// The mysql side takes the generic limit/offset.
			name: "generic limit with engine-specific offset elsewhere",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").Limit(5).Offset(10).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Offset(20) })
			},
			sql:  "SELECT * FROM `mytable` LIMIT ? OFFSET ?",
			args: []any{5, int64(10)},
		},
		{
			name: "engine-specific limit elsewhere with generic offset",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").Limit(5).Offset(10).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Limit(20) })
			},
			sql:  "SELECT * FROM `mytable` LIMIT ? OFFSET ?",
			args: []any{5, int64(10)},
		},
		{
			name: "generic limit changed after engine-specific offset elsewhere",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").Limit(5).Offset(10).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Offset(20) }).
					Limit(7)
			},
			sql:  "SELECT * FROM `mytable` LIMIT ? OFFSET ?",
			args: []any{7, int64(10)},
		},
		{
			name: "generic offset changed after engine-specific limit elsewhere",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("mytable").Limit(5).Offset(10).
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.Limit(20) }).
					Offset(7)
			},
			sql:  "SELECT * FROM `mytable` LIMIT ? OFFSET ?",
			args: []any{5, int64(7)},
		},
		{
			// The mysql dialect keeps the default RANDOM().
			name:  "random order uses RANDOM()",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").OrderByRandom().Limit(1) },
			sql:   "SELECT * FROM `Table` ORDER BY RANDOM() LIMIT ?",
			args:  []any{1},
		},
	})
}

func TestMysqlLastId(t *testing.T) {
	runCompileCases(t, NewMysql(), []compileCase{
		{
			// A return-id INSERT appends a last_insert_id statement.
			name: "insert return id appends last_insert_id",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").InsertReturnId(sqlk.Record{"Name": "x"})
			},
			sql:  "INSERT INTO `Users` (`Name`) VALUES (?);SELECT last_insert_id() as Id",
			args: []any{"x"},
		},
		{
			name: "plain insert does not append",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").Insert(sqlk.Record{"Name": "x"})
			},
			sql:  "INSERT INTO `Users` (`Name`) VALUES (?)",
			args: []any{"x"},
		},
		{
			name: "multi-row insert does not append",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").InsertRows([]string{"Name"}, []any{"x"}, []any{"y"})
			},
			sql:  "INSERT INTO `Users` (`Name`) VALUES (?), (?)",
			args: []any{"x", "y"},
		},
		{
			// The insert-from subquery carries its own pagination.
			name: "insert from paged subquery with cte",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("expensive_cars").
					With("old_cards", sqlk.NewQuery().From("all_cars").Where("year", "<", 2000)).
					InsertFrom([]string{"name", "model", "year"},
						sqlk.NewQuery().From("old_cars").Where("price", ">", 100).ForPage(2, 10))
			},
			sql:  "WITH `old_cards` AS (SELECT * FROM `all_cars` WHERE `year` < ?)\nINSERT INTO `expensive_cars` (`name`, `model`, `year`) SELECT * FROM `old_cars` WHERE `price` > ? LIMIT ? OFFSET ?",
			args: []any{2000, 100, 10, int64(10)},
		},
	})
}

func TestMysqlDeleteWithJoin(t *testing.T) {
	runCompileCases(t, NewMysql(), []compileCase{
		{
			// A joined delete uses MySql's multi-table form,
			// "DELETE table FROM table JOIN".
			name: "delete with join repeats the table as target",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").
					Join("Authors", "Authors.Id", "=", "Posts.AuthorId").
					WhereEq("Authors.Id", 5).
					Delete()
			},
			sql:  "DELETE `Posts` FROM `Posts` \nINNER JOIN `Authors` ON `Authors`.`Id` = `Posts`.`AuthorId` WHERE `Authors`.`Id` = ?",
			args: []any{5},
		},
		{
			// The from alias becomes the delete target.
			name: "delete with join and alias targets the alias",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts as P").
					Join("Authors", "Authors.Id", "=", "P.AuthorId").
					WhereEq("Authors.Id", 5).
					Delete()
			},
			sql:  "DELETE `P` FROM `Posts` AS `P` \nINNER JOIN `Authors` ON `Authors`.`Id` = `P`.`AuthorId` WHERE `Authors`.`Id` = ?",
			args: []any{5},
		},
		{
			name: "delete without join keeps the base shape",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Posts").WhereEq("Id", 7).Delete()
			},
			sql:  "DELETE FROM `Posts` WHERE `Id` = ?",
			args: []any{7},
		},
	})
}

func TestMysqlEngineLoopPorts(t *testing.T) {
	runCompileCases(t, NewMysql(), []compileCase{
		{
			name: "engine-scoped from",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.From("mssql") }).
					For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.From("mysql") })
			},
			sql: "SELECT * FROM `mysql`",
		},
		{
			name: "engine-scoped from raw",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.FromRaw("[mysql]") })
			},
			sql: "SELECT * FROM `mysql`",
		},
		{
			// From targets scoped to other engines stay invisible; the
			// generic from is used.
			name: "one from per engine falls back to the generic from",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("generic").
					For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.From("dnu") }).
					For(sqlk.EngineSqlserver, func(q *sqlk.Query) *sqlk.Query { return q.From("mssql") })
			},
			sql: "SELECT * FROM `generic`",
		},
		{
			// The mysql dialect keeps the base true/false literals.
			name:  "where true",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereTrue("IsActive") },
			sql:   "SELECT * FROM `Table` WHERE `IsActive` = true",
		},
		{
			name:  "where false",
			build: func(q *sqlk.Query) *sqlk.Query { return q.From("Table").WhereFalse("IsActive") },
			sql:   "SELECT * FROM `Table` WHERE `IsActive` = false",
		},
		{
			name: "union with bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").Union(sqlk.NewQuery().From("Laptops").WhereEq("Type", "A"))
			},
			sql:  "SELECT * FROM `Phones` UNION SELECT * FROM `Laptops` WHERE `Type` = ?",
			args: []any{"A"},
		},
		{
			// Identifier markers in a raw combine wrap in backticks.
			name: "raw union with bindings",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Phones").UnionRaw("UNION SELECT * FROM [Laptops] WHERE [Type] = ?", "A")
			},
			sql:  "SELECT * FROM `Phones` UNION SELECT * FROM `Laptops` WHERE `Type` = ?",
			args: []any{"A"},
		},
		{
			name: "adhoc table cte one row",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").WithTable("rows", []string{"a"}, []any{1})
			},
			sql:  "WITH `rows` AS (SELECT ? AS `a`)\nSELECT * FROM `rows`",
			args: []any{1},
		},
		{
			name: "adhoc table cte two rows union all",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("rows").WithTable("rows", []string{"a", "b", "c"},
					[]any{1, 2, 3}, []any{4, 5, 6})
			},
			sql:  "WITH `rows` AS (SELECT ? AS `a`, ? AS `b`, ? AS `c` UNION ALL SELECT ? AS `a`, ? AS `b`, ? AS `c`)\nSELECT * FROM `rows`",
			args: []any{1, 2, 3, 4, 5, 6},
		},
	})
}

func TestMysqlCompileIsIdempotent(t *testing.T) {
	// Compiling must not mutate query state: repeated compiles agree.
	build := func() *sqlk.Query {
		return sqlk.NewQuery().Select("Id", "Name").From("Table").OrderBy("Name").Limit(20).Offset(1)
	}
	comp := NewMysql()
	first := mustCompile(t, comp, build())
	second := mustCompile(t, comp, build())
	if first.SQL != second.SQL || !reflect.DeepEqual(first.Args, second.Args) {
		t.Errorf("repeated compiles differ: (%q, %#v) vs (%q, %#v)",
			first.SQL, first.Args, second.SQL, second.Args)
	}
	want := "SELECT `Id`, `Name` FROM `Table` ORDER BY `Name` LIMIT ? OFFSET ?"
	if first.SQL != want {
		t.Errorf("Compile(...) SQL = %q, want %q", first.SQL, want)
	}
	if want := []any{20, int64(1)}; !reflect.DeepEqual(first.Args, want) {
		t.Errorf("Compile(...) Args = %#v, want %#v", first.Args, want)
	}
}

func TestMysqlBuildSurface(t *testing.T) {
	// Representative output of the whole build surface under the mysql
	// compiler: the dialect specifics are covered above; these cases
	// confirm the remaining sections keep the base shapes.
	runCompileCases(t, NewMysql(), []compileCase{
		{
			name: "select with join group having order and pagination",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users as u").
					Select("u.Country").
					SelectRaw("count(*) as Total").
					Join("Cities as c", "c.Id", "=", "u.CityId").
					Where("u.Age", ">", 18).
					WhereNotNull("u.Email").
					GroupBy("u.Country").
					HavingRaw("count(*) > ?", 1).
					OrderByDesc("Total").
					ForPage(2, 10)
			},
			sql:  "SELECT `u`.`Country`, count(*) as Total FROM `Users` AS `u` \nINNER JOIN `Cities` AS `c` ON `c`.`Id` = `u`.`CityId` WHERE `u`.`Age` > ? AND `u`.`Email` IS NOT NULL GROUP BY `u`.`Country` HAVING count(*) > ? ORDER BY `Total` DESC LIMIT ? OFFSET ?",
			args: []any{18, 1, 10, int64(10)},
		},
		{
			name: "cte precedes and combine follows",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("a").
					With("t", sqlk.NewQuery().From("src").WhereEq("Ok", 1)).
					UnionAll(sqlk.NewQuery().From("b"))
			},
			sql:  "WITH `t` AS (SELECT * FROM `src` WHERE `Ok` = ?)\nSELECT * FROM `a` UNION ALL SELECT * FROM `b`",
			args: []any{1},
		},
		{
			name: "aggregate form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("A").WhereEq("Active", true).Count()
			},
			sql:  "SELECT COUNT(*) AS `count` FROM `A` WHERE `Active` = ?",
			args: []any{true},
		},
		{
			name: "insensitive like wraps the column with LOWER",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereContains("Name", "oh")
			},
			sql:  "SELECT * FROM `Users` WHERE LOWER(`Name`) like ?",
			args: []any{"%oh%"},
		},
		{
			name: "date condition uses the base part form",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Orders").WhereDatePartEq("year", "RequiredDate", 1996)
			},
			sql:  "SELECT * FROM `Orders` WHERE YEAR(`RequiredDate`) = ?",
			args: []any{1996},
		},
		{
			name: "update keeps the base shape",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("Users").WhereEq("Id", 1).Update(sqlk.Record{"Name": "x"})
			},
			sql:  "UPDATE `Users` SET `Name` = ? WHERE `Id` = ?",
			args: []any{"x", 1},
		},
		{
			name: "mysql-scoped clauses are visible to the mysql compiler",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EngineMysql, func(q *sqlk.Query) *sqlk.Query { return q.WhereEq("A", 1) }).
					WhereEq("B", 2)
			},
			sql:  "SELECT * FROM `T` WHERE `A` = ? AND `B` = ?",
			args: []any{1, 2},
		},
		{
			name: "other-engine clauses are invisible",
			build: func(q *sqlk.Query) *sqlk.Query {
				return q.From("T").
					For(sqlk.EnginePostgres, func(q *sqlk.Query) *sqlk.Query { return q.WhereEq("A", 1) }).
					WhereEq("B", 2)
			},
			sql:  "SELECT * FROM `T` WHERE `B` = ?",
			args: []any{2},
		},
	})
}
