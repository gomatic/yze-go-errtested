package errtested

// Reading a table case's expectation POSITIONALLY. A case that binds its
// sentinel by position — {"empty", ErrEmpty} — asserts exactly what a keyed
// one does, so both forms are read alike. The scan is syntax-only, so a
// case's fields must be visible in the same file: written inline, as
// table-driven tests usually do, or declared there as a named struct.

import "go/ast"

// caseType is a table case's struct type name.
type caseType string

// fieldIndex is a field's position in its struct's declaration order.
type fieldIndex int

// expectationPositions indexes, for each struct type the file declares, the
// position its want/expect-prefixed field occupies — what an unkeyed case
// literal fills at that position. A struct with no such field is absent, so
// nothing about it is ever read positionally.
func expectationPositions(file *ast.File) map[caseType]fieldIndex {
	positions := map[caseType]fieldIndex{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			recordExpectationPosition(spec, positions)
		}
	}
	return positions
}

// recordExpectationPosition records spec's expectation-field position when it
// declares a struct carrying one.
func recordExpectationPosition(spec ast.Spec, into map[caseType]fieldIndex) {
	typed, ok := spec.(*ast.TypeSpec)
	if !ok {
		return
	}
	structure, ok := typed.Type.(*ast.StructType)
	if !ok || structure.Fields == nil {
		return
	}
	if at, found := expectationIndex(structure.Fields.List); found {
		into[caseType(typed.Name.Name)] = at
	}
}

// expectationIndex is the position of the first want/expect-prefixed field,
// counting each name of a grouped field separately — the order an unkeyed
// literal fills.
func expectationIndex(list []*ast.Field) (fieldIndex, bool) {
	for at, name := range positionalNames(list) {
		if isExpectation(memberName(name)) {
			return fieldIndex(at), true
		}
	}
	return 0, false
}

// positionalNames flattens a struct's fields into the order an unkeyed literal
// fills them, one entry per name; an embedded field contributes one unnamed
// position.
func positionalNames(list []*ast.Field) []string {
	var names []string
	for _, field := range list {
		if len(field.Names) == 0 {
			names = append(names, "")
			continue
		}
		for _, name := range field.Names {
			names = append(names, name.Name)
		}
	}
	return names
}

// fromUnkeyedCases records the sentinel each unkeyed case literal binds at its
// type's expectation position. A positional binding is the same assertion a
// keyed one makes — `{"empty", ErrEmpty}` and `{name: "empty", wantErr:
// ErrEmpty}` say the same thing — so the two forms are read alike. The scan is
// syntax-only, so the case's fields must be visible here: written inline, as
// table-driven tests usually do, or declared as a named struct in the same
// file. Anything else is left to the keyed path.
func fromUnkeyedCases(lit *ast.CompositeLit, fields map[caseType]fieldIndex, into assertions) {
	if at, found := expectationAt(lit.Type, fields); found {
		markCase(lit, at, into)
		return
	}
	element, ok := elementExpr(lit.Type)
	if !ok {
		return
	}
	at, found := expectationAt(element, fields)
	if !found {
		return
	}
	for _, entry := range lit.Elts {
		if nested, ok := entry.(*ast.CompositeLit); ok {
			markCase(nested, at, into)
		}
	}
}

// expectationAt is the position a case type's want/expect field occupies:
// computed directly from an inline struct, or looked up for a named struct the
// file declares. A type carrying no expectation field is not read positionally.
func expectationAt(node ast.Expr, fields map[caseType]fieldIndex) (fieldIndex, bool) {
	switch typed := node.(type) {
	case *ast.StructType:
		if typed.Fields == nil {
			return 0, false
		}
		return expectationIndex(typed.Fields.List)
	case *ast.Ident:
		at, ok := fields[caseType(typed.Name)]
		return at, ok
	}
	return 0, false
}

// elementExpr is the element type expression of a slice, array, or map literal
// — the type its elided element literals carry.
func elementExpr(node ast.Expr) (ast.Expr, bool) {
	switch typed := node.(type) {
	case *ast.ArrayType:
		return typed.Elt, true
	case *ast.MapType:
		return typed.Value, true
	}
	return nil, false
}

// markCase records the sentinel lit binds at position at, leaving a keyed
// element to the keyed path.
func markCase(lit *ast.CompositeLit, at fieldIndex, into assertions) {
	if int(at) >= len(lit.Elts) {
		return
	}
	if _, keyed := lit.Elts[at].(*ast.KeyValueExpr); keyed {
		return
	}
	into.mark(nameOf(lit.Elts[at]))
}
