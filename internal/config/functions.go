package config

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"dario.cat/mergo"
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
		"deep_merge": deepMergeFunc,
		"contains":   stdlib.ContainsFunc,

		"trimprefix":   stdlib.TrimPrefixFunc,
		"trimsuffix":   stdlib.TrimSuffixFunc,
		"compact":      stdlib.CompactFunc,
		"distinct":     stdlib.DistinctFunc,
		"coalesce":     coalesceFunc,
		"coalescelist": stdlib.CoalesceListFunc,

		"startswith": startsWithFunc,
		"endswith":   endsWithFunc,
		"fileset":    filesetFunc,

		"element":   stdlib.ElementFunc,
		"flatten":   stdlib.FlattenFunc,
		"index":     indexFunc,
		"lookup":    stdlib.LookupFunc,
		"range":     stdlib.RangeFunc,
		"sort":      stdlib.SortFunc,
		"zipmap":    stdlib.ZipmapFunc,
		"alltrue":   alltrueFunc,
		"anytrue":   anytrueFunc,
		"transpose": transposeFunc,

		"abspath":    abspathFunc,
		"dirname":    dirnameFunc,
		"basename":   basenameFunc,
		"pathexpand": pathexpandFunc,
		"fileexists": fileexistsFunc,

		"base64encode": base64EncodeFunc,
		"base64decode": base64DecodeFunc,
		"urlencode":    urlencodeFunc,
		"md5":          md5Func,
		"sha1":         sha1Func,
		"sha256":       sha256Func,
		"sha512":       sha512Func,

		"url_get": urlGetFunc,
		"type":    typeFunc,

		"get_platform": getPlatformFunc,
	}
}

// deepMergeOptKeys lists the keys recognised inside a trailing options object
// passed to deep_merge(). Any object whose keys are all in this set is treated
// as options rather than as another map to merge.
var deepMergeOptKeys = map[string]bool{
	"append_slices":     true,
	"merge_slice_items": true,
}

// deep_merge(opts?, maps...) -> object. Deeply merges objects/maps; later
// values win. The first argument is optionally an object whose keys are all
// in deepMergeOptKeys — if so, it is interpreted as flags rather than as a
// map to merge. Putting opts first lets callers expand a list of maps with
// HCL's ... operator without conflicting with the opts argument.
//
//	append_slices     - concatenate slices instead of replacing.
//	merge_slice_items - merge slice elements pairwise by index.
//
// Backed by dario.cat/mergo. Non-object/map arguments are an error.
var deepMergeFunc = function.New(&function.Spec{
	Params: []function.Parameter{},
	VarParam: &function.Parameter{
		Name:             "values",
		Type:             cty.DynamicPseudoType,
		AllowDynamicType: true,
		AllowNull:        true,
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		return cty.DynamicPseudoType, nil
	},
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		maps, opts, err := splitDeepMergeArgs(args)
		if err != nil {
			return cty.NilVal, err
		}
		if len(maps) == 0 {
			return cty.EmptyObjectVal, nil
		}

		mergoOpts := []func(*mergo.Config){mergo.WithOverride}
		if opts.appendSlices {
			mergoOpts = append(mergoOpts, mergo.WithAppendSlice)
		}
		if opts.mergeSliceItems {
			mergoOpts = append(mergoOpts, mergo.WithSliceDeepCopy)
		}

		dst := map[string]any{}
		for i, m := range maps {
			src, ok := ToGo(m).(map[string]any)
			if !ok {
				return cty.NilVal, fmt.Errorf("deep_merge: argument %d is not an object/map", i+1)
			}
			if err := mergo.Merge(&dst, src, mergoOpts...); err != nil {
				return cty.NilVal, fmt.Errorf("deep_merge: %w", err)
			}
		}
		return FromGo(dst), nil
	},
})

type deepMergeOpts struct {
	appendSlices    bool
	mergeSliceItems bool
}

// splitDeepMergeArgs separates a leading options object (if any) from the
// list of maps to merge. An argument is treated as options when it is a
// non-null object/map whose keys are all in deepMergeOptKeys. Empty {} also
// counts as an (empty) options object.
func splitDeepMergeArgs(args []cty.Value) ([]cty.Value, deepMergeOpts, error) {
	var opts deepMergeOpts
	if len(args) == 0 {
		return nil, opts, nil
	}
	if isDeepMergeOpts(args[0]) {
		o, err := parseDeepMergeOpts(args[0])
		if err != nil {
			return nil, opts, err
		}
		return args[1:], o, nil
	}
	return args, opts, nil
}

