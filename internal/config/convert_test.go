package config

import (
	"reflect"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestToGo(t *testing.T) {
	cases := []struct {
		name string
		in   cty.Value
		want any
	}{
		{"string", cty.StringVal("hello"), "hello"},
		{"bool_true", cty.True, true},
		{"bool_false", cty.False, false},
		{"int", cty.NumberIntVal(42), int64(42)},
		{"float", cty.NumberFloatVal(3.14), 3.14},
		{"null_string", cty.NullVal(cty.String), nil},
		{"null_dynamic", cty.NullVal(cty.DynamicPseudoType), nil},
		{
			"list_of_strings",
			cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			[]any{"a", "b"},
		},
		{
			"tuple_mixed",
			cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.NumberIntVal(2), cty.True}),
			[]any{"a", int64(2), true},
		},
		{
			"map_of_strings",
			cty.MapVal(map[string]cty.Value{"k": cty.StringVal("v")}),
			map[string]any{"k": "v"},
		},
		{
			"object_nested",
			cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("svc"),
				"tags": cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
				"replicas": cty.NumberIntVal(3),
			}),
			map[string]any{
				"name":     "svc",
				"tags":     []any{"a", "b"},
				"replicas": int64(3),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToGo(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ToGo(%#v) = %#v (%T), want %#v (%T)", tc.in, got, got, tc.want, tc.want)
			}
		})
	}
}

func TestToGo_SetBecomesSlice(t *testing.T) {
	// Sets have unstable iteration; just check the returned slice is the right size
	// and contains the right values.
	v := cty.SetVal([]cty.Value{cty.StringVal("x"), cty.StringVal("y")})
	got := ToGo(v).([]any)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	have := map[string]bool{}
	for _, e := range got {
		have[e.(string)] = true
	}
	if !have["x"] || !have["y"] {
		t.Errorf("got %v, want elements x and y", got)
	}
}
