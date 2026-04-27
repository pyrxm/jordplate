package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestEnvFunc(t *testing.T) {
	t.Setenv("JORDPLATE_TEST", "value")

	cases := []struct {
		name string
		args []cty.Value
		env  map[string]string
		want string
	}{
		{"set", []cty.Value{cty.StringVal("JORDPLATE_TEST")}, nil, "value"},
		{"unset_no_default", []cty.Value{cty.StringVal("JORDPLATE_MISSING")}, nil, ""},
		{"unset_with_default", []cty.Value{cty.StringVal("JORDPLATE_MISSING"), cty.StringVal("fallback")}, nil, "fallback"},
		{"set_default_ignored", []cty.Value{cty.StringVal("JORDPLATE_TEST"), cty.StringVal("fallback")}, nil, "value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := envFunc.Call(tc.args)
			if err != nil {
				t.Fatalf("envFunc.Call: %v", err)
			}
			if got.AsString() != tc.want {
				t.Errorf("env%v = %q, want %q", tc.args, got.AsString(), tc.want)
			}
		})
	}
}

func TestFileFunc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := fileFunc.Call([]cty.Value{cty.StringVal(path)})
	if err != nil {
		t.Fatalf("fileFunc: %v", err)
	}
	if got.AsString() != "hello" {
		t.Errorf("file = %q, want hello", got.AsString())
	}

	if _, err := fileFunc.Call([]cty.Value{cty.StringVal(filepath.Join(dir, "missing"))}); err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestTryFileFunc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte("present"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := tryFileFunc.Call([]cty.Value{cty.StringVal(path), cty.StringVal("fallback")})
	if err != nil {
		t.Fatalf("tryfile (existing): %v", err)
	}
	if got.AsString() != "present" {
		t.Errorf("got %q, want present", got.AsString())
	}

	got, err = tryFileFunc.Call([]cty.Value{cty.StringVal(filepath.Join(dir, "missing")), cty.StringVal("fallback")})
	if err != nil {
		t.Fatalf("tryfile (missing): %v", err)
	}
	if got.AsString() != "fallback" {
		t.Errorf("got %q, want fallback", got.AsString())
	}
}

func TestExecFunc_Disabled(t *testing.T) {
	fn := execFunc(false)
	_, err := fn.Call([]cty.Value{cty.StringVal("echo"), cty.StringVal("hi")})
	if err == nil {
		t.Fatal("expected error when exec is disabled, got nil")
	}
}

func TestExecFunc_Allowed(t *testing.T) {
	fn := execFunc(true)
	got, err := fn.Call([]cty.Value{cty.StringVal("echo"), cty.StringVal("hello"), cty.StringVal("world")})
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if got.AsString() != "hello world" {
		t.Errorf("got %q, want 'hello world'", got.AsString())
	}
}

