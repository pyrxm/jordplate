package render

import (
	"fmt"
	"path"

	"github.com/nikolalohinski/gonja/v2"
	"github.com/nikolalohinski/gonja/v2/exec"
	"github.com/nikolalohinski/gonja/v2/loaders"
	"github.com/pyrxm/jordplate/internal/config"
	"github.com/zclconf/go-cty/cty"
)

// Render loads a Jinja2 template from sourcePath and renders it with values
// (a cty value, typically the `values` attribute of a template block).
//
// Undefined variables raise a render-time error rather than silently producing
// empty output, so typos in templates surface immediately.
func Render(sourcePath string, values cty.Value) (string, error) {
	loader, err := loaders.NewFileSystemLoader(path.Dir(sourcePath))
	if err != nil {
		return "", fmt.Errorf("create loader: %w", err)
	}

	cfg := gonja.DefaultConfig.Inherit()
	cfg.StrictUndefined = true

	tpl, err := exec.NewTemplate(path.Base(sourcePath), cfg, loader, gonja.DefaultEnvironment)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	ctx := map[string]any{}
	if values.IsKnown() && !values.IsNull() {
		converted := config.ToGo(values)
		if m, ok := converted.(map[string]any); ok {
			ctx = m
		} else {
			ctx["values"] = converted
		}
	}

	out, err := tpl.ExecuteToString(exec.NewContext(ctx))
	if err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}
	return out, nil
}
