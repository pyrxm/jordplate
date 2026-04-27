package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "jordplate",
	Short:         "Render Jinja2 templates from HCL configuration",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	argv0 := ""
	if len(os.Args) > 0 {
		argv0 = os.Args[0]
	}
	applyBinaryName(binaryName(argv0))
	return rootCmd.Execute()
}

// binaryName returns the name the binary was invoked as, stripped of any
// directory components and the Windows .exe suffix. Falls back to "jordplate"
// when argv0 is empty or the basename can't be determined.
func binaryName(argv0 string) string {
	if argv0 == "" {
		return "jordplate"
	}
	// Handle Windows-style paths even when running on POSIX (useful for tests).
	if i := strings.LastIndexAny(argv0, `/\`); i >= 0 {
		argv0 = argv0[i+1:]
	}
	base := strings.TrimSuffix(argv0, ".exe")
	if base == "" || base == "." {
		return "jordplate"
	}
	return base
}

// applyBinaryName rewrites the command name, long description, and the
// --config flag default so the tool can be rebranded by renaming the binary.
func applyBinaryName(name string) {
	rootCmd.Use = name
	rootCmd.Long = fmt.Sprintf(`%s renders Jinja2 template files using values defined in an HCL
configuration file. Locals can be computed from environment variables, files,
and (with --allow-exec) external commands.`, name)

	defaultCfg := name + ".hcl"
	renderConfigPath = defaultCfg
	if f := renderCmd.Flags().Lookup("config"); f != nil {
		f.DefValue = defaultCfg
	}
}
