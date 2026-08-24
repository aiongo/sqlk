package qdata

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/aiongo/sqlk"
)

// filter is the condition tree of the wire format: groups nested to any
// depth, each carrying a group_op; rules compile onto the core Where surface
// with the sixteen operator codes (see compileFilter).

// Wire-format group_op values.
const (
	GroupOpAnd = "and"
	GroupOpOr  = "or"
)

// The sixteen operator codes of wire-format rules.
const (
	OpEq = "eq" // equals
	OpNe = "ne" // not equals
	OpLt = "lt" // less than
	OpLe = "le" // less than or equal
	OpGt = "gt" // greater than
	OpGe = "ge" // greater than or equal
	OpIn = "in" // in the set
	OpNi = "ni" // not in the set
	OpIs = "is" // is NULL
	OpNs = "ns" // is not NULL
	OpBw = "bw" // prefix match
	OpBn = "bn" // negated prefix match
	OpEw = "ew" // suffix match
	OpEn = "en" // negated suffix match
	OpCn = "cn" // contains match
	OpNc = "nc" // does not contain match
)

// Filter is one condition group: rules and subgroups within are connected by
// group_op; wire-format keys group_op/rules/groups. An empty GroupOp
// defaults to and (validation and compilation agree; the field is not
// written back).
type Filter struct {
	GroupOp string   `json:"group_op"`
	Rules   []Rule   `json:"rules"`
	Groups  []Filter `json:"groups"`
}

// NewFilter returns an empty condition group.
func NewFilter() *Filter {
	return &Filter{}
}

// WithGroupOp sets how the members of this group are connected.
func (f *Filter) WithGroupOp(op string) *Filter {
	f.GroupOp = op
	return f
}

// WithRule appends a rule.
func (f *Filter) WithRule(rule Rule) *Filter {
	f.Rules = append(f.Rules, rule)
	return f
}

// WithGroup appends a subgroup.
func (f *Filter) WithGroup(group Filter) *Filter {
	f.Groups = append(f.Groups, group)
	return f
}

// validate checks this layer and all subgroups: an invalid group_op is
// rejected (the empty string defaults to and and is not a problem), and
// every rule's field and op are checked -- rules with empty data take part
// too.
func (f *Filter) validate() []error {
	var errs []error
	if f.GroupOp != "" && f.GroupOp != GroupOpAnd && f.GroupOp != GroupOpOr {
		errs = append(errs, &GroupOpError{Value: f.GroupOp})
	}
	for i := range f.Rules {
		errs = append(errs, f.Rules[i].validate()...)
	}
	for i := range f.Groups {
		errs = append(errs, f.Groups[i].validate()...)
	}
	return errs
}

// Rule is one filter rule: a column-operator-value triple with wire-format
// keys field/op/data. data accepts scalars and arrays alike (for the set
// operators); rules with empty data (nil, empty string, empty array) are
// skipped at compile time but still validated; is/ns ignore data.
type Rule struct {
	Field string `json:"field"`
	Op    string `json:"op"`
	Data  any    `json:"data"`
}

// NewRule creates a filter rule.
func NewRule(field, op string, data any) *Rule {
	return &Rule{Field: field, Op: op, Data: data}
}

// validate checks that field is non-empty and op is one of the sixteen
// operator codes (operatorList is the single source; the linear scan of 16
// items is no cost at validation time).
func (r *Rule) validate() []error {
	var errs []error
	if r.Field == "" {
		errs = append(errs, ErrRuleFieldRequired)
	}
	if !slices.Contains(operatorList, r.Op) {
		errs = append(errs, &OpError{Field: r.Field, Op: r.Op})
	}
	return errs
}

// resolvedGroup is the condition tree after Hook rewriting and empty-data
// removal: or records this group's connective, and rule data is normalized
// per operator (see normalizeData). With errors and skips stripped away,
// emit is a pure mapping of data onto builder verbs.
type resolvedGroup struct {
	or     bool
	rules  []Rule
	groups []resolvedGroup
}

