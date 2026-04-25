package cmd

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/spf13/cobra"
)

var (
	fmtCheck     bool
	fmtRecursive bool
)

var fmtCmd = &cobra.Command{
	Use:   "fmt [path...]",
	Short: "Rewrite HCL configuration files into a canonical format",
	Long: `fmt rewrites HCL files to a canonical format and indentation style.

If no paths are given, the current directory is formatted. Paths can be files
or directories; with --recursive, directories are walked recursively for files
ending in .hcl.

With --check, files are not modified. The command lists files that would
change and exits with status 1 if any file is not already formatted.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		paths := args
		if len(paths) == 0 {
			paths = []string{"."}
		}

		var changed []string
		for _, p := range paths {
			files, err := collectHCLFiles(p, fmtRecursive)
			if err != nil {
				return err
			}
			for _, f := range files {
				ch, err := formatHCLFile(f, fmtCheck)
				if err != nil {
					return fmt.Errorf("%s: %w", f, err)
				}
				if ch {
					changed = append(changed, f)
					fmt.Fprintln(cmd.OutOrStdout(), f)
				}
			}
		}

		if fmtCheck && len(changed) > 0 {
			return fmt.Errorf("%d file(s) need formatting", len(changed))
		}
		return nil
	},
}

// collectHCLFiles returns the .hcl files reachable from p. If p is a single
// file it is returned as-is (regardless of extension, so the user can format a
// non-standard filename explicitly). If p is a directory, .hcl files inside it
// are listed; with recursive=true, the walk descends into subdirectories.
func collectHCLFiles(p string, recursive bool) ([]string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{p}, nil
	}

	if !recursive {
		entries, err := os.ReadDir(p)
		if err != nil {
			return nil, err
		}
		var out []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".hcl") {
				out = append(out, filepath.Join(p, e.Name()))
			}
		}
		return out, nil
	}

	var out []string
	err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".hcl") {
			out = append(out, path)
		}
		return nil
	})
	return out, err
}

// formatHCLFile formats path. When check is true the file is not written; the
// returned bool reports whether the file would change.
func formatHCLFile(path string, check bool) (changed bool, err error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	formatted := hclwrite.Format(src)
	if bytes.Equal(src, formatted) {
		return false, nil
	}
	if check {
		return true, nil
	}
	if err := os.WriteFile(path, formatted, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

func init() {
	fmtCmd.Flags().BoolVar(&fmtCheck, "check", false, "check if files are correctly formatted; exit 1 if any file would change")
	fmtCmd.Flags().BoolVarP(&fmtRecursive, "recursive", "r", false, "recurse into subdirectories")
	rootCmd.AddCommand(fmtCmd)
}
