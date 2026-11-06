package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestMain clears the GitHub Actions environment the CLI reads flag defaults
// from: the test suite itself runs inside Actions, and must not append to the
// real $GITHUB_OUTPUT or post a comment to a real PR.
func TestMain(m *testing.M) {
	for _, k := range []string{"GITHUB_OUTPUT", "GITHUB_TOKEN", "GITHUB_REPOSITORY", "PR_NUMBER"} {
		os.Unsetenv(k)
	}
	os.Exit(m.Run())
}

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

func TestRunWritesStatsAndGitHubOutput(t *testing.T) {
	dir := t.TempDir()
	stats := filepath.Join(dir, "stats.json")
	ghOut := filepath.Join(dir, "gh-output")
	md := filepath.Join(dir, "summary.md")
	if _, err := run(t, "--plan-json", "../testdata/plan_modules.json",
		"-o", md, "--stats-json", stats, "--github-output", ghOut); err != nil {
		t.Fatalf("execute: %v", err)
	}

	var got map[string]any
	b, err := os.ReadFile(stats)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("stats json: %v\n%s", err, b)
	}
	for k, want := range map[string]float64{"destroy": 2, "replace": 2, "forget": 1, "total": 7} {
		if got[k] != want {
			t.Errorf("stats[%s] = %v, want %v", k, got[k], want)
		}
	}
	if got["has_risk"] != true {
		t.Errorf("stats has_risk = %v, want true", got["has_risk"])
	}

	out, err := os.ReadFile(ghOut)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"destroy=2", "replace=2", "forget=1", "total=7",
		"has-risk=true", "summary-file=" + md} {
		if !strings.Contains(string(out), line+"\n") {
			t.Errorf("github output missing %q:\n%s", line, out)
		}
	}
}

// $GITHUB_OUTPUT is picked up automatically, which is what the composite action
// relies on for its declared outputs.
func TestRunDefaultsGitHubOutputFromEnv(t *testing.T) {
	ghOut := filepath.Join(t.TempDir(), "gh-output")
	t.Setenv("GITHUB_OUTPUT", ghOut)
	if _, err := run(t, "--plan-json", "../testdata/plan.json"); err != nil {
		t.Fatalf("execute: %v", err)
	}
	b, err := os.ReadFile(ghOut)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "has-risk=true\n") || !strings.Contains(string(b), "destroy=1\n") {
		t.Errorf("unexpected github output:\n%s", b)
	}
}

// End to end: nothing from the plan's sensitive_values may reach the comment.
func TestRunDoesNotEchoSensitiveValues(t *testing.T) {
	out, err := run(t, "--plan-json", "../testdata/plan_modules.json")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, secret := range []string{"hunter2", "sensitive_values", "master_user_secret"} {
		if strings.Contains(out, secret) {
			t.Errorf("rendered comment leaks %q:\n%s", secret, out)
		}
	}
}

// A state document (or any other JSON) must fail loudly instead of rendering a
// reassuring "No changes" comment.
func TestRunRejectsNonPlanJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(p, []byte(`{"format_version":"1.0","values":{"root_module":{}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "--plan-json", p)
	if err == nil {
		t.Fatalf("expected an error for a state document, got:\n%s", out)
	}
	if strings.Contains(out, "No changes") {
		t.Errorf("state document rendered as an empty plan:\n%s", out)
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

func TestRunExplainsReplacements(t *testing.T) {
	out, err := run(t, "--plan-json", "../testdata/plan_modules.json")
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, out)
	}
	for _, sub := range []string{"| Why |", "forces replacement: `engine_version`, `identifier`", "tainted"} {
		if !strings.Contains(out, sub) {
			t.Errorf("output missing %q:\n%s", sub, out)
		}
	}
}