func TestCoalesceFunc(t *testing.T) {
	cases := []struct {
		name    string
		args    []cty.Value
		want    cty.Value
		wantErr bool
	}{
		{"skip_empty_strings", []cty.Value{cty.StringVal(""), cty.StringVal(""), cty.StringVal("found"), cty.StringVal("later")}, cty.StringVal("found"), false},
		{"skip_null", []cty.Value{cty.NullVal(cty.String), cty.StringVal("first")}, cty.StringVal("first"), false},
		{"mixed_types", []cty.Value{cty.StringVal(""), cty.NumberIntVal(42)}, cty.NumberIntVal(42), false},
		{"all_empty", []cty.Value{cty.StringVal(""), cty.StringVal("")}, cty.NilVal, true},
		{"all_null", []cty.Value{cty.NullVal(cty.String), cty.NullVal(cty.String)}, cty.NilVal, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := coalesceFunc.Call(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("coalesce: %v", err)
			}
			if !got.RawEquals(tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestStartsEndsWith(t *testing.T) {
	cases := []struct {
		fn        string
		s, p      string
		want      bool
	}{
		{"startswith", "hello-world", "hello", true},
		{"startswith", "hello-world", "world", false},
		{"endswith", "hello-world", "world", true},
		{"endswith", "hello-world", "hello", false},
	}
	for _, tc := range cases {
		t.Run(tc.fn+"_"+tc.s+"_"+tc.p, func(t *testing.T) {
			fn := startsWithFunc
			if tc.fn == "endswith" {
				fn = endsWithFunc
			}
			got, err := fn.Call([]cty.Value{cty.StringVal(tc.s), cty.StringVal(tc.p)})
			if err != nil {
				t.Fatalf("%s: %v", tc.fn, err)
			}
			if got.True() != tc.want {
				t.Errorf("%s(%q,%q) = %v, want %v", tc.fn, tc.s, tc.p, got.True(), tc.want)
			}
		})
	}
}

func TestFilesetFunc(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"a.yaml", "b.yaml", "c.txt", "sub/d.yaml", "sub/nested/e.yaml"} {
		full := filepath.Join(dir, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	cases := []struct {
		name    string
		pattern string
		want    []string
	}{
		{"shallow", "*.yaml", []string{"a.yaml", "b.yaml"}},
		{"recursive", "**/*.yaml", []string{"a.yaml", "b.yaml", "sub/d.yaml", "sub/nested/e.yaml"}},
		{"no_matches", "*.json", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := filesetFunc.Call([]cty.Value{cty.StringVal(dir), cty.StringVal(tc.pattern)})
			if err != nil {
				t.Fatalf("fileset: %v", err)
			}
			converted := ToGo(got)
			var paths []string
			if converted != nil {
				for _, v := range converted.([]any) {
					paths = append(paths, v.(string))
				}
			}
			sort.Strings(paths)
			if !reflect.DeepEqual(paths, tc.want) {
				t.Errorf("pattern %q: got %v, want %v", tc.pattern, paths, tc.want)
			}
		})
	}
}

func TestMergeFunc(t *testing.T) {
	dir := t.TempDir()
	hcl := `
locals {
  base    = { app = "svc", env = "dev" }
  extra   = { region = "us-east", env = "prod" }
  merged  = merge(local.base, local.extra)
}
template "t" {
  source      = "x.j2"
  destination = "x.out"
  values = { merged = local.merged }
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := ToGo(cfg.Locals["merged"]).(map[string]any)
	want := map[string]any{
		"app":    "svc",
		"env":    "prod", // later arg wins
		"region": "us-east",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("merged = %v, want %v", got, want)
	}
}

func TestContainsFunc(t *testing.T) {
	dir := t.TempDir()
	hcl := `
locals {
  envs        = ["dev", "staging", "prod"]
  has_prod    = contains(local.envs, "prod")
  has_qa      = contains(local.envs, "qa")
}
template "t" {
  source      = "x.j2"
  destination = "x.out"
  values = {
    has_prod = local.has_prod
    has_qa   = local.has_qa
  }
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Locals["has_prod"].True() {
		t.Error("contains(envs, prod) should be true")
	}
	if cfg.Locals["has_qa"].True() {
		t.Error("contains(envs, qa) should be false")
	}
}

func TestFilesetFunc_InvalidPattern(t *testing.T) {
	_, err := filesetFunc.Call([]cty.Value{cty.StringVal("."), cty.StringVal("[invalid")})
	if err == nil {
		t.Error("expected error for invalid pattern, got nil")
	}
}

func TestAlltrueAnytrue(t *testing.T) {
	cases := []struct {
		name           string
		list           cty.Value
		wantAll, wantAny bool
	}{
		{"empty", cty.ListValEmpty(cty.Bool), true, false},
		{"all_true_bool", cty.ListVal([]cty.Value{cty.True, cty.True}), true, true},
		{"all_true_strings", cty.ListVal([]cty.Value{cty.StringVal("true"), cty.StringVal("true")}), true, true},
		{"mixed", cty.TupleVal([]cty.Value{cty.True, cty.StringVal("true"), cty.False}), false, true},
		{"all_false", cty.ListVal([]cty.Value{cty.False, cty.False}), false, false},
		{"non_true_strings", cty.ListVal([]cty.Value{cty.StringVal("yes"), cty.StringVal("no")}), false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := alltrueFunc.Call([]cty.Value{tc.list})
			if err != nil {
				t.Fatalf("alltrue: %v", err)
			}
			if got.True() != tc.wantAll {
				t.Errorf("alltrue = %v, want %v", got.True(), tc.wantAll)
			}
			got, err = anytrueFunc.Call([]cty.Value{tc.list})
			if err != nil {
				t.Fatalf("anytrue: %v", err)
			}
			if got.True() != tc.wantAny {
				t.Errorf("anytrue = %v, want %v", got.True(), tc.wantAny)
			}
		})
	}
}

func TestTransposeFunc(t *testing.T) {
	in := cty.MapVal(map[string]cty.Value{
		"a": cty.ListVal([]cty.Value{cty.StringVal("1"), cty.StringVal("2")}),
		"b": cty.ListVal([]cty.Value{cty.StringVal("2"), cty.StringVal("3")}),
	})
	got, err := transposeFunc.Call([]cty.Value{in})
	if err != nil {
		t.Fatalf("transpose: %v", err)
	}
	want := map[string]any{
		"1": []any{"a"},
		"2": []any{"a", "b"},
		"3": []any{"b"},
	}
	if !reflect.DeepEqual(ToGo(got), want) {
		t.Errorf("got %v, want %v", ToGo(got), want)
	}
}

func TestPathFunctions(t *testing.T) {
	if got, err := basenameFunc.Call([]cty.Value{cty.StringVal("/a/b/c.txt")}); err != nil || got.AsString() != "c.txt" {
		t.Errorf("basename = %v, %v; want c.txt", got.AsString(), err)
	}
	if got, err := dirnameFunc.Call([]cty.Value{cty.StringVal("/a/b/c.txt")}); err != nil || got.AsString() != "/a/b" {
		t.Errorf("dirname = %v, %v; want /a/b", got.AsString(), err)
	}
	if got, err := abspathFunc.Call([]cty.Value{cty.StringVal(".")}); err != nil || !filepath.IsAbs(got.AsString()) {
		t.Errorf("abspath did not return absolute path: %v, %v", got.AsString(), err)
	}
}

func TestPathexpandFunc(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir available")
	}
	cases := []struct {
		in, want string
	}{
		{"~", home},
		{"~/foo", filepath.Join(home, "foo")},
		{"/literal/path", "/literal/path"},
		{"relative/path", "relative/path"},
	}
	for _, tc := range cases {
		got, err := pathexpandFunc.Call([]cty.Value{cty.StringVal(tc.in)})
		if err != nil {
			t.Fatalf("pathexpand(%q): %v", tc.in, err)
		}
		if got.AsString() != tc.want {
			t.Errorf("pathexpand(%q) = %q, want %q", tc.in, got.AsString(), tc.want)
		}
	}
}

func TestFileexistsFunc(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(fpath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := fileexistsFunc.Call([]cty.Value{cty.StringVal(fpath)})
	if err != nil || !got.True() {
		t.Errorf("existing file: got=%v err=%v", got, err)
	}

	got, err = fileexistsFunc.Call([]cty.Value{cty.StringVal(filepath.Join(dir, "missing"))})
	if err != nil || got.True() {
		t.Errorf("missing file: got=%v err=%v", got, err)
	}

	if _, err := fileexistsFunc.Call([]cty.Value{cty.StringVal(dir)}); err == nil {
		t.Error("fileexists on a directory should error")
	}
}

func TestBase64Funcs(t *testing.T) {
	enc, err := base64EncodeFunc.Call([]cty.Value{cty.StringVal("hello world")})
	if err != nil {
		t.Fatal(err)
	}
	if enc.AsString() != "aGVsbG8gd29ybGQ=" {
		t.Errorf("base64encode = %q", enc.AsString())
	}
	dec, err := base64DecodeFunc.Call([]cty.Value{enc})
	if err != nil {
		t.Fatal(err)
	}
	if dec.AsString() != "hello world" {
		t.Errorf("round-trip mismatch: %q", dec.AsString())
	}
	if _, err := base64DecodeFunc.Call([]cty.Value{cty.StringVal("not base64!!!")}); err == nil {
		t.Error("base64decode should error on invalid input")
	}
}

func TestUrlencodeFunc(t *testing.T) {
	got, err := urlencodeFunc.Call([]cty.Value{cty.StringVal("hello world & friends")})
	if err != nil {
		t.Fatal(err)
	}
	if got.AsString() != "hello+world+%26+friends" {
		t.Errorf("urlencode = %q", got.AsString())
	}
}

func TestHashFunctions(t *testing.T) {
	cases := []struct {
		name string
		fn   func(cty.Value) (cty.Value, error)
		want string
	}{
		{"md5", func(v cty.Value) (cty.Value, error) { return md5Func.Call([]cty.Value{v}) }, "5eb63bbbe01eeed093cb22bb8f5acdc3"},
		{"sha1", func(v cty.Value) (cty.Value, error) { return sha1Func.Call([]cty.Value{v}) }, "2aae6c35c94fcfb415dbe95f408b9ce91ee846ed"},
		{"sha256", func(v cty.Value) (cty.Value, error) { return sha256Func.Call([]cty.Value{v}) }, "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9"},
		{"sha512", func(v cty.Value) (cty.Value, error) { return sha512Func.Call([]cty.Value{v}) }, "309ecc489c12d6eb4cc40f50c902f2b4d0ed77ee511a7c7a9bcd3ca86d4cd86f989dd35bc5ff499670da34255b45b0cfd830e81f605dcf7dc5542e93ae9cd76f"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn(cty.StringVal("hello world"))
			if err != nil {
				t.Fatal(err)
			}
			if got.AsString() != tc.want {
				t.Errorf("%s(\"hello world\") = %q, want %q", tc.name, got.AsString(), tc.want)
			}
		})
	}
}

func TestUrlGetFunc(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("hello"))
		case "/bad":
			http.Error(w, "boom", 500)
		}
	}))
	defer srv.Close()

	got, err := urlGetFunc.Call([]cty.Value{cty.StringVal(srv.URL + "/ok")})
	if err != nil {
		t.Fatalf("url_get: %v", err)
	}
	if got.AsString() != "hello" {
		t.Errorf("got %q, want hello", got.AsString())
	}

	if _, err := urlGetFunc.Call([]cty.Value{cty.StringVal(srv.URL + "/bad")}); err == nil {
		t.Error("expected error for HTTP 500")
	}
}

