# jordplate


Render Jinja2 templates from HCL configuration. Templates use familiar Jinja2
syntax (loops, filters, includes); values are computed in HCL with locals,
functions, and Terraform-style `count` / `for_each` on template blocks.

![jordplate logo](./images/logo.png)


```hcl
locals {
  app_name    = "myservice"
  environment = env("DEPLOY_ENV", "dev")
  build_id    = exec("date", "+%Y%m%d-%H%M%S")
  image       = "registry.example.com/${local.app_name}:${local.build_id}"
}

template "deployment" {
  source      = "templates/deployment.yaml.j2"
  destination = "out/${local.environment}/deployment.yaml"
  values = {
    app_name    = local.app_name
    image       = local.image
    environment = local.environment
    replicas    = 3
  }
}
```

## Install

Pre-built binaries for Linux, macOS and Windows (amd64 / arm64) are published
on every tagged release: see the [releases page](../../releases). Download the
archive for your platform, extract `jordplate`, and put it on your `PATH`.

From source:

```sh
go install github.com/pyrxm/jordplate@latest
```

## Quick start

Create `jordplate.hcl`:

```hcl
locals {
  greeting = env("GREETING", "hello")
}

template "hello" {
  source      = "hello.j2"
  destination = "out/hello.txt"
  values = {
    greeting = local.greeting
    who      = "world"
  }
}
```

Create `hello.j2`:

```jinja2
{{ greeting }}, {{ who }}!
```

Render:

```sh
jordplate render --dry-run         # print to stdout
jordplate render                    # write out/hello.txt
GREETING=hi jordplate render        # override via env var
```

## Commands

### `render`

Renders every template block declared in the config file.

```
jordplate render [flags]

  -c, --config string   path to HCL configuration file (default "jordplate.hcl")
      --dry-run         print rendered output instead of writing files
      --allow-exec      allow exec() and pre_hook/post_hook blocks to run external commands
```

Source/destination paths in the config are resolved relative to the directory
of the HCL config file.

### `fmt`

Rewrites HCL files to a canonical format (same engine as `terraform fmt`).

```
jordplate fmt [path...] [flags]

      --check       check if files are correctly formatted; exit 1 if any file would change
  -r, --recursive   recurse into subdirectories
```

Defaults to the current directory. With `--check`, lists files that would
change and exits non-zero — suitable for CI.

## Configuration reference

### `locals`

A single block declaring named values. Locals can reference each other in any
order; a dependency-ordered evaluation resolves them.

```hcl
locals {
  name        = "svc"
  environment = env("DEPLOY_ENV", "dev")
  image       = "${local.name}:${local.environment}"   # references another local
}
```

### `template`

Each block produces one rendered file (or many, with `count` / `for_each`).

```hcl
template "name" {
  source      = "..."   # required: path to .j2 template
  destination = "..."   # required: output path
  values      = { ... } # optional: data passed to the template (defaults to {})
  count       = N       # optional: produce N instances
  for_each    = MAP     # optional: produce one instance per key
  enabled     = true    # optional: render only when true (default: true)
}
```

`source` and `destination` are paths relative to the HCL config file.

#### `count`

`count = N` produces N instances. Inside the block, `count.index` (0..N-1) is
in scope for `source`, `destination`, `values`, and `enabled`.

```hcl
template "shard" {
  count       = 3
  source      = "shard.j2"
  destination = "out/shard-${count.index}.yaml"
  values = {
    index = count.index
  }
}
```

#### `for_each`

`for_each = { ... }` produces one instance per map/object key. Inside the
block, `each.key` and `each.value` are in scope.

```hcl
template "region" {
  for_each = {
    us-east = { replicas = 3 }
    us-west = { replicas = 5 }
  }
  source      = "region.j2"
  destination = "out/${each.key}.yaml"
  values = {
    region   = each.key
    replicas = each.value.replicas
  }
}
```

`count` and `for_each` are mutually exclusive on the same block. `count = 0` or
`for_each = {}` produces no instances.

#### `enabled`

A boolean that controls whether the block is rendered (default `true`). Sees
the iteration context, so it can disable specific instances:

```hcl
template "shard" {
  count       = 3
  enabled     = count.index != 1   # skip shard[1]
  source      = "shard.j2"
  destination = "out/shard-${count.index}.yaml"
}
```

### `pre_hook` / `post_hook`

External commands that run before (`pre_hook`) and after (`post_hook`) the
template-rendering pass. Useful for fetching inputs, formatting outputs, or
applying generated manifests.

```hcl
pre_hook "fetch_secrets" {
  command = ["sh", "./scripts/fetch.sh"]
}

post_hook "format" {
  command = ["terraform", "fmt", "-recursive", "out/"]
}

post_hook "apply" {
  command    = ["kubectl", "apply", "-f", "out/"]
  depends_on = ["format"]
}
```

- `command` is a list of strings: `[program, arg1, arg2, ...]`. No shell is
  involved unless you invoke one explicitly (`["sh", "-c", "..."]`).
- `depends_on` lists other hooks of the **same kind** (a `pre_hook` cannot
  depend on a `post_hook` and vice versa). Hooks run in dependency order;
  cycles are rejected at load time.
- Hooks see `local.*` and any other top-level evaluation context, so commands
  can be parameterised: `command = ["echo", local.app_name]`.
