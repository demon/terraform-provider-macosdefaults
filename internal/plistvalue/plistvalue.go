// Package plistvalue converts between Terraform values and typed property list
// values, and decides whether two of them are equal.
//
// It is the only place that infers a plist type from an HCL value, and the
// only place that compares Terraform state against what's on disk. See the
// "Type model" section of DEVELOPMENT.md.
package plistvalue

import (
	"bytes"
	"maps"
	"math/big"
	"slices"
	"time"
)

// Kind is a property list type. The names are the values of the `type`
// attribute.
type Kind string

const (
	KindString  Kind = "string"
	KindInteger Kind = "integer"
	KindFloat   Kind = "float"
	KindBoolean Kind = "boolean"
	KindDate    Kind = "date"
	KindData    Kind = "data"
	KindArray   Kind = "array"
	KindDict    Kind = "dict"
)

// Kinds lists every Kind, in documentation order.
func Kinds() []Kind {
	return []Kind{KindString, KindInteger, KindFloat, KindBoolean, KindDate, KindData, KindArray, KindDict}
}

// Valid reports whether k is one of Kinds.
func (k Kind) Valid() bool {
	return slices.Contains(Kinds(), k)
}

// Value is one property list value. Only the field for its Kind is
// meaningful.
type Value struct {
	Kind Kind

	Str   string           // KindString
	Int   *big.Int         // KindInteger; plist integers can exceed int64
	Float float64          // KindFloat
	Bool  bool             // KindBoolean
	Date  time.Time        // KindDate, always UTC with one-second precision
	Data  []byte           // KindData
	Array []Value          // KindArray
	Dict  map[string]Value // KindDict
}

// String returns a string value.
func String(s string) Value { return Value{Kind: KindString, Str: s} }

// Integer returns an integer value.
func Integer(i int64) Value { return Value{Kind: KindInteger, Int: big.NewInt(i)} }

// Float returns a float (plist <real>) value.
func Float(f float64) Value { return Value{Kind: KindFloat, Float: f} }

// Boolean returns a boolean value.
func Boolean(b bool) Value { return Value{Kind: KindBoolean, Bool: b} }

// Date returns a date value, normalised to UTC and truncated to the second,
// which is all an XML plist can store.
func Date(t time.Time) Value {
	return Value{Kind: KindDate, Date: t.UTC().Truncate(time.Second)}
}

// Data returns a data value.
func Data(b []byte) Value { return Value{Kind: KindData, Data: b} }

// Array returns an array value.
func Array(vs ...Value) Value { return Value{Kind: KindArray, Array: vs} }

// Dict returns a dict value.
func Dict(m map[string]Value) Value { return Value{Kind: KindDict, Dict: m} }

// Equal reports whether a and b are the same preference value.
//
// Values of the same kind compare structurally: floats exactly, dates by
// instant, dict keys in any order, arrays in order. The one cross-kind rule is
// that a boolean equals the integer 1 (true) or 0 (false), because many apps
// rewrite booleans that way and that isn't drift.
func Equal(a, b Value) bool {
	if a.Kind != b.Kind {
		return boolIntEqual(a, b) || boolIntEqual(b, a)
	}
	switch a.Kind {
	case KindString:
		return a.Str == b.Str
	case KindInteger:
		return a.Int != nil && b.Int != nil && a.Int.Cmp(b.Int) == 0
	case KindFloat:
		return a.Float == b.Float
	case KindBoolean:
		return a.Bool == b.Bool
	case KindDate:
		return a.Date.Equal(b.Date)
	case KindData:
		return bytes.Equal(a.Data, b.Data)
	case KindArray:
		return slices.EqualFunc(a.Array, b.Array, Equal)
	case KindDict:
		return maps.EqualFunc(a.Dict, b.Dict, Equal)
	}
	return false
}

func boolIntEqual(boolean, integer Value) bool {
	if boolean.Kind != KindBoolean || integer.Kind != KindInteger || integer.Int == nil {
		return false
	}
	want := int64(0)
	if boolean.Bool {
		want = 1
	}
	return integer.Int.IsInt64() && integer.Int.Int64() == want
}