func isDeepMergeOpts(v cty.Value) bool {
	if v.IsNull() {
		return false
	}
	ty := v.Type()
	if !(ty.IsObjectType() || ty.IsMapType()) {
		return false
	}
	if v.LengthInt() == 0 {
		// Treat empty {} as an explicit "no options" sentinel rather than as
		// an empty map to merge; merging {} is a no-op anyway.
		return true
	}
	for it := v.ElementIterator(); it.Next(); {
		k, val := it.Element()
		if !deepMergeOptKeys[k.AsString()] {
			return false
		}
		if val.IsNull() || val.Type() != cty.Bool {
			return false
		}
	}
	return true
}

func parseDeepMergeOpts(v cty.Value) (deepMergeOpts, error) {
	var o deepMergeOpts
	for it := v.ElementIterator(); it.Next(); {
		k, val := it.Element()
		switch k.AsString() {
		case "append_slices":
			o.appendSlices = val.True()
		case "merge_slice_items":
			o.mergeSliceItems = val.True()
		default:
			return o, fmt.Errorf("deep_merge: unknown option %q", k.AsString())
		}
	}
	return o, nil
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

// -----------------------------------------------------------------------------
// Collection helpers: alltrue, anytrue, transpose
// -----------------------------------------------------------------------------

// isTruthyElement returns true for cty.True and the string "true".
// Anything else (false, numbers, other strings, null) is considered not-true.
func isTruthyElement(v cty.Value) bool {
	if v.IsNull() {
		return false
	}
	if v.RawEquals(cty.True) {
		return true
	}
	if v.Type() == cty.String && v.AsString() == "true" {
		return true
	}
	return false
}

// index(list, value) -> number. Returns the first index where value is found,
// errors if absent. Matches Terraform semantics; cty.IndexFunc indexes by
// number instead, so we provide our own.
var indexFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "list", Type: cty.DynamicPseudoType, AllowDynamicType: true},
		{Name: "value", Type: cty.DynamicPseudoType, AllowDynamicType: true, AllowNull: true},
	},
	Type: function.StaticReturnType(cty.Number),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		list := args[0]
		target := args[1]
		if list.IsNull() || !list.CanIterateElements() {
			return cty.NilVal, fmt.Errorf("index: first argument must be a list, set, or tuple")
		}
		i := 0
		for it := list.ElementIterator(); it.Next(); {
			_, v := it.Element()
			if v.RawEquals(target) {
				return cty.NumberIntVal(int64(i)), nil
			}
			i++
		}
		return cty.NilVal, fmt.Errorf("index: value not found in list")
	},
})

// alltrue(list) -> bool. Returns true if every element is truthy. Empty -> true.
var alltrueFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "list", Type: cty.DynamicPseudoType, AllowDynamicType: true}},
	Type:   function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		v := args[0]
		if v.IsNull() || !v.CanIterateElements() {
			return cty.NilVal, fmt.Errorf("alltrue: argument must be a list, set, or tuple")
		}
		for it := v.ElementIterator(); it.Next(); {
			_, e := it.Element()
			if !isTruthyElement(e) {
				return cty.False, nil
			}
		}
		return cty.True, nil
	},
})

// anytrue(list) -> bool. Returns true if any element is truthy. Empty -> false.
var anytrueFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "list", Type: cty.DynamicPseudoType, AllowDynamicType: true}},
	Type:   function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		v := args[0]
		if v.IsNull() || !v.CanIterateElements() {
			return cty.NilVal, fmt.Errorf("anytrue: argument must be a list, set, or tuple")
		}
		for it := v.ElementIterator(); it.Next(); {
			_, e := it.Element()
			if isTruthyElement(e) {
				return cty.True, nil
			}
		}
		return cty.False, nil
	},
})

// transpose(map) -> map. Swaps keys and values in a map of string lists.
// Element lists in the result are sorted for deterministic output.
var transposeFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "values", Type: cty.Map(cty.List(cty.String))},
	},
	Type: function.StaticReturnType(cty.Map(cty.List(cty.String))),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		in := args[0]
		if in.IsNull() {
			return cty.NilVal, fmt.Errorf("transpose: input must not be null")
		}
		out := map[string][]string{}
		for it := in.ElementIterator(); it.Next(); {
			k, list := it.Element()
			key := k.AsString()
			for it2 := list.ElementIterator(); it2.Next(); {
				_, v := it2.Element()
				out[v.AsString()] = append(out[v.AsString()], key)
			}
		}
		if len(out) == 0 {
			return cty.MapValEmpty(cty.List(cty.String)), nil
		}
		result := make(map[string]cty.Value, len(out))
		for k, lst := range out {
			sort.Strings(lst)
			vals := make([]cty.Value, len(lst))
			for i, s := range lst {
				vals[i] = cty.StringVal(s)
			}
			result[k] = cty.ListVal(vals)
		}
		return cty.MapVal(result), nil
	},
})

