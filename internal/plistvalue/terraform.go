package plistvalue

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"
	"slices"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

// ErrUnknown means a Terraform value isn't fully known yet (it depends on
// something computed during apply). Callers skip validation and inference
// until it is.
var ErrUnknown = errors.New("value is not yet known")

// FromTerraform converts a Terraform value (typically a types.Dynamic) into a
// plist value.
//
// With an empty override the kind is inferred: strings, bools, whole numbers
// (integer), fractional numbers (float), lists/tuples/sets (array) and
// maps/objects (dict), recursively. A non-empty override applies to the top
// level only, and must be compatible with the HCL value: a number for float
// or integer, an RFC 3339 string for date, a base64 string for data, and the
// matching shape for everything else.
//
// It returns an error wrapping ErrUnknown if any part of v is unknown, and a
// descriptive error (naming the path inside the value) if v can't be stored.
func FromTerraform(v attr.Value, override Kind) (Value, error) {
	if override != "" && !override.Valid() {
		return Value{}, fmt.Errorf("unknown type %q", override)
	}
	return fromTF(v, override, "")
}

func fromTF(v attr.Value, override Kind, at string) (Value, error) {
	if v == nil || v.IsNull() {
		return Value{}, pathErr(at, errors.New("null values can't be stored in a preference"))
	}
	if v.IsUnknown() {
		return Value{}, ErrUnknown
	}

	switch tv := v.(type) {
	case basetypes.DynamicValue:
		return fromTF(tv.UnderlyingValue(), override, at)

	case basetypes.StringValue:
		return fromString(tv.ValueString(), override, at)

	case basetypes.BoolValue:
		if err := expect(override, at, "a bool", KindBoolean); err != nil {
			return Value{}, err
		}
		return Boolean(tv.ValueBool()), nil

	case basetypes.NumberValue:
		return fromNumber(tv.ValueBigFloat(), override, at)

	case basetypes.ListValue:
		return fromElements(tv.Elements(), override, at)
	case basetypes.TupleValue:
		return fromElements(tv.Elements(), override, at)
	case basetypes.SetValue:
		return fromElements(tv.Elements(), override, at)

	case basetypes.MapValue:
		return fromAttributes(tv.Elements(), override, at)
	case basetypes.ObjectValue:
		return fromAttributes(tv.Attributes(), override, at)

	// Primitive types with the more specific Int64/Float64 wrappers can come
	// from nested schema types; handle them like numbers.
	case basetypes.Int64Value:
		return fromNumber(new(big.Float).SetInt64(tv.ValueInt64()), override, at)
	case basetypes.Float64Value:
		return fromNumber(big.NewFloat(tv.ValueFloat64()), override, at)
	}
	return Value{}, pathErr(at, fmt.Errorf("values of type %s can't be stored in a preference", v.Type(context.Background())))
}

func fromString(s string, override Kind, at string) (Value, error) {
	if err := CheckXMLText(s); err != nil {
		return Value{}, pathErr(at, err)
	}
	switch override {
	case "", KindString:
		return String(s), nil
	case KindDate:
		t, err := ParseDate(s)
		if err != nil {
			return Value{}, pathErr(at, err)
		}
		return Date(t), nil
	case KindData:
		b, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return Value{}, pathErr(at, fmt.Errorf("type = \"data\" needs a base64 string: %w", err))
		}
		return Data(b), nil
	}
	return Value{}, mismatch(at, override, "a string")
}

// ParseDate parses an RFC 3339 timestamp with whole-second precision, which
// is all a plist date can hold.
func ParseDate(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("type = \"date\" needs an RFC 3339 timestamp such as \"2024-01-02T15:04:05Z\": %w", err)
	}
	if t.Nanosecond() != 0 {
		return time.Time{}, fmt.Errorf("plist dates have one-second precision; %q has fractional seconds", s)
	}
	return t, nil
}

func fromNumber(f *big.Float, override Kind, at string) (Value, error) {
	if f == nil {
		return Value{}, pathErr(at, errors.New("number has no value"))
	}
	whole := f.IsInt()
	switch {
	case override == KindInteger || override == "" && whole:
		if !whole {
			return Value{}, pathErr(at, fmt.Errorf("type = \"integer\" needs a whole number, got %s", f.Text('g', -1)))
		}
		i, _ := f.Int(nil)
		return Value{Kind: KindInteger, Int: i}, nil
	case override == KindFloat || override == "":
		v, _ := f.Float64()
		if math.IsInf(v, 0) {
			return Value{}, pathErr(at, fmt.Errorf("%s is too large for a plist real", f.Text('g', -1)))
		}
		return Float(v), nil
	}
	return Value{}, mismatch(at, override, "a number")
}

