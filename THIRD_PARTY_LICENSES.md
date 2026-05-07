# Third-party licenses

jordplate is distributed under the [MIT License](LICENSE). It depends on the
following third-party Go modules, which retain their own licenses. Source for
each dependency is available at the URL listed; the version pinned by this
project is recorded in `go.mod` / `go.sum`.

## Direct dependencies

| Module | License | Source |
| --- | --- | --- |
| `github.com/bmatcuk/doublestar/v4` | MIT | https://github.com/bmatcuk/doublestar |
| `github.com/hashicorp/hcl/v2` | MPL-2.0 | https://github.com/hashicorp/hcl |
| `github.com/nikolalohinski/gonja/v2` | MIT | https://github.com/nikolalohinski/gonja |
| `github.com/spf13/cobra` | Apache-2.0 | https://github.com/spf13/cobra |
| `github.com/zclconf/go-cty` | MIT | https://github.com/zclconf/go-cty |
| `github.com/zclconf/go-cty-yaml` | Apache-2.0 | https://github.com/zclconf/go-cty-yaml |

## Indirect dependencies

| Module | License | Source |
| --- | --- | --- |
| `github.com/agext/levenshtein` | Apache-2.0 | https://github.com/agext/levenshtein |
| `github.com/apparentlymart/go-textseg/v15` | MIT (with Apache-2.0 Unicode tables) | https://github.com/apparentlymart/go-textseg |
| `github.com/dustin/go-humanize` | MIT | https://github.com/dustin/go-humanize |
| `github.com/google/go-cmp` | BSD-3-Clause | https://github.com/google/go-cmp |
| `github.com/inconshreveable/mousetrap` | Apache-2.0 | https://github.com/inconshreveable/mousetrap |
| `github.com/json-iterator/go` | MIT | https://github.com/json-iterator/go |
| `github.com/mitchellh/go-wordwrap` | MIT | https://github.com/mitchellh/go-wordwrap |
| `github.com/modern-go/concurrent` | Apache-2.0 | https://github.com/modern-go/concurrent |
| `github.com/modern-go/reflect2` | Apache-2.0 | https://github.com/modern-go/reflect2 |
| `github.com/pkg/errors` | BSD-2-Clause | https://github.com/pkg/errors |
| `github.com/sirupsen/logrus` | MIT | https://github.com/sirupsen/logrus |
| `github.com/spf13/pflag` | BSD-3-Clause | https://github.com/spf13/pflag |
| `golang.org/x/exp` | BSD-3-Clause | https://cs.opensource.google/go/x/exp |
| `golang.org/x/mod` | BSD-3-Clause | https://cs.opensource.google/go/x/mod |
| `golang.org/x/sync` | BSD-3-Clause | https://cs.opensource.google/go/x/sync |
| `golang.org/x/sys` | BSD-3-Clause | https://cs.opensource.google/go/x/sys |
| `golang.org/x/text` | BSD-3-Clause | https://cs.opensource.google/go/x/text |
| `golang.org/x/tools` | BSD-3-Clause | https://cs.opensource.google/go/x/tools |

## Notes on MPL-2.0 dependencies

`github.com/hashicorp/hcl/v2` is licensed under the
[Mozilla Public License 2.0](https://www.mozilla.org/en-US/MPL/2.0/). MPL-2.0
applies at the file level: jordplate consumes HCL as an unmodified library
dependency, so jordplate's own source files remain under MIT. Distributions of
jordplate (including compiled binaries) that include HCL must continue to make
HCL's source available — upstream at https://github.com/hashicorp/hcl
satisfies that obligation, and this notice records the pointer.

If you modify HCL itself and ship those modifications as part of jordplate,
MPL-2.0 §3 requires you to make the modified HCL source available under
MPL-2.0; that obligation is on the modifier, not on jordplate.
