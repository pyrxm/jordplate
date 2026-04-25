package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	ctyyaml "github.com/zclconf/go-cty-yaml"
)

func stdFunctions(opts Options) map[string]function.Function {
	return map[string]function.Function{
		"env":        envFunc,
		"file":       fileFunc,
		"tryfile":    tryFileFunc,
		"exec":       execFunc(opts.AllowExec),
		"try":        tryfunc.TryFunc,
		"can":        tryfunc.CanFunc,
		"upper":      stdlib.UpperFunc,
		"lower":      stdlib.LowerFunc,
		"trim":       stdlib.TrimSpaceFunc,
		"split":      stdlib.SplitFunc,
		"join":       stdlib.JoinFunc,
		"replace":    stdlib.ReplaceFunc,
		"jsonencode": stdlib.JSONEncodeFunc,
		"jsondecode": stdlib.JSONDecodeFunc,
		"yamlencode": ctyyaml.YAMLEncodeFunc,
		"yamldecode": ctyyaml.YAMLDecodeFunc,
		"format":     stdlib.FormatFunc,
		"concat":     stdlib.ConcatFunc,
		"length":     stdlib.LengthFunc,
		"keys":       stdlib.KeysFunc,
		"values":     stdlib.ValuesFunc,
		"merge":      stdlib.MergeFunc,

		"trimprefix":   stdlib.TrimPrefixFunc,
		"trimsuffix":   stdlib.TrimSuffixFunc,
		"compact":      stdlib.CompactFunc,
		"distinct":     stdlib.DistinctFunc,
		"coalesce":     coalesceFunc,
		"coalescelist": stdlib.CoalesceListFunc,

		"startswith": startsWithFunc,
		"endswith":   endsWithFunc,
		"fileset":    filesetFunc,
	}
}

// coalesce(args...) -> first arg that is neither null nor an empty string
// (matching Terraform semantics; cty's CoalesceFunc treats "" as a valid value).
var coalesceFunc = function.New(&function.Spec{
	Params: []function.Parameter{},
	VarParam: &function.Parameter{
		Name:             "vals",
		Type:             cty.DynamicPseudoType,
		AllowDynamicType: true,
		AllowNull:        true,
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		for _, a := range args {
			if a.IsNull() {
				continue
			}
			if a.Type() == cty.String && a.AsString() == "" {
				continue
			}
			return a.Type(), nil
		}
		return cty.NilType, fmt.Errorf("no non-null, non-empty arguments")
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		for _, a := range args {
			if a.IsNull() {
				continue
			}
			if a.Type() == cty.String && a.AsString() == "" {
				continue
			}
			return a, nil
		}
		return cty.NilVal, fmt.Errorf("no non-null, non-empty arguments")
	},
})

// startswith(str, prefix) -> bool
var startsWithFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "str", Type: cty.String},
		{Name: "prefix", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.BoolVal(strings.HasPrefix(args[0].AsString(), args[1].AsString())), nil
	},
})

// endswith(str, suffix) -> bool
var endsWithFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "str", Type: cty.String},
		{Name: "suffix", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.BoolVal(strings.HasSuffix(args[0].AsString(), args[1].AsString())), nil
	},
})

// fileset(path, pattern) -> sorted list of file paths (relative to path) matching
// pattern. Patterns support `*`, `?`, character classes, and `**` recursive globs.
var filesetFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "path", Type: cty.String},
		{Name: "pattern", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.Set(cty.String)),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		root := args[0].AsString()
		pattern := args[1].AsString()
		if !doublestar.ValidatePattern(pattern) {
			return cty.NilVal, fmt.Errorf("invalid fileset pattern %q", pattern)
		}
		matches, err := doublestar.Glob(os.DirFS(root), pattern, doublestar.WithFilesOnly())
		if err != nil {
			return cty.NilVal, fmt.Errorf("fileset(%q, %q): %w", root, pattern, err)
		}
		if len(matches) == 0 {
			return cty.SetValEmpty(cty.String), nil
		}
		vals := make([]cty.Value, 0, len(matches))
		for _, m := range matches {
			vals = append(vals, cty.StringVal(filepath.ToSlash(m)))
		}
		return cty.SetVal(vals), nil
	},
})

// env(name)           -> string, "" if unset
// env(name, default)  -> string, default if unset
var envFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "name", Type: cty.String},
	},
	VarParam: &function.Parameter{Name: "default", Type: cty.String},
	Type:     function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		name := args[0].AsString()
		if v, ok := os.LookupEnv(name); ok {
			return cty.StringVal(v), nil
		}
		if len(args) > 1 {
			return args[1], nil
		}
		return cty.StringVal(""), nil
	},
})

// file(path) -> string contents, errors if missing
var fileFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "path", Type: cty.String}},
	Type:   function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		b, err := os.ReadFile(args[0].AsString())
		if err != nil {
			return cty.NilVal, err
		}
		return cty.StringVal(string(b)), nil
	},
})

// tryfile(path, default) -> string contents, default if missing
var tryFileFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "path", Type: cty.String},
		{Name: "default", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		b, err := os.ReadFile(args[0].AsString())
		if err != nil {
			if os.IsNotExist(err) {
				return args[1], nil
			}
			return cty.NilVal, err
		}
		return cty.StringVal(string(b)), nil
	},
})

// exec(cmd, args...) -> trimmed stdout. Disabled unless --allow-exec is set.
func execFunc(allow bool) function.Function {
	return function.New(&function.Spec{
		Params:   []function.Parameter{{Name: "command", Type: cty.String}},
		VarParam: &function.Parameter{Name: "args", Type: cty.String},
		Type:     function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			if !allow {
				return cty.NilVal, fmt.Errorf("exec() is disabled; pass --allow-exec to enable external command execution")
			}
			cmdName := args[0].AsString()
			cmdArgs := make([]string, 0, len(args)-1)
			for _, a := range args[1:] {
				cmdArgs = append(cmdArgs, a.AsString())
			}
			out, err := exec.Command(cmdName, cmdArgs...).Output()
			if err != nil {
				if ee, ok := err.(*exec.ExitError); ok {
					return cty.NilVal, fmt.Errorf("exec(%s) failed: %w: %s", cmdName, err, string(ee.Stderr))
				}
				return cty.NilVal, fmt.Errorf("exec(%s) failed: %w", cmdName, err)
			}
			return cty.StringVal(strings.TrimRight(string(out), "\r\n")), nil
		},
	})
}