// compileFilter compiles the condition tree into out's Where section: it
// first resolves via resolveFilter (the Hook.Rule pointcut and empty-data
// removal; errors return immediately), then emit lands the result purely;
// when every rule is removed the tree is empty and emit adds no condition
// (nested empty groups are omitted too).
func compileFilter(out *sqlk.Query, hook Hook, f *Filter) error {
	resolved, err := resolveFilter(hook, f)
	if err != nil {
		return err
	}
	resolved.emit(out)
	return nil
}

// resolveFilter resolves one condition group recursively: each rule first
// passes through Hook.Rule for rewriting and admission (errors propagate as
// is), and the rewritten rule is validated again -- a hook can only tighten
// validation, never loosen it. Rules with empty data are dropped here
// (except is/ns, which use no data), and the remaining rules' data is
// normalized per operator. Subgroups recurse the same way.
func resolveFilter(hook Hook, f *Filter) (*resolvedGroup, error) {
	group := &resolvedGroup{or: f.GroupOp == GroupOpOr}
	for i := range f.Rules {
		rule, err := hook.Rule(f.Rules[i])
		if err != nil {
			return nil, err
		}
		if errs := rule.validate(); len(errs) > 0 {
			return nil, errors.Join(errs...)
		}
		if skipRule(rule.Op, rule.Data) {
			continue
		}
		rule.Data = normalizeData(rule.Op, rule.Data)
		group.rules = append(group.rules, rule)
	}
	for i := range f.Groups {
		sub, err := resolveFilter(hook, &f.Groups[i])
		if err != nil {
			return nil, err
		}
		group.groups = append(group.groups, *sub)
	}
	return group, nil
}

// emit lands the resolved condition tree onto core builder verbs: rules are
// appended per opBuilders, subgroups as parenthesized condition groups, each
// choosing the and/or form of this group's connective. The signature matches
// the core's parenthesized-group callback shape (func(*Query) *Query), so it
// can be passed whole as the group callback of WhereGroup/OrWhereGroup.
func (g *resolvedGroup) emit(q *sqlk.Query) *sqlk.Query {
	for _, rule := range g.rules {
		build := opBuilders[rule.Op]
		if g.or {
			q = build.or(q, rule.Field, rule.Data)
		} else {
			q = build.and(q, rule.Field, rule.Data)
		}
	}
	for i := range g.groups {
		if g.or {
			q = q.OrWhereGroup(g.groups[i].emit)
		} else {
			q = q.WhereGroup(g.groups[i].emit)
		}
	}
	return q
}

// opBuilder compiles one operator code into an appender of core Where
// conditions: and/or are the two connective forms of the same condition
// within a group. The data the appenders receive is already normalized per
// operator: the set family gets []any, the Like family a string, and is/ns
// use no data.
type opBuilder struct {
	and func(q *sqlk.Query, field string, data any) *sqlk.Query
	or  func(q *sqlk.Query, field string, data any) *sqlk.Query
}

// compare returns the appender for a comparison operator (eq/ne/lt/le/gt/ge
// map to =, !=, <, <=, >, >=); every operator is on the compiler's built-in
// whitelist.
func compare(sqlOperator string) opBuilder {
	return opBuilder{
		and: func(q *sqlk.Query, field string, data any) *sqlk.Query {
			return q.Where(field, sqlOperator, data)
		},
		or: func(q *sqlk.Query, field string, data any) *sqlk.Query {
			return q.OrWhere(field, sqlOperator, data)
		},
	}
}

// likeMethod is the shared signature of the core Like-family methods
// (identical across starts/ends/contains and all not/or combinations).
type likeMethod func(q *sqlk.Query, column, value string, opts ...sqlk.MatchOption) *sqlk.Query

// like adapts a core Like-family method into an operator appender. It always
// attaches CaseSensitive to produce a plain LIKE (no LOWER wrapper), leaving
// case sensitivity to the database collation; wildcard concatenation
// (prefix/suffix/contains) is landed by the core builder per the shape code.
func like(method likeMethod) func(q *sqlk.Query, field string, data any) *sqlk.Query {
	return func(q *sqlk.Query, field string, data any) *sqlk.Query {
		return method(q, field, data.(string), sqlk.CaseSensitive())
	}
}