func fromElements(elems []attr.Value, override Kind, at string) (Value, error) {
	if err := expect(override, at, "a list", KindArray); err != nil {
		return Value{}, err
	}
	out := make([]Value, 0, len(elems))
	for i, e := range elems {
		v, err := fromTF(e, "", fmt.Sprintf("%s[%d]", at, i))
		if err != nil {
			return Value{}, err
		}
		out = append(out, v)
	}
	return Array(out...), nil
}

func fromAttributes(attrs map[string]attr.Value, override Kind, at string) (Value, error) {
	if err := expect(override, at, "a map or object", KindDict); err != nil {
		return Value{}, err
	}
	out := make(map[string]Value, len(attrs))
	for _, k := range slices.Sorted(maps.Keys(attrs)) {
		if err := CheckXMLText(k); err != nil {
			return Value{}, pathErr(at, fmt.Errorf("key %q: %w", k, err))
		}
		v, err := fromTF(attrs[k], "", fmt.Sprintf("%s[%q]", at, k))
		if err != nil {
			return Value{}, err
		}
		out[k] = v
	}
	return Dict(out), nil
}

func expect(override Kind, at, got string, want Kind) error {
	if override == "" || override == want {
		return nil
	}
	return mismatch(at, override, got)
}

func mismatch(at string, override Kind, got string) error {
	return pathErr(at, fmt.Errorf("type = %q doesn't match %s value", override, got))
}

func pathErr(at string, err error) error {
	if at == "" {
		return err
	}
	return fmt.Errorf("value%s: %w", at, err)
}

// ToTerraform converts a plist value into the Terraform value a user would
// write for it, wrapped as a types.Dynamic: dates become RFC 3339 strings,
// data becomes base64, arrays become tuples and dicts become objects.
func ToTerraform(v Value) (types.Dynamic, error) {
	tv, err := toTF(v, "")
	if err != nil {
		return types.DynamicNull(), err
	}
	return types.DynamicValue(tv), nil
}

func toTF(v Value, at string) (attr.Value, error) {
	switch v.Kind {
	case KindString:
		return types.StringValue(v.Str), nil
	case KindInteger:
		if v.Int == nil {
			return nil, pathErr(at, errors.New("integer has no value"))
		}
		return types.NumberValue(new(big.Float).SetInt(v.Int)), nil
	case KindFloat:
		if math.IsNaN(v.Float) || math.IsInf(v.Float, 0) {
			return nil, pathErr(at, fmt.Errorf("%v can't be represented as a Terraform number", v.Float))
		}
		return types.NumberValue(big.NewFloat(v.Float)), nil
	case KindBoolean:
		return types.BoolValue(v.Bool), nil
	case KindDate:
		return types.StringValue(v.Date.UTC().Format(time.RFC3339)), nil
	case KindData:
		return types.StringValue(base64.StdEncoding.EncodeToString(v.Data)), nil
	case KindArray:
		elemTypes := make([]attr.Type, 0, len(v.Array))
		elems := make([]attr.Value, 0, len(v.Array))
		for i, e := range v.Array {
			tv, err := toTF(e, fmt.Sprintf("%s[%d]", at, i))
			if err != nil {
				return nil, err
			}
			elemTypes = append(elemTypes, tv.Type(context.Background()))
			elems = append(elems, tv)
		}
		// The types come from the values themselves, so they can't mismatch.
		return types.TupleValueMust(elemTypes, elems), nil
	case KindDict:
		attrTypes := make(map[string]attr.Type, len(v.Dict))
		attrs := make(map[string]attr.Value, len(v.Dict))
		for k, e := range v.Dict {
			tv, err := toTF(e, fmt.Sprintf("%s[%q]", at, k))
			if err != nil {
				return nil, err
			}
			attrTypes[k] = tv.Type(context.Background())
			attrs[k] = tv
		}
		return types.ObjectValueMust(attrTypes, attrs), nil
	}
	return nil, pathErr(at, fmt.Errorf("unknown kind %q", v.Kind))
}
