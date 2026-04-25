package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pyrxm/jordplate/internal/config"
	"github.com/pyrxm/jordplate/internal/render"
	"github.com/spf13/cobra"
)

var (
	renderConfigPath string
	renderDryRun     bool
	renderAllowExec  bool
)

var renderCmd = &cobra.Command{
	Use:   "render",
	Short: "Render templates declared in an HCL configuration file",
	RunE: func(cmd *cobra.Command, args []string) error {
		absCfg, err := filepath.Abs(renderConfigPath)
		if err != nil {
			return fmt.Errorf("resolve config path: %w", err)
		}

		cfg, err := config.Load(absCfg, config.Options{AllowExec: renderAllowExec})
		if err != nil {
			return err
		}

		baseDir := filepath.Dir(absCfg)
		for _, tpl := range cfg.Templates {
			if tpl.Disabled {
				fmt.Fprintf(cmd.OutOrStdout(), "skipped '%s' (disabled)\n", tpl.Name)
				continue
			}
			source := resolvePath(baseDir, tpl.Source)
			dest := resolvePath(baseDir, tpl.Destination)

			out, err := render.Render(source, tpl.Values)
			if err != nil {
				return fmt.Errorf("render template '%s': %w", tpl.Name, err)
			}

			if renderDryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "# template '%s' -> %s\n%s\n", tpl.Name, dest, out)
				continue
			}

			if err := writeFile(dest, out); err != nil {
				return fmt.Errorf("write template '%s': %w", tpl.Name, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "rendered '%s' -> %s\n", tpl.Name, dest)
		}
		return nil
	},
}

func resolvePath(baseDir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(baseDir, p)
}

func writeFile(path, contents string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(contents), 0o644)
}

func init() {
	renderCmd.Flags().StringVarP(&renderConfigPath, "config", "c", "jordplate.hcl", "path to HCL configuration file")
	renderCmd.Flags().BoolVar(&renderDryRun, "dry-run", false, "print rendered output instead of writing files")
	renderCmd.Flags().BoolVar(&renderAllowExec, "allow-exec", false, "allow the exec() HCL function to run external commands")
	rootCmd.AddCommand(renderCmd)
}
