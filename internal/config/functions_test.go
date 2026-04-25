package config

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestFilesetFunc_InvalidPattern(t *testing.T) {
	_, err := filesetFunc.Call([]cty.Value{cty.StringVal("."), cty.StringVal("[invalid")})
	if err == nil {
		t.Error("expected error for invalid pattern, got nil")
	}
}
