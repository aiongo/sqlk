package core

// Record is one row of column/value pairs: the shape carried by the
// key-value forms of the write verbs (`Insert`, `InsertReturnId`,
// `Update`) and the equality-shorthand condition maps (`WhereMap`,
// `HavingMap`). It is an alias for map[string]any, so a plain
// map[string]any literal is interchangeable with it.
type Record = map[string]any
