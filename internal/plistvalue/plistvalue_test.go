package plistvalue

import (
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// exported is real `defaults export` output (from a tf-acc. scratch domain),
// including its tabs, newlines and the multi-line <data> formatting.
const exported = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>k_array</key>
	<array>
		<integer>1</integer>
		<string>a</string>
		<array>
			<true/>
		</array>
	</array>
	<key>k_bigreal</key>
	<real>1e+21</real>
	<key>k_data</key>
	<data>
	aGVsbG8=
	</data>
	<key>k_date</key>
	<date>2024-01-02T03:04:05Z</date>
	<key>k_dict</key>
	<dict>
		<key>a</key>
		<integer>1</integer>
		<key>nested</key>
		<dict>
			<key>b</key>
			<false/>
		</dict>
	</dict>
	<key>k_emptystr</key>
	<string></string>
	<key>k_int</key>
	<integer>-42</integer>
	<key>k_real</key>
	<real>2.5</real>
	<key>k_realwhole</key>
	<real>48</real>
	<key>k_str</key>
	<string>hello &amp; -x</string>
	<key>k_true</key>
	<true/>
</dict>
</plist>
`

func exportedWant() map[string]Value {
	return map[string]Value{
		"k_array":     Array(Integer(1), String("a"), Array(Boolean(true))),
		"k_bigreal":   Float(1e21),
		"k_data":      Data([]byte("hello")),
		"k_date":      Date(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)),
		"k_dict":      Dict(map[string]Value{"a": Integer(1), "nested": Dict(map[string]Value{"b": Boolean(false)})}),
		"k_emptystr":  String(""),
		"k_int":       Integer(-42),
		"k_real":      Float(2.5),
		"k_realwhole": Float(48),
		"k_str":       String("hello & -x"),
		"k_true":      Boolean(true),
	}
}

func TestDecodeDomain(t *testing.T) {
	got, err := DecodeDomain([]byte(exported))
	if err != nil {
		t.Fatalf("DecodeDomain: %v", err)
	}
	want := exportedWant()
	if len(got) != len(want) {
		t.Errorf("got %d keys, want %d", len(got), len(want))
	}
	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			t.Errorf("missing key %q", k)
			continue
		}
		if gv.Kind != wv.Kind || !Equal(gv, wv) {
			t.Errorf("%s = %+v, want %+v", k, gv, wv)
		}
	}
}

func TestDecodeEmptyDomain(t *testing.T) {
	got, err := DecodeDomain([]byte(documentHeader + "<dict/>\n</plist>\n"))
	if err != nil {
		t.Fatalf("DecodeDomain: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d keys, want 0", len(got))
	}
}

func TestDecodeHugeInteger(t *testing.T) {
	got, err := DecodeDocument([]byte(documentHeader + "<integer>18446744073709551615</integer></plist>"))
	if err != nil {
		t.Fatalf("DecodeDocument: %v", err)
	}
	want, _ := new(big.Int).SetString("18446744073709551615", 10)
	if got.Kind != KindInteger || got.Int.Cmp(want) != 0 {
		t.Errorf("got %+v, want integer %s", got, want)
	}
}

func TestDecodeErrors(t *testing.T) {
	bad := map[string]string{
		"not xml":         "hello",
		"no plist":        "<?xml version=\"1.0\"?><dict/>",
		"empty plist":     documentHeader + "</plist>",
		"unknown element": documentHeader + "<thing/></plist>",
		"bad integer":     documentHeader + "<integer>1.5</integer></plist>",
		"bad real":        documentHeader + "<real>abc</real></plist>",
		"bad date":        documentHeader + "<date>yesterday</date></plist>",
		"bad data":        documentHeader + "<data>!!!</data></plist>",
		"dict no key":     documentHeader + "<dict><string>x</string></dict></plist>",
		"dict dangling":   documentHeader + "<dict><key>x</key></dict></plist>",
		"truncated":       documentHeader + "<dict><key>x</key><string>y",
		"two roots":       documentHeader + "<true/><false/></plist>",
	}
	for name, doc := range bad {
		if _, err := DecodeDocument([]byte(doc)); err == nil {
			t.Errorf("%s: DecodeDocument succeeded, want error", name)
		}
	}
	if _, err := DecodeDomain([]byte(documentHeader + "<array/></plist>")); err == nil {
		t.Error("DecodeDomain with an array root succeeded, want error")
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	for k, v := range exportedWant() {
		doc, err := EncodeDocument(v)
		if err != nil {
			t.Fatalf("%s: EncodeDocument: %v", k, err)
		}
		got, err := DecodeDocument(doc)
		if err != nil {
			t.Fatalf("%s: DecodeDocument(%s): %v", k, doc, err)
		}
		if got.Kind != v.Kind || !Equal(got, v) {
			t.Errorf("%s: round trip gave %+v, want %+v", k, got, v)
		}
	}
}

func TestEncodeXML(t *testing.T) {
	tests := []struct {
		v    Value
		want string
	}{
		{String("a<b & 'c'"), "<string>a&lt;b &amp; &#39;c&#39;</string>"},
		{String("line\nbreak"), "<string>line&#xA;break</string>"},
		{Integer(-3), "<integer>-3</integer>"},
		{Float(48), "<real>48</real>"},
		{Float(0.1), "<real>0.1</real>"},
		{Boolean(false), "<false/>"},
		{Date(time.Date(2024, 1, 2, 5, 4, 5, 0, time.FixedZone("x", 2*3600))), "<date>2024-01-02T03:04:05Z</date>"},
		{Data([]byte{0, 1, 2}), "<data>AAEC</data>"},
		{Array(), "<array></array>"},
		{Dict(map[string]Value{"b": Integer(2), "a": Boolean(true)}), "<dict><key>a</key><true/><key>b</key><integer>2</integer></dict>"},
	}
	for _, tt := range tests {
		got, err := EncodeXML(tt.v)
		if err != nil {
			t.Errorf("EncodeXML(%+v): %v", tt.v, err)
			continue
		}
		if got != tt.want {
			t.Errorf("EncodeXML(%+v) = %s, want %s", tt.v, got, tt.want)
		}
	}
}

func TestEncodeRejectsUnstorableText(t *testing.T) {
	for _, v := range []Value{
		String("bell\x07"),
		String("bad utf8 \xff"),
		Dict(map[string]Value{"key\x00": Boolean(true)}),
	} {
		if _, err := EncodeXML(v); err == nil {
			t.Errorf("EncodeXML(%+v) succeeded, want error", v)
		}
	}
}

func TestEqual(t *testing.T) {
	d := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	equal := [][2]Value{
		{Boolean(true), Integer(1)},
		{Integer(0), Boolean(false)},
		{Float(2.5), Float(2.5)},
		{Date(d), Date(d.In(time.FixedZone("x", 3600)))},
		{Dict(map[string]Value{"a": Integer(1), "b": String("x")}), Dict(map[string]Value{"b": String("x"), "a": Integer(1)})},
		{Array(Boolean(true)), Array(Integer(1))},
		{Array(), Array()},
		{Dict(nil), Dict(map[string]Value{})},
	}
	notEqual := [][2]Value{
		{Boolean(true), Integer(2)},
		{Boolean(true), Integer(0)},
		{Boolean(true), String("1")},
		{Integer(48), Float(48)},
		{Float(0.1), Float(0.1000001)},
		{String("a"), String("b")},
		{Array(Integer(1), Integer(2)), Array(Integer(2), Integer(1))},
		{Array(Integer(1)), Array(Integer(1), Integer(1))},
		{Dict(map[string]Value{"a": Integer(1)}), Dict(map[string]Value{"b": Integer(1)})},
		{Data([]byte("a")), Data([]byte("b"))},
		{Date(d), Date(d.Add(time.Second))},
	}
	for _, p := range equal {
		if !Equal(p[0], p[1]) || !Equal(p[1], p[0]) {
			t.Errorf("Equal(%+v, %+v) = false, want true", p[0], p[1])
		}
	}
	for _, p := range notEqual {
		if Equal(p[0], p[1]) || Equal(p[1], p[0]) {
			t.Errorf("Equal(%+v, %+v) = true, want false", p[0], p[1])
		}
	}
}

func mustTuple(t *testing.T, elems ...attr.Value) attr.Value {
	t.Helper()
	ts := make([]attr.Type, len(elems))
	for i, e := range elems {
		ts[i] = e.Type(t.Context())
	}
	v, diags := types.TupleValue(ts, elems)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return v
}

func mustObject(t *testing.T, attrs map[string]attr.Value) attr.Value {
	t.Helper()
	ts := make(map[string]attr.Type, len(attrs))
	for k, e := range attrs {
		ts[k] = e.Type(t.Context())
	}
	v, diags := types.ObjectValue(ts, attrs)
	if diags.HasError() {
		t.Fatal(diags)
	}
	return v
}

func num(f float64) attr.Value { return types.NumberValue(big.NewFloat(f)) }

func TestFromTerraformInference(t *testing.T) {
	tests := []struct {
		name string
		in   attr.Value
		want Value
	}{
		{"string", types.StringValue("x"), String("x")},
		{"bool", types.BoolValue(true), Boolean(true)},
		{"whole number", num(2), Integer(2)},
		{"negative whole", num(-7), Integer(-7)},
		{"fraction", num(2.5), Float(2.5)},
		{"tuple", mustTuple(t, num(1), types.StringValue("a")), Array(Integer(1), String("a"))},
		{"empty tuple", mustTuple(t), Array()},
		{"object", mustObject(t, map[string]attr.Value{"a": types.BoolValue(false), "n": mustTuple(t, num(0.5))}),
			Dict(map[string]Value{"a": Boolean(false), "n": Array(Float(0.5))})},
		{"dynamic wrapper", types.DynamicValue(types.StringValue("w")), String("w")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FromTerraform(tt.in, "")
			if err != nil {
				t.Fatalf("FromTerraform: %v", err)
			}
			if got.Kind != tt.want.Kind || !Equal(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestFromTerraformOverride(t *testing.T) {
	good := []struct {
		in   attr.Value
		kind Kind
		want Value
	}{
		{num(48), KindFloat, Float(48)},
		{num(48), KindInteger, Integer(48)},
		{types.StringValue("2024-01-02T05:04:05+02:00"), KindDate, Date(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC))},
		{types.StringValue("aGVsbG8="), KindData, Data([]byte("hello"))},
		{types.StringValue("x"), KindString, String("x")},
		{types.BoolValue(true), KindBoolean, Boolean(true)},
		{mustTuple(t), KindArray, Array()},
		{mustObject(t, map[string]attr.Value{}), KindDict, Dict(nil)},
	}
	for _, tt := range good {
		got, err := FromTerraform(tt.in, tt.kind)
		if err != nil {
			t.Errorf("FromTerraform(%v, %s): %v", tt.in, tt.kind, err)
			continue
		}
		if got.Kind != tt.want.Kind || !Equal(got, tt.want) {
			t.Errorf("FromTerraform(%v, %s) = %+v, want %+v", tt.in, tt.kind, got, tt.want)
		}
	}

	bad := []struct {
		in   attr.Value
		kind Kind
		msg  string
	}{
		{num(2.5), KindInteger, "whole number"},
		{types.StringValue("2"), KindInteger, "doesn't match a string"},
		{types.StringValue("not a date"), KindDate, "RFC 3339"},
		{types.StringValue("2024-01-02T03:04:05.5Z"), KindDate, "one-second precision"},
		{types.StringValue("!!"), KindData, "base64"},
		{types.BoolValue(true), KindString, "doesn't match a bool"},
		{num(1), KindBoolean, "doesn't match a number"},
		{mustTuple(t), KindDict, "doesn't match a list"},
		{types.StringValue("x"), Kind("blob"), "unknown type"},
	}
	for _, tt := range bad {
		_, err := FromTerraform(tt.in, tt.kind)
		if err == nil || !strings.Contains(err.Error(), tt.msg) {
			t.Errorf("FromTerraform(%v, %s) error = %v, want one containing %q", tt.in, tt.kind, err, tt.msg)
		}
	}
}

func TestFromTerraformRejects(t *testing.T) {
	tests := []struct {
		name string
		in   attr.Value
		msg  string
	}{
		{"null", types.StringNull(), "null"},
		{"nested null", mustTuple(t, types.StringValue("a"), types.StringNull()), "value[1]: null"},
		{"nested path", mustObject(t, map[string]attr.Value{"k": mustTuple(t, types.StringValue("ok\x01"))}), `value["k"][0]`},
		{"control char", types.StringValue("bell\x07"), "can't store"},
	}
	for _, tt := range tests {
		_, err := FromTerraform(tt.in, "")
		if err == nil || !strings.Contains(err.Error(), tt.msg) {
			t.Errorf("%s: error = %v, want one containing %q", tt.name, err, tt.msg)
		}
	}

	_, err := FromTerraform(mustTuple(t, types.StringUnknown()), "")
	if !errors.Is(err, ErrUnknown) {
		t.Errorf("unknown nested value: error = %v, want ErrUnknown", err)
	}
	_, err = FromTerraform(types.DynamicUnknown(), "")
	if !errors.Is(err, ErrUnknown) {
		t.Errorf("unknown dynamic: error = %v, want ErrUnknown", err)
	}
}

func TestToTerraformRoundTrip(t *testing.T) {
	for k, v := range exportedWant() {
		tv, err := ToTerraform(v)
		if err != nil {
			t.Fatalf("%s: ToTerraform: %v", k, err)
		}
		back, err := FromTerraform(tv, v.Kind)
		if err != nil {
			t.Fatalf("%s: FromTerraform(ToTerraform): %v", k, err)
		}
		if !Equal(back, v) {
			t.Errorf("%s: round trip gave %+v, want %+v", k, back, v)
		}
	}
}

func TestToTerraformShapes(t *testing.T) {
	tv, err := ToTerraform(Date(time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := tv.UnderlyingValue().(types.String); !ok || s.ValueString() != "2024-01-02T03:04:05Z" {
		t.Errorf("date = %v, want string 2024-01-02T03:04:05Z", tv)
	}

	tv, err = ToTerraform(Array(Integer(1), String("a")))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tv.UnderlyingValue().(types.Tuple); !ok {
		t.Errorf("array became %T, want types.Tuple", tv.UnderlyingValue())
	}

	tv, err = ToTerraform(Dict(map[string]Value{"a": Boolean(true)}))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tv.UnderlyingValue().(types.Object); !ok {
		t.Errorf("dict became %T, want types.Object", tv.UnderlyingValue())
	}

	if _, err := ToTerraform(Float(posInf())); err == nil {
		t.Error("ToTerraform(+Inf) succeeded, want error")
	}
}

func posInf() float64 {
	f, _ := new(big.Float).SetInf(false).Float64()
	return f
}
