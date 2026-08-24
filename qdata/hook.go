package qdata

// Hook is called at its pointcuts while a qdata.QData is converted into a
// *sqlk.Query (see QData.ToQuery), for security interception and field
// rewriting: returning a rewritten value feeds the rewrite into the
// conversion, returning an error aborts it, and the error propagates as
// is -- callers discriminate the interception reason via errors.Is with
// their own sentinel.
//
// Every pointcut fires before the value takes part in building: From sees
// the whole fetch-target list (primary table and conventional JOIN tables;
// rewrite it wholesale -- adding or dropping join tables, swapping the
// primary table all happen here), Select sees each projection item, OrderBy
// sees each orderby by, and Rule sees each filter rule (including rules
// about to be skipped for empty data). The raw-expression test for select
// works on the rewritten value (an item whose rewrite injects "(" takes the
// raw path too). A rewritten rule field compiles as an identifier (no
// raw-expression path); an empty rewritten field or an invalid op is still
// rejected, a rule whose rewritten data is empty is still skipped, and a
// rewritten from list that is empty or holds an empty element is still
// rejected -- a hook can only tighten validation, never loosen it. Count
// aggregate queries go through the From and Rule pointcuts (filter and
// conventional JOINs are kept); projection, ordering, and pagination are
// not applied and have no pointcut.
//
// Passing nil to ToQuery disables interception.
type Hook interface {
	From(from []string) ([]string, error)
	Select(column string) (string, error)
	OrderBy(by string) (string, error)
	Rule(rule Rule) (Rule, error)
}
