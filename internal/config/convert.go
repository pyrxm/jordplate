package config

import (
	"github.com/zclconf/go-cty/cty"
)

// FromGo converts a plain Go value (the kind ToGo emits: map/slice/string/
// bool/int64/float64/nil) into a cty.Value. Heterogeneous slices become
// tuples and heterogeneous maps become objects so mixed types round-trip
// safely.
func FromGo(v any) cty.Value {
	switch x := v.(type) {
	case nil:
		return cty.NullVal(cty.DynamicPseudoType)
	case string:
		return cty.StringVal(x)
	case bool:
		return cty.BoolVal(x)
	case int64:
		return cty.NumberIntVal(x)
	case float64:
		return cty.NumberFloatVal(x)
	case []any:
		if len(x) == 0 {
			return cty.EmptyTupleVal
		}
		vals := make([]cty.Value, len(x))
		for i, e := range x {
			vals[i] = FromGo(e)
		}
		return cty.TupleVal(vals)
	case map[string]any:
		if len(x) == 0 {
			return cty.EmptyObjectVal
		}
		vals := make(map[string]cty.Value, len(x))
		for k, e := range x {
			vals[k] = FromGo(e)
		}
		return cty.ObjectVal(vals)
	}
	return cty.NullVal(cty.DynamicPseudoType)
}

// ToGo converts a cty.Value into a plain Go value (map/slice/string/bool/int64/float64/nil)
// suitable for passing into a templating engine like gonja.
func ToGo(v cty.Value) any {
	if !v.IsKnown() || v.IsNull() {
		return nil
	}
	ty := v.Type()
	switch {
	case ty == cty.String:
		return v.AsString()
	case ty == cty.Bool:
		return v.True()
	case ty == cty.Number:
		bf := v.AsBigFloat()
		if bf.IsInt() {
			i, _ := bf.Int64()
			return i
		}
		f, _ := bf.Float64()
		return f
	case ty.IsListType(), ty.IsSetType(), ty.IsTupleType():
		out := make([]any, 0, v.LengthInt())
		for it := v.ElementIterator(); it.Next(); {
			_, val := it.Element()
			out = append(out, ToGo(val))
		}
		return out
	case ty.IsMapType(), ty.IsObjectType():
		out := map[string]any{}
		for it := v.ElementIterator(); it.Next(); {
			k, val := it.Element()
			out[k.AsString()] = ToGo(val)
		}
		return out
	}
	return nil
}