func TestTypeFunc(t *testing.T) {
	cases := []struct {
		name string
		in   cty.Value
		want string
	}{
		{"string", cty.StringVal("x"), "string"},
		{"number", cty.NumberIntVal(1), "number"},
		{"bool", cty.True, "bool"},
		{"list", cty.ListVal([]cty.Value{cty.StringVal("a")}), "list of string"},
		{"object", cty.ObjectVal(map[string]cty.Value{"k": cty.StringVal("v")}), `object`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := typeFunc.Call([]cty.Value{tc.in})
			if err != nil {
				t.Fatalf("type: %v", err)
			}
			if got.AsString() != tc.want {
				t.Errorf("type(%v) = %q, want %q", tc.in, got.AsString(), tc.want)
			}
		})
	}
}

func TestGetPlatformFunc(t *testing.T) {
	got, err := getPlatformFunc.Call(nil)
	if err != nil {
		t.Fatalf("get_platform: %v", err)
	}
	if got.AsString() != runtime.GOOS {
		t.Errorf("get_platform() = %q, want %q", got.AsString(), runtime.GOOS)
	}
}

// Smoke test for stdlib-backed functions to confirm wiring.
func TestStdlibWiring(t *testing.T) {
	dir := t.TempDir()
	hcl := `
locals {
  ranged    = range(0, 3)
  sorted    = sort(["c", "a", "b"])
  zipped    = zipmap(["a", "b"], [1, 2])
  flat      = flatten([[1, 2], [3], [4, 5]])
  picked    = element(["x", "y", "z"], 1)
  found     = index(["x", "y", "z"], "y")
  looked_up = lookup({ a = "alpha", b = "beta" }, "missing", "default")
}
template "t" {
  source      = "x.j2"
  destination = "x.out"
  values = {
    ranged    = local.ranged
    sorted    = local.sorted
    zipped    = local.zipped
    flat      = local.flat
    picked    = local.picked
    found     = local.found
    looked_up = local.looked_up
  }
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := func(name string) any { return ToGo(cfg.Locals[name]) }
	if !reflect.DeepEqual(got("ranged"), []any{int64(0), int64(1), int64(2)}) {
		t.Errorf("range = %v", got("ranged"))
	}
	if !reflect.DeepEqual(got("sorted"), []any{"a", "b", "c"}) {
		t.Errorf("sort = %v", got("sorted"))
	}
	if !reflect.DeepEqual(got("zipped"), map[string]any{"a": int64(1), "b": int64(2)}) {
		t.Errorf("zipmap = %v", got("zipped"))
	}
	if !reflect.DeepEqual(got("flat"), []any{int64(1), int64(2), int64(3), int64(4), int64(5)}) {
		t.Errorf("flatten = %v", got("flat"))
	}
	if got("picked") != "y" {
		t.Errorf("element = %v", got("picked"))
	}
	if got("found") != int64(1) {
		t.Errorf("index = %v", got("found"))
	}
	if got("looked_up") != "default" {
		t.Errorf("lookup = %v", got("looked_up"))
	}
}
