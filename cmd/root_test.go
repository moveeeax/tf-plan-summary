package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := newRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestRunRendersToStdout(t *testing.T) {
	out, err := run(t, "--plan-json", "../testdata/plan.json")
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	for _, sub := range []string{"Terraform Plan Summary", "**2** to add", "⚠️"} {
		if !strings.Contains(out, sub) {
			t.Errorf("output missing %q", sub)
		}
	}
}

func TestRunWritesFile(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "summary.md")
	if _, err := run(t, "--plan-json", "../testdata/plan.json", "-o", dst); err != nil {
		t.Fatalf("execute: %v", err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Terraform Plan Summary") {
		t.Errorf("file missing summary:\n%s", b)
	}
}

func TestRunMissingPlan(t *testing.T) {
	if _, err := run(t, "--plan-json", "/no/such/plan.json"); err == nil {
		t.Fatal("expected error for missing plan file")
	}
}

func TestSplitRepo(t *testing.T) {
	o, n, ok := splitRepo("moveeeax/tf-plan-summary")
	if !ok || o != "moveeeax" || n != "tf-plan-summary" {
		t.Errorf("splitRepo = %q/%q ok=%v", o, n, ok)
	}
	if _, _, ok := splitRepo("bad"); ok {
		t.Error("expected splitRepo to reject bad input")
	}
}