- Hook stdout/stderr are streamed live. The first hook that exits non-zero
  aborts the run.
- Working directory is the directory containing the HCL config file.

**Execution gating**: hooks share the `--allow-exec` flag with the `exec()`
function. Without `--allow-exec`, hooks are skipped with a notice on stderr —
templates still render. With `--dry-run`, hooks are never executed; instead
`would run <kind> '<name>': <cmd>` is printed for each.

## Functions

### Environment, files, and execution

| Function | Description |
| --- | --- |
| `env(name)` / `env(name, default)` | Read environment variable |
| `file(path)` | Read file contents (errors if missing) |
| `tryfile(path, default)` | Read file contents, return default if missing |
| `fileexists(path)` | `true` if a regular file exists at `path` |
| `fileset(dir, pattern)` | Set of file paths matching `pattern` under `dir`. Supports `**` recursive globs |
| `exec(cmd, args...)` | Run an external command and return trimmed stdout. Disabled unless `--allow-exec` is passed |
| `url_get(url)` | HTTP GET, returns response body. 10 s timeout, body capped at 10 MiB |

### Paths

| Function | Description |
| --- | --- |
| `abspath(path)` | Convert to an absolute path |
| `dirname(path)` | Directory portion of a path |
| `basename(path)` | Final element of a path |
| `pathexpand(path)` | Expand a leading `~` or `~/` to the user's home directory |

### Strings

| Function | Description |
| --- | --- |
| `upper(s)` / `lower(s)` / `trim(s)` | Case and whitespace |
| `split(sep, s)` / `join(sep, list)` | Split / join strings |
| `replace(s, search, replace)` | Substring replacement |
| `startswith(s, prefix)` / `endswith(s, suffix)` | Prefix / suffix tests |
| `trimprefix(s, prefix)` / `trimsuffix(s, suffix)` | Strip prefix / suffix |
| `format(spec, args...)` | `printf`-style formatting |
| `length(value)` | Length of string / list / map |

### Collections

| Function | Description |
| --- | --- |
| `keys(map)` / `values(map)` | Map key / value lists |
| `concat(lists...)` | Concatenate lists |
| `merge(maps...)` | Shallow merge of maps; later keys win |
| `deep_merge(opts?, maps...)` | Deep merge of maps. Optional **leading** object: `{ append_slices = true }` concatenates slices instead of overwriting; `{ merge_slice_items = true }` merges slice elements pairwise by index. Putting opts first lets callers expand a list with `...`, e.g. `deep_merge({ append_slices = true }, list_of_maps...)`. Backed by [`dario.cat/mergo`](https://github.com/darccio/mergo) |
| `compact(list)` | Remove empty strings from a list |
| `distinct(list)` | Remove duplicates from a list |
| `flatten(list)` | Flatten nested lists into a single list |
| `sort(list)` | Lexicographic sort of a string list |
| `range(start, limit, step?)` | Generate a list of numbers |
| `element(list, index)` | Element at the given numeric index |
| `index(list, value)` | First index where `value` appears in `list` (errors if absent) |
| `contains(list, value)` | `true` if `list` (or set) contains `value` |
| `lookup(map, key, default)` | Map value for `key`, or `default` if missing |
| `zipmap(keys, values)` | Build a map from parallel key/value lists |
| `transpose(map_of_lists)` | Swap keys and values in a map of string lists |
| `alltrue(list)` | `true` if every element is `true` or `"true"`; empty → `true` |
| `anytrue(list)` | `true` if any element is `true` or `"true"`; empty → `false` |
| `coalesce(args...)` | First argument that is neither null nor an empty string |
| `coalescelist(lists...)` | First non-empty list |

### Encoding & hashing

| Function | Description |
| --- | --- |
| `jsonencode(v)` / `jsondecode(s)` | JSON marshaling |
| `yamlencode(v)` / `yamldecode(s)` | YAML marshaling |
| `base64encode(s)` / `base64decode(s)` | Base64 round-trip |
| `urlencode(s)` | URL query escape |
| `md5(s)` / `sha1(s)` / `sha256(s)` / `sha512(s)` | Hex-encoded hashes |

### Control flow & introspection

| Function | Description |
| --- | --- |
| `try(expr, default)` | Return `expr`, or `default` if `expr` errors |
| `can(expr)` | `true` if `expr` evaluates without error |
| `type(value)` | Friendly name of the value's type |
| `get_platform()` | Host operating system (`linux`, `darwin`, `windows`, ...) |

## Templates

Templates use [gonja](https://github.com/nikolalohinski/gonja), a Go
implementation of Jinja2. The contents of `values` are exposed at the top
level of the template context, so `values = { name = "x" }` makes `{{ name }}`
available directly.

```jinja2
metadata:
  name: {{ name }}
  labels:
    {%- for k, v in tags.items() %}
    {{ k }}: {{ v }}
    {%- endfor %}
```

**Strict mode**: undefined variables raise a render error rather than silently
producing an empty string, so typos surface immediately at render time.

## Security

`exec()` runs arbitrary shell commands and is **disabled by default**. Pass
`--allow-exec` to enable it, and only do so for configurations you trust.
Treat HCL config files the same way you would treat shell scripts.

## License

[MIT](LICENSE). 

Third-party dependency licenses are listed in
[THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).
