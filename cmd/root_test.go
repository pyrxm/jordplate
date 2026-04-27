package cmd

import (
	"strings"
	"testing"
)

func TestBinaryNameStripsPathAndExe(t *testing.T) {
	cases := []struct {
		argv0 string
		want  string
	}{
		{"jordplate", "jordplate"},
		{"./jordplate", "jordplate"},
		{"/usr/local/bin/jordplate", "jordplate"},
		{"upplate", "upplate"},
		{"/opt/tools/upplate.exe", "upplate"},
		{`C:\tools\custom.exe`, "custom"},
		{"", "jordplate"},
	}
	for _, tc := range cases {
		t.Run(tc.argv0, func(t *testing.T) {
			if got := binaryName(tc.argv0); got != tc.want {
				t.Errorf("binaryName(%q) = %q, want %q", tc.argv0, got, tc.want)
			}
		})
	}
}

func TestApplyBinaryNameRewritesFlagDefault(t *testing.T) {
	origUse, origLong := rootCmd.Use, rootCmd.Long
	origCfg := renderConfigPath
	cfgFlag := renderCmd.Flags().Lookup("config")
	origDefValue := cfgFlag.DefValue
	defer func() {
		rootCmd.Use = origUse
		rootCmd.Long = origLong
		renderConfigPath = origCfg
		cfgFlag.DefValue = origDefValue
	}()

	applyBinaryName("upplate")

	if rootCmd.Use != "upplate" {
		t.Errorf("rootCmd.Use = %q, want upplate", rootCmd.Use)
	}
	if !strings.HasPrefix(rootCmd.Long, "upplate ") {
		t.Errorf("rootCmd.Long = %q, want it to start with 'upplate '", rootCmd.Long)
	}
	if renderConfigPath != "upplate.hcl" {
		t.Errorf("renderConfigPath = %q, want upplate.hcl", renderConfigPath)
	}
	if cfgFlag.DefValue != "upplate.hcl" {
		t.Errorf("--config DefValue = %q, want upplate.hcl", cfgFlag.DefValue)
	}
}
