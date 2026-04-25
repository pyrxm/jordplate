package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

const messyHCL = `locals {
  a    =    "x"
   b="y"
}
`

const cleanHCL = `locals {
  a = "x"
  b = "y"
}
`

func writeFileT(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestFormatHCLFile_Reformats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hcl")
	writeFileT(t, path, messyHCL)

	changed, err := formatHCLFile(path, false)
	if err != nil {
		t.Fatalf("formatHCLFile: %v", err)
	}
	if !changed {
		t.Error("expected changed=true for messy file")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != cleanHCL {
		t.Errorf("file not canonicalised:\n--- got ---\n%s--- want ---\n%s", got, cleanHCL)
	}
}

func TestFormatHCLFile_AlreadyFormatted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hcl")
	writeFileT(t, path, cleanHCL)

	changed, err := formatHCLFile(path, false)
	if err != nil {
		t.Fatalf("formatHCLFile: %v", err)
	}
	if changed {
		t.Error("expected changed=false for already-formatted file")
	}
}

func TestFormatHCLFile_CheckDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hcl")
	writeFileT(t, path, messyHCL)

	changed, err := formatHCLFile(path, true)
	if err != nil {
		t.Fatalf("formatHCLFile (check): %v", err)
	}
	if !changed {
		t.Error("expected changed=true in check mode for messy file")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != messyHCL {
		t.Errorf("check mode must not modify the file; got:\n%s", got)
	}
}

func TestCollectHCLFiles_SingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.hcl")
	writeFileT(t, path, cleanHCL)

	files, err := collectHCLFiles(path, false)
	if err != nil {
		t.Fatalf("collectHCLFiles: %v", err)
	}
	if !reflect.DeepEqual(files, []string{path}) {
		t.Errorf("got %v, want [%s]", files, path)
	}
}

func TestCollectHCLFiles_Directory(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "a.hcl"), "")
	writeFileT(t, filepath.Join(dir, "b.hcl"), "")
	writeFileT(t, filepath.Join(dir, "ignore.txt"), "")
	writeFileT(t, filepath.Join(dir, "sub", "c.hcl"), "")

	got, err := collectHCLFiles(dir, false)
	if err != nil {
		t.Fatalf("collectHCLFiles: %v", err)
	}
	sort.Strings(got)
	want := []string{
		filepath.Join(dir, "a.hcl"),
		filepath.Join(dir, "b.hcl"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("non-recursive: got %v, want %v", got, want)
	}
}

func TestCollectHCLFiles_Recursive(t *testing.T) {
	dir := t.TempDir()
	writeFileT(t, filepath.Join(dir, "a.hcl"), "")
	writeFileT(t, filepath.Join(dir, "sub", "b.hcl"), "")
	writeFileT(t, filepath.Join(dir, "sub", "deep", "c.hcl"), "")
	writeFileT(t, filepath.Join(dir, "sub", "ignore.txt"), "")

	got, err := collectHCLFiles(dir, true)
	if err != nil {
		t.Fatalf("collectHCLFiles: %v", err)
	}
	sort.Strings(got)
	want := []string{
		filepath.Join(dir, "a.hcl"),
		filepath.Join(dir, "sub", "b.hcl"),
		filepath.Join(dir, "sub", "deep", "c.hcl"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("recursive: got %v, want %v", got, want)
	}
}