// -----------------------------------------------------------------------------
// Filesystem helpers: abspath, dirname, basename, pathexpand, fileexists
// -----------------------------------------------------------------------------

func stringPathFunc(impl func(string) (string, error)) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "path", Type: cty.String}},
		Type:   function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			out, err := impl(args[0].AsString())
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(out), nil
		},
	})
}

var abspathFunc = stringPathFunc(filepath.Abs)
var dirnameFunc = stringPathFunc(func(p string) (string, error) { return filepath.Dir(p), nil })
var basenameFunc = stringPathFunc(func(p string) (string, error) { return filepath.Base(p), nil })

// pathexpand expands a leading "~" or "~/" to the user's home directory.
var pathexpandFunc = stringPathFunc(func(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		return filepath.Join(home, p[2:]), nil
	}
	return p, nil
})

// fileexists(path) -> bool. True for regular files, false if missing,
// errors if the path exists but is a directory.
var fileexistsFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "path", Type: cty.String}},
	Type:   function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		p := args[0].AsString()
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				return cty.False, nil
			}
			return cty.NilVal, err
		}
		if info.IsDir() {
			return cty.NilVal, fmt.Errorf("fileexists: %s is a directory, not a file", p)
		}
		return cty.True, nil
	},
})

// -----------------------------------------------------------------------------
// Encoding & hashing: base64encode/decode, urlencode, md5, sha1, sha256, sha512
// -----------------------------------------------------------------------------

var base64EncodeFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "s", Type: cty.String}},
	Type:   function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.StringVal(base64.StdEncoding.EncodeToString([]byte(args[0].AsString()))), nil
	},
})

var base64DecodeFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "s", Type: cty.String}},
	Type:   function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		b, err := base64.StdEncoding.DecodeString(args[0].AsString())
		if err != nil {
			return cty.NilVal, fmt.Errorf("base64decode: %w", err)
		}
		return cty.StringVal(string(b)), nil
	},
})

var urlencodeFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "s", Type: cty.String}},
	Type:   function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.StringVal(url.QueryEscape(args[0].AsString())), nil
	},
})

func hashHexFunc(hasher func() hash.Hash) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "data", Type: cty.String}},
		Type:   function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			h := hasher()
			h.Write([]byte(args[0].AsString()))
			return cty.StringVal(hex.EncodeToString(h.Sum(nil))), nil
		},
	})
}

var (
	md5Func    = hashHexFunc(md5.New)
	sha1Func   = hashHexFunc(sha1.New)
	sha256Func = hashHexFunc(sha256.New)
	sha512Func = hashHexFunc(sha512.New)
)

// -----------------------------------------------------------------------------
// Network: url_get
// -----------------------------------------------------------------------------

// url_get(url) -> string body. Performs an HTTP GET with a 10s timeout and
// caps the response body at 10 MiB.
var urlGetFunc = function.New(&function.Spec{
	Params: []function.Parameter{{Name: "url", Type: cty.String}},
	Type:   function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(args[0].AsString())
		if err != nil {
			return cty.NilVal, fmt.Errorf("url_get: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			return cty.NilVal, fmt.Errorf("url_get: HTTP %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
		if err != nil {
			return cty.NilVal, fmt.Errorf("url_get: read body: %w", err)
		}
		return cty.StringVal(string(body)), nil
	},
})

// -----------------------------------------------------------------------------
// Misc: type
// -----------------------------------------------------------------------------

// type(value) -> string. Returns the cty friendly-name of the value's type
// ("string", "number", "list of string", etc.).
var typeFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType, AllowDynamicType: true, AllowNull: true},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.StringVal(args[0].Type().FriendlyName()), nil
	},
})

// get_platform() -> string. Returns the host operating system as reported by
// the Go runtime: "linux", "darwin", "windows", "freebsd", etc.
var getPlatformFunc = function.New(&function.Spec{
	Params: []function.Parameter{},
	Type:   function.StaticReturnType(cty.String),
	Impl: func(_ []cty.Value, _ cty.Type) (cty.Value, error) {
		return cty.StringVal(runtime.GOOS), nil
	},
})
