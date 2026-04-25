package config

import (
	"github.com/zclconf/go-cty/cty"
)

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
