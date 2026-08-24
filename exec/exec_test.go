package exec_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"

	"github.com/aiongo/sqlk"
	"github.com/aiongo/sqlk/compiler"
	"github.com/aiongo/sqlk/exec"
)

// Execution-layer seam: a real data round trip on an in-memory
// modernc.org/sqlite database -- create table, build, execute, scan, assert.
// The driver is a test dependency only: the library binds itself to no
// database driver.

// Car is the scan-target struct; sqlx's default mapping lowercases field
// names while the driver reports column names as declared (Id and friends,
// capitalized), so explicit db tags align them (standard sqlx practice).
type Car struct {
	Id    int64   `db:"Id"`
	Brand string  `db:"Brand"`
	Year  int     `db:"Year"`
	Color *string `db:"Color"`
}

// newTestDB creates the table, seeds three rows, and returns the DB handle
// plus the underlying *sqlx.DB (for transaction cases that drive the
// connection directly). The in-memory database runs on a single connection,
// dodging the usual :memory: one-database-per-connection pitfall; it also
// means DB-handle calls must not be interleaved while a transaction is open
// (they would wait for the only connection).
func newTestDB(t *testing.T) (*exec.DB, *sqlx.DB) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { sqlDB.Close() })
	sqlDB.SetMaxOpenConns(1)
	db := sqlx.NewDb(sqlDB, "sqlite")

	ctx := context.Background()
	_, err = db.ExecContext(ctx, `CREATE TABLE Cars (
		Id INTEGER PRIMARY KEY AUTOINCREMENT,
		Brand TEXT NOT NULL,
		Year INT NOT NULL,
		Color TEXT NULL)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	rows := []struct {
		brand string
		year  int
		color *string
	}{
		{"Honda", 2020, new("Red")},
		{"Toyota", 2021, nil},
		{"BMW", 2022, new("Black")},
	}
	for _, r := range rows {
		if _, err := db.ExecContext(ctx, "INSERT INTO Cars(Brand, Year, Color) VALUES (?, ?, ?)", r.brand, r.year, r.color); err != nil {
			t.Fatalf("seed row: %v", err)
		}
	}
	return exec.New(db, compiler.NewSqlite()), db
}

func TestGetScansRows(t *testing.T) {
	db, _ := newTestDB(t)
	ctx := context.Background()

	cars, err := db.Get[Car](ctx, sqlk.NewQuery().From("Cars").OrderBy("Id"))
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(cars) != 3 {
		t.Fatalf("got %d cars, want 3", len(cars))
	}
	if cars[0].Brand != "Honda" || cars[0].Year != 2020 || *cars[0].Color != "Red" {
		t.Errorf("first row = %+v, want Honda 2020 Red", cars[0])
	}
	if cars[1].Color != nil {
		t.Errorf("null color scanned as %v, want nil", *cars[1].Color)
	}

	// Filtering and parameter binding round trip.
	hondas, err := db.Get[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Honda"))
	if err != nil {
		t.Fatalf("Get with where: %v", err)
	}
	if len(hondas) != 1 || hondas[0].Year != 2020 {
		t.Errorf("filtered rows = %+v, want single Honda", hondas)
	}

	// No match returns an empty slice, not an error.
	none, err := db.Get[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Nope"))
	if err != nil {
		t.Fatalf("Get no match: %v", err)
	}
	if len(none) != 0 {
		t.Errorf("got %d rows for no match, want 0", len(none))
	}
}

func TestFirstAndFirstOrDefault(t *testing.T) {
	db, _ := newTestDB(t)
	ctx := context.Background()

	car, err := db.First[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Toyota"))
	if err != nil {
		t.Fatalf("First: %v", err)
	}
	if car.Year != 2021 {
		t.Errorf("First year = %d, want 2021", car.Year)
	}

	// No rows: First returns a distinguishable error (equal to sql.ErrNoRows).
	_, err = db.First[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Nope"))
	if !errors.Is(err, exec.ErrNoRows) {
		t.Errorf("First no rows err = %v, want exec.ErrNoRows", err)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("First no rows err = %v, want sql.ErrNoRows", err)
	}

	// No rows: FirstOrDefault returns the zero value and nil.
	got, err := db.FirstOrDefault[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Nope"))
	if err != nil {
		t.Fatalf("FirstOrDefault no rows: %v", err)
	}
	if (Car{} != got) {
		t.Errorf("FirstOrDefault zero = %+v, want zero value", got)
	}

	first, err := db.FirstOrDefault[Car](ctx, sqlk.NewQuery().From("Cars").OrderBy("Id"))
	if err != nil {
		t.Fatalf("FirstOrDefault: %v", err)
	}
	if first.Brand != "Honda" {
		t.Errorf("FirstOrDefault brand = %q, want Honda", first.Brand)
	}
}

func TestExistsAndNotExist(t *testing.T) {
	db, _ := newTestDB(t)
	ctx := context.Background()

	exists, err := db.Exists(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Honda"))
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if !exists {
		t.Error("Exists(matching) = false, want true")
	}

	exists, err = db.Exists(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Nope"))
	if err != nil {
		t.Fatalf("Exists no match: %v", err)
	}
	if exists {
		t.Error("Exists(no match) = true, want false")
	}

	notExist, err := db.NotExist(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Nope"))
	if err != nil {
		t.Fatalf("NotExist: %v", err)
	}
	if !notExist {
		t.Error("NotExist(no match) = false, want true")
	}
}

func TestScalarAggregates(t *testing.T) {
	db, _ := newTestDB(t)
	ctx := context.Background()
	all := sqlk.NewQuery().From("Cars")

	count, err := db.Count[int64](ctx, all)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != 3 {
		t.Errorf("Count = %d, want 3", count)
	}

	// Counting a column skips NULLs, as SQL COUNT(col) does.
	colored, err := db.Count[int64](ctx, all, "Color")
	if err != nil {
		t.Fatalf("Count column: %v", err)
	}
	if colored != 2 {
		t.Errorf("Count(Color) = %d, want 2", colored)
	}

	sum, err := db.Sum[int](ctx, all, "Year")
	if err != nil {
		t.Fatalf("Sum: %v", err)
	}
	if sum != 6063 {
		t.Errorf("Sum(Year) = %d, want 6063", sum)
	}

	avg, err := db.Avg[float64](ctx, all, "Year")
	if err != nil {
		t.Fatalf("Avg: %v", err)
	}
	if avg != 2021 {
		t.Errorf("Avg(Year) = %f, want 2021", avg)
	}

	oldestYear, err := db.Min[int](ctx, all, "Year")
	if err != nil {
		t.Fatalf("Min: %v", err)
	}
	if oldestYear != 2020 {
		t.Errorf("Min(Year) = %d, want 2020", oldestYear)
	}

	newestYear, err := db.Max[int64](ctx, all, "Year")
	if err != nil {
		t.Fatalf("Max: %v", err)
	}
	if newestYear != 2022 {
		t.Errorf("Max(Year) = %d, want 2022", newestYear)
	}

	// Min/Max target columns are not limited to numbers.
	firstBrand, err := db.Min[string](ctx, all, "Brand")
	if err != nil {
		t.Fatalf("Min string: %v", err)
	}
	if firstBrand != "BMW" {
		t.Errorf("Min(Brand) = %q, want BMW", firstBrand)
	}

	// Aggregates combined with filter conditions.
	hondaCount, err := db.Count[int64](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Honda"))
	if err != nil {
		t.Fatalf("Count with where: %v", err)
	}
	if hondaCount != 1 {
		t.Errorf("Count(Brand=Honda) = %d, want 1", hondaCount)
	}

	// Scalar aggregates leave the caller's query untouched: reusing all
	// across aggregates is safe.
	again, err := db.Count[int64](ctx, all)
	if err != nil {
		t.Fatalf("Count again: %v", err)
	}
	if again != 3 {
		t.Errorf("Count again = %d, want 3", again)
	}
}

// loadFirstBrand simulates a business data-access function: it takes an
// *exec.Executor, so handles from inside and outside transactions both fit
// unchanged.
func loadFirstBrand(ctx context.Context, x *exec.Executor) (string, error) {
	car, err := x.First[Car](ctx, sqlk.NewQuery().From("Cars").OrderBy("Id"))
	if err != nil {
		return "", err
	}
	return car.Brand, nil
}

func TestTxIsomorphicAPI(t *testing.T) {
	db, sqlxDB := newTestDB(t)
	ctx := context.Background()

	// Data access outside a transaction.
	brand, err := loadFirstBrand(ctx, db.Executor)
	if err != nil {
		t.Fatalf("load outside tx: %v", err)
	}
	if brand != "Honda" {
		t.Fatalf("brand outside tx = %q, want Honda", brand)
	}

	// Inside a transaction: same function, same API, switching unnoticed;
	// the uncommitted write is visible only inside the transaction and
	// vanishes after rollback (the sqlx transaction opened here is wrapped
	// with NewTx).
	sqlxTx, err := sqlxDB.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	tx := exec.NewTx(sqlxTx, compiler.NewSqlite())
	t.Cleanup(func() { _ = tx.Rollback() })
	if _, err := sqlxTx.ExecContext(ctx, "INSERT INTO Cars(Brand, Year) VALUES ('InTx', 2024)"); err != nil {
		t.Fatalf("seed in tx: %v", err)
	}
	brand, err = loadFirstBrand(ctx, tx.Executor)
	if err != nil {
		t.Fatalf("load inside tx: %v", err)
	}
	if brand != "Honda" {
		t.Errorf("brand inside tx = %q, want Honda (tx reads committed data)", brand)
	}
	inTx, err := tx.Exists(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "InTx"))
	if err != nil {
		t.Fatalf("exists inside tx: %v", err)
	}
	if !inTx {
		t.Error("uncommitted row not visible inside tx")
	}
	if err := tx.Rollback(); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	// After rollback: the uncommitted write is gone; the handle from
	// DB.Begin is uniform with NewTx's.
	begun, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("db.Begin: %v", err)
	}
	t.Cleanup(func() { _ = begun.Rollback() })
	gone, err := begun.NotExist(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "InTx"))
	if err != nil {
		t.Fatalf("not exist in begun tx: %v", err)
	}
	if !gone {
		t.Error("rolled-back row still visible")
	}
	count, err := begun.Count[int64](ctx, sqlk.NewQuery().From("Cars"))
	if err != nil {
		t.Fatalf("count in begun tx: %v", err)
	}
	if count != 3 {
		t.Errorf("count after rollback = %d, want 3", count)
	}
	if err := begun.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestContextCancellation(t *testing.T) {
	db, _ := newTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := db.Get[Car](ctx, sqlk.NewQuery().From("Cars"))
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Get with canceled ctx err = %v, want context.Canceled", err)
	}
}

func TestCompileLogger(t *testing.T) {
	_, sqlxDB := newTestDB(t)
	ctx := context.Background()

	var logs []compiler.Result
	logged := exec.New(sqlxDB, compiler.NewSqlite(),
		exec.WithLogger(func(res compiler.Result) { logs = append(logs, res) }))

	if _, err := logged.Get[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Honda")); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("logged %d statements, want 1", len(logs))
	}
	wantSQL := `SELECT * FROM "Cars" WHERE "Brand" = ?`
	if logs[0].SQL != wantSQL {
		t.Errorf("logged SQL = %q, want %q", logs[0].SQL, wantSQL)
	}
	if len(logs[0].Args) != 1 || logs[0].Args[0] != "Honda" {
		t.Errorf("logged args = %v, want [Honda]", logs[0].Args)
	}

	// Statements that fail to compile do not trigger the callback.
	before := len(logs)
	if _, err := logged.Get[Car](ctx, sqlk.NewQuery().Select("Id")); err == nil {
		t.Fatal("Get without from target: want compile error")
	}
	if len(logs) != before {
		t.Errorf("logger fired on compile failure")
	}

	// Handles from Begin inherit the callback; exactly one log entry per
	// statement.
	tx, err := logged.Begin(ctx)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	if _, err := tx.Count[int64](ctx, sqlk.NewQuery().From("Cars")); err != nil {
		t.Fatalf("Count in tx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("logged %d statements total, want 2", len(logs))
	}
	if logs[1].SQL != `SELECT COUNT(*) AS "count" FROM "Cars"` {
		t.Errorf("logged tx SQL = %q, want count aggregate", logs[1].SQL)
	}
}

func TestCompileErrorFlowsOut(t *testing.T) {
	// Compilation fails before the connection is touched; no database needed.
	db := exec.New(nil, compiler.NewSqlite())

	// Compile-time problems pass through as is and are discriminable via
	// errors.Is (the compiler's joined error).
	_, err := db.Get[Car](context.Background(), sqlk.NewQuery().Select("Id"))
	if !errors.Is(err, compiler.ErrNoFromTarget) {
		t.Errorf("Get(no from) err = %v, want compiler.ErrNoFromTarget", err)
	}
	_, err = db.Exists(context.Background(), sqlk.NewQuery().From("Cars").Where("Id", "~~", 1))
	if !errors.Is(err, compiler.ErrOperatorNotAllowed) {
		t.Errorf("Exists(bad operator) err = %v, want compiler.ErrOperatorNotAllowed", err)
	}
}

func TestExecWritePath(t *testing.T) {
	db, _ := newTestDB(t)
	ctx := context.Background()

	// INSERT: one affected row, and the row is retrievable.
	affected, err := db.Exec(ctx, sqlk.NewQuery().From("Cars").Insert(sqlk.Record{"Brand": "Audi", "Year": 2023}))
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if affected != 1 {
		t.Errorf("insert affected = %d, want 1", affected)
	}

	// UPDATE that matches: affected rows and written values.
	affected, err = db.Exec(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Toyota").Update(sqlk.Record{"Color": "Blue"}))
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if affected != 1 {
		t.Errorf("update affected = %d, want 1", affected)
	}
	car, err := db.First[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Toyota"))
	if err != nil {
		t.Fatalf("read after update: %v", err)
	}
	if car.Color == nil || *car.Color != "Blue" {
		t.Errorf("updated color = %v, want Blue", car.Color)
	}

	// UPDATE with no match: zero affected rows, not an error.
	affected, err = db.Exec(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Nope").Update(sqlk.Record{"Color": "Blue"}))
	if err != nil {
		t.Fatalf("update no match: %v", err)
	}
	if affected != 0 {
		t.Errorf("update no match affected = %d, want 0", affected)
	}

	// Increment/Decrement: affected rows and the numeric change.
	affected, err = db.Exec(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Honda").Increment("Year", 5))
	if err != nil {
		t.Fatalf("increment: %v", err)
	}
	if affected != 1 {
		t.Errorf("increment affected = %d, want 1", affected)
	}
	car, err = db.FirstOrDefault[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Honda"))
	if err != nil {
		t.Fatalf("read after increment: %v", err)
	}
	if car.Year != 2025 {
		t.Errorf("year after increment = %d, want 2025", car.Year)
	}
	affected, err = db.Exec(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Honda").Decrement("Year", 10))
	if err != nil {
		t.Fatalf("decrement: %v", err)
	}
	if affected != 1 {
		t.Errorf("decrement affected = %d, want 1", affected)
	}
	car, err = db.FirstOrDefault[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Honda"))
	if err != nil {
		t.Fatalf("read after decrement: %v", err)
	}
	if car.Year != 2015 {
		t.Errorf("year after decrement = %d, want 2015", car.Year)
	}

	// DELETE: zero rows when no match, row count when matched.
	affected, err = db.Exec(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Nope").Delete())
	if err != nil {
		t.Fatalf("delete no match: %v", err)
	}
	if affected != 0 {
		t.Errorf("delete no match affected = %d, want 0", affected)
	}
	affected, err = db.Exec(ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Audi").Delete())
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if affected != 1 {
		t.Errorf("delete affected = %d, want 1", affected)
	}
	count, err := db.Count[int64](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Audi"))
	if err != nil {
		t.Fatalf("count after delete: %v", err)
	}
	if count != 0 {
		t.Errorf("count after delete = %d, want 0", count)
	}
}

func TestInsertGetId(t *testing.T) {
	db, _ := newTestDB(t)
	ctx := context.Background()

	// Consecutive inserts retrieve consecutive auto-increment IDs (seeded
	// rows already use 1..3, so new rows start at 4).
	for i, brand := range []string{"Toyota", "Toyota 2", "Toyota 3"} {
		id, err := db.InsertGetId[int64](ctx, sqlk.NewQuery().From("Cars"), sqlk.Record{"Brand": brand, "Year": 1900 + i})
		if err != nil {
			t.Fatalf("InsertGetId #%d: %v", i+1, err)
		}
		if want := int64(4 + i); id != want {
			t.Errorf("InsertGetId #%d = %d, want %d", i+1, id, want)
		}
	}

	// The retrieved ID points at the newly written row.
	car, err := db.First[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Id", 4))
	if err != nil {
		t.Fatalf("read inserted row: %v", err)
	}
	if car.Brand != "Toyota" || car.Year != 1900 {
		t.Errorf("inserted row = %+v, want Toyota 1900", car)
	}

	// The transaction handle works the same way (same execution surface).
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	id, err := tx.InsertGetId[int64](ctx, sqlk.NewQuery().From("Cars"), sqlk.Record{"Brand": "InTx", "Year": 2024})
	if err != nil {
		t.Fatalf("InsertGetId in tx: %v", err)
	}
	if id != 7 {
		t.Errorf("InsertGetId in tx = %d, want 7", id)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestInsertGetIdUnsupportedCompiler(t *testing.T) {
	// A compiler without LastId support (the base compiler, Oracle) rejects
	// InsertGetId up front with a distinguishable sentinel and does not run
	// the insert, so a caller cannot mistake "unsupported" for "no rows
	// matched" (ErrNoRows) and retry into a double write.
	db, sqlxDB := newTestDB(t)
	ctx := context.Background()
	base := exec.New(sqlxDB, compiler.New())
	_, err := base.InsertGetId[int64](ctx, sqlk.NewQuery().From("Cars"), sqlk.Record{"Brand": "Unsupported", "Year": 1900})
	if !errors.Is(err, exec.ErrLastIdUnsupported) {
		t.Fatalf("InsertGetId error = %v, want ErrLastIdUnsupported", err)
	}
	if errors.Is(err, exec.ErrNoRows) {
		t.Errorf("errors.Is(err, ErrNoRows) = true, want false (%v)", err)
	}
	// The insert was not run: no row with the unsupported brand exists.
	count, err := db.Count[int64](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Unsupported"))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("rows written for unsupported compiler = %d, want 0", count)
	}
}

func TestPaginate(t *testing.T) {
	db, _ := newTestDB(t)
	ctx := context.Background()

	// Seed 12 more rows, 15 in total.
	for i := range 12 {
		if _, err := db.InsertGetId[int64](ctx, sqlk.NewQuery().From("Cars"), sqlk.Record{"Brand": fmt.Sprintf("Extra %02d", i), "Year": 2000 + i}); err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}
	base := sqlk.NewQuery().From("Cars").OrderBy("Id")

	p1, err := db.Paginate[Car](ctx, base, 1, 6)
	if err != nil {
		t.Fatalf("paginate page 1: %v", err)
	}
	if p1.Total != 15 || p1.Page != 1 || p1.PerPage != 6 || len(p1.List) != 6 || !p1.HasMore() {
		t.Errorf("page 1 = {total %d, page %d, perPage %d, list %d, hasMore %v}, want {15, 1, 6, 6, true}", p1.Total, p1.Page, p1.PerPage, len(p1.List), p1.HasMore())
	}
	if p1.List[0].Brand != "Honda" {
		t.Errorf("page 1 starts with %q, want Honda", p1.List[0].Brand)
	}

	p2, err := db.Paginate[Car](ctx, base, 2, 6)
	if err != nil {
		t.Fatalf("paginate page 2: %v", err)
	}
	if len(p2.List) != 6 || !p2.HasMore() {
		t.Errorf("page 2 = {list %d, hasMore %v}, want {6, true}", len(p2.List), p2.HasMore())
	}
	if p2.List[0].Id != 7 {
		t.Errorf("page 2 starts with id %d, want 7 (offset applied)", p2.List[0].Id)
	}

	p3, err := db.Paginate[Car](ctx, base, 3, 6)
	if err != nil {
		t.Fatalf("paginate page 3: %v", err)
	}
	if len(p3.List) != 3 || p3.HasMore() {
		t.Errorf("page 3 = {list %d, hasMore %v}, want {3, false}", len(p3.List), p3.HasMore())
	}

	// Out-of-range page: empty list, no next page.
	p4, err := db.Paginate[Car](ctx, base, 4, 6)
	if err != nil {
		t.Fatalf("paginate page 4: %v", err)
	}
	if len(p4.List) != 0 || p4.HasMore() {
		t.Errorf("page 4 = {list %d, hasMore %v}, want {0, false}", len(p4.List), p4.HasMore())
	}

	// Empty result: total 0, no list query issued.
	empty, err := db.Paginate[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Nope"), 1, 6)
	if err != nil {
		t.Fatalf("paginate empty: %v", err)
	}
	if empty.Total != 0 || len(empty.List) != 0 || empty.HasMore() {
		t.Errorf("empty paginate = {total %d, list %d, hasMore %v}, want {0, 0, false}", empty.Total, len(empty.List), empty.HasMore())
	}

	// Invalid arguments: page/perPage below 1 return a distinguishable
	// error.
	if _, err := db.Paginate[Car](ctx, base, 0, 6); !errors.Is(err, exec.ErrInvalidPagination) {
		t.Errorf("paginate page 0 err = %v, want ErrInvalidPagination", err)
	}
	if _, err := db.Paginate[Car](ctx, base, 1, 0); !errors.Is(err, exec.ErrInvalidPagination) {
		t.Errorf("paginate perPage 0 err = %v, want ErrInvalidPagination", err)
	}

	// Paginate leaves the caller's query untouched: reusing base still
	// fetches everything.
	all, err := db.Get[Car](ctx, base)
	if err != nil {
		t.Fatalf("get after paginate: %v", err)
	}
	if len(all) != 15 {
		t.Errorf("get after paginate = %d rows, want 15 (query was mutated)", len(all))
	}
}

func TestPaginationResultHasMore(t *testing.T) {
	// Hand-constructed results pin HasMore's arithmetic without a
	// database. The extreme pair below overflows int64 as a product
	// ((1<<33)*(1<<31) == 1<<64, which wraps to 0 and would flip the old
	// product comparison to true); the divided comparison stays correct.
	huge := exec.PaginationResult[Car]{Page: 1 << 33, PerPage: 1 << 31, Total: 100}
	if huge.HasMore() {
		t.Errorf("HasMore() = true for page %d of %d per page over total %d, want false", huge.Page, huge.PerPage, huge.Total)
	}
	tests := []struct {
		name          string
		page, perPage int
		total         int64
		want          bool
	}{
		{"exact multiple has no more", 3, 5, 15, false},
		{"remainder leaves one more", 2, 5, 15, true},
		{"last partial page has no more", 3, 5, 13, false},
		{"zero per page has no more", 1, 0, 15, false},
		{"page one of empty total", 1, 5, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := exec.PaginationResult[Car]{Page: tt.page, PerPage: tt.perPage, Total: tt.total}
			if got := r.HasMore(); got != tt.want {
				t.Errorf("HasMore() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChunk(t *testing.T) {
	db, _ := newTestDB(t)
	ctx := context.Background()

	// Seed 4 more rows, 7 in total.
	for i := range 4 {
		if _, err := db.InsertGetId[int64](ctx, sqlk.NewQuery().From("Cars"), sqlk.Record{"Brand": fmt.Sprintf("Extra %d", i), "Year": 2010 + i}); err != nil {
			t.Fatalf("seed row %d: %v", i, err)
		}
	}

	// Full iteration: chunks of 3/3/1, stopping once exhausted.
	var sizes []int
	for rows, err := range db.Chunk[Car](ctx, sqlk.NewQuery().From("Cars").OrderBy("Id"), 3) {
		if err != nil {
			t.Fatalf("chunk: %v", err)
		}
		sizes = append(sizes, len(rows))
	}
	if len(sizes) != 3 || sizes[0] != 3 || sizes[1] != 3 || sizes[2] != 1 {
		t.Errorf("chunk sizes = %v, want [3 3 1]", sizes)
	}

	// Early exit: after break no more pages are fetched; only the first
	// chunk is yielded.
	yields := 0
	for rows, err := range db.Chunk[Car](ctx, sqlk.NewQuery().From("Cars"), 3) {
		if err != nil {
			t.Fatalf("chunk with break: %v", err)
		}
		yields++
		if len(rows) != 3 {
			t.Errorf("first chunk = %d rows, want 3", len(rows))
		}
		break
	}
	if yields != 1 {
		t.Errorf("chunk with break yielded %d times, want 1", yields)
	}

	// Empty result: one empty chunk is yielded.
	yields = 0
	for rows, err := range db.Chunk[Car](ctx, sqlk.NewQuery().From("Cars").WhereEq("Brand", "Nope"), 3) {
		if err != nil {
			t.Fatalf("chunk empty: %v", err)
		}
		yields++
		if len(rows) != 0 {
			t.Errorf("empty chunk = %d rows, want 0", len(rows))
		}
	}
	if yields != 1 {
		t.Errorf("empty chunk yielded %d times, want 1", yields)
	}

	// Errors surface through the iteration and end the sequence.
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	sawErr := false
	for rows, err := range db.Chunk[Car](canceled, sqlk.NewQuery().From("Cars"), 3) {
		if !errors.Is(err, context.Canceled) {
			t.Errorf("chunk canceled ctx err = %v, want context.Canceled", err)
		}
		if rows != nil {
			t.Errorf("chunk canceled ctx rows = %v, want nil", rows)
		}
		sawErr = true
		break
	}
	if !sawErr {
		t.Error("chunk canceled ctx produced no error")
	}
}
