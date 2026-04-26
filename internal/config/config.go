package config

import (
	"fmt"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// Options control optional behaviour during HCL evaluation.
type Options struct {
	// AllowExec enables the exec() HCL function. When false, exec() returns an
	// error so untrusted configs cannot run external commands.
	AllowExec bool
}

// Config is the fully-evaluated jordplate configuration.
type Config struct {
	Locals    map[string]cty.Value
	Templates []Template
}

// Template describes one render target declared by a `template "name" {}` block.
type Template struct {
	Name        string
	Source      string
	Destination string
	Values      cty.Value
	Enabled     bool
}

var fileSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{Type: "locals"},
		{Type: "template", LabelNames: []string{"name"}},
	},
}

var templateSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{Name: "source", Required: true},
		{Name: "destination", Required: true},
		{Name: "values"},
		{Name: "count"},
		{Name: "for_each"},
		{Name: "enabled"},
	},
}

// Load parses and evaluates the HCL config file at path.
func Load(path string, opts Options) (*Config, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCLFile(path)
	if diags.HasErrors() {
		return nil, diags
	}

	content, diags := file.Body.Content(fileSchema)
	if diags.HasErrors() {
		return nil, diags
	}

	funcs := stdFunctions(opts)

	locals, err := evaluateLocals(content.Blocks.OfType("locals"), funcs)
	if err != nil {
		return nil, err
	}

	evalCtx := &hcl.EvalContext{
		Functions: funcs,
		Variables: map[string]cty.Value{
			"local": objectOrEmpty(locals),
		},
	}

	templates, err := evaluateTemplates(content.Blocks.OfType("template"), evalCtx)
	if err != nil {
		return nil, err
	}

	return &Config{Locals: locals, Templates: templates}, nil
}

func evaluateLocals(blocks hcl.Blocks, funcs map[string]function.Function) (map[string]cty.Value, error) {
	resolved := map[string]cty.Value{}
	type pendingAttr struct {
		attr *hcl.Attribute
		deps []string
	}
	pending := map[string]pendingAttr{}

	type rawAttr struct {
		attr *hcl.Attribute
		raw  []string
	}
	raw := map[string]rawAttr{}
	for _, block := range blocks {
		attrs, diags := block.Body.JustAttributes()
		if diags.HasErrors() {
			return nil, diags
		}
		for name, attr := range attrs {
			if _, exists := raw[name]; exists {
				return nil, fmt.Errorf("local %q declared more than once", name)
			}
			raw[name] = rawAttr{attr: attr, raw: localDeps(attr.Expr.Variables())}
		}
	}
	// Filter deps to only locals that are actually declared. References to
	// undeclared locals (e.g. inside try()/can()) are left to evaluation, where
	// the function decides whether to swallow the error.
	for name, r := range raw {
		filtered := r.raw[:0]
		for _, d := range r.raw {
			if _, ok := raw[d]; ok {
				filtered = append(filtered, d)
			}
		}
		pending[name] = pendingAttr{attr: r.attr, deps: filtered}
	}

	for len(pending) > 0 {
		progress := false
		for name, p := range pending {
			if !depsResolved(p.deps, resolved) {
				continue
			}
			evalCtx := &hcl.EvalContext{
				Functions: funcs,
				Variables: map[string]cty.Value{
					"local": objectOrEmpty(resolved),
				},
			}
			val, diags := p.attr.Expr.Value(evalCtx)
			if diags.HasErrors() {
				return nil, diags
			}
			resolved[name] = val
			delete(pending, name)
			progress = true
		}
		if !progress {
			names := make([]string, 0, len(pending))
			for n := range pending {
				names = append(names, n)
			}
			sort.Strings(names)
			return nil, fmt.Errorf("locals form a cycle or reference unknown locals: %v", names)
		}
	}
	return resolved, nil
}

func evaluateTemplates(blocks hcl.Blocks, evalCtx *hcl.EvalContext) ([]Template, error) {
	var templates []Template
	for _, block := range blocks {
		content, diags := block.Body.Content(templateSchema)
		if diags.HasErrors() {
			return nil, diags
		}

		iters, err := planIterations(block.Labels[0], content.Attributes, evalCtx)
		if err != nil {
			return nil, err
		}

		for _, iter := range iters {
			t := Template{Name: iter.name}
			if err := evalString(content.Attributes["source"], iter.ctx, &t.Source); err != nil {
				return nil, fmt.Errorf("template %q: %w", iter.name, err)
			}
			if err := evalString(content.Attributes["destination"], iter.ctx, &t.Destination); err != nil {
				return nil, fmt.Errorf("template %q: %w", iter.name, err)
			}
			if attr, ok := content.Attributes["values"]; ok {
				val, diags := attr.Expr.Value(iter.ctx)
				if diags.HasErrors() {
					return nil, diags
				}
				t.Values = val
			} else {
				t.Values = cty.EmptyObjectVal
			}
			t.Enabled = true
			if attr, ok := content.Attributes["enabled"]; ok {
				val, diags := attr.Expr.Value(iter.ctx)
				if diags.HasErrors() {
					return nil, diags
				}
				if val.IsNull() || val.Type() != cty.Bool {
					return nil, fmt.Errorf("template '%s': enabled must be a boolean", iter.name)
				}
				t.Enabled = val.True()
			}
			templates = append(templates, t)
		}
	}
	return templates, nil
}

