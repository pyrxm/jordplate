package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyrxm/jordplate/internal/config"
)

func TestRunHooks_NoOpWhenEmpty(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := runHooks(&stdout, &stderr, "pre_hook", nil, t.TempDir(), true, false); err != nil {
		t.Fatalf("runHooks(empty): %v", err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Errorf("empty hooks produced output: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunHooks_ExecutesInOrder(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	hooks := []config.Hook{
		{Name: "first", Command: []string{"sh", "-c", "echo a >> " + marker}},
		{Name: "second", Command: []string{"sh", "-c", "echo b >> " + marker}},
	}
	var stdout, stderr bytes.Buffer
	if err := runHooks(&stdout, &stderr, "pre_hook", hooks, dir, true, false); err != nil {
		t.Fatalf("runHooks: %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if string(got) != "a\nb\n" {
		t.Errorf("marker = %q, want %q", string(got), "a\nb\n")
	}
	if !strings.Contains(stdout.String(), "running pre_hook 'first'") {
		t.Errorf("stdout missing first announcement: %q", stdout.String())
	}
}

func TestRunHooks_DryRunDoesNotExecute(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	hooks := []config.Hook{
		{Name: "h", Command: []string{"sh", "-c", "echo a >> " + marker}},
	}
	var stdout, stderr bytes.Buffer
	if err := runHooks(&stdout, &stderr, "pre_hook", hooks, dir, true, true); err != nil {
		t.Fatalf("runHooks: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("dry-run created marker file: stat err = %v", err)
	}
	if !strings.Contains(stdout.String(), "would run pre_hook 'h'") {
		t.Errorf("stdout missing dry-run notice: %q", stdout.String())
	}
}

func TestRunHooks_SkipsWhenAllowExecFalse(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	hooks := []config.Hook{
		{Name: "h", Command: []string{"sh", "-c", "echo a >> " + marker}},
	}
	var stdout, stderr bytes.Buffer
	if err := runHooks(&stdout, &stderr, "pre_hook", hooks, dir, false, false); err != nil {
		t.Fatalf("runHooks: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("--allow-exec=false ran the hook: stat err = %v", err)
	}
	if !strings.Contains(stderr.String(), "skipping 1 pre_hook(s)") {
		t.Errorf("stderr missing skip notice: %q", stderr.String())
	}
}

func TestRunHooks_PropagatesNonZeroExit(t *testing.T) {
	hooks := []config.Hook{
		{Name: "boom", Command: []string{"sh", "-c", "exit 7"}},
	}
	var stdout, stderr bytes.Buffer
	err := runHooks(&stdout, &stderr, "post_hook", hooks, t.TempDir(), true, false)
	if err == nil {
		t.Fatal("expected error from non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "post_hook 'boom' failed") {
		t.Errorf("error = %v, want post_hook 'boom' failed", err)
	}
}
