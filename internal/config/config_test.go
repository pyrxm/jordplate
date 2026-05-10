package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

// writeHCL writes contents to <dir>/jordplate.hcl and returns the path.
func writeHCL(t *testing.T, dir, contents string) string {
	t.Helper()
	path := filepath.Join(dir, "jordplate.hcl")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write hcl: %v", err)
	}
	return path
}

func TestLoad_BasicLocalsAndTemplate(t *testing.T) {
	dir := t.TempDir()
	hcl := `
locals {
  app = "svc"
  env = "dev"
}
template "deployment" {
  source      = "tpl.j2"
  destination = "out.txt"
  values = {
    app = local.app
    env = local.env
  }
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := cfg.Locals["app"].AsString(), "svc"; got != want {
		t.Errorf("locals.app = %q, want %q", got, want)
	}
	if len(cfg.Templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(cfg.Templates))
	}
	tpl := cfg.Templates[0]
	if tpl.Name != "deployment" {
		t.Errorf("template name = %q, want deployment", tpl.Name)
	}
	if tpl.Source != "tpl.j2" || tpl.Destination != "out.txt" {
		t.Errorf("template paths = %q -> %q", tpl.Source, tpl.Destination)
	}
	values := ToGo(tpl.Values).(map[string]any)
	if values["app"] != "svc" || values["env"] != "dev" {
		t.Errorf("template values = %v", values)
	}
}

func TestLoad_LocalsCrossReference_OutOfOrder(t *testing.T) {
	dir := t.TempDir()
	// `image` references `name`/`tag` which are declared later — must still resolve.
	hcl := `
locals {
  image = "${local.name}:${local.tag}"
  tag   = "v1"
  name  = "svc"
}
template "t" {
  source      = "x.j2"
  destination = "x.out"
  values = { image = local.image }
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Locals["image"].AsString(); got != "svc:v1" {
		t.Errorf("image = %q, want svc:v1", got)
	}
}

func TestLoad_LocalsCycle(t *testing.T) {
	dir := t.TempDir()
	hcl := `
locals {
  a = local.b
  b = local.a
}
template "t" {
  source      = "x"
  destination = "y"
}
`
	_, err := Load(writeHCL(t, dir, hcl), Options{})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want it to mention cycle", err)
	}
}

func TestLoad_DuplicateLocal(t *testing.T) {
	dir := t.TempDir()
	hcl := `
locals {
  a = "first"
}
locals {
  a = "second"
}
template "t" {
  source      = "x"
  destination = "y"
}
`
	_, err := Load(writeHCL(t, dir, hcl), Options{})
	if err == nil {
		t.Fatal("expected duplicate-local error, got nil")
	}
	if !strings.Contains(err.Error(), "declared more than once") {
		t.Errorf("error = %q, want duplicate message", err)
	}
}