type iteration struct {
	name string
	ctx  *hcl.EvalContext
}

// planIterations expands a template block into one iteration per count/for_each
// element. When neither is present, a single iteration is returned that uses
// baseCtx unchanged.
func planIterations(label string, attrs hcl.Attributes, baseCtx *hcl.EvalContext) ([]iteration, error) {
	countAttr := attrs["count"]
	forEachAttr := attrs["for_each"]

	if countAttr != nil && forEachAttr != nil {
		return nil, fmt.Errorf("template %q: count and for_each are mutually exclusive", label)
	}

	if countAttr == nil && forEachAttr == nil {
		return []iteration{{name: label, ctx: baseCtx}}, nil
	}

	if countAttr != nil {
		val, diags := countAttr.Expr.Value(baseCtx)
		if diags.HasErrors() {
			return nil, diags
		}
		if val.IsNull() || val.Type() != cty.Number {
			return nil, fmt.Errorf("template %q: count must be a number", label)
		}
		bf := val.AsBigFloat()
		if !bf.IsInt() {
			return nil, fmt.Errorf("template %q: count must be a whole number", label)
		}
		n, _ := bf.Int64()
		if n < 0 {
			return nil, fmt.Errorf("template %q: count must be >= 0", label)
		}
		iters := make([]iteration, 0, n)
		for i := int64(0); i < n; i++ {
			child := baseCtx.NewChild()
			child.Variables = map[string]cty.Value{
				"count": cty.ObjectVal(map[string]cty.Value{
					"index": cty.NumberIntVal(i),
				}),
			}
			iters = append(iters, iteration{
				name: fmt.Sprintf("%s[%d]", label, i),
				ctx:  child,
			})
		}
		return iters, nil
	}

	val, diags := forEachAttr.Expr.Value(baseCtx)
	if diags.HasErrors() {
		return nil, diags
	}
	if val.IsNull() {
		return nil, fmt.Errorf("template %q: for_each must not be null", label)
	}
	ty := val.Type()

	switch {
	case ty.IsMapType() || ty.IsObjectType():
		type kv struct {
			k string
			v cty.Value
		}
		pairs := make([]kv, 0, val.LengthInt())
		for it := val.ElementIterator(); it.Next(); {
			k, v := it.Element()
			pairs = append(pairs, kv{k.AsString(), v})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
		iters := make([]iteration, 0, len(pairs))
		for _, p := range pairs {
			child := baseCtx.NewChild()
			child.Variables = map[string]cty.Value{
				"each": cty.ObjectVal(map[string]cty.Value{
					"key":   cty.StringVal(p.k),
					"value": p.v,
				}),
			}
			iters = append(iters, iteration{
				name: fmt.Sprintf("%s[%q]", label, p.k),
				ctx:  child,
			})
		}
		return iters, nil

	case ty.IsSetType():
		if !ty.ElementType().Equals(cty.String) {
			return nil, fmt.Errorf("template %q: for_each set must contain only strings, got set of %s", label, ty.ElementType().FriendlyName())
		}
		keys := make([]string, 0, val.LengthInt())
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			keys = append(keys, v.AsString())
		}
		sort.Strings(keys)
		iters := make([]iteration, 0, len(keys))
		for _, k := range keys {
			child := baseCtx.NewChild()
			child.Variables = map[string]cty.Value{
				"each": cty.ObjectVal(map[string]cty.Value{
					"key":   cty.StringVal(k),
					"value": cty.StringVal(k),
				}),
			}
			iters = append(iters, iteration{
				name: fmt.Sprintf("%s[%q]", label, k),
				ctx:  child,
			})
		}
		return iters, nil

	default:
		return nil, fmt.Errorf("template %q: for_each must be a map, object, or set of strings, got %s", label, ty.FriendlyName())
	}
}

func evalString(attr *hcl.Attribute, ctx *hcl.EvalContext, out *string) error {
	val, diags := attr.Expr.Value(ctx)
	if diags.HasErrors() {
		return diags
	}
	if val.Type() != cty.String {
		return fmt.Errorf("%s must be a string", attr.Name)
	}
	*out = val.AsString()
	return nil
}

func localDeps(traversals []hcl.Traversal) []string {
	var deps []string
	for _, t := range traversals {
		if t.RootName() != "local" || len(t) < 2 {
			continue
		}
		if step, ok := t[1].(hcl.TraverseAttr); ok {
			deps = append(deps, step.Name)
		}
	}
	return deps
}

func depsResolved(deps []string, resolved map[string]cty.Value) bool {
	for _, d := range deps {
		if _, ok := resolved[d]; !ok {
			return false
		}
	}
	return true
}

func objectOrEmpty(m map[string]cty.Value) cty.Value {
	if len(m) == 0 {
		return cty.EmptyObjectVal
	}
	return cty.ObjectVal(m)
}