// opBuilders maps the sixteen operator codes onto core Where capabilities:
// the comparison family goes through whitelisted-operator conditions, in/ni
// through value-list set conditions, is/ns through NULL tests, and the Like
// family through starts/ends/contains and their negations.
var opBuilders = map[string]opBuilder{
	OpEq: compare("="),
	OpNe: compare("!="),
	OpLt: compare("<"),
	OpLe: compare("<="),
	OpGt: compare(">"),
	OpGe: compare(">="),
	OpIn: {
		and: func(q *sqlk.Query, field string, data any) *sqlk.Query {
			return q.WhereIn(field, data.([]any)...)
		},
		or: func(q *sqlk.Query, field string, data any) *sqlk.Query {
			return q.OrWhereIn(field, data.([]any)...)
		},
	},
	OpNi: {
		and: func(q *sqlk.Query, field string, data any) *sqlk.Query {
			return q.WhereNotIn(field, data.([]any)...)
		},
		or: func(q *sqlk.Query, field string, data any) *sqlk.Query {
			return q.OrWhereNotIn(field, data.([]any)...)
		},
	},
	OpIs: {
		and: func(q *sqlk.Query, field string, _ any) *sqlk.Query { return q.WhereNull(field) },
		or:  func(q *sqlk.Query, field string, _ any) *sqlk.Query { return q.OrWhereNull(field) },
	},
	OpNs: {
		and: func(q *sqlk.Query, field string, _ any) *sqlk.Query { return q.WhereNotNull(field) },
		or:  func(q *sqlk.Query, field string, _ any) *sqlk.Query { return q.OrWhereNotNull(field) },
	},
	OpBw: {and: like((*sqlk.Query).WhereStarts), or: like((*sqlk.Query).OrWhereStarts)},
	OpBn: {and: like((*sqlk.Query).WhereNotStarts), or: like((*sqlk.Query).OrWhereNotStarts)},
	OpEw: {and: like((*sqlk.Query).WhereEnds), or: like((*sqlk.Query).OrWhereEnds)},
	OpEn: {and: like((*sqlk.Query).WhereNotEnds), or: like((*sqlk.Query).OrWhereNotEnds)},
	OpCn: {and: like((*sqlk.Query).WhereContains), or: like((*sqlk.Query).OrWhereContains)},
	OpNc: {and: like((*sqlk.Query).WhereNotContains), or: like((*sqlk.Query).OrWhereNotContains)},
}

// skipRule reports whether a rule is skipped at compile time for empty
// data: is/ns use no data and are never skipped; the other operators skip
// when data is nil, an empty string, or an empty array (arrays are widened
// through valuesOf before the length check).
func skipRule(op string, data any) bool {
	if op == OpIs || op == OpNs {
		return false
	}
	switch v := data.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	default:
		return len(valuesOf(data)) == 0
	}
}

// normalizeData normalizes data per operator: the set family is widened to a
// value slice (arrays expanded, a single scalar wrapped); the Like family is
// widened to a string (scalars such as numbers via %v text); the rest pass
// through. Precondition: data is non-empty (see skipRule).
func normalizeData(op string, data any) any {
	switch op {
	case OpIn, OpNi:
		return valuesOf(data)
	case OpBw, OpBn, OpEw, OpEn, OpCn, OpNc:
		return fmt.Sprint(data)
	default:
		return data
	}
}

// valuesOf widens data into a value slice: arrays ([]any, []string, and
// other slice or array types) expand element by element; a single scalar is
// wrapped into a one-element slice.
func valuesOf(data any) []any {
	switch v := data.(type) {
	case []any:
		return v
	case []string:
		values := make([]any, len(v))
		for i, s := range v {
			values[i] = s
		}
		return values
	}
	value := reflect.ValueOf(data)
	if value.Kind() == reflect.Slice || value.Kind() == reflect.Array {
		values := make([]any, value.Len())
		for i := range values {
			values[i] = value.Index(i).Interface()
		}
		return values
	}
	return []any{data}
}
