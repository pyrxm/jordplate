package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func writeTpl(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}
	return path
}

func TestRender_HappyPath(t *testing.T) {
	dir := t.TempDir()
	tpl := writeTpl(t, dir, "deployment.j2", `name={{ name }} replicas={{ replicas }}`)

	values := cty.ObjectVal(map[string]cty.Value{
		"name":     cty.StringVal("svc"),
		"replicas": cty.NumberIntVal(3),
	})
	got, err := Render(tpl, values)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "name=svc replicas=3" {
		t.Errorf("got %q", got)
	}
}

func TestRender_StrictUndefinedRaises(t *testing.T) {
	dir := t.TempDir()
	// `mising` typo (vs `missing`); strict mode should reject this.
	tpl := writeTpl(t, dir, "t.j2", `value={{ mising }}`)

	values := cty.ObjectVal(map[string]cty.Value{
		"missing": cty.StringVal("ok"),
	})
	_, err := Render(tpl, values)
	if err == nil {
		t.Fatal("expected error for undefined variable, got nil")
	}
	if !strings.Contains(err.Error(), "mising") {
		t.Errorf("error %q should name the missing variable", err)
	}
}

func TestRender_NilValuesProducesEmptyContext(t *testing.T) {
	dir := t.TempDir()
	tpl := writeTpl(t, dir, "t.j2", `static text`)

	got, err := Render(tpl, cty.NilVal)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "static text" {
		t.Errorf("got %q", got)
	}
}

func TestRender_LoopsAndDicts(t *testing.T) {
	dir := t.TempDir()
	tpl := writeTpl(t, dir, "t.j2", `{% for k, v in tags.items() | sort %}{{ k }}={{ v }};{% endfor %}`)

	values := cty.ObjectVal(map[string]cty.Value{
		"tags": cty.ObjectVal(map[string]cty.Value{
			"app": cty.StringVal("svc"),
			"env": cty.StringVal("prod"),
		}),
	})
	got, err := Render(tpl, values)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != "app=svc;env=prod;" {
		t.Errorf("got %q", got)
	}
}

func TestRender_NonExistentTemplate(t *testing.T) {
	_, err := Render("/no/such/template.j2", cty.EmptyObjectVal)
	if err == nil {
		t.Fatal("expected error for missing template, got nil")
	}
}
