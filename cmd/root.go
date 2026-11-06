// Package cmd wires the tf-plan-summary CLI.
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/moveeeax/tf-plan-summary/internal/comment"
	"github.com/moveeeax/tf-plan-summary/internal/plan"
	"github.com/moveeeax/tf-plan-summary/internal/render"
)

func newRootCmd() *cobra.Command {
	var (
		planPath  string
		output    string
		statsPath string
		ghOutput  string
		marker    string
		token     string
		repo      string
		prNumber  int
		protect   []string
		ignore    []string
		failOn    string
	)

	root := &cobra.Command{
		Use:   "tf-plan-summary",
		Short: "Summarize a terraform plan JSON as a sticky PR comment",
		Long: "tf-plan-summary parses `terraform show -json plan.bin`, groups resource\n" +
			"actions (create/update/replace/destroy), and renders a Markdown summary. With\n" +
			"a token, repo, and PR number it upserts a single sticky comment.",
		Example: "  tf-plan-summary --plan-json plan.json\n" +
			"  tf-plan-summary --plan-json plan.json --repo owner/name --pr 42 --github-token $GITHUB_TOKEN",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !validFailOn(failOn) {
				return fmt.Errorf("invalid --fail-on %q, want risk, critical or never", failOn)
			}
			policy := plan.Policy{Protect: protect, IgnoreRisk: ignore}
			if err := policy.Validate(); err != nil {
				return err
			}
			data, err := os.ReadFile(planPath)
			if err != nil {
				return fmt.Errorf("read plan json: %w", err)
			}
			summary, err := plan.Parse(data)
			if err != nil {
				return err
			}
			summary.Apply(policy)
			body := render.Markdown(summary, marker)

			// Write/print the rendered Markdown.
			summaryFile := ""
			if output != "" && output != "-" {
				if err := os.WriteFile(output, []byte(body), 0o644); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
				summaryFile = output
			} else {
				fmt.Fprint(cmd.OutOrStdout(), body)
			}

			// Machine-readable counts, so a workflow or script can gate on the
			// plan instead of grepping the Markdown.
			stats := summary.Stats()
			if statsPath != "" {
				if err := writeStatsJSON(statsPath, stats); err != nil {
					return err
				}
			}
			if ghOutput != "" {
				if err := writeGitHubOutput(ghOutput, stats, summaryFile); err != nil {
					return err
				}
			}

			// Post the sticky comment only when fully configured. The gate is
			// checked last, so a failing plan still gets its comment.
			if token == "" || repo == "" || prNumber == 0 {
				return gate(failOn, stats)
			}
			owner, name, ok := splitRepo(repo)
			if !ok {
				return fmt.Errorf("invalid --repo %q, want owner/name", repo)
			}
			ctx := context.Background()
			c, err := comment.NewGitHubCommenter(ctx, nil, token, owner, name, prNumber)
			if err != nil {
				return err
			}
			res, err := comment.Upsert(ctx, c, marker, body)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "sticky comment %s (id %d)\n", res.Action, res.ID)
			return gate(failOn, stats)
		},
	}

	root.Flags().StringVar(&planPath, "plan-json", "", "path to `terraform show -json` output (required)")
	root.Flags().StringVarP(&output, "output", "o", "-", "write Markdown to a file (default: stdout)")
	root.Flags().StringVar(&statsPath, "stats-json", "", "write machine-readable counts to this JSON file")
	root.Flags().StringVar(&ghOutput, "github-output", os.Getenv("GITHUB_OUTPUT"),
		"append key=value step outputs to this file (defaults to $GITHUB_OUTPUT)")
	root.Flags().StringVar(&marker, "marker", render.DefaultMarker, "hidden marker identifying the sticky comment")
	root.Flags().StringVar(&token, "github-token", os.Getenv("GITHUB_TOKEN"), "GitHub token for posting the comment")
	root.Flags().StringVar(&repo, "repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name of the repository")
	root.Flags().IntVar(&prNumber, "pr", envInt("PR_NUMBER"), "pull request number to comment on")
	root.Flags().StringArrayVar(&protect, "protect", nil,
		"glob of protected resources, matched against the address, the type, or the address inside its module (repeatable)")
	root.Flags().StringArrayVar(&ignore, "ignore-risk", nil,
		"glob of resources whose replace/destroy/forget does not count as risk (repeatable; --protect wins)")
	root.Flags().StringVar(&failOn, "fail-on", "never",
		"exit with status 2 on: risk (has-risk), critical (has-critical-risk), or never")
	_ = root.MarkFlagRequired("plan-json")
	return root
}

// GateError is returned when --fail-on trips. main exits with status 2 for it,
// so a workflow can tell a gated plan from a failed run.
type GateError struct{ Reason string }

func (e *GateError) Error() string { return "plan gate: " + e.Reason }

func validFailOn(s string) bool {
	return s == "risk" || s == "critical" || s == "never"
}

// gate turns the --fail-on setting and the plan's stats into an error.
func gate(failOn string, st plan.Stats) error {
	switch {
	case failOn == "critical" && st.HasCriticalRisk:
		return &GateError{fmt.Sprintf("the plan destroys %d and replaces %d protected resource(s)",
			st.ProtectedDestroy, st.ProtectedReplace)}
	case failOn == "risk" && st.HasRisk:
		return &GateError{fmt.Sprintf("the plan replaces %d, destroys %d and forgets %d resource(s)",
			st.Replace, st.Destroy, st.Forget)}
	}
	return nil
}

// writeStatsJSON writes the counts as a small JSON object.
func writeStatsJSON(path string, st plan.Stats) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return fmt.Errorf("encode stats: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write stats json: %w", err)
	}
	return nil
}

// writeGitHubOutput appends the counts in GitHub Actions step-output format, so
// a workflow can gate on them, e.g.
// `if: steps.summary.outputs.has-risk == 'true'`.
func writeGitHubOutput(path string, st plan.Stats, summaryFile string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "create=%d\n", st.Create)
	fmt.Fprintf(&b, "update=%d\n", st.Update)
	fmt.Fprintf(&b, "replace=%d\n", st.Replace)
	fmt.Fprintf(&b, "destroy=%d\n", st.Destroy)
	fmt.Fprintf(&b, "forget=%d\n", st.Forget)
	fmt.Fprintf(&b, "unknown=%d\n", st.Unknown)
	fmt.Fprintf(&b, "total=%d\n", st.Total)
	fmt.Fprintf(&b, "has-risk=%t\n", st.HasRisk)
	fmt.Fprintf(&b, "protected-destroy=%d\n", st.ProtectedDestroy)
	fmt.Fprintf(&b, "protected-replace=%d\n", st.ProtectedReplace)
	fmt.Fprintf(&b, "has-critical-risk=%t\n", st.HasCriticalRisk)
	if summaryFile != "" && !strings.ContainsAny(summaryFile, "\r\n") {
		fmt.Fprintf(&b, "summary-file=%s\n", summaryFile)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open github output: %w", err)
	}
	if _, err := f.WriteString(b.String()); err != nil {
		f.Close()
		return fmt.Errorf("write github output: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("write github output: %w", err)
	}
	return nil
}

func splitRepo(s string) (owner, name string, ok bool) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

func envInt(key string) int {
	n, _ := strconv.Atoi(os.Getenv(key))
	return n
}

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}