func TestLoad_TryHandlesUndeclaredLocal(t *testing.T) {
	dir := t.TempDir()
	// local.does_not_exist is undeclared; try() should fall through to the default.
	hcl := `
locals {
  fallback = try(local.does_not_exist, "default-value")
}
template "t" {
  source      = "x.j2"
  destination = "x.out"
  values = { v = local.fallback }
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Locals["fallback"].AsString(); got != "default-value" {
		t.Errorf("fallback = %q, want default-value", got)
	}
}

func TestLoad_ExecGating(t *testing.T) {
	dir := t.TempDir()
	hcl := `
locals {
  out = exec("echo", "hello")
}
template "t" {
  source      = "x.j2"
  destination = "x.out"
  values = { v = local.out }
}
`
	path := writeHCL(t, dir, hcl)

	if _, err := Load(path, Options{AllowExec: false}); err == nil {
		t.Error("expected error when AllowExec=false, got nil")
	} else if !strings.Contains(err.Error(), "exec() is disabled") {
		t.Errorf("error = %q, want it to mention exec() is disabled", err)
	}

	cfg, err := Load(path, Options{AllowExec: true})
	if err != nil {
		t.Fatalf("Load with AllowExec: %v", err)
	}
	if got := cfg.Locals["out"].AsString(); got != "hello" {
		t.Errorf("exec output = %q, want hello", got)
	}
}

func TestLoad_TemplateValuesOptional(t *testing.T) {
	dir := t.TempDir()
	hcl := `
template "t" {
  source      = "x.j2"
  destination = "x.out"
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Templates[0].Values.RawEquals(cty.EmptyObjectVal) {
		t.Errorf("missing values should default to empty object, got %#v", cfg.Templates[0].Values)
	}
}

func TestLoad_Count(t *testing.T) {
	dir := t.TempDir()
	hcl := `
template "shard" {
  count       = 3
  source      = "tpl.j2"
  destination = "out/${count.index}.txt"
  values = {
    n = count.index
  }
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Templates) != 3 {
		t.Fatalf("got %d templates, want 3", len(cfg.Templates))
	}
	for i, tpl := range cfg.Templates {
		wantName := []string{"shard[0]", "shard[1]", "shard[2]"}[i]
		wantDest := []string{"out/0.txt", "out/1.txt", "out/2.txt"}[i]
		if tpl.Name != wantName {
			t.Errorf("templates[%d].Name = %q, want %q", i, tpl.Name, wantName)
		}
		if tpl.Destination != wantDest {
			t.Errorf("templates[%d].Destination = %q, want %q", i, tpl.Destination, wantDest)
		}
		v := ToGo(tpl.Values).(map[string]any)
		if v["n"] != int64(i) {
			t.Errorf("templates[%d].values.n = %v, want %d", i, v["n"], i)
		}
	}
}

func TestLoad_CountZero(t *testing.T) {
	dir := t.TempDir()
	hcl := `
template "shard" {
  count       = 0
  source      = "tpl.j2"
  destination = "out/${count.index}.txt"
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Templates) != 0 {
		t.Errorf("count=0 should produce no templates, got %d", len(cfg.Templates))
	}
}

func TestLoad_CountInvalid(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"negative", `count = -1`, "must be >= 0"},
		{"non_integer", `count = 1.5`, "whole number"},
		{"non_number", `count = "three"`, "must be a number"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			hcl := `
template "t" {
  ` + tc.body + `
  source      = "x"
  destination = "y"
}
`
			_, err := Load(writeHCL(t, dir, hcl), Options{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestLoad_ForEachMap(t *testing.T) {
	dir := t.TempDir()
	hcl := `
template "region" {
  for_each = {
    us-east = { replicas = 3 }
    us-west = { replicas = 5 }
  }
  source      = "tpl.j2"
  destination = "out/${each.key}.yaml"
  values = {
    name     = each.key
    replicas = each.value.replicas
  }
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Templates) != 2 {
		t.Fatalf("got %d templates, want 2", len(cfg.Templates))
	}
	// Iteration order is sorted by key.
	want := []struct {
		name, dest string
		replicas   int64
	}{
		{`region["us-east"]`, "out/us-east.yaml", 3},
		{`region["us-west"]`, "out/us-west.yaml", 5},
	}
	for i, w := range want {
		got := cfg.Templates[i]
		if got.Name != w.name {
			t.Errorf("templates[%d].Name = %q, want %q", i, got.Name, w.name)
		}
		if got.Destination != w.dest {
			t.Errorf("templates[%d].Destination = %q, want %q", i, got.Destination, w.dest)
		}
		v := ToGo(got.Values).(map[string]any)
		if v["replicas"] != w.replicas {
			t.Errorf("templates[%d].values.replicas = %v, want %d", i, v["replicas"], w.replicas)
		}
	}
}

func TestLoad_ForEachInvalid(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"list", `for_each = ["a", "b"]`, "for_each must be"},
		{"string", `for_each = "nope"`, "for_each must be"},
		{"both_set", "count = 1\n  for_each = { a = 1 }", "mutually exclusive"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			hcl := `
template "t" {
  ` + tc.body + `
  source      = "x"
  destination = "y"
}
`
			_, err := Load(writeHCL(t, dir, hcl), Options{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestLoad_ForEachEmpty(t *testing.T) {
	dir := t.TempDir()
	hcl := `
template "region" {
  for_each    = {}
  source      = "tpl.j2"
  destination = "out/${each.key}.yaml"
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(cfg.Templates) != 0 {
		t.Errorf("empty for_each should produce no templates, got %d", len(cfg.Templates))
	}
}

func TestLoad_Enabled(t *testing.T) {
	dir := t.TempDir()
	hcl := `
template "on" {
  source      = "x.j2"
  destination = "x.out"
}
template "off" {
  enabled     = false
  source      = "x.j2"
  destination = "x.out"
}
template "shard" {
  count       = 3
  enabled     = count.index != 1
  source      = "x.j2"
  destination = "out/${count.index}"
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := map[string]bool{
		"on":       true,
		"off":      false,
		"shard[0]": true,
		"shard[1]": false, // count.index == 1
		"shard[2]": true,
	}
	got := map[string]bool{}
	for _, tpl := range cfg.Templates {
		got[tpl.Name] = tpl.Enabled
	}
	for name, wantEnabled := range want {
		if got[name] != wantEnabled {
			t.Errorf("templates[%q].Enabled = %v, want %v", name, got[name], wantEnabled)
		}
	}
}

func TestLoad_EnabledNotBool(t *testing.T) {
	dir := t.TempDir()
	hcl := `
template "t" {
  enabled     = "yes"
  source      = "x"
  destination = "y"
}
`
	_, err := Load(writeHCL(t, dir, hcl), Options{})
	if err == nil {
		t.Fatal("expected error for non-boolean enabled, got nil")
	}
	if !strings.Contains(err.Error(), "must be a boolean") {
		t.Errorf("error = %q, want it to mention boolean", err)
	}
}

func TestLoad_MissingRequiredAttribute(t *testing.T) {
	dir := t.TempDir()
	// destination is required.
	hcl := `template "t" { source = "x.j2" }`
	_, err := Load(writeHCL(t, dir, hcl), Options{})
	if err == nil {
		t.Fatal("expected error for missing required attribute, got nil")
	}
}

func TestLoad_HooksOrderedByDependsOn(t *testing.T) {
	dir := t.TempDir()
	hcl := `
locals {
  app = "svc"
}
pre_hook "build" {
  command    = ["echo", "build", local.app]
  depends_on = ["fetch"]
}
pre_hook "fetch" {
  command = ["echo", "fetch"]
}
post_hook "deploy" {
  command    = ["echo", "deploy"]
  depends_on = ["format"]
}
post_hook "format" {
  command = ["echo", "format"]
}
template "t" {
  source      = "x.j2"
  destination = "x.out"
}
`
	cfg, err := Load(writeHCL(t, dir, hcl), Options{})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := names(cfg.PreHooks); !equalSlice(got, []string{"fetch", "build"}) {
		t.Errorf("pre_hooks order = %v, want [fetch build]", got)
	}
	if got := names(cfg.PostHooks); !equalSlice(got, []string{"format", "deploy"}) {
		t.Errorf("post_hooks order = %v, want [format deploy]", got)
	}
	// Locals should be available inside command expressions.
	if got := cfg.PreHooks[1].Command; !equalSlice(got, []string{"echo", "build", "svc"}) {
		t.Errorf("build command = %v, want [echo build svc]", got)
	}
}

func TestLoad_HookCycle(t *testing.T) {
	dir := t.TempDir()
	hcl := `
pre_hook "a" {
  command    = ["true"]
  depends_on = ["b"]
}
pre_hook "b" {
  command    = ["true"]
  depends_on = ["a"]
}
template "t" {
  source      = "x"
  destination = "y"
}
`
	_, err := Load(writeHCL(t, dir, hcl), Options{})
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("error = %q, want it to mention cycle", err)
	}
}

func TestLoad_HookSelfDependency(t *testing.T) {
	dir := t.TempDir()
	hcl := `
pre_hook "a" {
  command    = ["true"]
  depends_on = ["a"]
}
template "t" {
  source      = "x"
  destination = "y"
}
`
	_, err := Load(writeHCL(t, dir, hcl), Options{})
	if err == nil || !strings.Contains(err.Error(), "depends on itself") {
		t.Fatalf("error = %v, want self-dependency error", err)
	}
}

func TestLoad_HookUnknownDependency(t *testing.T) {
	dir := t.TempDir()
	hcl := `
pre_hook "a" {
  command    = ["true"]
  depends_on = ["nope"]
}
template "t" {
  source      = "x"
  destination = "y"
}
`
	_, err := Load(writeHCL(t, dir, hcl), Options{})
	if err == nil || !strings.Contains(err.Error(), "unknown pre_hook") {
		t.Fatalf("error = %v, want unknown-dependency error", err)
	}
}

func TestLoad_HookCrossKindDependencyFails(t *testing.T) {
	// pre_hook cannot depend on a post_hook (and vice versa) — they live in
	// separate namespaces because they run at different phases.
	dir := t.TempDir()
	hcl := `
pre_hook "a" {
  command    = ["true"]
  depends_on = ["b"]
}
post_hook "b" {
  command = ["true"]
}
template "t" {
  source      = "x"
  destination = "y"
}
`
	_, err := Load(writeHCL(t, dir, hcl), Options{})
	if err == nil || !strings.Contains(err.Error(), "unknown pre_hook") {
		t.Fatalf("error = %v, want unknown-dependency error", err)
	}
}

func TestLoad_HookDuplicateName(t *testing.T) {
	dir := t.TempDir()
	hcl := `
pre_hook "a" { command = ["true"] }
pre_hook "a" { command = ["true"] }
template "t" {
  source      = "x"
  destination = "y"
}
`
	_, err := Load(writeHCL(t, dir, hcl), Options{})
	if err == nil || !strings.Contains(err.Error(), "declared more than once") {
		t.Fatalf("error = %v, want duplicate-name error", err)
	}
}

func TestLoad_HookCommandValidation(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing", "pre_hook \"a\" {\n}", `"command"`},
		{"empty", "pre_hook \"a\" {\n  command = []\n}", "at least one element"},
		{"not_list", "pre_hook \"a\" {\n  command = \"echo hi\"\n}", "list of strings"},
		{"non_string_element", "pre_hook \"a\" {\n  command = [\"echo\", 1]\n}", "list of strings"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			hcl := tc.body + "\ntemplate \"t\" {\n  source      = \"x\"\n  destination = \"y\"\n}\n"
			_, err := Load(writeHCL(t, dir, hcl), Options{})
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
		})
	}
}

func names(hs []Hook) []string {
	out := make([]string, len(hs))
	for i, h := range hs {
		out[i] = h.Name
	}
	return out
}

func equalSlice[T comparable](a, b []T) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
