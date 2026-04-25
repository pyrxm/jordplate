package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "jordplate",
	Short: "Render Jinja2 templates from HCL configuration",
	Long: `jordplate renders Jinja2 template files using values defined in an HCL
configuration file. Locals can be computed from environment variables, files,
and (with --allow-exec) external commands.`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() error {
	return rootCmd.Execute()
}
