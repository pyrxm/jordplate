package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

		if err := runHooks(cmd.OutOrStdout(), cmd.ErrOrStderr(), "pre_hook", cfg.PreHooks, baseDir, renderAllowExec, renderDryRun); err != nil {
			return err
		}

		for _, tpl := range cfg.Templates {
			if !tpl.Enabled {
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

		if err := runHooks(cmd.OutOrStdout(), cmd.ErrOrStderr(), "post_hook", cfg.PostHooks, baseDir, renderAllowExec, renderDryRun); err != nil {
			return err
		}
		return nil
	},
}

// runHooks executes a list of hooks in the order returned by config.Load.
// Output is streamed live to the caller's stdout/stderr. Hooks are skipped
// (with a notice) when allowExec is false or dryRun is true.
func runHooks(stdout, stderr io.Writer, kind string, hooks []config.Hook, baseDir string, allowExec, dryRun bool) error {
	if len(hooks) == 0 {
		return nil
	}
	if dryRun {
		for _, h := range hooks {
			fmt.Fprintf(stdout, "would run %s '%s': %s\n", kind, h.Name, strings.Join(h.Command, " "))
		}
		return nil
	}
	if !allowExec {
		fmt.Fprintf(stderr, "skipping %d %s(s); pass --allow-exec to run them\n", len(hooks), kind)
		return nil
	}
	for _, h := range hooks {
		fmt.Fprintf(stdout, "running %s '%s': %s\n", kind, h.Name, strings.Join(h.Command, " "))
		c := exec.Command(h.Command[0], h.Command[1:]...)
		c.Dir = baseDir
		c.Stdout = stdout
		c.Stderr = stderr
		if err := c.Run(); err != nil {
			return fmt.Errorf("%s '%s' failed: %w", kind, h.Name, err)
		}
	}
	return nil
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
	renderCmd.Flags().BoolVar(&renderAllowExec, "allow-exec", false, "allow exec() and pre_hook/post_hook blocks to run external commands")
	rootCmd.AddCommand(renderCmd)
}
